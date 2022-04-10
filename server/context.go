package server

import (
	"context"
	"fmt"
	"github.com/bxsec/gotool/protocol"
	"reflect"
	"sync"
	"time"
)

// NewContext creates a routeContext pointer.
func NewContext() *routeContext {
	return &routeContext{
		rawCtx: context.Background(),
	}
}

type Context interface {
	context.Context

	SetValue(key, val interface{})
	// WithContext sets the underline context.
	// It's very useful to control the workflow when send to response channel.
	WithContext(ctx context.Context) Context

	// Session returns the current session.
	Session() Session

	// SetSession sets session.
	SetSession(sess Session) Context

	GetPath() string
	GetMethod() string


	// Request returns request message entry.
	Request() *protocol.Message

	// SetRequest encodes data with session's codec and sets request message entry.
	SetRequest(id, data interface{}) error

	// MustSetRequest encodes data with session's codec and sets request message entry.
	// panics on error.
	MustSetRequest(id, data interface{}) Context

	// SetRequestMessage sets request message entry directly.
	SetRequestMessage(entry *protocol.Message) Context

	// Bind decodes request message entry to v.
	Bind(v interface{}) error

	// Response returns the response message entry.
	Response() *protocol.Message

	// SetResponse encodes data with session's codec and sets response message entry.
	SetResponse(id, data interface{}) error

	// MustSetResponse encodes data with session's codec and sets response message entry.
	// panics on error.
	MustSetResponse(id, data interface{}) Context

	// SetResponseMessage sets response message entry directly.
	SetResponseMessage(entry *protocol.Message) Context

	// Send sends itself to current session.
	Send() bool

	// SendTo sends itself to session.
	SendTo(session Session) bool
	
	// Remove deletes the key from storage.
	Remove(key string)

	// Copy returns a copy of Context.
	Copy() Context

	Next()

	GetParams() map[string]string
	SetParams(map[string]string)

	Handlers() *[]HandlerFunc

	Json(code int, obj interface{})
	String(code int, format string, values ...interface{})
	Fail(code int, format string, values ...interface{})
}

type routeContext struct {
	rawCtx context.Context
	Params   map[string]string

	mu sync.RWMutex
	storage     map[string]interface{}

	Method string
	Path string

	// middleware
	handlers []HandlerFunc
	index    int

	reqMsg *protocol.Message
	rspMsg *protocol.Message

	session Session

	// Engine pointer
	engine *Engine
}



// Deadline implements the context.Context Deadline method.
func (c *routeContext) Deadline() (time.Time, bool) {
	return c.rawCtx.Deadline()
}

// Done implements the context.Context Done method.
func (c *routeContext) Done() <-chan struct{} {
	return c.rawCtx.Done()
}

// Err implements the context.Context Err method.
func (c *routeContext) Err() error {
	return c.rawCtx.Err()
}



// WithContext sets the underline context.
func (c *routeContext) WithContext(ctx context.Context) Context {
	c.rawCtx = ctx
	return c
}

// Session implements Context.Session method.
func (c *routeContext) Session() Session {
	return c.session
}

// SetSession sets session.
func (c *routeContext) SetSession(sess Session) Context {
	c.session = sess
	return c
}

func (c *routeContext) GetPath() string {
	return c.Path
}
func (c *routeContext) GetMethod() string {
	return c.Method
}

func (c *routeContext) GetParams() map[string]string {
	return c.Params
}

func (c *routeContext) SetParams(params map[string]string) {
	c.Params = params
}

func (c *routeContext) Handlers() *[]HandlerFunc {
	return &c.handlers
}

// Request implements Context.Request method.
func (c *routeContext) Request() *protocol.Message {
	return c.reqMsg
}

