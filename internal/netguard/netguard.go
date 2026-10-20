// Package netguard is the shared outbound-HTTP safety layer used by MCP, HTTP
// hooks and WebFetch. It validates URLs, pins the dialed IP to the address that
// was validated (no DNS rebinding), and re-validates every redirect hop.
package netguard

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/http"
	"net/netip"
	"net/url"
	"strings"
	"time"
)

const (
	defaultTimeout      = 30 * time.Second
	defaultMaxRedirects = 3
)

// Policy describes which destinations an outbound client may reach.
// The zero value allows public destinations only.
type Policy struct {
	// AllowPrivate permits RFC 1918, link-local, ULA and CGNAT destinations.
	AllowPrivate bool
	// AllowLoopback permits 127.0.0.0/8, ::1 and localhost names.
	AllowLoopback bool
	// HTTPLoopbackOnly requires https unless the destination is loopback.
	HTTPLoopbackOnly bool
	// Timeout bounds the whole request; default 30s.
	Timeout time.Duration
	// MaxRedirects bounds redirect hops; default 3.
	MaxRedirects int
	// Resolver overrides DNS resolution (tests). Default is the system resolver.
	Resolver func(ctx context.Context, host string) ([]netip.Addr, error)
}

// ValidateURL checks the scheme and host of rawURL against p without resolving DNS.
func ValidateURL(rawURL string, p Policy) error {
	u, err := url.Parse(strings.TrimSpace(rawURL))
	if err != nil {
		return fmt.Errorf("invalid url: %w", err)
	}
	return validateParsed(u, p)
}

func validateParsed(u *url.URL, p Policy) error {
	if u.Scheme != "https" && u.Scheme != "http" {
		return fmt.Errorf("unsupported scheme %q; only http and https are allowed", u.Scheme)
	}
	host := u.Hostname()
	if host == "" {
		return errors.New("missing host")
	}
	loopback := false
	if isLocalName(host) {
		if !p.AllowLoopback {
			return fmt.Errorf("localhost destination denied: %s", host)
		}
		loopback = true
	} else if addr, ok := parseHostAddr(host); ok {
		if Blocked(addr, p) {
			return fmt.Errorf("destination denied: %s", host)
		}
		loopback = Unmap(addr).IsLoopback()
	}
	if u.Scheme == "http" && p.HTTPLoopbackOnly && !loopback {
		return errors.New("http allowed only for loopback")
	}
	return nil
}

// Blocked reports whether addr is forbidden under p. Unspecified and multicast
// addresses are always blocked.
func Blocked(addr netip.Addr, p Policy) bool {
	addr = Unmap(addr)
	switch {
	case !addr.IsValid(), addr.IsUnspecified(), addr.IsMulticast():
		return true
	case addr.IsLoopback():
		return !p.AllowLoopback
	case addr.IsPrivate(), addr.IsLinkLocalUnicast(), addr.IsInterfaceLocalMulticast(), isCGNAT(addr):
		return !p.AllowPrivate
	}
	return false
}

// Unmap returns the IPv4 address embedded in IPv4-mapped (::ffff:a.b.c.d),
// NAT64 (64:ff9b::/96) and 6to4 (2002::/16) IPv6 forms; other addresses are
// returned without zone.
func Unmap(addr netip.Addr) netip.Addr {
	addr = addr.WithZone("").Unmap()
	if !addr.Is6() {
		return addr
	}
	b := addr.As16()
	if b[0] == 0x00 && b[1] == 0x64 && b[2] == 0xff && b[3] == 0x9b && allZero(b[4:12]) {
		return netip.AddrFrom4([4]byte{b[12], b[13], b[14], b[15]})
	}
	if b[0] == 0x20 && b[1] == 0x02 {
		return netip.AddrFrom4([4]byte{b[2], b[3], b[4], b[5]})
	}
	return addr
}

func allZero(b []byte) bool {
	for _, v := range b {
		if v != 0 {
			return false
		}
	}
	return true
}

