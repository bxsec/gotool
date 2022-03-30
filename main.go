package main

import (
	"github.com/bxsec/gotool/netx"
)

func main() {
	server := netx.NewServer()
	server.Serve("tcp", "localhost:8972")
}

