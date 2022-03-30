package netx

import (
	"bytes"
	"errors"
	"fmt"
	"github.com/bxsec/gotool/netx/connect"
	"github.com/bxsec/gotool/protocol"
	"github.com/panjf2000/gnet"
	errors2 "github.com/panjf2000/gnet/pkg/errors"
	"github.com/panjf2000/gnet/pkg/pool/goroutine"
	"net"
	"sync"
	"time"

	glog "github.com/bxsec/gotool/log"
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
	msgAdapter protocol.IMessage

	transportServer gnet.Server

	mu sync.RWMutex
	tcpClient map[gnet.Conn]connect.IConnect
}

func (s *TcpTransport) getDoneChan() <-chan struct{} {
	return s.doneChan
}


func (s *TcpTransport) OnInitComplete(server gnet.Server) (action Action) {
	glog.Infof("Test codec server is listening on %s (multi-cores: %t, loops: %d)\n",
		server.Addr.String(), server.Multicore, server.NumEventLoop)
	s.transportServer = server
	return
}

func (s *TcpTransport) OnShutdown(server gnet.Server) {
	return
}

// OnOpened fires when a new connection has been opened.
// The Conn c has information about the connection such as it's local and remote address.
// The parameter out is the return value which is going to be sent back to the peer.
// It is usually not recommended to send large amounts of data back to the peer in OnOpened.
//
// Note that the bytes returned by OnOpened will be sent back to the peer without being encoded.
func (s *TcpTransport) OnOpened(c gnet.Conn) (out []byte, action Action) {
	conn := &connect.TcpConnect{Conn: c}
	s.mu.Lock()
	s.tcpClient[c] = conn
	s.mu.Unlock()
	s.server.OnAccess(conn)
	return nil,None
}

// OnClosed fires when a connection has been closed.
// The parameter err is the last known connection error.
func (s *TcpTransport) OnClosed(c gnet.Conn, err error) (action Action) {
	s.mu.RLock()
	conn, e := s.tcpClient[c]
	s.mu.RUnlock()
	if e == true {
		s.server.OnDisconnect(conn)
	}

	s.mu.Lock()
	delete(s.tcpClient, c)
	s.mu.Unlock()
	return None
}

func (s *TcpTransport) Tick() (delay time.Duration, action Action) {
	return
}



func (cs *TcpTransport) React(frame []byte, c gnet.Conn) (out []byte, action gnet.Action) {
	fmt.Println("frame:", string(frame))

	// store customize protocol header param using `c.SetContext()`
	//item := protocol.CustomLengthFieldProtocol{Version: protocol.DefaultProtocolVersion, ActionType: protocol.ActionData}
	//c.SetContext(item)
	conn := &connect.TcpConnect{Conn: c}
	if cs.async {
		data := append([]byte{}, frame...)
		_ = cs.workerPool.Submit(func() {
			cs.server.React(conn, data)
		})
		return
	}
	out, ac := cs.server.React(conn, frame)
	action = gnet.Action(ac)
	return
}




// ITcpTransport Method

func (s *TcpTransport) Initialize(server IServer) {
	s.server = server
}

func (s *TcpTransport) SetMessageAdapter(msgAdapter protocol.IMessage) {
	s.msgAdapter = msgAdapter
}
func (s *TcpTransport) Shutdown() error {
	return nil
}

// serveListener accepts incoming connections on the Listener ln,
// creating a new service goroutine for each.
// The service goroutines read requests and then call services to reply to them.
func (s *TcpTransport) Serve(network, address string) (err error) {
	s.multicore = true
	s.addr = fmt.Sprintf("%s://%s", network,address)
	s.async = true
	s.workerPool = goroutine.Default()
	s.codec = &CustomLengthFieldProtocol{
		MsgAdapter: s.msgAdapter,
	}

	return gnet.Serve(s, s.addr, gnet.WithMulticore(s.multicore),
		gnet.WithTCPKeepAlive(time.Minute*5), gnet.WithCodec(s.codec))
}

func (s *TcpTransport) Address() net.Addr {
	return s.transportServer.Addr
}

// CustomLengthFieldProtocol : custom protocol
// custom protocol header contains Version, ActionType and DataLength fields
// its payload is Data field
type CustomLengthFieldProtocol struct {
	MsgAdapter protocol.IMessage
}

// Encode ... 将Conn中的Context和buf编码成将要传输的格式
func (cc *CustomLengthFieldProtocol) Encode(c gnet.Conn, buf []byte) ([]byte, error) {
	return buf,nil
}

// Decode ... 将网络中的数据解密成
func (cc *CustomLengthFieldProtocol) Decode(c gnet.Conn) ([]byte, error) {
	// parse header
	if cc.MsgAdapter == nil {
		glog.Error("tcp transport not msg adapter")
		return nil,errors.New("tcp transport not msg adapter")
	}


	headerLen :=  cc.MsgAdapter.GetHeaderLen()// uint16+uint16+uint32
	if size, header := c.ReadN(headerLen); size == headerLen {
		r := bytes.NewReader(header)

		dataLen, err := cc.MsgAdapter.ParseToBodyLen(r)
		// to check the protocol version and actionType,
		// reset buffer if the version or actionType is not correct
		if err != nil {
			return nil, err
		}


		protocolLen := headerLen + dataLen
		if dataSize, data := c.ReadN(protocolLen); dataSize == protocolLen {
			c.ShiftN(protocolLen)
			// log.Println("parse success:", data, dataSize)

			// return the payload of the data
			return data, nil
		}
		// log.Println("not enough payload data:", dataLen, protocolLen, dataSize)
		return nil, errors2.ErrIncompletePacket

	}

	// log.Println("not enough header data:", size)
	return nil, errors2.ErrIncompletePacket
}

