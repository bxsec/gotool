package protocol

import "io"

type IMessage interface {
	GetHeaderLen() int
	ParseToBodyLen(r io.Reader) (int,error)
	Decode(r io.Reader) error
}
