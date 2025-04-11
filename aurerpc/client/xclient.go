package client

import (
	"context"
	"errors"
	"io"
	"reflect"
	"strings"
	"sync"

	"aurerpc/discovery"
	"aurerpc/server"
)

// 支持负载均衡的客户端
type XClient struct {
	d       discovery.Discovery  // 集成注册中心
	mode    discovery.SelectMode // 选择负载均衡方式
	opt     *server.Option       // rpc连接选项
	mu      sync.Mutex
	clients map[string]*Client
}

var _ io.Closer = (*XClient)(nil)

// 需要传入三个参数，服务发现实例 Discovery，负载均衡模式 SelectMode 以及协议选项 Option
// 尽量复用已经创建好的 Socket 连接，使用 clients 保存创建成功的 Client 实例
func NewXClient(d discovery.Discovery, mode discovery.SelectMode, opt *server.Option) *XClient {
	return &XClient{
		d:       d,
		mode:    mode,
		opt:     opt,
		clients: make(map[string]*Client),
	}
}

func (xc *XClient) Close() error {
	xc.mu.Lock()
	defer xc.mu.Unlock()

	var errs []error
	for key, client := range xc.clients {
		if err := client.Close(); err != nil {
			errs = append(errs, err)
		}
		delete(xc.clients, key)
	}

	if len(errs) == 0 {
		return nil
	}
	return errors.New("[rpc xClient] failed to close clients: " + aggregateErrors(errs))
}

func aggregateErrors(errs []error) string {
	var errStrings []string
	for _, err := range errs {
		errStrings = append(errStrings, err.Error())
	}
	return strings.Join(errStrings, "; ")
}

func (xc *XClient) dial(rpcAddr string) (*Client, error) {
	xc.mu.Lock()
	defer xc.mu.Unlock()
	// 1. 检查 xc.clients 是否有缓存的 Client，如果有，检查是否可用状态
	// 如果是则返回缓存的 Client，如果不可用，则从缓存中删除
	client, ok := xc.clients[rpcAddr]
	if ok && !client.IsAvailable() {
		_ = client.Close()
		delete(xc.clients, rpcAddr)
		client = nil
	}

	// 2. 没有缓存的 client，需要创建新的 Client
	if client == nil {
		var err error
		client, err = XDial(rpcAddr, xc.opt)
		if err != nil {
			return nil, err
		}
		xc.clients[rpcAddr] = client
	}
	return client, nil
}

func (xc *XClient) call(ctx context.Context, rpcAddr, serviceMethod string, args, reply any) error {
	rpcClient, err := xc.dial(rpcAddr)
	if err != nil {
		return err
	}
	return rpcClient.Call(ctx, serviceMethod, args, reply)
}

// 负载均衡的请求分发方式
//
// Call 调用指定函数，等待其完成，并返回其错误状态。
// xc 将选择合适的服务器。
func (xc *XClient) Call(ctx context.Context, serviceMethod string, args, reply any) error {
	serverAddr, err := xc.d.Get(xc.mode)
	if err != nil {
		return err
	}
	return xc.call(ctx, serverAddr, serviceMethod, args, reply)
}

// 广播：将请求发送到所有服务实例，并等待所有实例的响应。适用于需要确保所有实例处理请求的场景。
//
// TODO: 负载均衡概念，实现方式
//
// Broadcast 将请求广播到所有的服务实例，如果任意一个实例发生错误，则返回其中一个错误
// 如果调用成功，则返回其中一个的结果
//
// 1. 为了提升性能，请求是并发的
// 2. 并发情况下需要使用互斥锁保证 error 和 reply 能被正确赋值
// 3. 借助 context.WithCancel 确保有错误发生时，快速失败
func (xc *XClient) Broadcast(ctx context.Context, serviceMethod string, args, reply any) error {
	servers, err := xc.d.GetAll()
	if err != nil {
		return err
	}

	var (
		wg sync.WaitGroup
		mu sync.Mutex // protect e and replyDone
		e  error
	)

	replyDone := reply == nil // if reply is nil, don't need to set value
	ctx, cancel := context.WithCancel(ctx)
	defer cancel() // 确保在方法结束后取消 ctx，避免泄漏
	for _, rpcAddr := range servers {
		wg.Add(1)
		go func(rpcAddr string) {
			defer wg.Done()
			var clonedReply any
			if reply != nil {
				clonedReply = reflect.New(reflect.ValueOf(reply).Elem().Type()).Interface()
			}
			err := xc.call(ctx, rpcAddr, serviceMethod, args, reply)
			mu.Lock()
			if err != nil && e == nil {
				e = err
				cancel()
			}
			if err == nil && !replyDone {
				reflect.ValueOf(reply).Elem().Set(reflect.ValueOf(clonedReply).Elem())
			}
			mu.Unlock()
		}(rpcAddr)
	}
	wg.Wait()
	return e
}
