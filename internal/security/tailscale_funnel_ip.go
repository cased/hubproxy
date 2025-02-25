package security

import (
	"crypto/tls"
	"net"
	"net/http"

	"tailscale.com/ipn"
)

// Hack to set http.Request.RemoteAddr to the client's IP address
//
// See Tailscale snippet for reference:
// <https://github.com/tailscale/tailscale/blob/8d7033f/cmd/tsidp/tsidp.go#L1040-L1059>
func TailscaleFunnelIP(h http.Handler) http.Handler {
	fn := func(w http.ResponseWriter, r *http.Request) {
		netConn := r.Context().Value("connection").(net.Conn)
		if tlsConn, ok := netConn.(*tls.Conn); ok {
			netConn = tlsConn.NetConn()
		}
		if fc, ok := netConn.(*ipn.FunnelConn); ok {
			r.RemoteAddr = fc.Src.String()
		}
		h.ServeHTTP(w, r)
	}

	return http.HandlerFunc(fn)
}
