package protocol

import (
	"encoding/binary"
	"errors"
	"fmt"
	"io"

	"github.com/smallnest/rpcx/util"
	"github.com/valyala/bytebufferpool"
)

var bufferPool = util.NewLimitedPool(512, 4096)

// Compressors are compressors supported by rpcx. You can add customized compressor in Compressors.
var Compressors = map[CompressType]Compressor{
	None: &RawDataCompressor{},
	Gzip: &GzipCompressor{},
}

// MaxRpcMessageLength is the max length of a Message.
// Default is 0 that means does not limit length of RpcMessages.
// It is used to validate when read RpcMessages from io.Reader.
var MaxRpcMessageLength = 0

const (
	magicNumber byte = 0x08
)

func MagicNumber() byte {
	return magicNumber
}

var (
	// ErrMetaKVMissing some keys or values are missing.
	ErrMetaKVMissing = errors.New("wrong metadata lines. some keys or values are missing")
	// ErrRpcMessageTooLong Message is too long
	ErrRpcMessageTooLong = errors.New("Message is too long")

	ErrUnsupportedCompressor = errors.New("unsupported compressor")
)

const (
	// ServiceError contains error info of service invocation
	ServiceError = "__rpcx_error__"
)

// RpcMessageType is Message type of requests and responses.
type RpcMessageType byte

const (
	// Request is Message type of request
	Request RpcMessageType = iota
	// Response is Message type of response
	Response
)

// RpcMessageStatusType is status of RpcMessages.
type RpcMessageStatusType byte

const (
	// Normal is normal requests and responses.
	Normal RpcMessageStatusType = iota
	// Error indicates some errors occur.
	Error
)

// CompressType defines decompression type.
type CompressType byte

const (
	// None does not compress.
	None CompressType = iota
	// Gzip uses gzip compression.
	Gzip
)

type IRpcMessage interface {
	GetHeaderLen() int
	ParseToBodyLen(r io.Reader) (int, error)
	Decode(r io.Reader) error
}

// Message is the generic type of Request and Response.
type Message struct {
	MessageHeader
	MessageBody
}

// NewRpcMessage creates an empty Message.
func NewRpcMessage() *Message {
	header := Header([12]byte{})
	header[0] = magicNumber

	return &Message{
		MessageHeader: MessageHeader{
			Header: &header,
		},
		MessageBody: MessageBody{},
	}
}

// Clone clones from an Message.
func (m Message) Clone() *Message {
	header := *m.Header
	c := GetPooledMsg()
	header.SetCompressType(None)

	c.Header = &header

	c.ServicePath = m.ServicePath
	c.ServiceMethod = m.ServiceMethod
	return c
}

// Encode encodes RpcMessages.
func (m Message) Encode() []byte {
	data := m.EncodeSlicePointer()
	return *data
}

// EncodeSlicePointer encodes RpcMessages as a byte slice pointer we can use pool to improve.
func (m Message) EncodeSlicePointer() *[]byte {
	bb := bytebufferpool.Get()
	encodeMetadata(m.Metadata, bb)
	meta := bb.Bytes()

	spL := len(m.ServicePath)
	smL := len(m.ServiceMethod)

	var err error
	payload := m.Payload
	if m.CompressType() != None {
		compressor := Compressors[m.CompressType()]
		if compressor == nil {
			m.SetCompressType(None)
		} else {
			payload, err = compressor.Zip(m.Payload)
			if err != nil {
				m.SetCompressType(None)
				payload = m.Payload
			}
		}
	}

	totalL := (4 + spL) + (4 + smL) + (4 + len(meta)) + (4 + len(payload))

	// header + dataLen + spLen + sp + smLen + sm + metaL + meta + payloadLen + payload
	metaStart := 12 + 4 + (4 + spL) + (4 + smL)

	payLoadStart := metaStart + (4 + len(meta))
	l := 12 + 4 + totalL

	data := bufferPool.Get(l)
	copy(*data, m.Header[:])

	// totalLen
	binary.BigEndian.PutUint32((*data)[12:16], uint32(totalL))

	binary.BigEndian.PutUint32((*data)[16:20], uint32(spL))
	copy((*data)[20:20+spL], util.StringToSliceByte(m.ServicePath))

	binary.BigEndian.PutUint32((*data)[20+spL:24+spL], uint32(smL))
	copy((*data)[24+spL:metaStart], util.StringToSliceByte(m.ServiceMethod))

	binary.BigEndian.PutUint32((*data)[metaStart:metaStart+4], uint32(len(meta)))
	copy((*data)[metaStart+4:], meta)

	bytebufferpool.Put(bb)

	binary.BigEndian.PutUint32((*data)[payLoadStart:payLoadStart+4], uint32(len(payload)))
	copy((*data)[payLoadStart+4:], payload)

	return data
}

// PutData puts the byte slice into pool.
func PutData(data *[]byte) {
	bufferPool.Put(data)
}

