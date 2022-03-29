package netx

import (
	"github.com/bxsec/gotool/netx/connect"
)

type Action int

const (
	// None indicates that no action should occur following an event.
	None Action = iota

	// Close closes the connection.
	Close

	// Shutdown shutdowns the server.
	Shutdown
)

type IServer interface {
	OnAccess(conn connect.IConnect)

	React(conn connect.IConnect, frame []byte) (out []byte, action Action)

	OnDisconnect(conn connect.IConnect)
}
