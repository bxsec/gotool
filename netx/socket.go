package netx

import (
	"bufio"
	"context"
	"crypto/tls"
	"github.com/bxsec/gotool/log"
	"github.com/bxsec/gotool/protocol"
	"github.com/bxsec/gotool/share"
	"io"
	"net"
	"runtime"
	"strings"
	"sync/atomic"
	"time"
)

func (s *Server) serveConn(conn net.Conn) {
	if s.isShutdown() {
		s.closeConn(conn)
		return
	}

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

		s.closeConn(conn)
	}()

	if tlsConn, ok := conn.(*tls.Conn); ok {
		if d := s.readTimeout; d != 0 {
			conn.SetReadDeadline(time.Now().Add(d))
		}
		if d := s.writeTimeout; d != 0 {
			conn.SetWriteDeadline(time.Now().Add(d))
		}
		if err := tlsConn.Handshake(); err != nil {
			log.Errorf("rpcx: TLS handshake error from %s: %v", conn.RemoteAddr(), err)
			return
		}
	}

	r := bufio.NewReaderSize(conn, ReaderBuffsize)

	var writeCh chan *[]byte
	if s.AsyncWrite {
		writeCh = make(chan *[]byte, 1)
		defer close(writeCh)
		go s.serveAsyncWrite(conn, writeCh)
	}

	for {
		if s.isShutdown() {
			return
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
				continue
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
			continue
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
	}
}
