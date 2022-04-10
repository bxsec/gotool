package netx

import (
	"github.com/bxsec/gotool/protocol"
	"net"
)

type INetTransport interface {
	SetServer(server IServer)
	SetMessageAdapter(msgAdapter protocol.IRpcMessage)
	Serve(network,address string) error
	Shutdown() error
	// Close immediately closes all active net.Listeners.
	Address() net.Addr
}
