package server

import (
	"crypto/tls"
	"fmt"
	"github.com/bxsec/gotool/codec"
	"github.com/bxsec/gotool/log"
	"github.com/fasthttp/websocket"
	"net"
	"net/http"
	"strings"
	"time"
)

const (
	DefaultRespQueueSize     = 1024
	DefaultWriteAttemptTimes = 1
	tempErrDelay             = time.Millisecond * 5
)

var ErrServerStopped = fmt.Errorf("server stopped")

// ServerOption is the option for Server.
type ServerOption struct {
	SocketReadBufferSize  int           // sets the socket read buffer size.
	SocketWriteBufferSize int           // sets the socket write buffer size.
	SocketSendDelay       bool          // sets the socket delay or not.
	ReadTimeout           time.Duration // sets the timeout for connection read.
	WriteTimeout          time.Duration // sets the timeout for connection write.
	Packer                Packer        // packs and unpacks packet payload, default packer is the DefaultPacker.
	Codec                 codec.Codec         // encodes and decodes the message data, can be nil.
	RespQueueSize         int           // sets the response channel size of session, DefaultRespQueueSize will be used if < 0.
	DoNotPrintRoutes      bool          // whether to print registered route handlers to the console.

	// WriteAttemptTimes sets the max attempt times for packet writing in each session.
	// The DefaultWriteAttemptTimes will be used if <= 0.
	WriteAttemptTimes int

	// AsyncRouter represents whether to execute a route HandlerFunc of each session in a goroutine.
	// true means execute in a goroutine.
	AsyncRouter bool
}

type Engine struct {
	*RouterGroup
	router *router
	groups []*RouterGroup // store all groups


	//stream
	Listener net.Listener

	// Packer is the message packer, will be passed to session.
	Packer Packer

	// Codec is the message codec, will be passed to session.
	Codec codec.Codec

	// OnSessionCreate is an event hook, will be invoked when session's created.
	OnSessionCreate func(sess Session)

	// OnSessionClose is an event hook, will be invoked when session's closed.
	OnSessionClose func(sess Session)

	socketReadBufferSize  int
	socketWriteBufferSize int
	socketSendDelay       bool
	readTimeout           time.Duration
	writeTimeout          time.Duration
	respQueueSize         int
	printRoutes           bool
	accepting             chan struct{}
	stopped               chan struct{}
	writeAttemptTimes     int
	asyncRouter           bool

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
// Serve starts to listen TCP and keeps accepting TCP connection in a loop.
// The loop breaks when error occurred, and the error will be returned.
func (s *Engine) RunStream(addr string) error {
	lis, err := s.listen(addr)
	if err != nil {
		return err
	}
	s.Listener = lis
	return s.acceptLoop()
}

// ServeTLS starts serve TCP with TLS.
func (s *Engine) ServeTLS(addr string, config *tls.Config) error {
	lis, err := s.listen(addr)
	if err != nil {
		return err
	}
	s.Listener = tls.NewListener(lis, config)

	return s.acceptLoop()
}

func (engine *Engine) ServeHTTP(w http.ResponseWriter, req *http.Request) {
	var middlewares []HandlerFunc
	for _, group := range engine.groups {
		if strings.HasPrefix(req.URL.Path, group.prefix) {
			middlewares = append(middlewares, group.middlewares...)
		}
	}


	c := NewContext()
	c.handlers = middlewares
	c.engine = engine
	engine.router.handle(c)
}

func (engine *Engine) ServeWS(ws *websocket.Conn) {
	defer ws.Close() // nolint

	sess := newWsSession(ws, &sessionOption{
		Packer:        engine.Packer,
		Codec:         engine.Codec,
		respQueueSize: engine.respQueueSize,
		asyncRouter:   engine.asyncRouter,
	})
	if engine.OnSessionCreate != nil {
		go engine.OnSessionCreate(sess)
	}

	go sess.readInbound(engine.router, engine.readTimeout)               // start reading message packet from connection.
	go sess.writeOutbound(engine.writeTimeout, engine.writeAttemptTimes) // start writing message packet to connection.

	select {
	case <-sess.closed: // wait for session finished.
	case <-engine.stopped: // or the server is stopped.
	}

	if engine.OnSessionClose != nil {
		go engine.OnSessionClose(sess)
	}
}

// acceptLoop accepts TCP connections in a loop, and handle connections in goroutines.
// Returns error when error occurred.
func (s *Engine) acceptLoop() error {
	close(s.accepting)
	for {
		if s.isStopped() {
			log.Errorf("server accept loop stopped")
			return ErrServerStopped
		}

		conn, err := s.Listener.Accept()
		if err != nil {
			if s.isStopped() {
				log.Error("server accept loop stopped")
				return ErrServerStopped
			}
			if ne, ok := err.(net.Error); ok && ne.Temporary() {
				log.Errorf("accept err: %s; retrying in %s", err, tempErrDelay)
				time.Sleep(tempErrDelay)
				continue
			}
			return fmt.Errorf("accept err: %s", err)
		}
		if s.socketReadBufferSize > 0 {
			if c, ok := conn.(*net.TCPConn); ok {
				if err := c.SetReadBuffer(s.socketReadBufferSize); err != nil {
					return fmt.Errorf("conn set read buffer err: %s", err)
				}
			}
		}
		if s.socketWriteBufferSize > 0 {
			if c, ok := conn.(*net.TCPConn); ok {
				if err := c.SetWriteBuffer(s.socketWriteBufferSize); err != nil {
					return fmt.Errorf("conn set write buffer err: %s", err)
				}
			}
		}
		if s.socketSendDelay {
			if c, ok := conn.(*net.TCPConn); ok {
				if err := c.SetNoDelay(false); err != nil {
					return fmt.Errorf("conn set no delay err: %s", err)
				}
			}
		}
		go s.handleConn(conn)
	}
}

func (s *Engine) listen(addr string) (net.Listener, error) {
	address, err := net.ResolveTCPAddr("tcp", addr)
	if err != nil {
		return nil, err
	}
	lis, err := net.ListenTCP("tcp", address)
	if err != nil {
		return nil, err
	}
	return lis, nil
}



func (engine *Engine) handleConn(conn net.Conn) {
	defer conn.Close() // nolint

	sess := newStreamSession(conn, &sessionOption{
		Packer:        engine.Packer,
		Codec:         engine.Codec,
		respQueueSize: engine.respQueueSize,
		asyncRouter:   engine.asyncRouter,
	})
	if engine.OnSessionCreate != nil {
		go engine.OnSessionCreate(sess)
	}

	go sess.readInbound(engine.router, engine.readTimeout)               // start reading message packet from connection.
	go sess.writeOutbound(engine.writeTimeout, engine.writeAttemptTimes) // start writing message packet to connection.

	select {
	case <-sess.closed: // wait for session finished.
	case <-engine.stopped: // or the server is stopped.
	}

	if engine.OnSessionClose != nil {
		go engine.OnSessionClose(sess)
	}
}

// NotFoundHandler sets the not-found handler for router.
func (engine *Engine) NotFoundHandler(handler HandlerFunc) {
	//engine.router.setNotFoundHandler(handler)
}

func (engine *Engine) isStopped() bool {
	select {
	case <-engine.stopped:
		return true
	default:
		return false
	}
}