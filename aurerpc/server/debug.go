package server

import (
	"fmt"
	"log"
	"net/http"
	"text/template"

	"aurerpc/constants"
)

// HTML 模版
const debugText = `<html>
	<body>
	<title>AureRPC Services</title>
	{{range .}}
	<hr>
	Service {{.Name}}
	<hr>
		<table>
		<th align=center>Method</th><th align=center>Calls</th>
		{{range $name, $mtype := .Method}}
			<tr>
			<td align=left font=fixed>{{$name}}({{$mtype.ArgType}}, {{$mtype.ReplyType}}) error</td>
			<td align=center>{{$mtype.NumCalls}}</td>
			</tr>
		{{end}}
		</table>
	{{end}}
	</body>
	</html>`

// 使用 text/template 将 debugText 解析为一个模版对象
var debug = template.Must(template.New("RPC debug").Parse(debugText))

type debugHTTP struct {
	*Server
}

// 存储每个服务的名称和方法信息
type debugService struct {
	Name   string
	Method map[string]*MethodType
}

// Runs at /debug/aurerpc
func (server debugHTTP) ServeHTTP(w http.ResponseWriter, req *http.Request) {
	// Build a sorted version of the data.
	var services []debugService
	server.serviceMap.Range(func(namei, svci any) bool {
		svc := svci.(*service)
		services = append(services, debugService{
			Name:   namei.(string),
			Method: svc.method,
		})
		return true
	})
	// 使用模版引擎将数据渲染为HTML并写入响应
	err := debug.Execute(w, services)
	if err != nil {
		_, _ = fmt.Fprintln(w, "rpc: error executing template:", err.Error())
	}
}

func (server *Server) HandleHTTPDebug() {
	// 注册路由处理 RPC 请求
	http.Handle(constants.DefaultRPCPath, server)
	// 注册路由处理调试请求
	http.Handle(constants.DefaultDebugPath, debugHTTP{server})
	log.Println("[RPC server] debug path:", constants.DefaultDebugPath)
}

func HandleHTTPDebug() {
	DefaultServer.HandleHTTPDebug()
}
