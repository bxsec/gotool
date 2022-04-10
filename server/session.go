package server

import (
	"bytes"
	"fmt"
	"github.com/bxsec/gotool/codec"
	"github.com/bxsec/gotool/log"
	"github.com/bxsec/gotool/protocol"
	"github.com/fasthttp/websocket"
	"github.com/google/uuid"
	"net"
	"sync"
	"time"
)

// Session represents a TCP session.
type Session interface {
	// ID returns current session's id.
	ID() interface{}

	// SetID sets current session's id.
	SetID(id interface{})

	// Send sends the ctx to the respQueue.
	Send(ctx Context) bool

	// Codec returns the codec, can be nil.
	Codec() codec.Codec

	// Close closes current session.
	Close()

	// AllocateContext gets a *share.Context ships with current session.
	AllocateContext() Context
}




type baseSession struct {
	id          interface{}   // session's ID.
	closed      chan struct{} // to close()
	closeOnce   sync.Once     // ensure one session only close once
	respQueue   chan Context // response queue channel, pushed in Send() and popped in writeOutbound()
	packer      Packer        // to pack and unpack message
	codec       codec.Codec   // encode/decode message data
	ctxPool     sync.Pool     // router *share.Context pool
	asyncRouter bool          // calls router HandlerFunc in a goroutine if false
}

// sessionOption is the extra options for session.
type sessionOption struct {
	Packer        Packer
	Codec         codec.Codec
	respQueueSize int
	asyncRouter   bool
}

// newSession creates a new session.
// Parameter conn is the TCP connection,
// opt includes packer, codec, and channel size.
// Returns a session pointer.
func newStreamSession(conn net.Conn, opt *sessionOption) *streamSession {
	return &streamSession{
		baseSession: baseSession{
			id:          uuid.NewString(), // use uuid as default
			closed:      make(chan struct{}),
			respQueue:   make(chan Context, opt.respQueueSize),
			packer:      opt.Packer,
			codec:       opt.Codec,
			ctxPool:     sync.Pool{New: func() interface{} { return NewContext() }},
			asyncRouter: opt.asyncRouter,
		},
		conn:        conn,
	}
}

func newWsSession(conn *websocket.Conn,  opt *sessionOption) *wsSession {
	return &wsSession{
		baseSession: baseSession{
			id:          uuid.NewString(), // use uuid as default
			closed:      make(chan struct{}),
			respQueue:   make(chan Context, opt.respQueueSize),
			packer:      opt.Packer,
			codec:       opt.Codec,
			ctxPool:     sync.Pool{New: func() interface{} { return NewContext() }},
			asyncRouter: opt.asyncRouter,
		},
		conn:        conn,
	}
}


// ID returns the session's id.
func (s *baseSession) ID() interface{} {
	return s.id
}

// SetID sets session id.
// Can be called in server.OnSessionCreate() callback.
func (s *baseSession) SetID(id interface{}) {
	s.id = id
}

// Send pushes response message entry to respQueue.
// Returns false if session is closed or ctx is done.
func (s *baseSession) Send(ctx Context) (ok bool) {
	select {
	case <-ctx.Done():
		return false
	case <-s.closed:
		return false
	case s.respQueue <- ctx:
		return true
	}
}

// Codec implements Session Codec.
func (s *baseSession) Codec() codec.Codec {
	return s.codec
}

// Close closes the session, but doesn't close the connection.
// The connection will be closed in the server once the session's closed.
func (s *baseSession) Close() {
	s.closeOnce.Do(func() { close(s.closed) })
}

// AllocateContext gets a *share.Context from pool and reset all but session.
func (s *baseSession) AllocateContext() Context {
	c := s.ctxPool.Get().(*routeContext)
	c.reset()
	c.SetSession(s)
	return c
}


func (s *baseSession) handleReq(router *router, reqEntry *protocol.Message) {
	ctx := s.AllocateContext().SetRequestMessage(reqEntry)
	router.handle(ctx)
	s.Send(ctx)
}

func (s *baseSession) packResponse(ctx Context) ([]byte, error) {
	defer s.ctxPool.Put(ctx)
	if ctx.Response() == nil {
		return nil, nil
	}
	return s.packer.Pack(ctx.Response())
}

type streamSession struct {
	baseSession
	conn        net.Conn      // tcp connection
}


// readInbound reads message packet from connection in a loop.
// And send unpacked message to reqQueue, which will be consumed in router.
// The loop breaks if errors occurred or the session is closed.
func (s *streamSession) readInbound(router *router, timeout time.Duration) {
	for {
		select {
		case <-s.closed:
			return
		default:
		}
		if timeout > 0 {
			if err := s.conn.SetReadDeadline(time.Now().Add(timeout)); err != nil {
				log.Errorf("session %s set read deadline err: %s", s.id, err)
				break
			}
		}
		reqEntry, err := s.packer.Unpack(s.conn)
		if err != nil {
			log.Errorf("session %s unpack inbound packet err: %s", s.id, err)
			break
		}
		if reqEntry == nil {
			continue
		}

		if s.asyncRouter {
			go s.handleReq(router, reqEntry)
		} else {
			s.handleReq(router, reqEntry)
		}
	}
	log.Errorf("session %s readInbound exit because of error", s.id)
	s.Close()
}


