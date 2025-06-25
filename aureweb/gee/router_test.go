package gee

import (
	"reflect"
	"testing"
)

func newTestRouter() *router {
	r := newRouter()
	r.addRoute("GET", "/", nil)
	r.addRoute("GET", "/hello/:name", nil)
	r.addRoute("GET", "/hello/b/c", nil)
	r.addRoute("GET", "/assets/*filepath", nil)
	return r
}

func TestParsePattern(t *testing.T) {
	ok := reflect.DeepEqual(parsePattern("/p/:name"), []string{"p", ":name"})
	ok = ok && reflect.DeepEqual(parsePattern("/p/*"), []string{"p", "*"})
	ok = ok && reflect.DeepEqual(parsePattern("/p/*name/*"), []string{"p", "*name"})
	if !ok {
		t.Fatal("test parsePattern failed")
	}
}

func TestGetRoute(t *testing.T) {
	r := newTestRouter()
	n, params := r.getRoute("GET", "/hello/geektutu")
	if n == nil {
		t.Fatal("nil shouldn't be returned")
	}
	if n.pattern != "/hello/:name" {
		t.Fatal("should match /hello/:name")
	}
	if params["name"] != "geektutu" {
		t.Fatal("name should be equal to 'geektutu'")
	}

	n, _ = r.getRoute("GET", "/hello/b/c")
	if n == nil {
		t.Fatal("nil shouldn't be returned")
	}
	if n.pattern != "/hello/b/c" {
		t.Fatal("should match /hello/b/c")
	}

	_, params = r.getRoute("GET", "/assets/file.txt")
	if params["filepath"] != "file.txt" {
		t.Fatal("filepath should be equal to 'file.txt'")
	}

	_, params = r.getRoute("GET", "/assets/css/test.css")
	if params["filepath"] != "css/test.css" {
		t.Fatal("filepath should be equal to 'css/test.css'")
	}
}
