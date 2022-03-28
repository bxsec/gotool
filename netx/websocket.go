package netx

import (
	"github.com/gofiber/websocket/v2"
)

func (s *Server) ServeWS(conn *websocket.Conn) {
	s.mu.Lock()
	s.activeConn[conn] = struct{}{}
	s.mu.Unlock()

	conn.PayloadType = websocket.BinaryFrame
	s.serveConn(conn)
}
