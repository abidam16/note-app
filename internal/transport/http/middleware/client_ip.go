package middleware

import (
	"context"
	"net"
	"net/http"
	"strings"
)

const ContextClientIPKey contextKey = "client_ip"

type ClientIPConfig struct {
	TrustProxyHeaders bool
	TrustedProxyCIDRs []string
}

type clientIPResolver struct {
	trustProxyHeaders bool
	trustedProxies    []*net.IPNet
}

func ResolveClientIP(config ClientIPConfig) func(http.Handler) http.Handler {
	resolver := clientIPResolver{
		trustProxyHeaders: config.TrustProxyHeaders,
		trustedProxies:    parseTrustedProxyCIDRs(config.TrustedProxyCIDRs),
	}

	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			clientIP := resolver.resolve(r)
			ctx := context.WithValue(r.Context(), ContextClientIPKey, clientIP)
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}

func RequestClientIP(r *http.Request) string {
	if r == nil {
		return "unknown"
	}
	if clientIP, _ := r.Context().Value(ContextClientIPKey).(string); strings.TrimSpace(clientIP) != "" {
		return strings.TrimSpace(clientIP)
	}
	return peerIPFromRemoteAddr(r.RemoteAddr)
}

func (r clientIPResolver) resolve(req *http.Request) string {
	peerIP := peerIPFromRemoteAddr(req.RemoteAddr)
	if !r.trustProxyHeaders {
		return peerIP
	}
	peer := net.ParseIP(peerIP)
	if peer == nil || !ipInTrustedNetworks(peer, r.trustedProxies) {
		return peerIP
	}
	if forwarded := forwardedIP(req.Header.Get("X-Forwarded-For")); forwarded != "" {
		return forwarded
	}
	if realIP := singleHeaderIP(req.Header.Get("X-Real-IP")); realIP != "" {
		return realIP
	}
	return peerIP
}

func parseTrustedProxyCIDRs(values []string) []*net.IPNet {
	networks := make([]*net.IPNet, 0, len(values))
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value == "" {
			continue
		}
		_, network, err := net.ParseCIDR(value)
		if err != nil {
			continue
		}
		networks = append(networks, network)
	}
	return networks
}

func ipInTrustedNetworks(ip net.IP, networks []*net.IPNet) bool {
	for _, network := range networks {
		if network.Contains(ip) {
			return true
		}
	}
	return false
}

func peerIPFromRemoteAddr(remoteAddr string) string {
	remoteAddr = strings.TrimSpace(remoteAddr)
	if remoteAddr == "" {
		return "unknown"
	}
	host, _, err := net.SplitHostPort(remoteAddr)
	if err == nil && strings.TrimSpace(host) != "" {
		return host
	}
	if ip := net.ParseIP(remoteAddr); ip != nil {
		return ip.String()
	}
	return remoteAddr
}

func forwardedIP(header string) string {
	for _, part := range strings.Split(header, ",") {
		if ip := singleHeaderIP(part); ip != "" {
			return ip
		}
	}
	return ""
}

func singleHeaderIP(value string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return ""
	}
	if ip := net.ParseIP(value); ip != nil {
		return ip.String()
	}
	return ""
}
