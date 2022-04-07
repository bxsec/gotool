package connect

import (
	"github.com/gorilla/websocket"
	"net"
	"time"
)

type WsConnect struct {
	Conn *websocket.Conn
	mt int
	ctx interface{}
}
// Context returns a user-defined context.
func (ws *WsConnect) Context() (ctx interface{}) {
	return ws.ctx
}

// SetContext sets a user-defined context.
func (ws *WsConnect) SetContext(ctx interface{}) {
	ws.ctx = ctx
}

// Read reads data from the connection.
// Read can be made to time out and return an error after a fixed
// time limit; see SetDeadline and SetReadDeadline.
func (ws *WsConnect) Read(b []byte) (n int, err error) {
	_, p, err := ws.ReadMessage()

	copy(p, b)
	return len(p), err
}


func (ws *WsConnect) ReadMessage() (messageType int, p []byte, err error) {
	messageType, p, err = ws.Conn.ReadMessage()
	ws.mt = messageType
	return
}

func (ws *WsConnect) WriteMessage(messageType int, data []byte) error {
	return ws.Conn.WriteMessage(messageType, data)
}

// Write writes data to the connection.
// Write can be made to time out and return an error after a fixed
// time limit; see SetDeadline and SetWriteDeadline.
func (ws *WsConnect) Write(b []byte) (n int, err error) {
	return len(b), ws.Conn.WriteMessage(ws.mt, b)
}

// Close closes the connection.
// Any blocked Read or Write operations will be unblocked and return errors.
func (ws *WsConnect) Close() error {
	return ws.Conn.Close()
}

// LocalAddr returns the local network address, if known.
func (ws *WsConnect) LocalAddr() net.Addr {
	return ws.Conn.LocalAddr()
}

// RemoteAddr returns the remote network address, if known.
func (ws *WsConnect) RemoteAddr() net.Addr {
	return ws.Conn.RemoteAddr()
}

// SetDeadline sets the read and write deadlines associated
// with the connection. It is equivalent to calling both
// SetReadDeadline and SetWriteDeadline.
//
// A deadline is an absolute time after which I/O operations
// fail instead of blocking. The deadline applies to all future
// and pending I/O, not just the immediately following call to
// Read or Write. After a deadline has been exceeded, the
// connection can be refreshed by setting a deadline in the future.
//
// If the deadline is exceeded a call to Read or Write or to other
// I/O methods will return an error that wraps os.ErrDeadlineExceeded.
// This can be tested using errors.Is(err, os.ErrDeadlineExceeded).
// The error's Timeout method will return true, but note that there
// are other possible errors for which the Timeout method will
// return true even if the deadline has not been exceeded.
//
// An idle timeout can be implemented by repeatedly extending
// the deadline after successful Read or Write calls.
//
// A zero value for t means I/O operations will not time out.
func (ws *WsConnect) SetDeadline(t time.Time) error {
	err := ws.Conn.SetReadDeadline(t)
	if err != nil {
		return err
	}
	return ws.Conn.SetWriteDeadline(t)
}

// SetReadDeadline sets the deadline for future Read calls
// and any currently-blocked Read call.
// A zero value for t means Read will not time out.
func (ws *WsConnect) SetReadDeadline(t time.Time) error {
	return ws.Conn.SetReadDeadline(t)
}

// SetWriteDeadline sets the deadline for future Write calls
// and any currently-blocked Write call.
// Even if write times out, it may return n > 0, indicating that
// some of the data was successfully written.
// A zero value for t means Write will not time out.
func (ws *WsConnect) SetWriteDeadline(t time.Time) error {
	return ws.Conn.SetWriteDeadline(t)
}