// writeOutbound fetches message from respQueue channel and writes to TCP connection in a loop.
// Parameter writeTimeout specified the connection writing timeout.
// The loop breaks if errors occurred, or the session is closed.
func (s *streamSession) writeOutbound(writeTimeout time.Duration, attemptTimes int) {
	for {
		var ctx Context
		select {
		case <-s.closed:
			return
		case ctx = <-s.respQueue:
		}

		outboundMsg, err := s.packResponse(ctx)
		if err != nil {
			log.Errorf("session %s pack outbound message err: %s", s.id, err)
			continue
		}
		if outboundMsg == nil {
			continue
		}

		if writeTimeout > 0 {
			if err := s.conn.SetWriteDeadline(time.Now().Add(writeTimeout)); err != nil {
				log.Errorf("session %s set write deadline err: %s", s.id, err)
				break
			}
		}

		if err := s.attemptConnWrite(outboundMsg, attemptTimes); err != nil {
			log.Errorf("session %s conn write err: %s", s.id, err)
			break
		}
	}
	s.Close()
	recovertyTrace(fmt.Sprintf("session %s writeOutbound exit because of error", s.id))
}

func (s *streamSession) attemptConnWrite(outboundMsg []byte, attemptTimes int) (err error) {
	for i := 0; i < attemptTimes; i++ {
		time.Sleep(tempErrDelay * time.Duration(i))
		_, err = s.conn.Write(outboundMsg)

		// breaks if err is not nil, or it's the last attempt.
		if err == nil || i == attemptTimes-1 {
			break
		}

		// check if err is `net.Error`
		ne, ok := err.(net.Error)
		if !ok {
			break
		}
		if ne.Timeout() {
			break
		}
		if ne.Temporary() {
			log.Errorf("session %s conn write err: %s; retrying in %s", s.id, err, tempErrDelay*time.Duration(i+1))
			continue
		}
		break // if err is not temporary, break the loop.
	}
	return
}




type wsSession struct {
	baseSession
	conn        *websocket.Conn      // tcp connection
}

// readInbound reads message packet from connection in a loop.
// And send unpacked message to reqQueue, which will be consumed in router.
// The loop breaks if errors occurred or the session is closed.
func (s *wsSession) readInbound(router *router, timeout time.Duration) {
	for {
		select {
		case <-s.closed:
			return
		default:
		}
		if timeout > 0 {
			if err := s.conn.SetReadDeadline(time.Now().Add(timeout)); err != nil {
				log.Errorf("session %s set read deadline err: %s", s.id, err)
				break
			}
		}
		_, recvData, err := s.conn.ReadMessage()
		if err != nil {
			continue
		}
		if len(recvData) <= 0 {
			continue
		}


		reader := bytes.NewReader(recvData)

		reqEntry, err := s.packer.Unpack(reader)
		if err != nil {
			log.Errorf("session %s unpack inbound packet err: %s", s.id, err)
			break
		}
		if reqEntry == nil {
			continue
		}

		if s.asyncRouter {
			go s.handleReq(router, reqEntry)
		} else {
			s.handleReq(router, reqEntry)
		}
	}
	log.Errorf("session %s readInbound exit because of error", s.id)
	s.Close()
}


// writeOutbound fetches message from respQueue channel and writes to TCP connection in a loop.
// Parameter writeTimeout specified the connection writing timeout.
// The loop breaks if errors occurred, or the session is closed.
func (s *wsSession) writeOutbound(writeTimeout time.Duration, attemptTimes int) {
	for {
		var ctx Context
		select {
		case <-s.closed:
			return
		case ctx = <-s.respQueue:
		}

		outboundMsg, err := s.packResponse(ctx)
		if err != nil {
			log.Errorf("session %s pack outbound message err: %s", s.id, err)
			continue
		}
		if outboundMsg == nil {
			continue
		}

		if writeTimeout > 0 {
			if err := s.conn.SetWriteDeadline(time.Now().Add(writeTimeout)); err != nil {
				log.Errorf("session %s set write deadline err: %s", s.id, err)
				break
			}
		}

		if err := s.attemptConnWrite(outboundMsg, attemptTimes); err != nil {
			log.Errorf("session %s conn write err: %s", s.id, err)
			break
		}
	}
	s.Close()
	recovertyTrace(fmt.Sprintf("session %s writeOutbound exit because of error", s.id))
}

func (s *wsSession) attemptConnWrite(outboundMsg []byte, attemptTimes int) (err error) {
	for i := 0; i < attemptTimes; i++ {
		time.Sleep(tempErrDelay * time.Duration(i))
		err = s.conn.WriteMessage(websocket.TextMessage, outboundMsg)

		// breaks if err is not nil, or it's the last attempt.
		if err == nil || i == attemptTimes-1 {
			break
		}

		// check if err is `net.Error`
		ne, ok := err.(net.Error)
		if !ok {
			break
		}
		if ne.Timeout() {
			break
		}
		if ne.Temporary() {
			log.Errorf("session %s conn write err: %s; retrying in %s", s.id, err, tempErrDelay*time.Duration(i+1))
			continue
		}
		break // if err is not temporary, break the loop.
	}
	return
}
