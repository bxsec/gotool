package share

import (
	"github.com/fasthttp/websocket"
	"net/http"
	"strings"
)

type Engine struct {
	*RouterGroup
	router *router
	groups []*RouterGroup // store all groups

}

// New is the constructor of gee.Engine
func New() *Engine {
	engine := &Engine{router: newRouter()}
	engine.RouterGroup = &RouterGroup{engine: engine}
	engine.groups = []*RouterGroup{engine.RouterGroup}
	return engine
}

// Default use Logger() & Recovery middlewares
func Default() *Engine {
	engine := New()
	engine.Use(Logger(), Recovery())
	return engine
}


func (engine *Engine) RunHttp(addr string) (err error) {
	return http.ListenAndServe(addr, engine)
}

func (engine *Engine) ServeHTTP(w http.ResponseWriter, req *http.Request) {
	var middlewares []HandlerFunc
	for _, group := range engine.groups {
		if strings.HasPrefix(req.URL.Path, group.prefix) {
			middlewares = append(middlewares, group.middlewares...)
		}
	}
	c := newHttpContext(w, req)
	c.handlers = middlewares
	c.engine = engine
	engine.router.handle(c)
}

func (engine *Engine) ServeWS(ws *websocket.Conn, path string, method string, payload []byte) {
	var middlewares []HandlerFunc
	for _, group := range engine.groups {
		if strings.HasPrefix(path, group.prefix) {
			middlewares = append(middlewares, group.middlewares...)
		}
	}
	c := newWsContext(ws, path, method, payload)
	c.handlers = middlewares
	c.engine = engine
	engine.router.handle(c)
}