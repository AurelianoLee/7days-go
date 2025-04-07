package main

import (
	"fmt"
	"log"
	"net"
	"sync"
	"time"

	"aurerpc/client"
	"aurerpc/server"
)

func startServer(addr chan string) {
	// pick a free port
	l, err := net.Listen("tcp", ":0")
	if err != nil {
		log.Fatal("network error:", err)
	}
	log.Println("start rpc server on", l.Addr())
	addr <- l.Addr().String()
	server.Accept(l)
}

func main() {
	log.SetFlags(0)
	addr := make(chan string)
	go startServer(addr)

	// 一个客户端与服务端的连接，等待服务器启动并获取服务器的地址
	client, _ := client.Dial("tcp", <-addr)
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
			args := fmt.Sprintf("aurerpc req %d", i)
			var reply string
			if err := client.Call("Foo.Sum", args, &reply); err != nil {
				log.Fatal("call Foo.Sum error:", err)
			}
			log.Println("call Foo.Sum reply:", reply)
		}(i)
	}
	wg.Wait()
}
