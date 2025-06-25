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
