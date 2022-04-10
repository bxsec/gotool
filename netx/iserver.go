package netx

import (
	"github.com/bxsec/gotool/netx/connect"
	"github.com/bxsec/gotool/protocol"
	"github.com/bxsec/gotool/service"
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
	OnDisconnect(conn connect.IConnect)
	OnShutdown(netTransport INetTransport)

	React(conn connect.IConnect, frame []byte) (out []byte, action Action)

	SetMessageAdapter(msgAdapter protocol.IRpcMessage)
	SetServiceManager(sm *service.ServiceManager)
}
