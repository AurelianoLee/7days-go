# Web框架

为什么实现一个Web应用，我们会想到使用框架呢？框架在实践中为我们实现了什么功能？

一些我们常用的需求：

- 动态路由：例如 `hello/:name`，`hello/*` 这类的规则。
- 鉴权：没有分组/统一鉴权的能力。
- 模板：没有统一简化的HTML机制。

当我们离开框架时，使用基础库时，需要频繁手工处理的地方，就是框架的价值。

框架的核心能力：

- 路由：将请求映射到函数，支持动态路由。`hello/:name`
- 模板：使用内置模板引擎提供模板渲染机制。
- 工具集：提供对cookies, headers 等处理机制。
- 插件：提供插件机制，可以选择安装到全局，也可以只针对某几个路由生效。

## Day1 HTTP 基础

`HandlerFunc` 给框架用户，用来定义路由映射的处理方法。
`(*Engine).Get() / (*Engine).Post()` 用户用来注册路由到映射表 Router。
Engine实现的 ServeHTTP 方法的作用就是，解析请求的路径，查找路由映射表，如果查到，就执行注册的处理方法，如果查不到，就返回`404 NOT FOUND`。

## Day2 Context & Router

### Context 上下文

- 将`路由 Router`独立出来，方便之后增强。
- 设计`上下文 context`，封装 request 和 response，提供对JSON，HTML等类型的支持。

- `Handler` 的参数变成了 `gee.Context`，提供了查询`Query/PostForm`参数的功能。
- `gee.Context`封装了`HTML/String/JSON`函数，能够快速构造HTTP响应。

#### 为什么要设计Context

对于Web服务来说，归根结底就是根据请求`*http.Request`，构造响应`http.ResponseWriter`。
但是这两个对象提供的接口太细：如果我们要构造一个完整的响应，需要考虑消息头Header和消息体Body,
而Header中包含了状态码StatusCode，消息类型ContentType等几乎每次请求都需要设置。
因此，需要对用户大量重复、繁杂的，甚至是容易出错的代码进行封装。针对常用场景，能够高效地构造出HTTP响应是一个好的框架必须考虑的。

对于框架来说，还需要支撑额外的功能。

- 解析动态路由`hello/:name`时，参数`:name`的值存储的位置。
- 框架需要支持中间件，中间件产生的信息存放的位置。

`Context` 随着每一个请求的出现而产生，请求的结束而销毁，和当前请求强相关的信息都应该由`context`承载。
设计 `Context` 结构，扩展性和复杂性留在了内部，对外简化了接口。
路由的处理函数，以及将要实现的中间件，参数都统一使用 `Context` 实例。

### Router 路由

将路由相关的方法和结构提取出来，放到一个新的文件中，方便下一次对router的功能进行增强，例如提供动态路由的支持。

### Conclusion

context 封装 `req & resp`，同时暴露常用的属性，对从`req`提取参数的方法进行封装，快速构建`resp`的方法进行封装。
router 处理路由的 handle，同时为之后扩展动态路由提供方便。

### Test

```bash
curl -i http://localhost:9999/

curl "http://localhost:9999/hello?name=geektutu"

curl "http://localhost:9999/login" -X POST -d 'username=geektutu&password=1234'
```

## Day3 Router 动态路由

- 使用 Trie 树实现动态路由解析
- 支持两种模式`:name`和`*filepath`

HTTP请求的路径恰好是由`/`分隔的多段构成的，因此，每一段可以作为一个前缀树的节点。
通过树结构的查询，如果中间某一层的节点都不满足条件，那么就说明没有匹配到正确的路由，查询结束。

动态路由需要具备以下功能：

- 参数匹配：例如`/p/:lang/doc`，可以匹配`/p/en/doc`和`/p/zh/doc`。
- 通配符：`/static/*filepath`，可以匹配`/static/fav.ico`和`/static/js/jQuery.js`，这种模式常用于静态服务器，能够递归地匹配子路径。

对于路由来说，最重要的功能有两个：注册和匹配。

- 注册路由时，需要映射 handler。-> 对于节点的插入，递归查找每一层的节点，如果没有匹配到当前part的节点，则新建一个。同时只有在插入到路径的最后一个节点时，才会在trie的pattern中设置当前的完整路径。因此，当查询路由时，这个pattern会很有用。
- 访问路由时，需要快速找到对应的 handler。-> 对于节点的查询，递归查询每一层的节点，退出规则是：匹配到了`*`，匹配失败，或者匹配到了第`len(parts)`层节点。

将`Trie`树应用到路由中，使用 `roots` 来存储每种请求方式的 `Trie` 树根节点，使用 `handlers` 来存储每种请求方式的 `HandlerFunc`。

`getRoute`函数中，还解析了`:`和`*`两种匹配符的参数，返回一个map。
例如`/p/go/doc`匹配到`/p/:lang/doc`，解析结果为`{lang: "go"}`，`/static/css/geektutu.css`匹配到`/static/*filepath`，解析结果为`{filepath: "css/geektutu.css"}`。

在`HandlerFunc`中，希望能够访问到解析的参数，因此，需要对Context对象增加一个属性和方法，来提供对路由参数的访问。在路由解析后，我们会得到一个`params`，把这个`params`存放到`Context`中，通过`c.Param("lang")`的方式可以获取到对应的值。

```bash
curl "http://localhost:9999/hello/geektutu"

curl "http://localhost:9999/assets/css/file.css"
```