// SetRequest sets request by id and data.
func (c *routeContext) SetRequest(id, data interface{}) error {
	codec := c.session.Codec()
	if codec == nil {
		return fmt.Errorf("codec is nil")
	}
	dataRaw, err := codec.Encode(data)
	if err != nil {
		return err
	}
	c.reqMsg = &protocol.Message{
		ID:     id,
		Path:   "",
		Method: "",
		Payload:   dataRaw,
	}
	return nil
}

// MustSetRequest implements Context.MustSetRequest method.
func (c *routeContext) MustSetRequest(id, data interface{}) Context {
	if err := c.SetRequest(id, data); err != nil {
		panic(err)
	}
	return c
}

// SetRequestMessage sets request message entry.
func (c *routeContext) SetRequestMessage(message *protocol.Message) Context {
	c.reqMsg = message
	return c
}

// Bind implements Context.Bind method.
func (c *routeContext) Bind(v interface{}) error {
	if c.session.Codec() == nil {
		return fmt.Errorf("message codec is nil")
	}
	return c.session.Codec().Decode(c.reqMsg.Payload, v)
}

// Response implements Context.Response method.
func (c *routeContext) Response() *protocol.Message {
	return c.rspMsg
}

// SetResponse implements Context.SetResponse method.
func (c *routeContext) SetResponse(id, data interface{}) error {
	codec := c.session.Codec()
	if codec == nil {
		return fmt.Errorf("codec is nil")
	}
	dataRaw, err := codec.Encode(data)
	if err != nil {
		return err
	}
	c.rspMsg = &protocol.Message{
		ID:   id,
		Path: c.Path,
		Method: c.Method,
		Payload: dataRaw,
	}
	return nil
}

// MustSetResponse implements Context.MustSetResponse method.
func (c *routeContext) MustSetResponse(id, data interface{}) Context {
	if err := c.SetResponse(id, data); err != nil {
		panic(err)
	}
	return c
}

// SetResponseMessage implements Context.SetResponseMessage method.
func (c *routeContext) SetResponseMessage(msg *protocol.Message) Context {
	c.rspMsg = msg
	return c
}

// Send implements Context.Send method.
func (c *routeContext) Send() bool {
	return c.session.Send(c)
}

// SendTo implements Context.SendTo method.
func (c *routeContext) SendTo(sess Session) bool {
	return sess.Send(c)
}


// Remove implements Context.Remove method.
func (c *routeContext) Remove(key string) {
	c.mu.Lock()
	delete(c.storage, key)
	c.mu.Unlock()
}

// Copy implements Context.Copy method.
func (c *routeContext) Copy() Context {
	return &routeContext{
		rawCtx:    c.rawCtx,
		storage:   c.storage,
		session:   c.session,
		reqMsg:  c.reqMsg,
		rspMsg: c.rspMsg,
	}
}

func (c *routeContext) reset() {
	c.rawCtx = context.Background()
	c.session = nil
	c.reqMsg = nil
	c.rspMsg = nil
	c.storage = nil
}

func (c *routeContext) Value(key interface{}) interface{} {
	if keyAsString, ok := key.(string); ok {
		c.mu.Lock()
		defer c.mu.Unlock()
		if c.storage == nil {
			c.storage = make(map[string]interface{})
		}

		if v, ok := c.storage[keyAsString]; ok {
			return v
		}
		return c.rawCtx.Value(key)
	}

	return nil
}

func (c *routeContext) SetValue(key, val interface{}) {

	if keyAsString, ok := key.(string); ok {
		c.mu.Lock()
		defer c.mu.Unlock()

		if c.storage == nil {
			c.storage = make(map[string]interface{})
		}
		c.storage[keyAsString] = val
	}


}

// DeleteKey delete the kv pair by key.
func (c *routeContext) DeleteKey(key interface{}) {

	if keyAsString, ok := key.(string); ok {
		c.mu.Lock()
		defer c.mu.Unlock()

		if c.storage == nil || key == nil {
			return
		}
		delete(c.storage, keyAsString)
	}


}

