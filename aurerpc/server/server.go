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
	"io"
	"log"
	"net"

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

func (server *Server) ServeConn(conn io.ReadWriteCloser) {
	// 明确表示了对 Close() 返回值的处理方式，同时避免了潜在的编译警告
	defer func() { _ = conn.Close() }()
	var opt Option
	if err := json.NewDecoder(conn).Decode(&opt); err != nil {
		log.Println("rpc server: options error:", err)
		return
	}
}
