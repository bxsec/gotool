package protocol

import (
	"encoding/binary"
	"github.com/bxsec/gotool/codec"
)

// Header is the first part of Message and has fixed size.
// Format:
//
type Header [12]byte

// CheckMagicNumber checks whether header starts rpcx magic number.
func (h Header) CheckMagicNumber() bool {
	return h[0] == magicNumber
}

// Version returns version of rpcx protocol.
func (h Header) Version() byte {
	return h[1]
}

// SetVersion sets version for this header.
func (h *Header) SetVersion(v byte) {
	h[1] = v
}

// RpcMessageType returns the Message type.
func (h Header) MessageType() RpcMessageType {
	return RpcMessageType(h[2]&0x80) >> 7
}

// SetRpcMessageType sets Message type.
func (h *Header) SetMessageType(mt RpcMessageType) {
	h[2] = h[2] | (byte(mt) << 7)
}

// IsHeartbeat returns whether the Message is heartbeat Message.
func (h Header) IsHeartbeat() bool {
	return h[2]&0x40 == 0x40
}

// SetHeartbeat sets the heartbeat flag.
func (h *Header) SetHeartbeat(hb bool) {
	if hb {
		h[2] = h[2] | 0x40
	} else {
		h[2] = h[2] &^ 0x40
	}
}

// IsOneway returns whether the Message is one-way Message.
// If true, server won't send responses.
func (h Header) IsOneway() bool {
	return h[2]&0x20 == 0x20
}

// SetOneway sets the oneway flag.
func (h *Header) SetOneway(oneway bool) {
	if oneway {
		h[2] = h[2] | 0x20
	} else {
		h[2] = h[2] &^ 0x20
	}
}

// CompressType returns compression type of RpcMessages.
func (h Header) CompressType() CompressType {
	return CompressType((h[2] & 0x1C) >> 2)
}

// SetCompressType sets the compression type.
func (h *Header) SetCompressType(ct CompressType) {
	h[2] = (h[2] &^ 0x1C) | ((byte(ct) << 2) & 0x1C)
}

// RpcMessageStatusType returns the Message status type.
func (h Header) MessageStatusType() RpcMessageStatusType {
	return RpcMessageStatusType(h[2] & 0x03)
}

// SetRpcMessageStatusType sets Message status type.
func (h *Header) SetMessageStatusType(mt RpcMessageStatusType) {
	h[2] = (h[2] &^ 0x03) | (byte(mt) & 0x03)
}

// SerializeType returns serialization type of payload.
func (h Header) SerializeType() codec.CodecType {
	return codec.CodecType((h[3] & 0xF0) >> 4)
}

// SetSerializeType sets the serialization type.
func (h *Header) SetSerializeType(st codec.CodecType) {
	h[3] = (h[3] &^ 0xF0) | (byte(st) << 4)
}

// Seq returns sequence number of RpcMessages.
func (h Header) Seq() uint64 {
	return binary.BigEndian.Uint64(h[4:])
}

// SetSeq sets  sequence number.
func (h *Header) SetSeq(seq uint64) {
	binary.BigEndian.PutUint64(h[4:], seq)
}

type MessageHeader struct {
	*Header
}

func (mh *MessageHeader) GetHeaderLen() int {
	return 16
}

var (
	zeroHeaderArray Header
	zeroHeader      = zeroHeaderArray[1:]
)

func resetHeader(h *Header) {
	copy(h[1:], zeroHeader)
}
