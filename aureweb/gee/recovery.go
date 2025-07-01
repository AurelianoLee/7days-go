package gee

import (
	"fmt"
	"log"
	"net/http"
	"runtime"
	"strings"
)

// trace 返回调用栈信息
func trace(message string) string {
	// runtime.Callers 返回调用栈信息
	// 跳过前三个调用
	// 第0个是 runtime.Callers 本身
	// 第1个是 Recovery 本身
	// 第2个是调用 Recovery 的函数（通常是 Recovery 的中间件）
	// 所以从第3个开始是真正的调用栈
	var pcs [32]uintptr
	n := runtime.Callers(3, pcs[:])

	var str strings.Builder
	str.WriteString(message + "\nTraceback:")
	// 对每个程序计数器值
	for _, pc := range pcs[:n] {
		fn := runtime.FuncForPC(pc)   // 根据程序计数器获取函数信息
		file, line := fn.FileLine(pc) // 获取该函数调用点的文件名和行号
		str.WriteString(fmt.Sprintf("\n\t%s:%d", file, line))
	}
	return str.String()
}

func Recovery() HandlerFunc {
	return func(c *Context) {
		defer func() {
			if err := recover(); err != nil {
				message := fmt.Sprintf("%s", err)
				log.Printf("[Recovery] panic recovered:\n%s\n", trace(message))
				c.Fail(http.StatusInternalServerError, "Internal Server Error")
			}
		}()
		c.Next()
	}
}
