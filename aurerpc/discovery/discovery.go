package discovery

import (
	"errors"
	"math"
	"math/rand"
	"sync"
	"time"
)

type SelectMode int

const (
	RandomSelect SelectMode = iota
	RoundRobinSelect
)

// interface 类型，包含了服务发现所需要的接口
type Discovery interface {
	Refresh() error                      // 从注册中心更新服务列表
	Update(servers []string) error       // 手动更新服务列表
	Get(mode SelectMode) (string, error) // 根据负载均衡策略，选择一个服务实例，返回一个服务器地址
	GetAll() ([]string, error)           // 返回所有的服务实例
}

// r 是一个生产随机数的实例，初始化时使用时间戳设定随机数种子，避免每次产生相同的随机数序列
// index 记录 Round Robin 算法已经轮询到的位置，为了避免每次从0开始，初始化时随机设定一个值
//
// MultiServerDiscovery 是一种无需注册中心即可发现多台服务器的方法
// 用户需要明确提供服务器地址
//
// MultiServerDiscovery is a discovery for multi servers without a registry center
// user provides the server addresses explicitly instead
type MultiServerDiscovery struct {
	r       *rand.Rand   // generate random number
	mu      sync.RWMutex // protect following
	servers []string
	index   int // record the selected position for robin algorithm
}

func NewMultiServerDiscovery(servers []string) *MultiServerDiscovery {
	d := &MultiServerDiscovery{
		servers: servers,
		r:       rand.New(rand.NewSource(time.Now().UnixNano())),
	}
	d.index = d.r.Intn(math.MaxInt32 - 1)
	return d
}

var _ Discovery = (*MultiServerDiscovery)(nil)

// Refresh doesn't make sense for MultiServerDiscovery, so ignore it
func (d *MultiServerDiscovery) Refresh() error {
	return nil
}

// Update the servers of discovery dynamically if needed
func (d *MultiServerDiscovery) Update(servers []string) error {
	d.mu.Lock()
	defer d.mu.Unlock()
	d.servers = servers
	return nil
}

// Get a server according to mode
// return a server address
func (d *MultiServerDiscovery) Get(mode SelectMode) (string, error) {
	d.mu.Lock()
	defer d.mu.Unlock()
	n := len(d.servers)
	if n == 0 {
		return "", errors.New("rpc discovery: no available servers")
	}

	switch mode {
	case RandomSelect:
		return d.servers[d.r.Intn(n)], nil
	case RoundRobinSelect:
		s := d.servers[d.index%n] // servers could be updated, so mode n to ensure safety
		d.index = (d.index + 1) % n
		return s, nil
	default:
		return "", errors.New("rpc discovery: no support select mode")
	}
}

// returns all servers in discovery
func (d *MultiServerDiscovery) GetAll() ([]string, error) {
	d.mu.RLock()
	defer d.mu.RUnlock()
	// return a copy of d.servers
	servers := make([]string, len(d.servers))
	copy(servers, d.servers)
	return servers, nil
}
