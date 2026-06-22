package mcp

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"net/http/httptrace"
	"net/netip"
	"net/url"
	"strings"
	"time"
)

// ValidateHTTPDestination checks whether an HTTP endpoint is allowed.
// Rules:
// - https is required unless host is loopback/localhost
// - private/link-local/unspecified/multicast IPs are denied unless allowPrivate is true
func ValidateHTTPDestination(rawURL string, allowPrivate bool) error {
	u, err := url.Parse(strings.TrimSpace(rawURL))
	if err != nil {
		return fmt.Errorf("invalid url: %w", err)
	}
	if u.Scheme != "https" && u.Scheme != "http" {
		return fmt.Errorf("unsupported scheme %q", u.Scheme)
	}
	host := u.Hostname()
	if host == "" {
		return fmt.Errorf("missing host")
	}
	if host == "localhost" {
		return nil
	}
	if ip := net.ParseIP(host); ip != nil {
		addr, ok := netip.AddrFromSlice(ip)
		if !ok {
			return fmt.Errorf("invalid ip")
		}
		if !allowPrivate && isPrivateish(addr) && !addr.IsLoopback() {
			return fmt.Errorf("private destination denied: %s", host)
		}
		if u.Scheme == "http" && !addr.IsLoopback() {
			return fmt.Errorf("http allowed only for loopback")
		}
		return nil
	}
	if u.Scheme == "http" {
		return fmt.Errorf("http allowed only for loopback/localhost")
	}
	return nil
}

func isPrivateish(a netip.Addr) bool {
	return a.IsPrivate() || a.IsLinkLocalUnicast() || a.IsLoopback() || a.IsMulticast() || a.IsUnspecified()
}

func NewSafeHTTPClient(allowPrivate bool, timeout time.Duration) *http.Client {
	if timeout <= 0 {
		timeout = 30 * time.Second
	}
	transport := http.DefaultTransport.(*http.Transport).Clone()
	dialer := &net.Dialer{Timeout: timeout}
	transport.DialContext = func(ctx context.Context, network, address string) (net.Conn, error) {
		host, port, err := net.SplitHostPort(address)
		if err != nil {
			host = address
			port = ""
		}
		if err := validateDialHost(ctx, host, allowPrivate); err != nil {
			return nil, err
		}
		if port == "" {
			return dialer.DialContext(ctx, network, host)
		}
		return dialer.DialContext(ctx, network, net.JoinHostPort(host, port))
	}
	return &http.Client{
		Timeout:   timeout,
		Transport: transport,
	}
}

func validateDialHost(ctx context.Context, host string, allowPrivate bool) error {
	host = strings.TrimSpace(host)
	if host == "" {
		return fmt.Errorf("empty dial host")
	}
	if host == "localhost" {
		return nil
	}
	if ip := net.ParseIP(host); ip != nil {
		addr, ok := netip.AddrFromSlice(ip)
		if !ok {
			return fmt.Errorf("invalid ip")
		}
		if !allowPrivate && isPrivateish(addr) && !addr.IsLoopback() {
			return fmt.Errorf("private destination denied: %s", host)
		}
		return nil
	}
	resolver := net.DefaultResolver
	if trace := httptrace.ContextClientTrace(ctx); trace != nil && trace.DNSStart != nil {
		trace.DNSStart(httptrace.DNSStartInfo{Host: host})
	}
	ips, err := resolver.LookupIPAddr(ctx, host)
	if err != nil {
		return fmt.Errorf("dns lookup failed: %w", err)
	}
	for _, ip := range ips {
		addr, ok := netip.AddrFromSlice(ip.IP)
		if !ok {
			continue
		}
		if !allowPrivate && isPrivateish(addr) && !addr.IsLoopback() {
			return fmt.Errorf("hostname resolves to private destination: %s", host)
		}
	}
	return nil
}
