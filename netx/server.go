package netx

import (
	"bytes"
	"context"
	"crypto/tls"
	"errors"
	"fmt"
	errors2 "github.com/bxsec/gotool/errors"
	"github.com/bxsec/gotool/pool"
	method_service "github.com/bxsec/gotool/service"
	"io"
	"net"
	"os"
	"os/exec"
	"reflect"
	"regexp"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/bxsec/gotool/log"
	"github.com/bxsec/gotool/netx/connect"
	"github.com/bxsec/gotool/protocol"
	"github.com/bxsec/gotool/share"
)

// ErrServerClosed is returned by the Server's Serve, ListenAndServe after a call to Shutdown or Close.
var (
	ErrServerClosed  = errors.New("http: Server closed")
	ErrReqReachLimit = errors.New("request reached rate limit")
)

const (
	// ReaderBuffsize is used for bufio reader.
	ReaderBuffsize = 1024
	// WriterBuffsize is used for bufio writer.
	WriterBuffsize = 1024

	// WriteChanSize is used for response.
	WriteChanSize = 1024 * 1024
)

// contextKey is a value for use with context.WithValue. It's used as
// a pointer so it fits in an interface{} without allocation.
type contextKey struct {
	name string
}

func (k *contextKey) String() string { return "rpcx context value " + k.name }

var (
	// RemoteConnContextKey is a context key. It can be used in
	// services with context.WithValue to access the connection arrived on.
	// The associated value will be of type net.Conn.
	RemoteConnContextKey = &contextKey{"remote-conn"}
	// StartRequestContextKey records the start time
	StartRequestContextKey = &contextKey{"start-parse-request"}
	// StartSendRequestContextKey records the start time
	StartSendRequestContextKey = &contextKey{"start-send-request"}
	// TagContextKey is used to record extra info in handling services. Its value is a map[string]interface{}
	TagContextKey = &contextKey{"service-tag"}
	// HttpConnContextKey is used to store http connection.
	HttpConnContextKey = &contextKey{"http-conn"}
)

type Handler func(ctx *Context) error

// XServer is rpcx server that use TCP or UDP.
type XServer struct {
	//ln                 net.Listener
	readTimeout  time.Duration
	writeTimeout time.Duration
	AsyncWrite bool // set true if your server only serves few clients

	router map[string]Handler
	msgAdapter protocol.IMessage

	mu         sync.RWMutex
	activeConn map[connect.IConnect]struct{}

	serviceManager *method_service.ServiceManager

	doneChan   chan struct{}
	seq        uint64

	inShutdown int32
	onShutdown []func(s *XServer)
	onRestart  []func(s *XServer)

	// TLSConfig for creating tls tcp connection.
	tlsConfig *tls.Config
	// BlockCrypt for kcp.BlockCrypt
	options map[string]interface{}

	tcpTransport INetTransport

	// CORS options
	//corsOptions *CORSOptions

	Plugins PluginContainer

	// AuthFunc can be used to auth.
	AuthFunc func(ctx context.Context, req *protocol.Message, token string) error

	handlerMsgNum int32

	HandleServiceError func(error)
}

// NewServer returns a server.
func NewServer(options ...OptionFn) *XServer {

	s := &XServer{
		Plugins:    &pluginContainer{},
		options:    make(map[string]interface{}),
		activeConn: make(map[connect.IConnect]struct{}),
		doneChan:   make(chan struct{}),
		serviceManager: method_service.NewServiceManager(),
		tcpTransport: NewTcpTransport(),
		router:     make(map[string]Handler),
		AsyncWrite: false, // 除非你想benchmark或者极致优化，否则建议你设置为false
		msgAdapter:protocol.NewMessage(),
	}
	s.tcpTransport.SetMessageAdapter(s.msgAdapter)

	for _, op := range options {
		op(s)
	}

	if s.options["TCPKeepAlivePeriod"] == nil {
		s.options["TCPKeepAlivePeriod"] = 3 * time.Minute
	}
	return s
}

// Address returns listened address.
func (s *XServer) Address() net.Addr {
	return s.tcpTransport.Address()
}

