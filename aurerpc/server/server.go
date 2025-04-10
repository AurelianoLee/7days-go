/*
 * 对于 RPC 协议，知晓如何从 body 中读取需要的信息，这部分协商是需要自主设计的
 * 为了提升性能，一般会在报文的最开始规划固定的字节，来协商相关的信息
 * 比如第一个字节用来表示序列化方式，第二个字节用来表示压缩方式
 * 第3-6字节表示header的长度，第7-10字节表示body的长度
 *
 * 对于 AureRPC 来说，目前需要协商的唯一一项是消息的编码方式，这部分信息放到结构体 Option 中承载
 */

/*
 * 一般来说，涉及协议协商，都需要设计固定的字节来传输，但是为了实现上更简单，AureRPC 客户端固定采用 JSON 编码 Option
 * 后续 header 和 body 的编码方式由 Option 中的 CodecType 决定
 * |Option{MagicNumber:xxx, CodecType:xxx}|Header{ServiceMethod:xxx,}|Body any|
 * |<----------- 固定JSON编码 ------------->|<----- 编码方式由 CodeType 决定 ---->|
 *
 * 一次连接中：
 * |Option|Header1|Body1|Header2|Body2|...
 */

package server

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	"reflect"
	"strings"
	"sync"
	"time"

	"aurerpc/codec"
	"aurerpc/constants"
)

const MagicNumber = 0x3bef5c

// RPC 连接建立时确定是否是对应的RPC协议，编码方式，超时时间
type Option struct {
	MagicNumber int        // MagicNumber marks this is aureRPC request
	CodecType   codec.Type // client choose which codec to use

	// add timeout handle
	ConnectTimeout time.Duration // 0 means no limit
	HandleTimeout  time.Duration
}

var DefaultOption = &Option{
	MagicNumber:    MagicNumber,
	CodecType:      codec.GobType,
	ConnectTimeout: time.Second * 10,
}

// Server represents a server.
type Server struct {
	serviceMap sync.Map
}

// NewServer returns a new Server.
func NewServer() *Server {
	return &Server{}
}

// DefaultServer is the default instance of Server.
var DefaultServer = NewServer()

// Accept accepts connections on the listener and serves requests
// for each incoming connection.
func (server *Server) Accept(lis net.Listener) {
	// for 循环等待 socket 连接建立，并开启子协程处理
	for {
		conn, err := lis.Accept()
		if err != nil {
			log.Println("rpc server: accept error:", err)
			return
		}
		go server.ServeConn(conn)
	}
}

// Accept accepts connections on the listener and serves requests
// for each incoming connection.
// Accept 函数是对 DefaultServer.Accept 的封装，用于简化接收和处理 RPC 请求的流程。
// 如果想启动服务，传入 listener，直接调用 Accept 即可。
// lis, _ := net.Listen("tcp", ":1234")
// aurerpc.Accept(lis)
func Accept(lis net.Listener) {
	DefaultServer.Accept(lis)
}

// ServeConn runs the server on a single connection.
// ServeConn blocks, serving the connection until the client hangs up.
// ServeConn 在单个连接上运行服务器
// ServeConn 阻塞，为连接提供服务直到客户端挂起
func (server *Server) ServeConn(conn io.ReadWriteCloser) {
	// 明确表示了对 Close() 返回值的处理方式，同时避免了潜在的编译警告
	defer func() { _ = conn.Close() }()
	var opt Option
	if err := json.NewDecoder(conn).Decode(&opt); err != nil {
		log.Println("rpc server: receive options error:", err)
		return
	}

	if opt.MagicNumber != MagicNumber {
		log.Printf("rpc server: invalid magic number: %x", opt.MagicNumber)
		return
	}
	f := codec.NewCodecFuncMap[opt.CodecType]
	if f == nil {
		log.Printf("rpc server: invalid codec type %s", opt.CodecType)
		return
	}
	// 第二次握手
	if err := json.NewEncoder(conn).Encode(&opt); err != nil {
		log.Println("rpc server: send options error: ", err)
		return
	}
	// 解析 opt 无误后，
	server.serveCodec(f(conn), &opt)
}

var invalidRequest = struct{}{}

// 1. handleRequest使用了协程并发请求
// 2. 处理请求是并发的，但是回复请求的报文必须是逐个发送的，并发容易导致多个回复报文交织在一起，
// 客户端无法解析。在这里使用锁（sending）保证
// 3. 只有在header解析失败时，才终止循环
func (server *Server) serveCodec(cc codec.Codec, opts *Option) {
	sending := new(sync.Mutex) // make sure to send a complete response
	wg := new(sync.WaitGroup)  // wait until all request are handled
	// for 无限制地等待请求的到来，直到发生错误（连接被关闭，接收到的报文有问题）
	for {
		// 1. 读取请求
		req, err := server.readRequest(cc)
		if err != nil {
			if req == nil {
				break // it's not possible to recover, so close the connection
			}
			req.h.Error = err.Error()
			// 3. 回复请求
			server.sendResponse(cc, req.h, invalidRequest, sending)
			continue
		}
		wg.Add(1)
		// 2. 处理请求
		go server.handleRequest(cc, req, sending, wg, opts.HandleTimeout)
	}
	wg.Wait()
	_ = cc.Close()
}

// request stores all info of a call
type request struct {
	h            *codec.Header // header of request
	argv, replyv reflect.Value // argv and replyv of request
	mtype        *MethodType
	svc          *service
}

