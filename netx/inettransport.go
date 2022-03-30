package netx

import (
	"github.com/bxsec/gotool/protocol"
	"net"
)

type INetTransport interface {
	Initialize(server IServer)
	SetMessageAdapter(msgAdapter protocol.IMessage)
	Serve(network,address string) error
	Shutdown() error
	Address() net.Addr
}
