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
	"fmt"
	"io"
	"log"
	"net"
	"reflect"
	"sync"

	"aurerpc/codec"
)

const MagicNumber = 0x3bef5c

type Option struct {
	MagicNumber int        // MagicNumber marks this is aureRPC request
	CodecType   codec.Type // client choose which codec to use
}

var DefaultOption = &Option{
	MagicNumber: MagicNumber,
	CodecType:   codec.GobType,
}

// Server represents a server.
type Server struct{}

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
		log.Println("rpc server: options error:", err)
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
	// 解析 opt 无误后，
	server.serveCodec(f(conn))
}

var invalidRequest = struct{}{}

// 1. handleRequest使用了协程并发请求
// 2. 处理请求是并发的，但是回复请求的报文必须是逐个发送的，并发容易导致多个回复报文交织在一起，客户端无法解析。在这里使用锁（sending）保证
// 3. 只有在header解析失败时，才终止循环
func (server *Server) serveCodec(cc codec.Codec) {
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
		go server.handleRequest(cc, req, sending, wg)
	}
	wg.Wait()
	_ = cc.Close()
}

// request stores all info of a call
type request struct {
	h            *codec.Header // header of request
	argv, replyv reflect.Value // argv and replyv of request
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
	// TODO: now we don't know the type of request argv
	// day 1: just suppose it's string
	req.argv = reflect.New(reflect.TypeOf(""))
	if err = cc.ReadBody(req.argv.Interface()); err != nil {
		log.Println("rpc server: read requset argv err:", err)
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

func (server *Server) handleRequest(cc codec.Codec, req *request, sending *sync.Mutex, wg *sync.WaitGroup) {
	// todo: should call registered rpc methods to get the right replyv
	// day 1: just print argv and send hello message
	defer wg.Done()
	log.Println(req.h, req.argv.Elem())
	req.replyv = reflect.ValueOf(fmt.Sprintf("aurerpc resp %d", req.h.Seq))
	server.sendResponse(cc, req.h, req.replyv.Interface(), sending)
}
