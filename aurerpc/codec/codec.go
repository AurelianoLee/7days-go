/*
 * 一个典型的RPC调用：
 * err = client.Call("Arith.Multiply", args, &reply)
 * client 发送的请求为服务名 Arith，方法名 Multiply，参数 args 三个
 * 服务端的响应包括错误 error，返回值 reply
 * 将请求和响应中的参数和返回值抽象成 body，剩余的信息放在 header 中，那么就可以抽象出数据结构 Header
 */

package codec

import "io"

type Header struct {
	ServiceMethod string // format "Service.Method"
	Seq           uint64 // sequence number chosen by client
	Error         string
}

// Codec 对消息体进行编解码的接口，方便实现不同的 codec 实例
type Codec interface {
	io.Closer
	ReadHeader(*Header) error
	ReadBody(any) error
	Write(*Header, any) error
}

type NewCodecFunc func(io.ReadWriteCloser) Codec

type Type string

const (
	GobType  Type = "application/gob"
	JsonType Type = "application/json" // not implemented
)

var NewCodecFuncMap map[Type]NewCodecFunc

func init() {
	NewCodecFuncMap = make(map[Type]NewCodecFunc)
	NewCodecFuncMap[GobType] = NewGobCodec
}
