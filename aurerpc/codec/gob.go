package codec

import (
	"bufio"
	"encoding/gob"
	"io"
	"log"
)

// GobCodec
//
// conn 通过 TCP/Unix 建立 socket 时得到的连接实例
// dec, enc 对应 gob 的 Decoder 和 Encoder
// buf 为了防止阻塞而创建的带缓冲的 Writer
type GobCodec struct {
	conn io.ReadWriteCloser
	buf  *bufio.Writer
	dec  *gob.Decoder
	enc  *gob.Encoder
}

// 确保 GobCodec 实现了 Codec 接口
// 1. 类型断言：通过将 (*GobCodec)(nil) 赋值给 _ Codec，可以验证 GobCodec 是否实现了 Codec 接口
// 2. 编译时检查：如果 (*GobCodec)(nil) 不是 Codec 类型，编译器会报错，提示 GobCodec 未实现 Codec 接口
// 3. 无运行时开销：从未使用的变量声明，因此它不会对运行时性能产生任何影响
// 一行保护性代码
var _ Codec = (*GobCodec)(nil)

func NewGobCodec(conn io.ReadWriteCloser) Codec {
	buf := bufio.NewWriter(conn)
	return &GobCodec{
		conn: conn,
		buf:  buf,
		dec:  gob.NewDecoder(conn),
		enc:  gob.NewEncoder(buf),
	}
}

// Problem: 在运行 client_test 中 TestClientCall 时，测试可能会在 ReadHeader 卡死
// 最有可能的原因是因为粘包，导致在服务端使用 json Encoder 吞掉部分 Header，导致发生错误
//
// Solution:
// 1. 两次握手，在服务端收到这个 opt 后，将这个 opt 发送给客户端验证
// 2. 确定 opt 长度，在发送 opt 之前，发送 opt 的 len
func (c *GobCodec) ReadHeader(h *Header) error {
	return c.dec.Decode(h)
}

func (c *GobCodec) ReadBody(body any) error {
	return c.dec.Decode(body)
}

func (c *GobCodec) Write(h *Header, body any) (err error) {
	defer func() {
		_ = c.buf.Flush()
		if err != nil {
			_ = c.Close()
		}
	}()

	if err := c.enc.Encode(h); err != nil {
		log.Println("rpc codec: gob error encoding header:", err)
		return err
	}
	if err := c.enc.Encode(body); err != nil {
		log.Println("rpc codec: gob error encoding body:", err)
		return err
	}
	return nil
}

func (c *GobCodec) Close() error {
	return c.conn.Close()
}
