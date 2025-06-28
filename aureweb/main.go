package main

import (
	"aureweb/gee"
	"net/http"
)

func main() {
	r := gee.New()

	// day 1 add http server
	// r.GET("/", func(w http.ResponseWriter, req *http.Request) {
	// 	fmt.Fprintf(w, "URL.Path = %q\n", req.URL.Path)
	// })

	// r.GET("/hello", func(w http.ResponseWriter, req *http.Request) {
	// 	for k, v := range req.Header {
	// 		fmt.Fprintf(w, "Header[%q] = %q\n", k, v)
	// 	}
	// })

	// day 2 add context and router
	// r.GET("/", func(c *gee.Context) {
	// 	c.HTML(http.StatusOK, "<h1>Hello Gee</h1>")
	// })

	// r.GET("/hello", func(c *gee.Context) {
	// 	c.String(http.StatusOK, "hello %s, you're at %s\n", c.Query("name"), c.Path)
	// })

	// r.POST("/login", func(c *gee.Context) {
	// 	c.JSON(http.StatusOK, gee.H{
	// 		"username": c.PostForm("username"),
	// 		"password": c.PostForm("password"),
	// 	})
	// })

	// day 3 add router and params
	// r.GET("/", func(c *gee.Context) {
	// 	c.HTML(http.StatusOK, "<h1>Hello Gee</h1>")
	// })
	// r.GET("/hello", func(c *gee.Context) {
	// 	c.String(http.StatusOK, "hello %s, you're at %s\n", c.Query("name"), c.Path)
	// })
	// r.GET("/hello/:name", func(c *gee.Context) {
	// 	c.String(http.StatusOK, "hello %s, you're at %s\n", c.Param("name"), c.Path)
	// })
	// r.GET("/assets/*filepath", func(c *gee.Context) {
	// 	c.JSON(http.StatusOK, gee.H{"filepath": c.Param("filepath")})
	// })

	// day 4 group router
	r.GET("/index", func(c *gee.Context) {
		c.HTML(http.StatusOK, "<h1>Index Page</h1>")
	})
	v1 := r.Group("/v1")
	{
		v1.GET("/", func(c *gee.Context) {
			c.HTML(http.StatusOK, "<h1>Hello Gee</h1>")
		})
		v1.GET("/hello", func(c *gee.Context) {
			// e.g. /v1/hello?name=aure
			c.String(http.StatusOK, "hello %s, you're at %s\n", c.Query("name"), c.Path)
		})
	}

	v2 := r.Group("/v2")
	{
		v2.GET("/hello/:name", func(c *gee.Context) {
			// e.g. /v2/hello/aure
			c.String(http.StatusOK, "hello %s, you're at %s\n", c.Param("name"), c.Path)
		})
		v2.POST("/login", func(c *gee.Context) {
			c.JSON(http.StatusOK, gee.H{
				"username": c.PostForm("username"),
				"password": c.PostForm("password"),
			})
		})
	}

	r.Run(":9999")
}
