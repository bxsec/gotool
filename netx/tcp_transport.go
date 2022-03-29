package netx

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"github.com/panjf2000/gnet"
	"github.com/panjf2000/gnet/pkg/pool/goroutine"
	"time"

	"github.com/bxsec/gotool/log"
)

type TcpTransport struct {
	*gnet.EventServer
	addr       string
	multicore  bool
	async      bool
	codec      gnet.ICodec
	workerPool *goroutine.Pool

	doneChan chan struct{}

	server IServer
}

func (s *TcpTransport) getDoneChan() <-chan struct{} {
	return s.doneChan
}


func (s *TcpTransport) OnInitComplete(server gnet.Server) (action Action) {
	log.Infof("Test codec server is listening on %s (multi-cores: %t, loops: %d)\n",
		server.Addr.String(), server.Multicore, server.NumEventLoop)
	return
}

func (s *TcpTransport) OnShutdown(server gnet.Server) {
	return
}

func (s *TcpTransport) Tick() (delay time.Duration, action Action) {
	return
}



func (cs *TcpTransport) React(frame []byte, c gnet.Conn) (out []byte, action gnet.Action) {
	fmt.Println("frame:", string(frame))

	// store customize protocol header param using `c.SetContext()`
	//item := protocol.CustomLengthFieldProtocol{Version: protocol.DefaultProtocolVersion, ActionType: protocol.ActionData}
	//c.SetContext(item)

	if cs.async {
		data := append([]byte{}, frame...)
		_ = cs.workerPool.Submit(func() {
			c.AsyncWrite(data)
		})
		return
	}



	out = frame
	return
}




// ITcpTransport Method

func (s *TcpTransport) Initialize(server IServer) {
	s.server = server
}

// serveListener accepts incoming connections on the Listener ln,
// creating a new service goroutine for each.
// The service goroutines read requests and then call services to reply to them.
func (s *TcpTransport) Serve(network, address string) (err error) {
	s.multicore = true
	s.addr = fmt.Sprintf("%s://%s", network,address)
	s.async = true
	s.workerPool = goroutine.Default()


	return gnet.Serve(s, s.addr, gnet.WithMulticore(s.multicore),
		gnet.WithTCPKeepAlive(time.Minute*5), gnet.WithCodec(codec))
}


/ CustomLengthFieldProtocol : custom protocol
// custom protocol header contains Version, ActionType and DataLength fields
// its payload is Data field
type CustomLengthFieldProtocol struct {
	Version    uint16
	ActionType uint16
	DataLength uint32
	Data       []byte
}

// Encode ... 将Conn中的Context和buf编码成将要传输的格式
func (cc *CustomLengthFieldProtocol) Encode(c gnet.Conn, buf []byte) ([]byte, error) {
	result := make([]byte, 0)
	buffer := bytes.NewBuffer(result)

	// take out the param
	item := c.Context().(CustomLengthFieldProtocol)

	if err := binary.Write(buffer, binary.BigEndian, item.Version); err != nil {
		s := fmt.Sprintf("Pack version error , %v", err)
		return nil, errors.New(s)
	}

	if err := binary.Write(buffer, binary.BigEndian, item.ActionType); err != nil {
		s := fmt.Sprintf("Pack type error , %v", err)
		return nil, errors.New(s)
	}
	dataLen := uint32(len(buf))
	if err := binary.Write(buffer, binary.BigEndian, dataLen); err != nil {
		s := fmt.Sprintf("Pack datalength error , %v", err)
		return nil, errors.New(s)
	}
	if dataLen > 0 {
		if err := binary.Write(buffer, binary.BigEndian, buf); err != nil {
			s := fmt.Sprintf("Pack data error , %v", err)
			return nil, errors.New(s)
		}
	}

	return buffer.Bytes(), nil
}

// Decode ... 将网络中的数据解密成
func (cc *CustomLengthFieldProtocol) Decode(c gnet.Conn) ([]byte, error) {
	// parse header
	headerLen := DefaultHeadLength // uint16+uint16+uint32
	if size, header := c.ReadN(headerLen); size == headerLen {
		byteBuffer := bytes.NewBuffer(header)
		var pbVersion, actionType uint16
		var dataLength uint32
		_ = binary.Read(byteBuffer, binary.BigEndian, &pbVersion)
		_ = binary.Read(byteBuffer, binary.BigEndian, &actionType)
		_ = binary.Read(byteBuffer, binary.BigEndian, &dataLength)
		// to check the protocol version and actionType,
		// reset buffer if the version or actionType is not correct

		if dataLength < 1 {
			return nil, nil
		}

		if pbVersion != DefaultProtocolVersion || !isCorrectAction(actionType) {
			fmt.Println("reset")
			c.ResetBuffer()
			log.Println("not normal protocol:", pbVersion, DefaultProtocolVersion, actionType, dataLength)
			return nil, errors.New("not normal protocol")
		}
		// parse payload
		dataLen := int(dataLength) // max int32 can contain 210MB payload
		protocolLen := headerLen + dataLen
		if dataSize, data := c.ReadN(protocolLen); dataSize == protocolLen {
			c.ShiftN(protocolLen)
			// log.Println("parse success:", data, dataSize)

			// return the payload of the data
			return data[headerLen:], nil
		}
		// log.Println("not enough payload data:", dataLen, protocolLen, dataSize)
		return nil, gerrors.ErrIncompletePacket

	}
	// log.Println("not enough header data:", size)
	return nil, gerrors.ErrIncompletePacket
}

// default custom protocol const
const (
	DefaultHeadLength = 8

	DefaultProtocolVersion = 0x8001 // test protocol version

	ActionPing = 0x0001 // ping
	ActionPong = 0x0002 // pong
	ActionData = 0x00F0 // business
)

func isCorrectAction(actionType uint16) bool {
	switch actionType {
	case ActionPing, ActionPong, ActionData:
		return true
	default:
		return false
	}
}