func isCGNAT(a netip.Addr) bool {
	return a.Is4() && a.As4()[0] == 100 && a.As4()[1]&0xc0 == 64
}

func isLocalName(host string) bool {
	h := strings.ToLower(strings.TrimSuffix(strings.TrimSpace(host), "."))
	return h == "localhost" || strings.HasSuffix(h, ".localhost")
}

func parseHostAddr(host string) (netip.Addr, bool) {
	host = strings.TrimSuffix(strings.TrimPrefix(host, "["), "]")
	addr, err := netip.ParseAddr(host)
	if err != nil {
		return netip.Addr{}, false
	}
	return addr, true
}

// NewClient returns an http.Client that only dials validated IPs and
// re-validates every redirect target.
func NewClient(p Policy) *http.Client {
	timeout := p.Timeout
	if timeout <= 0 {
		timeout = defaultTimeout
	}
	maxRedirects := p.MaxRedirects
	if maxRedirects <= 0 {
		maxRedirects = defaultMaxRedirects
	}
	transport := http.DefaultTransport.(*http.Transport).Clone()
	transport.Proxy = nil // a proxy would receive the dial instead of the validated target
	dialer := &net.Dialer{Timeout: timeout}
	transport.DialContext = func(ctx context.Context, network, address string) (net.Conn, error) {
		host, port, err := net.SplitHostPort(address)
		if err != nil {
			return nil, fmt.Errorf("invalid dial address %q: %w", address, err)
		}
		addrs, err := resolveValidated(ctx, host, p)
		if err != nil {
			return nil, err
		}
		var lastErr error
		for _, a := range addrs {
			conn, derr := dialer.DialContext(ctx, network, net.JoinHostPort(a.String(), port))
			if derr == nil {
				return conn, nil
			}
			lastErr = derr
		}
		return nil, lastErr
	}
	return &http.Client{
		Timeout:   timeout,
		Transport: transport,
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			if len(via) >= maxRedirects {
				return fmt.Errorf("too many redirects (max %d)", maxRedirects)
			}
			if err := validateParsed(req.URL, p); err != nil {
				return fmt.Errorf("redirect blocked: %w", err)
			}
			return nil
		},
	}
}

// resolveValidated resolves host once and returns its addresses only if every
// one is allowed. The caller dials the returned IPs, never the name.
func resolveValidated(ctx context.Context, host string, p Policy) ([]netip.Addr, error) {
	host = strings.TrimSpace(host)
	if host == "" {
		return nil, errors.New("empty dial host")
	}
	if isLocalName(host) && !p.AllowLoopback {
		return nil, fmt.Errorf("localhost destination denied: %s", host)
	}
	var addrs []netip.Addr
	if a, ok := parseHostAddr(host); ok {
		addrs = []netip.Addr{a}
	} else if isLocalName(host) {
		addrs = []netip.Addr{netip.MustParseAddr("127.0.0.1"), netip.IPv6Loopback()}
	} else {
		var err error
		if p.Resolver != nil {
			addrs, err = p.Resolver(ctx, host)
		} else {
			var ips []net.IPAddr
			ips, err = net.DefaultResolver.LookupIPAddr(ctx, host)
			for _, ip := range ips {
				if a, ok := netip.AddrFromSlice(ip.IP); ok {
					addrs = append(addrs, a)
				}
			}
		}
		if err != nil {
			return nil, fmt.Errorf("dns lookup failed: %w", err)
		}
	}
	if len(addrs) == 0 {
		return nil, fmt.Errorf("no addresses for host %s", host)
	}
	for _, a := range addrs {
		if Blocked(a, p) {
			return nil, fmt.Errorf("destination denied: %s resolves to %s", host, a.WithZone(""))
		}
	}
	return addrs, nil
}

// ValidateDialHost resolves host and checks every answer against p.
func ValidateDialHost(ctx context.Context, host string, p Policy) error {
	_, err := resolveValidated(ctx, host, p)
	return err
}
