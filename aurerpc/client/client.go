package client

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net"
	"sync"

	"aurerpc/codec"
	"aurerpc/server"
)

type Call struct {
	Seq           uint64
	ServiceMethod string     // format: "<service>.<method>"
	Args          any        // arguments to the function
	Reply         any        // reply from the function
	Error         error      // if err occurred, it will be placed here
	Done          chan *Call // used to notify caller that call is complete
}

func (call *Call) done() {
	call.Done <- call
}

// cc 是消息的编解码器，和服务端类似，用来序列化/反序列化消息
// sending 是一个互斥锁，和服务端类似，保证请求的有序发送，防止出现多个请求报文混淆
// header 是每个请求的消息头，header 只有在请求发送时才需要，而请求发送是互斥的，因此每个客户端只需要一个，
// 声明在 Client 结构体中可以复用
// seq 用于给发送的请求编号，每个请求拥有唯一编号
// pending 存储未处理完的请求，键是编号，值是 Call 实例
// closing 和 shutdown 任意一个值为 true，则表示 Client 处于不可用的状态
// closing 是用户主动关闭，即调用 Close 方法
// shutdown 有错误发生

// Client represents a RPC Client.
// There may be multiple outstanding Calls associated
// with a single Client, and a Client may be used by
// multiple goroutines simultaneously.
// 一个客户端可能有多个未完成的调用，并且一个客户端可能被多个 goroutine 同时使用。
type Client struct {
	cc       codec.Codec
	opt      *server.Option
	sending  sync.Mutex // protects following
	header   codec.Header
	mu       sync.Mutex // protects following
	seq      uint64
	pending  map[uint64]*Call
	closing  bool // user has called Close
	shutdown bool // server has told us to stop
}

var _ io.Closer = (*Client)(nil)

func (client *Client) Close() error {
	client.mu.Lock()
	defer client.mu.Unlock()
	if client.closing {
		return ErrShutdown
	}
	client.closing = true
	return client.cc.Close()
}

func (client *Client) IsAvailable() bool {
	client.mu.Lock()
	defer client.mu.Unlock()
	return !client.shutdown && !client.closing
}

// registerCall 客户端注册调用
func (client *Client) registerCall(call *Call) (uint64, error) {
	client.mu.Lock()
	defer client.mu.Unlock()
	if client.closing || client.shutdown {
		return 0, ErrShutdown
	}
	call.Seq = client.seq           // 分配序列号
	client.pending[call.Seq] = call // 将调用注册到待处理 map 中
	client.seq++                    // 客户端序列号++
	return call.Seq, nil
}

// removeCall 根据序列号取出等待处理的调用 Call
func (client *Client) removeCall(seq uint64) *Call {
	client.mu.Lock()
	defer client.mu.Unlock()
	call := client.pending[seq]
	delete(client.pending, seq)
	return call
}

// terminateCalls 服务端或客户端发生错误时调用，将 shutdown 设置为 true
// 并且将错误信息通知所有 pending 状态的 call
func (client *Client) terminateCalls(err error) {
	client.sending.Lock()
	defer client.sending.Unlock()
	client.mu.Lock()
	defer client.mu.Unlock()
	client.shutdown = true
	for _, call := range client.pending {
		call.Error = err
		call.done()
	}
}

func (client *Client) receive() {
	var err error
	// 客户端死循环处理发来的请求
	for err == nil {
		var h codec.Header
		// cc 编解码器解析 header
		if err = client.cc.ReadHeader(&h); err != nil {
			break
		}
		// 客户端处理对应序列号的请求调用
		call := client.removeCall(h.Seq)
		switch {
		case call == nil:
			err = client.cc.ReadBody(nil)
		case h.Error != "":
			call.Error = fmt.Errorf(h.Error)
			err = client.cc.ReadBody(nil)
			call.done()
		default:
			err = client.cc.ReadBody(call.Reply)
			if err != nil {
				call.Error = errors.New("reading body err " + err.Error())
			}
			call.done()
		}
	}
	// if error occurs, terminateCalls pending calls
	client.terminateCalls(err)
}

