package main

import (
	"context"
	"log"
	"net"
	"net/http"
	"sync"
	"time"

	"aurerpc/client"
	"aurerpc/discovery"
	"aurerpc/server"
)

// ---------------------------- server --------------------------------

type Foo int

type Args struct {
	Num1, Num2 int
}

func (f Foo) Sum(args Args, reply *int) error {
	*reply = args.Num1 + args.Num2
	return nil
}

// For testing xclient timeout
func (f Foo) Sleep(args Args, reply *int) error {
	time.Sleep(time.Second * time.Duration(args.Num1))
	*reply = args.Num1 + args.Num2
	return nil
}

func startDefaultServer(addr chan string) {
	var foo Foo
	// FIXME:
	// rpcServer := server.NewServer()
	if err := server.Register(&foo); err != nil {
		log.Fatal("register error: ", err)
	}
	// pick a free port
	l, err := net.Listen("tcp", ":0")
	if err != nil {
		log.Fatal("network error:", err)
	}
	log.Println("start rpc server on", l.Addr())
	addr <- l.Addr().String()
	server.Accept(l)
}

func startHTTPServer(addrCh chan string) {
	var foo Foo
	l, _ := net.Listen("tcp", ":9999")
	_ = server.Register(&foo)
	server.HandleHTTPDebug()
	addrCh <- l.Addr().String()
	_ = http.Serve(l, nil)
}

func startServer(addr chan string) {
	var foo Foo
	rpcServer := server.NewServer()
	if err := rpcServer.Register(&foo); err != nil {
		log.Fatal("register error: ", err)
	}
	// pick a free port
	l, err := net.Listen("tcp", ":0")
	if err != nil {
		log.Fatal("network error:", err)
	}
	log.Println("start rpc server on", l.Addr())
	addr <- l.Addr().String()
	// Note: this is new server, not default server
	rpcServer.Accept(l)
}

// ------------------------------ main --------------------------------
func main() {
	log.SetFlags(0)

	// start a basic server and client
	// addr := make(chan string)
	// go startDefaultServer(addr)
	// call(addr)

	// start http server and client
	// {
	// 	addr := make(chan string)
	// 	go callFromHTTPClient(addr)
	// 	startHTTPServer(addr)
	// }

	// start a load balance client and server
	{
		ch1 := make(chan string)
		ch2 := make(chan string)
		// start two server
		go startServer(ch1)
		go startServer(ch2)

		addr1 := <-ch1
		addr2 := <-ch2

		time.Sleep(time.Second)
		callForLoadBalance(addr1, addr2)
		broadcastForLoadBalance(addr1, addr2)
	}
}

// ------------------------------ client --------------------------------

// the basic call
func call(addrCh chan string) {
	// 一个客户端与服务端的连接，等待服务器启动并获取服务器的地址
	client, _ := client.Dial("tcp", <-addrCh)
	defer func() { _ = client.Close() }()

	// in fact, following code is like a simple aurerpc client
	// 模拟了一个客户端与服务端的连接，等待服务器启动并获取服务器的地址
	// 这个连接是一个 IO 操作
	// conn, _ := net.Dial("tcp", <-addr)
	// defer func() { _ = conn.Close() }()

	time.Sleep(time.Second)
	var wg sync.WaitGroup
	// send request & receive response
	for i := range 5 {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			args := &Args{Num1: i, Num2: i * i}
			ctx, cancel := context.WithTimeout(context.Background(), time.Second*10)
			defer cancel()
			var reply int
			if err := client.Call(ctx, "Foo.Sum", args, &reply); err != nil {
				log.Fatal("call Foo.Sum error:", err)
			}
			// log.Println("call Foo.Sum reply:", reply)
			log.Printf("%d + %d = %d", args.Num1, args.Num2, reply)
		}(i)
	}
	wg.Wait()
}

// a client used http call
func callFromHTTPClient(addrCh chan string) {
	client, _ := client.DialHTTP("tcp", <-addrCh)
	defer func() {
		_ = client.Close()
	}()

	time.Sleep(time.Second)
	// send request & receive response
	var wg sync.WaitGroup
	for i := range 5 {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			args := &Args{Num1: i, Num2: i * i}
			var reply int
			if err := client.Call(context.Background(), "Foo.Sum", args, &reply); err != nil {
				log.Fatal("call Foo.Sum failed: ", err)
			}
			log.Printf("%d + %d = %d", args.Num1, args.Num2, reply)
		}(i)
	}
	wg.Wait()
}

// print load balance info
func foo(ctx context.Context, xc *client.XClient, typ, serviceMethod string, args *Args) {
	var reply int
	var err error
	switch typ {
	case "call":
		err = xc.Call(ctx, serviceMethod, args, &reply)
	case "broadcast":
		err = xc.Broadcast(ctx, serviceMethod, args, &reply)
	}
	if err != nil {
		log.Printf("%s %s error: %v", typ, serviceMethod, err)
	} else {
		log.Printf("%s %s success: %d + %d = %d", typ, serviceMethod, args.Num1, args.Num2, reply)
	}
}

// use a load balance client
func callForLoadBalance(addr1, addr2 string) {
	d := discovery.NewMultiServerDiscovery([]string{"tcp@" + addr1, "tcp@" + addr2})
	xc := client.NewXClient(d, discovery.RandomSelect, nil)
	defer func() {
		_ = xc.Close()
	}()
	// send request & receive response
	var wg sync.WaitGroup
	for i := range 5 {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			foo(context.Background(), xc, "call", "Foo.Sum", &Args{Num1: i, Num2: i * i})
		}(i)
	}
	wg.Wait()
}

// use a load balance client
func broadcastForLoadBalance(addr1, addr2 string) {
	d := discovery.NewMultiServerDiscovery([]string{"tcp@" + addr1, "tcp@" + addr2})
	xc := client.NewXClient(d, discovery.RandomSelect, nil)
	defer func() {
		_ = xc.Close()
	}()

	var wg sync.WaitGroup
	for i := range 5 {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			foo(context.Background(), xc, "broadcast", "Foo.Sum", &Args{Num1: i, Num2: i * i})
			ctx, cancel := context.WithTimeout(context.Background(), time.Second*2)
			defer cancel()
			foo(ctx, xc, "broadcast", "Foo.Sleep", &Args{Num1: i, Num2: i * i})
		}(i)
	}
	wg.Wait()
}
