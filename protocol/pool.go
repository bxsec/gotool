package protocol

import "sync"

var msgPool = sync.Pool{
	New: func() interface{} {
		header := Header([12]byte{})
		header[0] = magicNumber

		return &RpcMessage{
			Header: &header,
		}
	},
}

// GetPooledMsg gets a pooled message.
func GetPooledMsg() *RpcMessage {
	return msgPool.Get().(*RpcMessage)
}

// FreeMsg puts a msg into the pool.
func FreeMsg(msg *RpcMessage) {
	if msg != nil && cap(msg.data) < 1024 {
		msg.Reset()
		msgPool.Put(msg)
	}
}

var poolUint32Data = sync.Pool{
	New: func() interface{} {
		data := make([]byte, 4)
		return &data
	},
}
