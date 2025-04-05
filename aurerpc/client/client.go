package client

import (
	"errors"
	"fmt"
	"io"
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
