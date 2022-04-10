package codec

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/apache/thrift/lib/go/thrift"
	"github.com/golang/protobuf/proto"
	"github.com/tinylib/msgp/msgp"
	"github.com/vmihailenco/msgpack/v5"
	pb "google.golang.org/protobuf/proto"
	"reflect"
)


// CodecType defines serialization type of payload.
type CodecType byte

const (
	// CodecNone uses raw []byte and don't serialize/deserialize
	CodecNone CodecType = iota
	// JSON for payload.
	JSON
	// ProtoBuffer for payload.
	ProtoBuffer
	// MsgPack for payload
	MsgPack
	// Thrift
	// Thrift for payload
	Thrift
)

//go:generate mockgen -destination internal/mock/codec_mock.go -package mock . Codec

// Codec is a generic codec for encoding and decoding data.
type Codec interface {
	// Encode encodes data into []byte.
	// Returns error when error occurred.
	Encode(v interface{}) ([]byte, error)

	// Decode decodes data into v.
	// Returns error when error occurred.
	Decode(data []byte, v interface{}) error
}

// ByteCodec uses raw slice pf bytes and don't encode/decode.
type ByteCodec struct{}

// Encode returns raw slice of bytes.
func (c *ByteCodec) Encode(i interface{}) ([]byte, error) {
	if data, ok := i.([]byte); ok {
		return data, nil
	}
	if data, ok := i.(*[]byte); ok {
		return *data, nil
	}

	return nil, fmt.Errorf("%T is not a []byte", i)
}

// Decode returns raw slice of bytes.
func (c *ByteCodec) Decode(data []byte, i interface{}) error {
	reflect.Indirect(reflect.ValueOf(i)).SetBytes(data)
	return nil
}

// JSONCodec uses json marshaler and unmarshaler.
type JsonCodec struct{}

// Encode encodes an object into slice of bytes.
func (c *JsonCodec) Encode(i interface{}) ([]byte, error) {
	return json.Marshal(i)
}

// Decode decodes an object from slice of bytes.
func (c *JsonCodec) Decode(data []byte, i interface{}) error {
	d := json.NewDecoder(bytes.NewBuffer(data))
	d.UseNumber()
	return d.Decode(i)
}

// PBCodec uses protobuf marshaler and unmarshaler.
type PBCodec struct{}

// Encode encodes an object into slice of bytes.
func (c *PBCodec) Encode(i interface{}) ([]byte, error) {
	if m, ok := i.(proto.Marshaler); ok {
		return m.Marshal()
	}

	if m, ok := i.(pb.Message); ok {
		return pb.Marshal(m)
	}

	return nil, fmt.Errorf("%T is not a proto.Marshaler or pb.Message", i)
}

// Decode decodes an object from slice of bytes.
func (c *PBCodec) Decode(data []byte, i interface{}) error {
	if m, ok := i.(proto.Unmarshaler); ok {
		return m.Unmarshal(data)
	}

	if m, ok := i.(pb.Message); ok {
		return pb.Unmarshal(data, m)
	}

	return fmt.Errorf("%T is not a proto.Unmarshaler  or pb.Message", i)
}

// MsgpackCodec uses messagepack marshaler and unmarshaler.
type MsgpackCodec struct{}

// Encode encodes an object into slice of bytes.
func (c *MsgpackCodec) Encode(i interface{}) ([]byte, error) {
	if m, ok := i.(msgp.Marshaler); ok {
		return m.MarshalMsg(nil)
	}
	var buf bytes.Buffer
	enc := msgpack.NewEncoder(&buf)
	// enc.UseJSONTag(true)
	err := enc.Encode(i)
	return buf.Bytes(), err
}

// Decode decodes an object from slice of bytes.
func (c *MsgpackCodec) Decode(data []byte, i interface{}) error {
	if m, ok := i.(msgp.Unmarshaler); ok {
		_, err := m.UnmarshalMsg(data)
		return err
	}
	dec := msgpack.NewDecoder(bytes.NewReader(data))
	// dec.UseJSONTag(true)
	err := dec.Decode(i)
	return err
}

type ThriftCodec struct{}

func (c *ThriftCodec) Encode(i interface{}) ([]byte, error) {
	b := thrift.NewTMemoryBufferLen(1024)
	p := thrift.NewTBinaryProtocolFactoryDefault().GetProtocol(b)
	t := &thrift.TSerializer{
		Transport: b,
		Protocol:  p,
	}
	t.Transport.Close()
	if msg, ok := i.(thrift.TStruct); ok {
		return t.Write(context.Background(), msg)
	}
	return nil, errors.New("type assertion failed")
}

func (c *ThriftCodec) Decode(data []byte, i interface{}) error {
	t := thrift.NewTMemoryBufferLen(1024)
	p := thrift.NewTBinaryProtocolFactoryDefault().GetProtocol(t)
	d := &thrift.TDeserializer{
		Transport: t,
		Protocol:  p,
	}
	d.Transport.Close()
	return d.Read(context.Background(), i.(thrift.TStruct), data)
}
