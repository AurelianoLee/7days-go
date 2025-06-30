package gee

import (
	"html/template"
	"log"
	"net/http"
	"path"
	"strings"
)

// 定义了类型 HandlerFunc，这是提供给框架用户的，用来定义路由映射的处理方法
// type HandlerFunc func(http.ResponseWriter, *http.Request)
type HandlerFunc func(*Context)

// 实现了路由映射表，提供了用户注册路由到映射表 Router 的方法，包装了启动服务的函数
// 嵌入 RouterGroup 后，(*Engine).engine 就指向了 Engine 本身
type Engine struct {
	// 嵌入 RouterGroup
	// 此处不会有重复定义的问题
	// 因为 Engine 里用到的是 *RouterGroup 的指针，而不是值
	// 同理，RouterGroup 里用到的是 *Engine 的指针，而不是值
	*RouterGroup
	router *router
	groups []*RouterGroup
	// for http render
	htmlTemplates *template.Template
	funcMap       template.FuncMap
}

type RouterGroup struct {
	prefix      string
	middlewares []HandlerFunc
	// 设计模式：回指 Back-Reference
	// 通过在 RouterGroup 中嵌入 Engine 的指针，任何一个 RouterGroup 都可以访问整个引擎的全局配置
	engine *Engine
}

func New() *Engine {
	engine := &Engine{router: newRouter()}
	engine.RouterGroup = &RouterGroup{engine: engine} // 回指自己
	engine.groups = []*RouterGroup{engine.RouterGroup}
	return engine
}

// Group is defined to create a new RouterGroup
// remember all groups share the same Engine instance
func (group *RouterGroup) Group(prefix string) *RouterGroup {
	engine := group.engine // father engine
	newGroup := &RouterGroup{
		prefix: group.prefix + prefix, // 这里会加上所有的prefix
		engine: engine,                // 设置engine字段，确保新组能访问到engine实例
	}
	engine.groups = append(engine.groups, newGroup)
	return newGroup
}

// func (engine *Engine) addRoute(method string, pattern string, handler HandlerFunc) {
// 	engine.router.addRoute(method, pattern, handler)
// }

// func (engine *Engine) GET(pattern string, handler HandlerFunc) {
// 	engine.addRoute("GET", pattern, handler)
// }

// func (engine *Engine) POST(pattern string, handler HandlerFunc) {
// 	engine.addRoute("POST", pattern, handler)
// }

// 将和路由有关的函数，都交给 RouterGroup 实现
// 这样 Engine 只负责启动服务和处理请求，不涉及路由和处理方法的注册
// engine 嵌入 RouterGroup，engine 可以直接使用 `GET` 和 `POST` 方法
func (group *RouterGroup) addRoute(method string, comp string, handler HandlerFunc) {
	pattern := group.prefix + comp
	log.Printf("Route %4s - %s", method, pattern)
	group.engine.router.addRoute(method, pattern, handler)
}

func (group *RouterGroup) GET(pattern string, handler HandlerFunc) {
	group.addRoute("GET", pattern, handler)
}

func (group *RouterGroup) POST(pattern string, handler HandlerFunc) {
	group.addRoute("POST", pattern, handler)
}

// Use 注册中间件
func (group *RouterGroup) Use(middlewares ...HandlerFunc) {
	group.middlewares = append(group.middlewares, middlewares...)
}

func (group *RouterGroup) createStaticHandler(relativePath string, fs http.FileSystem) HandlerFunc {
	// 将相对路径转换为绝对路径
	// 例如：/assets/*filepath -> ~/go/src/aureweb/static/*filepath
	absolutePath := path.Join(group.prefix, relativePath)
	// 创建一个文件服务器，这个文件服务器会处理请求，并返回文件内容
	// 例如：~/go/src/aureweb/static/*filepath -> ~/go/src/aureweb/static/file1.txt
	fileServer := http.StripPrefix(absolutePath, http.FileServer(fs))
	return func(c *Context) {
		file := c.Param("filepath")
		if _, err := fs.Open(file); err != nil {
			c.Status(http.StatusNotFound)
			return
		}
		fileServer.ServeHTTP(c.Writer, c.Req)
	}
}

// serve static files
func (group *RouterGroup) Static(relativePath string, root string) {
	handler := group.createStaticHandler(relativePath, http.Dir(root))
	urlPattern := path.Join(relativePath, "/*filepath")
	group.GET(urlPattern, handler)
}

func (engine *Engine) SetFuncMap(funcMap template.FuncMap) {
	engine.funcMap = funcMap
}

func (engine *Engine) LoadHTMLGlob(pattern string) {
	// template.New("") 创建一个新的、名字为空的模板，这个对象是所有模板的根节点
	// (*Template).Funcs() 给模板引擎注册一个自定义的模板函数，里面可以存放自定义的Go函数，这些函数可以在模板文件中直接调用
	// 例如注册一个 `FormatAsDate` 的函数，在模板文件中可以直接使用 {{ .now | FormatAsDate }} 这样的方法调用
	// (*Template).ParseGlob() 批量解析模板文件，这些文件的扩展名必须是 `.tmpl`
	// 这些模板文件会被解析成一个树形结构，每个模板文件都是一个节点，这些节点会被存储在 `engine.htmlTemplates` 中
	engine.htmlTemplates = template.Must(template.New("").Funcs(engine.funcMap).ParseGlob(pattern))
}

func (engine *Engine) Run(addr string) (err error) {
	return http.ListenAndServe(addr, engine)
}

// w & req 是标准库中 HTTP 服务器在接收到请求时自动创建并传入的
func (engine *Engine) ServeHTTP(w http.ResponseWriter, req *http.Request) {
	var middlewares []HandlerFunc
	for _, group := range engine.groups {
		if strings.HasPrefix(req.URL.Path, group.prefix) { // 如果请求路径有前缀，则添加中间件
			middlewares = append(middlewares, group.middlewares...)
		}
	}
	c := newContext(w, req)
	c.handlers = middlewares
	// day6 template
	c.engine = engine
	engine.router.handle(c)
}
