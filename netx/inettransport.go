package netx

import "github.com/bxsec/gotool/protocol"

type INetTransport interface {
	Initialize(server IServer)
	SetMessageAdapter(msgAdapter protocol.IMessage)
	Serve(network,address string) error
}
