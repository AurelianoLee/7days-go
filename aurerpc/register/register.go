package register

import (
	"log"
	"net/http"
	"sort"
	"strings"
	"sync"
	"time"
)

const (
	defaultPath             = "/_aurerpc_/registry"
	defaultTimeout          = 5 * time.Minute // 超时时间
	HeaderGetAllServersList = "X-Aurerpc-Servers"
	HeaderPostAppend        = "X-Aurerpc-Server"
)

type Registry struct {
	timeout  time.Duration
	mu       sync.Mutex
	services map[string]*ServerItem
}

type ServerItem struct {
	Addr  string
	Start time.Time
}

func New(timeout time.Duration) *Registry {
	if timeout <= 0 {
		timeout = defaultTimeout
	}
	return &Registry{
		timeout:  timeout,
		services: make(map[string]*ServerItem),
	}
}

var DefaultRegistry = New(defaultTimeout)

// putServer add server address to registry center, if it exists, update its start time
//
// 将服务器地址添加到注册中心，如果已存在则更新其开始时间
func (r *Registry) putServer(addr string) {
	r.mu.Lock()
	defer r.mu.Unlock()

	if item, ok := r.services[addr]; ok {
		item.Start = time.Now() // 更新服务的开始时间
	} else {
		r.services[addr] = &ServerItem{
			Addr:  addr,
			Start: time.Now(),
		}
	}
}

// listAliveServers list all alive servers and remove those that have timed out
func (r *Registry) listAliveServers() []string {
	r.mu.Lock()
	defer r.mu.Unlock()

	var aliveServers []string
	for addr, item := range r.services {
		if time.Since(item.Start) < r.timeout {
			aliveServers = append(aliveServers, addr)
		} else {
			delete(r.services, addr)
		}
	}
	sort.Strings(aliveServers)
	return aliveServers
}

// ServeHTTP runs at /_aurerpc_/registry, handles GET and POST requests
func (r *Registry) ServeHTTP(w http.ResponseWriter, req *http.Request) {
	switch req.Method {
	case http.MethodGet:
		aliveServers := r.listAliveServers()
		w.Header().Set(HeaderGetAllServersList, strings.Join(aliveServers, ","))
	case http.MethodPost:
		addr := req.Header.Get(HeaderPostAppend)
		if addr == "" {
			http.Error(w, "Server address is required", http.StatusBadRequest)
			return
		}
		r.putServer(addr)
		w.WriteHeader(http.StatusOK)
	default:
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
	}
}

// HandleHTTP binds the registry to a specific path
func (r *Registry) HandleHTTP(registryPath string) {
	http.Handle(registryPath, r) // 将 registryPath 绑定到实例 r 上
	log.Println("Aurerpc registry is running at", registryPath)
}

func HandleHTTP() {
	DefaultRegistry.HandleHTTP(defaultPath)
}

func sendHeartbeat(registry, addr string) error {
	log.Println("Sending heartbeat to registry:", registry, "from server:", addr)
	httpClient := &http.Client{}
	req, err := http.NewRequest(http.MethodPost, registry, nil)
	if err != nil {
		log.Println("Failed to create heartbeat request:", err)
		return err
	}
	req.Header.Set(HeaderPostAppend, addr)
	if _, err := httpClient.Do(req); err != nil {
		log.Println("Failed to send heartbeat:", err)
		return err
	}
	return nil
}

func Heartbeat(registry, addr string, interval time.Duration) {
	if interval <= 0 {
		interval = defaultTimeout - 1*time.Minute
	}

	err := sendHeartbeat(registry, addr) // initial heartbeat
	if err != nil {
		log.Println("Initial heartbeat failed:", err)
		return
	}
	go func() {
		ticker := time.NewTicker(interval)
		defer ticker.Stop()
		// should not use for { select { case <-ticker.C: } } if not other channel
		// to exit this goroutine, otherwise it will block forever
		for range ticker.C {
			if err := sendHeartbeat(registry, addr); err != nil {
				log.Println("Heartbeat failed:", err)
				break
			}
		}
	}()
	log.Println("Heartbeat goroutine started for server:", addr)
}
