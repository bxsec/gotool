package netx

import (
	"crypto/tls"
	"time"
)

// OptionFn configures options of server.
type ServerOptionFn func(server *XServer)

// // WithOptions sets multiple options.
// func WithOptions(ops map[string]interface{}) OptionFn {
// 	return func(s *Server) {
// 		for k, v := range ops {
// 			s.options[k] = v
// 		}
// 	}
// }

// WithTLSConfig sets tls.Config.
func WithTLSConfig(cfg *tls.Config) ServerOptionFn {
	return func(s *XServer) {
		s.tlsConfig = cfg
	}
}

// WithReadTimeout sets readTimeout.
func WithReadTimeout(readTimeout time.Duration) ServerOptionFn {
	return func(s *XServer) {
		s.readTimeout = readTimeout
	}
}

// WithWriteTimeout sets writeTimeout.
func WithWriteTimeout(writeTimeout time.Duration) ServerOptionFn {
	return func(s *XServer) {
		s.writeTimeout = writeTimeout
	}
}

type TransportOptionFn func(transport *TcpTransport)