// WriteTo writes Message to writers.
func (m Message) WriteTo(w io.Writer) (int64, error) {
	nn, err := w.Write(m.Header[:])
	n := int64(nn)
	if err != nil {
		return n, err
	}

	bb := bytebufferpool.Get()
	encodeMetadata(m.Metadata, bb)
	meta := bb.Bytes()

	spL := len(m.ServicePath)
	smL := len(m.ServiceMethod)

	payload := m.Payload
	if m.CompressType() != None {
		compressor := Compressors[m.CompressType()]
		if compressor == nil {
			return n, ErrUnsupportedCompressor
		}
		payload, err = compressor.Zip(m.Payload)
		if err != nil {
			return n, err
		}
	}

	totalL := (4 + spL) + (4 + smL) + (4 + len(meta)) + (4 + len(payload))
	err = binary.Write(w, binary.BigEndian, uint32(totalL))
	if err != nil {
		return n, err
	}

	// write servicePath and serviceMethod
	err = binary.Write(w, binary.BigEndian, uint32(len(m.ServicePath)))
	if err != nil {
		return n, err
	}
	_, err = w.Write(util.StringToSliceByte(m.ServicePath))
	if err != nil {
		return n, err
	}
	err = binary.Write(w, binary.BigEndian, uint32(len(m.ServiceMethod)))
	if err != nil {
		return n, err
	}
	_, err = w.Write(util.StringToSliceByte(m.ServiceMethod))
	if err != nil {
		return n, err
	}

	// write meta
	err = binary.Write(w, binary.BigEndian, uint32(len(meta)))
	if err != nil {
		return n, err
	}
	_, err = w.Write(meta)
	if err != nil {
		return n, err
	}

	bytebufferpool.Put(bb)

	// write payload
	err = binary.Write(w, binary.BigEndian, uint32(len(payload)))
	if err != nil {
		return n, err
	}

	nn, err = w.Write(payload)
	return int64(nn), err
}

// 获取包头长度，指定数据长度获取包体的长度
func (m *Message) GetHeaderLen() int {
	return m.MessageHeader.GetHeaderLen()
}

func (m *Message) ParseToBodyLen(r io.Reader) (int, error) {
	// validate rest length for each step?

	// parse header
	_, err := io.ReadFull(r, m.Header[:1])
	if err != nil {
		return 0, err
	}
	if !m.Header.CheckMagicNumber() {
		return 0, fmt.Errorf("wrong magic number: %v", m.Header[0])
	}

	_, err = io.ReadFull(r, m.Header[1:])
	if err != nil {
		return 0, err
	}

	// total
	lenData := poolUint32Data.Get().(*[]byte)
	_, err = io.ReadFull(r, *lenData)
	if err != nil {
		poolUint32Data.Put(lenData)
		return 0, err
	}
	l := binary.BigEndian.Uint32(*lenData)
	poolUint32Data.Put(lenData)

	if MaxRpcMessageLength > 0 && int(l) > MaxRpcMessageLength {
		return 0, ErrRpcMessageTooLong
	}

	return int(l), nil
}

// Read reads a Message from r.
func Read(r io.Reader) (*Message, error) {
	msg := NewRpcMessage()
	err := msg.Decode(r)
	if err != nil {
		return nil, err
	}
	return msg, nil
}

// Decode decodes a Message from reader.
func (m *Message) Decode(r io.Reader) error {
	// validate rest length for each step?

	// parse header
	_, err := io.ReadFull(r, m.Header[:1])
	if err != nil {
		return err
	}
	if !m.Header.CheckMagicNumber() {
		return fmt.Errorf("wrong magic number: %v", m.Header[0])
	}

	_, err = io.ReadFull(r, m.Header[1:])
	if err != nil {
		return err
	}

	// total
	lenData := poolUint32Data.Get().(*[]byte)
	_, err = io.ReadFull(r, *lenData)
	if err != nil {
		poolUint32Data.Put(lenData)
		return err
	}
	l := binary.BigEndian.Uint32(*lenData)
	poolUint32Data.Put(lenData)

	if MaxRpcMessageLength > 0 && int(l) > MaxRpcMessageLength {
		return ErrRpcMessageTooLong
	}

	totalL := int(l)
	if cap(m.data) >= totalL { // reuse data
		m.data = m.data[:totalL]
	} else {
		m.data = make([]byte, totalL)
	}
	data := m.data
	_, err = io.ReadFull(r, data)
	if err != nil {
		return err
	}

	n := 0
	// parse servicePath
	l = binary.BigEndian.Uint32(data[n:4])
	n = n + 4
	nEnd := n + int(l)
	m.ServicePath = util.SliceByteToString(data[n:nEnd])
	n = nEnd

	// parse serviceMethod
	l = binary.BigEndian.Uint32(data[n : n+4])
	n = n + 4
	nEnd = n + int(l)
	m.ServiceMethod = util.SliceByteToString(data[n:nEnd])
	n = nEnd

	// parse meta
	l = binary.BigEndian.Uint32(data[n : n+4])
	n = n + 4
	nEnd = n + int(l)

	if l > 0 {
		m.Metadata, err = decodeMetadata(l, data[n:nEnd])
		if err != nil {
			return err
		}
	}
	n = nEnd

	// parse payload
	l = binary.BigEndian.Uint32(data[n : n+4])
	_ = l
	n = n + 4
	m.Payload = data[n:]

	if m.CompressType() != None {
		compressor := Compressors[m.CompressType()]
		if compressor == nil {
			return ErrUnsupportedCompressor
		}
		m.Payload, err = compressor.Unzip(m.Payload)
		if err != nil {
			return err
		}
	}

	return err
}

// Reset clean data of this Message but keep allocated data
func (m *Message) Reset() {
	resetHeader(m.Header)
	m.Metadata = nil
	m.Payload = []byte{}
	m.data = m.data[:0]
	m.ServicePath = ""
	m.ServiceMethod = ""
}
