package gee

import (
	"net/http"
	"strings"
)

type router struct {
	roots    map[string]*node
	handlers map[string]HandlerFunc
}

// 初始化路由，创建roots和handlers的map
//
// roots key 是 method，value是trie树的根节点
// eg: map[string]*node{"GET": &node{}, "POST": &node{}}
//
// handlers key 是 method-pattern，value是handler函数
// eg: map[string]HandlerFunc{"GET-/p/:lang/doc": func(c *gee.Context) {}}
func newRouter() *router {
	return &router{
		roots:    make(map[string]*node),
		handlers: make(map[string]HandlerFunc),
	}
}

// 解析pattern，返回一个字符串切片，每个元素是pattern中的一个部分
//
// 例如，/p/:lang/doc 解析为 ["p", ":lang", "doc"]
// 解析规则是：
// 1. 将pattern按/分割
// 2. 如果元素不为空，则添加到parts中
// 3. 如果元素以*开头，则停止解析
func parsePattern(pattern string) []string {
	vs := strings.Split(pattern, "/")
	parts := make([]string, 0)
	for _, item := range vs {
		if item != "" {
			parts = append(parts, item)
			if item[0] == '*' {
				break
			}
		}
	}
	return parts
}

func (r *router) addRoute(method string, pattern string, handler HandlerFunc) {
	// log.Printf("Route %4s - %s", method, pattern)
	// key := method + "-" + pattern
	// r.handlers[key] = handler

	parts := parsePattern(pattern)
	// 如果method对应的trie树不存在，则新建一个
	_, ok := r.roots[method]
	if !ok {
		r.roots[method] = &node{}
	}
	r.roots[method].insert(pattern, parts, 0)
	key := method + "-" + pattern
	r.handlers[key] = handler
}

func (r *router) getRoute(method string, path string) (*node, map[string]string) {
	// searchParts 包含的是用户请求的实际的路径值，不包含*和:
	searchParts := parsePattern(path)
	root, ok := r.roots[method]
	if !ok {
		return nil, nil
	}

	node := root.search(searchParts, 0)
	if node != nil {
		// parts 包含的是路由注册时的模式，包括*和:
		parts := parsePattern(node.pattern)
		params := make(map[string]string)
		for index, part := range parts {
			if part[0] == ':' {
				// 如果part以:开头，则将part的值作为params的key，searchParts[index]作为params的value
				params[part[1:]] = searchParts[index]
			}
			if part[0] == '*' && len(part) > 1 {
				// 如果part以*开头，则将searchParts中从index开始的元素拼接起来，作为params的值
				params[part[1:]] = strings.Join(searchParts[index:], "/")
			}
		}
		return node, params
	}
	return nil, nil
}

func (r *router) handle(c *Context) {
	// 如果当前请求的路由在路由表中，则执行对应的handler
	node, params := r.getRoute(c.Method, c.Path)
	if node != nil {
		c.Params = params
		key := c.Method + "-" + node.pattern
		handler := r.handlers[key]
		c.handlers = append(c.handlers, handler)
	} else {
		c.handlers = append(c.handlers, func(c *Context) {
			c.String(http.StatusNotFound, "404 NOT FOUND: %s\n", c.Path)
		})
	}
	c.Next()
}