func WithValue(parent context.Context, key, val interface{}) Context {

	if keyAsString, ok := key.(string); ok {
		if key == nil {
			panic("nil key")
		}
		if !reflect.TypeOf(key).Comparable() {
			panic("key is not comparable")
		}

		tags := make(map[string]interface{})
		tags[keyAsString] = val
		return &routeContext{rawCtx: parent, storage: tags}
	}
	return nil
}

//func WithLocalValue(ctx Context, key, val interface{}) Context {
//	if keyAsString, ok := key.(string); ok {
//		if key == nil {
//			panic("nil key")
//		}
//		if !reflect.TypeOf(key).Comparable() {
//			panic("key is not comparable")
//		}
//
//		if ctx.storage == nil {
//			ctx.storage = make(map[string]interface{})
//		}
//
//		ctx.storage[keyAsString] = val
//		return return &routeContext{rawCtx: ctx, storage: tags}
//	}
//	return nil
//}

func (c *routeContext) Next() {
	c.index++
	s := len(c.handlers)
	for ; c.index < s; c.index++ {
		c.handlers[c.index](c)
	}
}

func (c *routeContext) Json(code int, obj interface{}) {

}

func (c *routeContext) String(code int, format string, values ...interface{}) {

}

func (c *routeContext) Fail(code int, format string, values ...interface{}) {

}

//func (c *Context) PostForm(key string) string {
//	switch c.contextType {
//	case httpContext:
//		{
//			return c.Req.FormValue(key)
//		}
//	case wsContext:
//		{
//			return ""
//		}
//	case streamContext:
//		{
//			return ""
//		}
//	default:
//		return ""
//	}
//
//}
//
//func (c *Context) Query(key string) string {
//
//	switch c.contextType {
//	case httpContext:
//		{
//			return c.Req.URL.Query().Get(key)
//		}
//	case wsContext:
//		{
//			return ""
//		}
//	case streamContext:
//		{
//			return ""
//		}
//	default:
//		return ""
//	}
//}
//
//func (c *Context) Status(code int) {
//	c.StatusCode = code
//
//	switch c.contextType {
//	case httpContext:
//		{
//			c.Writer.WriteHeader(code)
//		}
//	case wsContext:
//		{
//		}
//	case streamContext:
//		{
//		}
//	default:
//	}
//}
//
//func (c *Context) SetHeader(key string, value string) {
//	c.Writer.Header().Set(key, value)
//}
//

//
//func (c *Context) JSON(code int, obj interface{}) {
//
//	c.Status(code)
//
//	switch c.contextType {
//	case httpContext:
//		{
//			c.SetHeader("Content-Type", "application/json")
//			encoder := json.NewEncoder(c.Writer)
//			if err := encoder.Encode(obj); err != nil {
//				http.Error(c.Writer, err.Error(), 500)
//			}
//		}
//	case wsContext:
//		{
//		}
//	case streamContext:
//		{
//		}
//	default:
//	}
//
//
//}
//
//func (c *Context) Data(code int, data []byte) {
//	c.Status(code)
//
//	switch c.contextType {
//	case httpContext:
//		{
//			c.Writer.Write(data)
//		}
//	case wsContext:
//		{
//		}
//	case streamContext:
//		{
//		}
//	default:
//	}
//}
//
//func (c *Context) HTML(code int, html string) {
//	c.Status(code)
//
//	switch c.contextType {
//	case httpContext:
//		{
//			c.SetHeader("Content-Type", "text/html")
//			c.Writer.Write([]byte(html))
//		}
//	case wsContext:
//		{
//		}
//	case streamContext:
//		{
//		}
//	default:
//	}
//}
//
//func (c *Context) Fail(code int, html string) {
//	c.Status(code)
//
//	switch c.contextType {
//	case httpContext:
//		{
//			c.SetHeader("Content-Type", "text/html")
//			c.Writer.Write([]byte(html))
//		}
//	case wsContext:
//		{
//		}
//	case streamContext:
//		{
//		}
//	default:
//	}
//}