func (s *XServer) AddHandler(servicePath, serviceMethod string, handler func(*Context) error) {
	s.router[servicePath+"."+serviceMethod] = handler
}

// ActiveClientConn returns active connections.
func (s *XServer) ActiveClientConn() []connect.IConnect {
	s.mu.RLock()
	defer s.mu.RUnlock()
	result := make([]connect.IConnect, 0, len(s.activeConn))
	for clientConn := range s.activeConn {
		result = append(result, clientConn)
	}
	return result
}

// SendMessage a request to the specified client.
// The client is designated by the conn.
// conn can be gotten from context in services:
//
//   ctx.Value(RemoteConnContextKey)
//
// servicePath, serviceMethod, metadata can be set to zero values.
func (s *XServer) SendMessage(conn net.Conn, servicePath, serviceMethod string,
	metadata map[string]string, data []byte) error {
	ctx := share.WithValue(context.Background(), StartSendRequestContextKey, time.Now().UnixNano())
	s.Plugins.DoPreWriteRequest(ctx)

	req := protocol.GetPooledMsg()
	req.SetMessageType(protocol.Request)

	seq := atomic.AddUint64(&s.seq, 1)
	req.SetSeq(seq)
	req.SetOneway(true)
	req.SetSerializeType(protocol.SerializeNone)
	req.ServicePath = servicePath
	req.ServiceMethod = serviceMethod
	req.Metadata = metadata
	req.Payload = data

	b := req.EncodeSlicePointer()
	_, err := conn.Write(*b)
	protocol.PutData(b)

	s.Plugins.DoPostWriteRequest(ctx, req, err)
	protocol.FreeMsg(req)
	return err
}

func (s *XServer) getDoneChan() <-chan struct{} {
	return s.doneChan
}

// Serve starts and listens RPC requests.
// It is blocked until receiving connections from clients.
func (s *XServer) Serve(network, address string) (err error) {
	return s.tcpTransport.Serve(network, address)
}

func (s *XServer) serveAsyncWrite(conn net.Conn, writeCh chan *[]byte) {
	for {
		select {
		case <-s.doneChan:
			return
		case data := <-writeCh:
			if data == nil {
				return
			}
			conn.Write(*data)
			protocol.PutData(data)
		}
	}
}

func parseServerTimeout(ctx *share.Context, req *protocol.Message) context.CancelFunc {
	if req == nil || req.Metadata == nil {
		return nil
	}

	st := req.Metadata[share.ServerTimeout]
	if st == "" {
		return nil
	}

	timeout, err := strconv.ParseInt(st, 10, 64)
	if err != nil {
		return nil
	}

	newCtx, cancel := context.WithTimeout(ctx.Context, time.Duration(timeout)*time.Millisecond)
	ctx.Context = newCtx
	return cancel
}

func (s *XServer) isShutdown() bool {
	return atomic.LoadInt32(&s.inShutdown) == 1
}


func (s *XServer) readRequest(ctx context.Context, r io.Reader) (req *protocol.Message, err error) {
	err = s.Plugins.DoPreReadRequest(ctx)
	if err != nil {
		return nil, err
	}
	// pool req?
	req = protocol.GetPooledMsg()
	err = req.Decode(r)
	if err == io.EOF {
		return req, err
	}
	perr := s.Plugins.DoPostReadRequest(ctx, req, err)
	if err == nil {
		err = perr
	}
	return req, err
}

func (s *XServer) auth(ctx context.Context, req *protocol.Message) error {
	if s.AuthFunc != nil {
		token := req.Metadata[share.AuthKey]
		return s.AuthFunc(ctx, req, token)
	}

	return nil
}

