package gee

import "net/http"

// 定义了类型 HandlerFunc，这是提供给框架用户的，用来定义路由映射的处理方法
// type HandlerFunc func(http.ResponseWriter, *http.Request)
type HandlerFunc func(*Context)

// 实现了路由映射表，提供了用户注册路由到映射表 Router 的方法，包装了启动服务的函数
type Engine struct {
	router *router
}

func New() *Engine {
	return &Engine{router: newRouter()}
}

func (engine *Engine) addRoute(method string, pattern string, handler HandlerFunc) {
	engine.router.addRoute(method, pattern, handler)
}

func (engine *Engine) GET(pattern string, handler HandlerFunc) {
	engine.addRoute("GET", pattern, handler)
}

func (engine *Engine) POST(pattern string, handler HandlerFunc) {
	engine.addRoute("POST", pattern, handler)
}

func (engine *Engine) Run(addr string) (err error) {
	return http.ListenAndServe(addr, engine)
}

func (engine *Engine) ServeHTTP(w http.ResponseWriter, req *http.Request) {
	c := newContext(w, req)
	engine.router.handle(c)
}
