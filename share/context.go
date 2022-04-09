package share

import (
	"context"
	"encoding/json"
	"fmt"
	"github.com/fasthttp/websocket"
	"net"
	"net/http"
	"reflect"
	"sync"
)

const (
	unknowContext int = iota
	httpContext
	wsContext
	streamContext

)


// Context is a rpcx customized Context that can contains multiple values.
type Context struct {
	Conn net.Conn

	// http origin objects
	Writer http.ResponseWriter
	Req    *http.Request

	// websocket
	WebsocketConn *websocket.Conn

	contextType int

	Params   map[string]string
	mu sync.RWMutex
	Keys     map[interface{}]interface{}
	
	
	context.Context

	Method string
	Path string
	Payload []byte

	// response info
	StatusCode int
	// middleware
	handlers []HandlerFunc
	index    int

	// engine pointer
	engine *Engine
}

func NewContext(ctx context.Context) *Context {
	return &Context{
		contextType: unknowContext,
		Context: ctx,
		Keys:    make(map[interface{}]interface{}),
	}
}

func newHttpContext(w http.ResponseWriter, req *http.Request) *Context {
	return &Context{
		contextType: httpContext,
		Writer: w,
		Req:    req,
		Path:   req.URL.Path,
		Method: req.Method,
	}
}

func newWsContext(ws *websocket.Conn, path string, method string, payload []byte) *Context {
	return &Context{
		contextType: wsContext,
		WebsocketConn: ws,
		Path:   path,
		Method: method,
		Payload: payload,
	}
}

func newStreamContext(conn net.Conn, path string, method string, payload []byte) *Context {
	return &Context{
		contextType: streamContext,
		Conn: conn,
		Path:   path,
		Method: method,
		Payload: payload,
	}
}

func (c *Context) Value(key interface{}) interface{} {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.Keys == nil {
		c.Keys = make(map[interface{}]interface{})
	}

	if v, ok := c.Keys[key]; ok {
		return v
	}
	return c.Context.Value(key)
}

func (c *Context) SetValue(key, val interface{}) {
	c.mu.Lock()
	defer c.mu.Unlock()

	if c.Keys == nil {
		c.Keys = make(map[interface{}]interface{})
	}
	c.Keys[key] = val
}

// DeleteKey delete the kv pair by key.
func (c *Context) DeleteKey(key interface{}) {
	c.mu.Lock()
	defer c.mu.Unlock()

	if c.Keys == nil || key == nil {
		return
	}
	delete(c.Keys, key)
}

//func (c *Context) String() string {
//	return fmt.Sprintf("%v.WithValue(%v)", c.Context, c.tags)
//}

func WithValue(parent context.Context, key, val interface{}) *Context {
	if key == nil {
		panic("nil key")
	}
	if !reflect.TypeOf(key).Comparable() {
		panic("key is not comparable")
	}

	tags := make(map[interface{}]interface{})
	tags[key] = val
	return &Context{Context: parent, Keys: tags}
}

func WithLocalValue(ctx *Context, key, val interface{}) *Context {
	if key == nil {
		panic("nil key")
	}
	if !reflect.TypeOf(key).Comparable() {
		panic("key is not comparable")
	}

	if ctx.Keys == nil {
		ctx.Keys = make(map[interface{}]interface{})
	}

	ctx.Keys[key] = val
	return ctx
}

func (c *Context) Next() {
	c.index++
	s := len(c.handlers)
	for ; c.index < s; c.index++ {
		c.handlers[c.index](c)
	}
}

func (c *Context) PostForm(key string) string {
	switch c.contextType {
	case httpContext:
		{
			return c.Req.FormValue(key)
		}
	case wsContext:
		{
			return ""
		}
	case streamContext:
		{
			return ""
		}
	default:
		return ""
	}

}

func (c *Context) Query(key string) string {

	switch c.contextType {
	case httpContext:
		{
			return c.Req.URL.Query().Get(key)
		}
	case wsContext:
		{
			return ""
		}
	case streamContext:
		{
			return ""
		}
	default:
		return ""
	}
}

func (c *Context) Status(code int) {
	c.StatusCode = code

	switch c.contextType {
	case httpContext:
		{
			c.Writer.WriteHeader(code)
		}
	case wsContext:
		{
		}
	case streamContext:
		{
		}
	default:
	}
}

func (c *Context) SetHeader(key string, value string) {
	c.Writer.Header().Set(key, value)
}

func (c *Context) String(code int, format string, values ...interface{}) {
	c.Status(code)
	switch c.contextType {
	case httpContext:
		{
			c.SetHeader("Content-Type", "text/plain")
			c.Writer.Write([]byte(fmt.Sprintf(format, values...)))
		}
	case wsContext:
		{
		}
	case streamContext:
		{
		}
	default:
	}
}

func (c *Context) JSON(code int, obj interface{}) {

	c.Status(code)

	switch c.contextType {
	case httpContext:
		{
			c.SetHeader("Content-Type", "application/json")
			encoder := json.NewEncoder(c.Writer)
			if err := encoder.Encode(obj); err != nil {
				http.Error(c.Writer, err.Error(), 500)
			}
		}
	case wsContext:
		{
		}
	case streamContext:
		{
		}
	default:
	}


}

func (c *Context) Data(code int, data []byte) {
	c.Status(code)

	switch c.contextType {
	case httpContext:
		{
			c.Writer.Write(data)
		}
	case wsContext:
		{
		}
	case streamContext:
		{
		}
	default:
	}
}

func (c *Context) HTML(code int, html string) {
	c.Status(code)

	switch c.contextType {
	case httpContext:
		{
			c.SetHeader("Content-Type", "text/html")
			c.Writer.Write([]byte(html))
		}
	case wsContext:
		{
		}
	case streamContext:
		{
		}
	default:
	}
}

func (c *Context) Fail(code int, html string) {
	c.Status(code)

	switch c.contextType {
	case httpContext:
		{
			c.SetHeader("Content-Type", "text/html")
			c.Writer.Write([]byte(html))
		}
	case wsContext:
		{
		}
	case streamContext:
		{
		}
	default:
	}
}