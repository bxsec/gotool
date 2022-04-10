package test

import (
	"fmt"
	"github.com/DarthPestilane/easytcp"
	"github.com/DarthPestilane/easytcp/message"
)

func NewTest() {
	s := easytcp.NewServer(&easytcp.ServerOption{
		SocketReadBufferSize:  0,
		SocketWriteBufferSize: 0,
		SocketSendDelay:       false,
		ReadTimeout:           0,
		WriteTimeout:          0,
		Packer:                easytcp.NewDefaultPacker(),
		Codec:                 nil,
		RespQueueSize:         0,
		DoNotPrintRoutes:      false,
		WriteAttemptTimes:     0,
		AsyncRouter:           false,
	})

	s.AddRoute(1001，func(c easytcp.Context) {
		req := c.Request()

		fmt.Printf("[server] request received | id: %d; size: %d; data: %s\n", req.ID, len(req.Data), req.Data)
		c.SetResponseMessage(&message.Entry{
			ID:   nil,
			Data: nil,
		})
	}
	s.Use()
}
