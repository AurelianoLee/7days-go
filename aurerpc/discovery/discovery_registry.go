package discovery

import (
	"aurerpc/register"
	"log"
	"net/http"
	"strings"
	"time"
)

type RegistryDiscovery struct {
	*MultiServerDiscovery
	registry   string        // registry address
	timeout    time.Duration // timeout for service registration
	lastUpdate time.Time     // last update servers list time from registry
}

const (
	defaultUpdateTimeout = 10 * time.Second
)

func NewRegistryDiscovery(registryAddr string, timeout time.Duration) *RegistryDiscovery {
	if timeout <= 0 {
		timeout = defaultUpdateTimeout
	}
	return &RegistryDiscovery{
		MultiServerDiscovery: NewMultiServerDiscovery(make([]string, 0)),
		registry:             registryAddr,
		timeout:              timeout,
	}
}

// Update 注册中心触发的服务列表更新
func (d *RegistryDiscovery) Update(servers []string) error {
	d.MultiServerDiscovery.Update(servers)
	d.lastUpdate = time.Now()
	return nil
}

// Refresh 从注册中心获取最新的服务列表
func (d *RegistryDiscovery) Refresh() error {
	d.mu.Lock()
	defer d.mu.Unlock()
	// 1. 检查是否需要刷新
	if d.lastUpdate.Add(d.timeout).After(time.Now()) {
		// no need to refresh, still within the timeout
		return nil
	}
	log.Printf("[RPC registry] refresh discovery from registry %s", d.registry)

	// 2. 从注册中心获取最新的服务列表
	resp, err := http.Get(d.registry)
	if err != nil {
		log.Printf("[RPC registry] refresh discovery from registry %s failed: %v", d.registry, err)
		return err
	}

	// 3. 从Header中获取服务器列表
	servers := strings.Split(resp.Header.Get(register.HeaderGetAllServersList), ",")
	d.servers = make([]string, 0, len(servers))

	// 4. 遍历服务器列表，去除空白字符并添加到d.servers中
	for _, s := range servers {
		if s = strings.TrimSpace(s); s != "" {
			// only add non-empty server addresses
			d.servers = append(d.servers, s)
		}
	}
	d.lastUpdate = time.Now() // update last update time
	log.Printf("[RPC registry] refresh discovery from registry %s success, servers: %v", d.registry, d.servers)
	return nil
}

func (d *RegistryDiscovery) Get(mode SelectMode) (string, error) {
	// 在获取服务器之前先刷新服务列表，确保服务列表没有过期
	if err := d.Refresh(); err != nil {
		return "", err
	}
	return d.MultiServerDiscovery.Get(mode)
}

func (d *RegistryDiscovery) GetAll() ([]string, error) {
	// 在获取所有服务器之前先刷新服务列表，确保服务列表没有过期
	if err := d.Refresh(); err != nil {
		return nil, err
	}
	return d.MultiServerDiscovery.GetAll()
}
