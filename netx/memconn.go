package netx

import (
	"net"

	"github.com/akutz/memconn"
)

func init() {
	makeListeners["memu"] = memconnMakeListener
}

func memconnMakeListener(s *XServer, address string) (ln net.Listener, err error) {
	return memconn.Listen("memu", address)
}