// NewClient 创建 Client 实例
func NewClient(conn net.Conn, opt *server.Option) (*Client, error) {
	// 根据 opt 选择对应的解码器
	f := codec.NewCodecFuncMap[opt.CodecType]
	if f == nil {
		err := fmt.Errorf("invalid codec type %s", opt.CodecType)
		log.Println("rpc client: codec error:", err)
		return nil, err
	}
	// send options with server
	// conn表示一个客户端和服务端的连接
	// 创建一个写入conn的编码器，opt是客户端在连接RPC时希望使用的配置
	if err := json.NewEncoder(conn).Encode(opt); err != nil {
		log.Println("rpc client: options error:", err)
		_ = conn.Close()
		return nil, err
	}
	return newClientCodec(f(conn), opt), nil
}

func newClientCodec(cc codec.Codec, opt *server.Option) *Client {
	client := &Client{
		cc:      cc,
		opt:     opt,
		seq:     1, // starts with 1, 0 means invalid call.
		pending: make(map[uint64]*Call),
	}
	go client.receive()
	return client
}

func parseOptions(opts ...*server.Option) (*server.Option, error) {
	// if opts is nil or pass nil as parameter
	if len(opts) == 0 || opts[0] == nil {
		return server.DefaultOption, nil
	}
	if len(opts) != 1 {
		return nil, errors.New("number of options is more than 1")
	}
	opt := opts[0]
	opt.MagicNumber = server.DefaultOption.MagicNumber
	if opt.CodecType == "" {
		opt.CodecType = server.DefaultOption.CodecType
	}
	return opt, nil
}

// Dial connects to an RPC server at the specified network address
func Dial(network, address string, opts ...*server.Option) (client *Client, err error) {
	opt, err := parseOptions(opts...)
	if err != nil {
		return nil, err
	}
	conn, err := net.Dial(network, address)
	if err != nil {
		return nil, err
	}
	// close the connection if client is nil
	defer func() {
		if client == nil {
			_ = conn.Close()
		}
	}()
	return NewClient(conn, opt)
}

func (client *Client) send(call *Call) {
	// make sure that the client will send a complete request
	client.sending.Lock()
	defer client.sending.Unlock()

	// register this call.
	seq, err := client.registerCall(call)
	if err != nil {
		call.Error = err
		call.done()
		return
	}

	// prepare request header
	client.header.ServiceMethod = call.ServiceMethod
	client.header.Seq = seq
	client.header.Error = ""

	// encode and send the request
	if err := client.cc.Write(&client.header, call.Args); err != nil {
		call := client.removeCall(seq)
		// call may be nil, it usually means that Write partially failed,
		// client has received the response and handled
		if call != nil {
			call.Error = err
			call.done()
		}
	}
}

// Go 和 Call 是客户端暴露给用户的两个 RPC 服务调用接口
// Go 是异步调用，而 Call 是同步调用
// Call 是对 Go 的封装，阻塞 call.Done，等待响应返回
// Go invokes the function asynchronously
// It returns the Call structure representing the invocation.
// The done channel will signal when the call is complete by returning the same Call object.
// If done is nil, Go will allocate a new channel.
// If non-nil, done must be buffered or Go will deliberately crash.
func (client *Client) Go(serviceMethod string, args, reply any, done chan *Call) *Call {
	if done == nil {
		done = make(chan *Call, 10)
	} else if cap(done) == 0 {
		log.Panic("rpc client: done channel is unbuffered")
	}
	call := &Call{
		ServiceMethod: serviceMethod,
		Args:          args,
		Reply:         reply,
		Done:          done,
	}
	client.send(call)
	return call
}

// Call invokes the named function, waits for it to complete,
// and returns its error status.
// The done channel will signal when the call is complete
// by returning the same Call object.
func (client *Client) Call(serviceMethod string, args, reply any) error {
	call := <-client.Go(serviceMethod, args, reply, make(chan *Call, 1)).Done
	return call.Error
}
