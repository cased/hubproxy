package security

import (
	"crypto/tls"
	"log/slog"
	"net"
	"net/http"

	"tailscale.com/ipn"
)

type contextKey string

const (
	connectionContextKey contextKey = "connection"
)

// Hack to set http.Request.RemoteAddr to the client's IP address
//
// See Tailscale snippet for reference:
// <https://github.com/tailscale/tailscale/blob/8d7033f/cmd/tsidp/tsidp.go#L1040-L1059>
func TailscaleFunnelIP(logger *slog.Logger) func(http.Handler) http.Handler {
	return func(h http.Handler) http.Handler {
		fn := func(w http.ResponseWriter, r *http.Request) {
			v := r.Context().Value(connectionContextKey)
			if v == nil {
				logger.Warn("expected request context to set connection",
					"RemoteAddr", r.RemoteAddr)
			} else if netConn, ok := v.(net.Conn); ok {
				logger.Debug("extracted connection from request context", "netConn", netConn)
				if tlsConn, ok := netConn.(*tls.Conn); ok {
					logger.Debug("unwrapping tls.Conn", "netConn", netConn, "tlsConn", tlsConn)
					netConn = tlsConn.NetConn()
				}
				if funnelConn, ok := netConn.(*ipn.FunnelConn); ok {
					logger.Debug("request context connection is a FunnelConn", "netConn", netConn, "fc", funnelConn)
					realRemoteAddr := funnelConn.Src.String()
					logger.Debug("changing request RemoteAddr", "from", r.RemoteAddr, "to", realRemoteAddr)
					r.RemoteAddr = realRemoteAddr
				} else {
					logger.Warn("request context connection is not a ipn.FunnelConn", "netConn", netConn)
				}
			} else {
				logger.Warn("expected request context connection to be a net.Conn, but was",
					"contextValue", v,
					"RemoteAddr", r.RemoteAddr)
			}
			h.ServeHTTP(w, r)
		}

		return http.HandlerFunc(fn)
	}
}