func (s *XServer) handleRequest(ctx context.Context, req *protocol.Message) (res *protocol.Message, err error) {
	serviceName := req.ServicePath
	methodName := req.ServiceMethod

	res = req.Clone()

	res.SetMessageType(protocol.Response)
	service, method, err := s.serviceManager.ServiceMethod(serviceName, methodName)
	if service == nil {
		err = errors.New("rpcx: can't find service " + serviceName)
		return handleError(res, err)
	}

	if method == nil {
		err = errors.New("rpcx: can't find method " + methodName)
		return handleError(res, err)
	}

	argType := method.GetArgType()
	replyType := method.GetReplyType()

	// get a argv object from object pool
	argv := pool.ReflectTypePools.Get(argType)

	serializer := share.Serializes[req.SerializeType()]
	if serializer == nil {
		err = fmt.Errorf("can not find codec for %d", req.SerializeType())
		return handleError(res, err)
	}

	err = serializer.Decode(req.Payload, argv)
	if err != nil {
		return handleError(res, err)
	}

	// and get a reply object from object pool
	replyv := pool.ReflectTypePools.Get(replyType)

	argv, err = s.Plugins.DoPreCall(ctx, serviceName, methodName, argv)
	if err != nil {
		// return reply to object pool
		pool.ReflectTypePools.Put(replyType, replyv)
		return handleError(res, err)
	}


	if argType.Kind() != reflect.Ptr {
		err = service.Call(ctx, method, reflect.ValueOf(argv).Elem(), reflect.ValueOf(replyv))
	} else {
		err = service.Call(ctx, method, reflect.ValueOf(argv), reflect.ValueOf(replyv))
	}

	if err == nil {
		replyv, err = s.Plugins.DoPostCall(ctx, serviceName, methodName, argv, replyv)
	}

	// return argc to object pool
	pool.ReflectTypePools.Put(argType, argv)

	if err != nil {
		if replyv != nil {
			data, err := serializer.Encode(replyv)
			// return reply to object pool
			pool.ReflectTypePools.Put(replyType, replyv)
			if err != nil {
				return handleError(res, err)
			}
			res.Payload = data
		}
		return handleError(res, err)
	}

	if !req.IsOneway() {
		data, err := serializer.Encode(replyv)
		// return reply to object pool
		pool.ReflectTypePools.Put(replyType, replyv)
		if err != nil {
			return handleError(res, err)
		}
		res.Payload = data
	} else if replyv != nil {
		pool.ReflectTypePools.Put(replyType, replyv)
	}

	if share.Trace {
		log.Debugf("server called service %+v for an request %+v", service, req)
	}

	return res, nil
}

func handleError(res *protocol.Message, err error) (*protocol.Message, error) {
	res.SetMessageStatusType(protocol.Error)
	if res.Metadata == nil {
		res.Metadata = make(map[string]string)
	}
	res.Metadata[protocol.ServiceError] = err.Error()
	return res, err
}

func (s *XServer) OnAccess(conn connect.IConnect) {
	if s.isShutdown() {
		conn.Close()
		return
	}

	conn, ok := s.Plugins.DoPostConnAccept(conn)
	if !ok {
		conn.Close()
		return
	}

	s.mu.Lock()
	s.activeConn[conn] = struct{}{}
	s.mu.Unlock()

	if share.Trace {
		log.Debugf("XServer accepted an conn: %v", conn.RemoteAddr().String())
	}
}

func (s *XServer) OnDisconnect(conn connect.IConnect) {
	s.mu.Lock()
	delete(s.activeConn, conn)
	s.mu.Unlock()
	s.Plugins.DoPostConnClose(conn)
}

func (s *XServer) OnShutdown(netTransport INetTransport) {

}

