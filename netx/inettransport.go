package netx


type INetTransport interface {
	Initialize(server IServer)
	Serve(network,address string) error
}
