package netx

import (
	connect2 "github.com/bxsec/gotool/netx/connect"
	"github.com/gofiber/websocket/v2"
)

func (s *XServer) ServeWS(conn *websocket.Conn) {
	if s.isShutdown() {
		conn.Close()
		return
	}

	connect := &connect2.WsConnect{Conn: conn}
	s.OnAccess(connect)

	for {
		mt, payload, err := conn.ReadMessage()
		if err != nil {
			break
		}

		out,_ := s.React(connect, payload)

		if out != nil {
			conn.WriteMessage(mt, out)
		}
	}

	s.OnDisconnect(connect)
}