func (s *XServer) React(conn connect.IConnect, frame []byte) (out []byte, action Action) {
	defer func() {
		if err := recover(); err != nil {
			const size = 64 << 10
			buf := make([]byte, size)
			ss := runtime.Stack(buf, false)
			if ss > size {
				ss = size
			}
			buf = buf[:ss]
			log.Errorf("serving %s panic error: %s, stack:\n %s", conn.RemoteAddr(), err, buf)
		}

		if share.Trace {
			log.Debugf("server closed conn: %v", conn.RemoteAddr().String())
		}

		conn.Close()
	}()

	if s.isShutdown() {
		return nil,Shutdown
	}

	r := bytes.NewReader(frame)

	var writeCh chan *[]byte
	if s.AsyncWrite {
		writeCh = make(chan *[]byte, 1)
		defer close(writeCh)
		go s.serveAsyncWrite(conn, writeCh)
	}

	t0 := time.Now()
	if s.readTimeout != 0 {
		conn.SetReadDeadline(t0.Add(s.readTimeout))
	}

	ctx := share.WithValue(context.Background(), RemoteConnContextKey, conn)

	req, err := s.readRequest(ctx, r)
	if err != nil {
		protocol.FreeMsg(req)

		if err == io.EOF {
			log.Infof("client has closed this connection: %s", conn.RemoteAddr().String())
		} else if strings.Contains(err.Error(), "use of closed network connection") {
			log.Infof("rpcx: connection %s is closed", conn.RemoteAddr().String())
		} else if errors.Is(err, ErrReqReachLimit) {
			if !req.IsOneway() {
				res := req.Clone()
				res.SetMessageType(protocol.Response)
				if len(res.Payload) > 1024 && req.CompressType() != protocol.None {
					res.SetCompressType(req.CompressType())
				}
				handleError(res, err)
				s.Plugins.DoPreWriteResponse(ctx, req, res, err)
				data := res.EncodeSlicePointer()
				if s.AsyncWrite {
					writeCh <- data
				} else {
					conn.Write(*data)
					protocol.PutData(data)
				}
				s.Plugins.DoPostWriteResponse(ctx, req, res, err)
				protocol.FreeMsg(res)
			} else {
				s.Plugins.DoPreWriteResponse(ctx, req, nil, err)
			}
			protocol.FreeMsg(req)
			return
		} else {
			log.Warnf("rpcx: failed to read request: %v", err)
		}
		return
	}

	if s.writeTimeout != 0 {
		conn.SetWriteDeadline(t0.Add(s.writeTimeout))
	}

	if share.Trace {
		log.Debugf("server received an request %+v from conn: %v", req, conn.RemoteAddr().String())
	}

	ctx = share.WithLocalValue(ctx, StartRequestContextKey, time.Now().UnixNano())
	closeConn := false
	if !req.IsHeartbeat() {
		err = s.auth(ctx, req)
		closeConn = err != nil
	}

	if err != nil {
		if !req.IsOneway() {
			res := req.Clone()
			res.SetMessageType(protocol.Response)
			if len(res.Payload) > 1024 && req.CompressType() != protocol.None {
				res.SetCompressType(req.CompressType())
			}
			handleError(res, err)
			s.Plugins.DoPreWriteResponse(ctx, req, res, err)
			data := res.EncodeSlicePointer()
			if s.AsyncWrite {
				writeCh <- data
			} else {
				conn.Write(*data)
				protocol.PutData(data)
			}
			s.Plugins.DoPostWriteResponse(ctx, req, res, err)
			protocol.FreeMsg(res)
		} else {
			s.Plugins.DoPreWriteResponse(ctx, req, nil, err)
		}
		protocol.FreeMsg(req)
		// auth failed, closed the connection
		if closeConn {
			log.Infof("auth failed for conn %s: %v", conn.RemoteAddr().String(), err)
			return
		}
		return
	}

	go func() {
		defer func() {
			if r := recover(); r != nil {
				// maybe panic because the writeCh is closed.
				log.Errorf("failed to handle request: %v", r)
			}
		}()

		atomic.AddInt32(&s.handlerMsgNum, 1)
		defer atomic.AddInt32(&s.handlerMsgNum, -1)

		if req.IsHeartbeat() {
			s.Plugins.DoHeartbeatRequest(ctx, req)
			req.SetMessageType(protocol.Response)
			data := req.EncodeSlicePointer()
			if s.AsyncWrite {
				writeCh <- data
			} else {
				conn.Write(*data)
				protocol.PutData(data)
			}
			protocol.FreeMsg(req)
			return
		}

		resMetadata := make(map[string]string)
		ctx = share.WithLocalValue(share.WithLocalValue(ctx, share.ReqMetaDataKey, req.Metadata),
			share.ResMetaDataKey, resMetadata)

		cancelFunc := parseServerTimeout(ctx, req)
		if cancelFunc != nil {
			defer cancelFunc()
		}

		s.Plugins.DoPreHandleRequest(ctx, req)

		if share.Trace {
			log.Debugf("server handle request %+v from conn: %v", req, conn.RemoteAddr().String())
		}

		// first use handler
		if handler, ok := s.router[req.ServicePath+"."+req.ServiceMethod]; ok {
			sctx := NewContext(ctx, conn, req, writeCh)
			err := handler(sctx)
			if err != nil {
				log.Errorf("[handler internal error]: servicepath: %s, servicemethod, err: %v", req.ServicePath, req.ServiceMethod, err)
			}

			return
		}

		//
		res, err := s.handleRequest(ctx, req)
		if err != nil {
			if s.HandleServiceError != nil {
				s.HandleServiceError(err)
			} else {
				log.Warnf("rpcx: failed to handle request: %v", err)
			}
		}

		s.Plugins.DoPreWriteResponse(ctx, req, res, err)
		if !req.IsOneway() {
			if len(resMetadata) > 0 { // copy meta in context to request
				meta := res.Metadata
				if meta == nil {
					res.Metadata = resMetadata
				} else {
					for k, v := range resMetadata {
						if meta[k] == "" {
							meta[k] = v
						}
					}
				}
			}

			if len(res.Payload) > 1024 && req.CompressType() != protocol.None {
				res.SetCompressType(req.CompressType())
			}
			data := res.EncodeSlicePointer()
			if s.AsyncWrite {
				writeCh <- data
			} else {
				conn.Write(*data)
				protocol.PutData(data)
			}

		}

		s.Plugins.DoPostWriteResponse(ctx, req, res, err)

		if share.Trace {
			log.Debugf("server write response %+v for an request %+v from conn: %v", res, req, conn.RemoteAddr().String())
		}

		protocol.FreeMsg(req)
		protocol.FreeMsg(res)
	}()
	return nil,None
}

