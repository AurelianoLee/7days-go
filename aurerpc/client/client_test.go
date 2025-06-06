package client

import (
	"context"
	"fmt"
	"net"
	"os"
	"runtime"
	"strings"
	"testing"
	"time"

	"aurerpc/server"
)

func _assert(condition bool, msg string, v ...any) {
	if !condition {
		panic(fmt.Sprintf("assertion failed: "+msg, v...))
	}
}

// 测试连接超时
func TestClientDialTimeout(t *testing.T) {
	t.Parallel()
	l, _ := net.Listen("tcp", ":0")

	f := func(conn net.Conn, opt *server.Option) (*Client, error) {
		_ = conn.Close()
		time.Sleep(time.Second * 2)
		return nil, nil
	}

	t.Run("timeout", func(t *testing.T) {
		_, err := dialTimeout(f, "tcp", l.Addr().String(), &server.Option{ConnectTimeout: time.Second})
		_assert(err != nil && strings.Contains(err.Error(), "connect timeout"), "expect a timeout error")
	})
	t.Run("0", func(t *testing.T) {
		_, err := dialTimeout(f, "tcp", l.Addr().String(), &server.Option{ConnectTimeout: 0})
		_assert(err == nil, "0 means no limit")
	})
}

type Bar int

func (b Bar) Timeout(argv int, reply *int) error {
	time.Sleep(time.Second * 2)
	return nil
}

func startServer(addr chan string) {
	var b Bar
	_ = server.Register(&b)
	l, _ := net.Listen("tcp", ":0")
	addr <- l.Addr().String()
	server.Accept(l)
}

func TestClientCall(t *testing.T) {
	t.Parallel()
	addrCh := make(chan string)
	go startServer(addrCh)
	addr := <-addrCh
	time.Sleep(time.Second)
	t.Run("client timeout", func(t *testing.T) {
		client, _ := Dial("tcp", addr)
		ctx, cancel := context.WithTimeout(context.Background(), time.Second)
		defer cancel()
		var reply int
		err := client.Call(ctx, "Bar.Timeout", 1, &reply)
		_assert(err != nil && strings.Contains(err.Error(), ctx.Err().Error()), "expect a timeout error")
	})
	t.Run("server handle timeout", func(t *testing.T) {
		client, _ := Dial("tcp", addr, &server.Option{
			HandleTimeout: time.Second,
		})
		var reply int
		err := client.Call(context.Background(), "Bar.Timeout", 1, &reply)
		_assert(err != nil && strings.Contains(err.Error(), "handle timeout"), "expect a timeout error")
	})
}

func TestXDial(t *testing.T) {
	t.Logf("\nruntime.GOOS is %s\n", runtime.GOOS)
	if runtime.GOOS == "linux" || runtime.GOOS == "darwin" {
		ch := make(chan struct{})
		errChan := make(chan error, 1)
		addr := "/tmp/aurerpc.sock"
		go func() {
			_ = os.Remove(addr)
			l, err := net.Listen("unix", addr)
			if err != nil {
				errChan <- err
				return
			}
			ch <- struct{}{}
			server.Accept(l)
		}()

		select {
		case <-ch:
			_, err := XDial("unix@" + addr)
			_assert(err == nil, "failed to connect unix socket")
			t.Log("\nconnect successfully")
		case <-errChan:
			t.Fatal("failed to listen unix socket")
		}
	}
}