## Day4 Group 分组控制

在真实的业务中，往往是某一组路由需要相似的处理，例如：

- 以`/post`开头的路由匿名可访问。
- 以`/admin`开头的路由需要鉴权。
- 以`/api`开头的路由是`RESTFul`接口，可以对接第三方平台，需要三方平台鉴权。

大部分情况下的路由分组，是以相同的前缀来区分的。因此这里的分组控制是根据前缀来区分的，同时还支持分组的嵌套。
作用在父分组上的中间件，也会作用在子分组上，子分组还可以应用自己特有的中间件。

中间件可以给框架提供无限的扩展能力，应用在分组上，可以使分组控制的收益更加明显。
例如`/admin`分组可以应用鉴权中间件，`/`分组可以应用日志中间件，`/`默认是最顶层的分组，也就意味着给所有的路由添加了记录日志的能力。

### 分组嵌套

一个Group对象需要具备哪些属性呢？首先是前缀`prefix`，比如`/`，`/api`。
要支持分组嵌套，需要知道该`Group`的父亲是谁。
中间件是应用在分组上的，还需要记录存储在该分组上的中间件。

设计`RouterGroup`，包含`prefix`、`middleware`、`RouterGroup`和`Engine`。将原先的`Engine`改造为嵌入`RouterGroup`，加上`router`和`groups []*RouterGroup`。
把`addRouter()`、`GET`、`POST`方法放入`RouterGroup`中，添加路径时加上`group`的`prefix`。

```bash
curl "http://localhost:9999/index"

curl "http://localhost:9999/v1"

curl "http://localhost:9999/v1/hello?name=aure"

curl "http://localhost:9999/v2/hello/aure"

curl "http://localhost:9999/v2/login" -X POST -d 'username=aure&password=1234'
```

## Day5 MiddleWare 中间件

中间件就是非业务的技术类组件，但是不太适合由框架统一支持。框架必须有一个插口，允许用户自己定义功能，嵌入到框架中，仿佛这个功能是框架原生支持的一般。

对于中间件而言，有两个关键点：

- 插入点：插入点不能太深，如果插入点太底层，中间件会实现的比较复杂，如果插入点离用户太近，用户不如直接定义一组函数，放在`handlerfunc`中手工调用。
- 中间件的输入：中间件的输入不宜过多，用户不清楚具体的参数；输入太少，用户的发挥空间有限。

### 中间件设计

中间件的定义与路由映射的 `Handler` 一致，处理的输入是 `Context` 对象。插入点是框架接收到请求初始化 `Context` 对象后，允许用户使用自己定义的中间件做一些额外的处理，以及对 `Context` 进行二次加工。
通过调用`(*Context).Next()` 函数，中间件可以等待用户自己定义的 `Handler` 处理结束后，再做一些额外的操作，例如计算本次处理所用的时间。
`c.Next()` 表示执行其他的中间件或 `HandlerFunc`。

`gee.go` 的 `ServeHTTP()` 需要根据前缀将所有的中间件注册到 `(*Context).handler[]` 中。
`router.go`里的 `handle` 需要将这个路由注册的 `HandlerFunc` 注册到 `(*Context).handler[]` 中。

```bash
curl http://localhost:9999/

curl http://localhost:9999/v2/hello/geektutu
```

## Day6 Template 模板

实现前后端分离。

采用 Go 标准库自带的 `template` 库实现对html模板的解析。方便进行前后端分离。

Go 标准库 `template` 库使用指南：

- [text/template](https://pkg.go.dev/text/template)
- [html/template](https://pkg.go.dev/html/template)
- [go-by-example](https://gobyexample.com/text-templates)

## Day7 Panic 错误处理

对于一个 Web 框架，可能会有各种情况发生，例如用户输入不正确的参数、触发了某些异常、数组越界、空指针等。
如果因为这些原因导致系统宕机，是不可接受的。因此需要错误处理机制。

可以使用中间件来支持框架的错误处理。

中间件的设计就是在 `routerHandler` 之前和之后插入可以运行的函数，在所有的函数之前插入捕获函数。

```go
defer func() {
    if err := recover(); err != nil {
        //...
    }
}()

c.Next()
```

在`trace()`中使用了 `var pcs [32]uintptr n := runtime.Callers(3, pcs[:])` 的写法。为什么不直接使用切片语法？

```go
// 当前代码：栈分配，零GC压力
var pcs [32]uintptr
n := runtime.Callers(3, pcs[:])

// 替代方案：堆分配，需要GC
pcs := make([]uintptr, 32)
n := runtime.Callers(3, pcs)
```

1. 内存分配优化
2. 性能优势
   1. 栈分配：数组在栈上分配，分配和释放都是零成本
   2. 避免GC
   3. 更好的缓存局部性：栈内存通常有更好的缓存命中率
3. Go语言处理固定大小缓冲区的标准模式
    ```go
    var buf [1024]byte
    n, err := reader.Read(buf[:])

    var key [32]byte
    copy(key[:], data)
    ```
4. 性能测试

```bash
goos: linux
goarch: amd64
cpu: AMD Ryzen 7 7840H with Radeon 780M Graphics    
BenchmarkArray-16        2193489               516.3 ns/op            88 B/op          8 allocs/op
BenchmarkSlice-16        2274103               514.8 ns/op            88 B/op          8 allocs/op
```