func (s *XServer) SetMessageAdapter(msgAdapter protocol.IMessage) {
	s.msgAdapter = msgAdapter
}


func (s *XServer) Register(rcvr interface{}, metadata string) error {
	sname, err := s.serviceManager.Register(rcvr, "", false)
	if err != nil {
		return err
	}
	return s.Plugins.DoRegister(sname, rcvr, metadata)
}

// RegisterName is like Register but uses the provided name for the type
// instead of the receiver's concrete type.
func (s *XServer) RegisterName(name string, rcvr interface{}, metadata string) error {
	_, err := s.serviceManager.Register(rcvr, name, true)
	if err != nil {
		return err
	}
	if s.Plugins == nil {
		s.Plugins = &pluginContainer{}
	}
	return s.Plugins.DoRegister(name, rcvr, metadata)
}

// RegisterFunction publishes a function that satisfy the following conditions:
//	- three arguments, the first is of context.Context, both of exported type for three arguments
//	- the third argument is a pointer
//	- one return value, of type error
// The client accesses function using a string of the form "servicePath.Method".
func (s *XServer) RegisterFunction(servicePath string, fn interface{}, metadata string) error {
	fname, err := s.serviceManager.RegisterFunction(servicePath, fn, "", false)
	if err != nil {
		return err
	}

	return s.Plugins.DoRegisterFunction(servicePath, fname, fn, metadata)
}

// RegisterFunctionName is like RegisterFunction but uses the provided name for the function
// instead of the function's concrete type.
func (s *XServer) RegisterFunctionName(servicePath string, name string, fn interface{}, metadata string) error {
	_, err := s.serviceManager.RegisterFunction(servicePath, fn, name, true)
	if err != nil {
		return err
	}

	return s.Plugins.DoRegisterFunction(servicePath, name, fn, metadata)
}

func (s *XServer) UnregisterAll() error {
	serviceNameArr := s.serviceManager.UnregisterAll()

	var es []error
	for _,serviceName := range serviceNameArr {
		err := s.Plugins.DoUnregister(serviceName)
		if err != nil {
			es = append(es, err)
		}
	}

	if len(es) > 0 {
		return errors2.NewMultiError (es)
	}
	return nil
}



// Close immediately closes all active net.Listeners.
func (s *XServer) Close() error {
	s.mu.Lock()
	defer s.mu.Unlock()

	var err error
	for c := range s.activeConn {
		c.Close()
	}
	s.closeDoneChanLocked()
	return err
}

