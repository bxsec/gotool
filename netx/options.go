package netx

import "time"

// WithTCPKeepAlivePeriod sets tcp keepalive period.
func WithTCPKeepAlivePeriod(period time.Duration) ServerOptionFn {
	return func(s *XServer) {
		s.options["TCPKeepAlivePeriod"] = period
	}
}