func (server *Server) readRequestHeader(cc codec.Codec) (*codec.Header, error) {
	var h codec.Header
	if err := cc.ReadHeader(&h); err != nil {
		if err != io.EOF && err != io.ErrUnexpectedEOF {
			log.Println("rpc server: read header error:", err)
		}
		return nil, err
	}
	return &h, nil
}

func (server *Server) readRequest(cc codec.Codec) (*request, error) {
	h, err := server.readRequestHeader(cc)
	if err != nil {
		return nil, err
	}
	req := &request{h: h}
	req.svc, req.mtype, err = server.findService(h.ServiceMethod)
	if err != nil {
		return req, err
	}
	req.argv = req.mtype.newArgv()
	req.replyv = req.mtype.newReplyv()

	// make sure that argvi is a pointer, ReadBody need a pointer as parameter
	argvi := req.argv.Interface()
	if req.argv.Type().Kind() != reflect.Ptr {
		argvi = req.argv.Addr().Interface()
	}
	// newArgv 只是创建了一个空的容器，定义了参数的结构
	// 真正的数据填充是由 ReadBody 方法完成的，而 ReadBody 的数据来源是网络连接 conn
	if err = cc.ReadBody(argvi); err != nil {
		log.Println("rpc server: read request argv err:", err)
	}
	return req, nil
}

func (server *Server) sendResponse(cc codec.Codec, h *codec.Header, body any, sending *sync.Mutex) {
	sending.Lock()
	defer sending.Unlock()
	if err := cc.Write(h, body); err != nil {
		log.Println("rpc server: write response error:", err)
	}
}

func (server *Server) handleRequest(cc codec.Codec, req *request, sending *sync.Mutex,
	wg *sync.WaitGroup, timeout time.Duration) {
	defer wg.Done()
	called := make(chan struct{})
	sent := make(chan struct{})
	go func() {
		err := req.svc.call(req.mtype, req.argv, req.replyv)
		called <- struct{}{}
		if err != nil {
			req.h.Error = err.Error()
			server.sendResponse(cc, req.h, invalidRequest, sending)
			sent <- struct{}{}
			return
		}
		server.sendResponse(cc, req.h, req.replyv.Interface(), sending)
		sent <- struct{}{}
	}()

	if timeout == 0 {
		<-called
		<-sent
		return
	}

	select {
	case <-time.After(timeout):
		// TODO: 超时的情况下，上面新开的协程如果继续写入了called和sent，会导致这两个channel阻塞
		req.h.Error = fmt.Sprintf("rpc server: request handle timeout: expect within %s", timeout)
		server.sendResponse(cc, req.h, invalidRequest, sending)
	case <-called:
		<-sent
	}
}

// Register published in the server the set of methods
func (server *Server) Register(rcvr any) error {
	s := newService(rcvr)
	if _, dup := server.serviceMap.LoadOrStore(s.name, s); dup {
		return fmt.Errorf("rpc: service already defined: %s", s.name)
	}
	return nil
}

// Register publishes the receiver's methods in the DefaultServer.
func Register(rcvr any) error {
	return DefaultServer.Register(rcvr)
}

// findService 通过 serviceMethod 从 serviceMap 中找到对应的 service
func (server *Server) findService(serviceMethod string) (svc *service, mType *MethodType, err error) {
	// 分割服务名和方法名
	dot := strings.LastIndex(serviceMethod, ".")
	if dot < 0 {
		err = errors.New("rpc server: service/method request ill-formed: " + serviceMethod)
		return
	}
	serviceName, methodName := serviceMethod[:dot], serviceMethod[dot+1:]

	// 先在 serviceMap 中找到对应的 service 实例，再从 service 实例的 method 中，找到对应的 methodType
	svci, ok := server.serviceMap.Load(serviceName)
	if !ok {
		err = errors.New("rpc server: can't find service " + serviceName)
		return
	}
	svc = svci.(*service)
	mType = svc.method[methodName]
	if mType == nil {
		err = errors.New("rpc server: can't find method " + methodName)
	}
	return
}

// ----------------------- HTTP --------------------------------

// ServeHTTP implements an http.Handler that answers RPC requests.
func (server *Server) ServeHTTP(w http.ResponseWriter, req *http.Request) {
	if req.Method != "CONNECT" {
		// 设置响应头
		w.Header().Set("Content-Type", "text/plain; charset=utf-8")
		w.WriteHeader(http.StatusMethodNotAllowed)
		_, _ = io.WriteString(w, "405 must CONNECT\n")
		return
	}
	// 劫持 HTTP 连接
	// 1. 使用类型断言将 w 转换为 http.Hijack 类型
	// 2. 调用 Hijack 方法劫持当前的 HTTP 连接
	conn, _, err := w.(http.Hijacker).Hijack()
	if err != nil {
		log.Println("rpc hijacking ", req.RemoteAddr, ": ", err.Error())
		return
	}
	// 自定义响应：通知客户端连接已成功升级
	// 客户端收到该响应后，可以切换到自定义的 RPC 协议进行通信
	_, _ = io.WriteString(conn, "HTTP/1.0 "+constants.Connected+"\n\n")
	server.ServeConn(conn)
}

// HandleHTTP registers an HTTP handler for RPC messages on rpcPath.
// It is still necessary to invoke http.Serve(), typically in a go statement.
func (server *Server) HandleHTTP() {
	http.Handle(constants.DefaultRPCPath, server)
}

// HandleHTTP is a convenient approach for default server to register HTTP handlers.
func HandleHTTP() {
	DefaultServer.HandleHTTP()
}