// RegisterOnShutdown registers a function to call on Shutdown.
// This can be used to gracefully shutdown connections.
func (s *XServer) RegisterOnShutdown(f func(s *XServer)) {
	s.mu.Lock()
	s.onShutdown = append(s.onShutdown, f)
	s.mu.Unlock()
}

// RegisterOnRestart registers a function to call on Restart.
func (s *XServer) RegisterOnRestart(f func(s *XServer)) {
	s.mu.Lock()
	s.onRestart = append(s.onRestart, f)
	s.mu.Unlock()
}

var shutdownPollInterval = 1000 * time.Millisecond

// Shutdown gracefully shuts down the server without interrupting any
// active connections. Shutdown works by first closing the
// listener, then closing all idle connections, and then waiting
// indefinitely for connections to return to idle and then shut down.
// If the provided context expires before the shutdown is complete,
// Shutdown returns the context's error, otherwise it returns any
// error returned from closing the Server's underlying Listener.
func (s *XServer) Shutdown(ctx context.Context) error {
	var err error
	if atomic.CompareAndSwapInt32(&s.inShutdown, 0, 1) {
		log.Info("shutdown begin")

		//关闭conn继续读取
	//	s.mu.Lock()
	//	s.ln.Close()
	//	for conn := range s.activeConn {
	//
	//		if tcpConn, ok := conn.(*net.TCPConn); ok {
	//			tcpConn.CloseRead()
	//		}
	//	}
	//	s.mu.Unlock()
	//
	//	// wait all in-processing requests finish.
	//	ticker := time.NewTicker(shutdownPollInterval)
	//	defer ticker.Stop()
	//outer:
	//	for {
	//		if s.checkProcessMsg() {
	//			break
	//		}
	//		select {
	//		case <-ctx.Done():
	//			err = ctx.Err()
	//			break outer
	//		case <-ticker.C:
	//		}
	//	}


		s.mu.Lock()
		for conn := range s.activeConn {
			conn.Close()
		}
		s.closeDoneChanLocked()
		s.mu.Unlock()

		log.Info("shutdown end")

	}
	return err
}

// Restart restarts this server gracefully.
// It starts a new rpcx server with the same port with SO_REUSEPORT socket option,
// and shutdown this rpcx server gracefully.
func (s *XServer) Restart(ctx context.Context) error {
	pid, err := s.startProcess()
	if err != nil {
		return err
	}
	log.Infof("restart a new rpcx server: %d", pid)

	// TODO: is it necessary?
	time.Sleep(3 * time.Second)
	return s.Shutdown(ctx)
}

func (s *XServer) startProcess() (int, error) {
	argv0, err := exec.LookPath(os.Args[0])
	if err != nil {
		return 0, err
	}

	// Pass on the environment and replace the old count key with the new one.
	var env []string
	env = append(env, os.Environ()...)

	originalWD, _ := os.Getwd()
	allFiles := []*os.File{os.Stdin, os.Stdout, os.Stderr}
	process, err := os.StartProcess(argv0, os.Args, &os.ProcAttr{
		Dir:   originalWD,
		Env:   env,
		Files: allFiles,
	})
	if err != nil {
		return 0, err
	}
	return process.Pid, nil
}

func (s *XServer) checkProcessMsg() bool {
	size := atomic.LoadInt32(&s.handlerMsgNum)
	log.Info("need handle in-processing msg size:", size)
	return size == 0
}

func (s *XServer) closeDoneChanLocked() {
	select {
	case <-s.doneChan:
		// Already closed. Don't close again.
	default:
		// Safe to close here. We're the only closer, guarded
		// by s.mu.RegisterName
		close(s.doneChan)
	}
}

var ip4Reg = regexp.MustCompile(`^(([0-9]|[1-9][0-9]|1[0-9]{2}|2[0-4][0-9]|25[0-5])\.){3}([0-9]|[1-9][0-9]|1[0-9]{2}|2[0-4][0-9]|25[0-5])$`)

func validIP4(ipAddress string) bool {
	ipAddress = strings.Trim(ipAddress, " ")
	i := strings.LastIndex(ipAddress, ":")
	ipAddress = ipAddress[:i] // remove port

	return ip4Reg.MatchString(ipAddress)
}

