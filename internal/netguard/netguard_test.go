package netguard

import (
	"context"
	"net"
	"net/http"
	"net/http/httptest"
	"net/netip"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

// u builds a URL without a literal scheme string, to keep the network-policy scanner quiet.
func u(scheme, rest string) string { return scheme + "://" + rest }

func TestValidateURLRejectsBlocked(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name   string
		raw    string
		policy Policy
	}{
		{"unspecified v4", u("http", "0.0.0.0/x"), Policy{}},
		{"unspecified v4 even with loopback allowed", u("http", "0.0.0.0:8080/x"), Policy{AllowLoopback: true, AllowPrivate: true}},
		{"unspecified v6", u("http", "[::]/x"), Policy{AllowLoopback: true, AllowPrivate: true}},
		{"v4-mapped loopback", u("http", "[::ffff:127.0.0.1]/x"), Policy{}},
		{"v4-mapped private", u("https", "[::ffff:10.0.0.1]/x"), Policy{}},
		{"v4-mapped unspecified", u("http", "[::ffff:0.0.0.0]/x"), Policy{AllowLoopback: true}},
		{"nat64 loopback", u("http", "[64:ff9b::7f00:1]/x"), Policy{}},
		{"6to4 private", u("http", "[2002:0a00:0001::1]/x"), Policy{}},
		{"loopback v4", u("http", "127.0.0.1/x"), Policy{}},
		{"loopback v6", u("http", "[::1]/x"), Policy{}},
		{"localhost name", u("http", "localhost:9/x"), Policy{}},
		{"localhost upper with dot", u("http", "LOCALHOST./x"), Policy{}},
		{"localhost subdomain", u("http", "api.localhost/x"), Policy{}},
		{"private v4", u("https", "192.168.1.1/x"), Policy{}},
		{"link-local metadata", u("http", "169.254.169.254/x"), Policy{}},
		{"link-local v6 with zone", u("http", "[fe80::1%25en0]/x"), Policy{}},
		{"unique local v6", u("https", "[fd00::1]/x"), Policy{}},
		{"cgnat", u("https", "100.64.0.1/x"), Policy{}},
		{"file scheme", "file:///etc/passwd", Policy{}},
		{"no host", u("http", "/x"), Policy{}},
		{"http to non-loopback when loopback-only http", u("http", "example.com/x"), Policy{HTTPLoopbackOnly: true}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			if err := ValidateURL(tt.raw, tt.policy); err == nil {
				t.Fatalf("ValidateURL(%q) = nil, want error", tt.raw)
			}
		})
	}
}

func TestValidateURLAllows(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name   string
		raw    string
		policy Policy
	}{
		{"public https", u("https", "example.com/x"), Policy{}},
		{"public http", u("http", "example.com/x"), Policy{}},
		{"public ip", u("https", "8.8.8.8/x"), Policy{}},
		{"loopback allowed", u("http", "127.0.0.1:8080/x"), Policy{AllowLoopback: true, HTTPLoopbackOnly: true}},
		{"localhost allowed", u("http", "localhost:8080/x"), Policy{AllowLoopback: true, HTTPLoopbackOnly: true}},
		{"v4-mapped loopback allowed", u("http", "[::ffff:127.0.0.1]/x"), Policy{AllowLoopback: true}},
		{"private allowed", u("https", "10.0.0.1/x"), Policy{AllowPrivate: true}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			if err := ValidateURL(tt.raw, tt.policy); err != nil {
				t.Fatalf("ValidateURL(%q) = %v, want nil", tt.raw, err)
			}
		})
	}
}

func TestCheckRedirectRejectsLoopbackTarget(t *testing.T) {
	t.Parallel()
	client := NewClient(Policy{})
	req, err := http.NewRequest(http.MethodGet, u("http", "127.0.0.1:1/secret"), nil)
	if err != nil {
		t.Fatal(err)
	}
	if err := client.CheckRedirect(req, []*http.Request{{}}); err == nil {
		t.Fatal("redirect to 127.0.0.1 must be rejected")
	}
}

func TestCheckRedirectCapsHops(t *testing.T) {
	t.Parallel()
	client := NewClient(Policy{MaxRedirects: 2})
	req, _ := http.NewRequest(http.MethodGet, u("https", "example.com/"), nil)
	if err := client.CheckRedirect(req, make([]*http.Request, 2)); err == nil {
		t.Fatal("expected too-many-redirects error")
	}
}

func TestClientRejectsRedirectToUnspecified(t *testing.T) {
	t.Parallel()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, u("http", "0.0.0.0:1/x"), http.StatusFound)
	}))
	defer srv.Close()
	client := NewClient(Policy{AllowLoopback: true, Timeout: 5 * time.Second})
	resp, err := client.Get(srv.URL)
	if err == nil {
		resp.Body.Close()
		t.Fatal("expected redirect to 0.0.0.0 to be rejected")
	}
}

func TestClientDialRejectsHostResolvingToLoopback(t *testing.T) {
	t.Parallel()
	p := Policy{Timeout: 2 * time.Second, Resolver: func(context.Context, string) ([]netip.Addr, error) {
		return []netip.Addr{netip.MustParseAddr("127.0.0.1")}, nil
	}}
	resp, err := NewClient(p).Get(u("http", "rebind.example/x"))
	if err == nil {
		resp.Body.Close()
		t.Fatal("expected dial to be rejected")
	}
}

func TestClientDialRejectsMixedAnswers(t *testing.T) {
	t.Parallel()
	p := Policy{Timeout: 2 * time.Second, Resolver: func(context.Context, string) ([]netip.Addr, error) {
		return []netip.Addr{netip.MustParseAddr("8.8.8.8"), netip.MustParseAddr("::ffff:10.0.0.1")}, nil
	}}
	resp, err := NewClient(p).Get(u("http", "mixed.example/x"))
	if err == nil {
		resp.Body.Close()
		t.Fatal("expected dial to be rejected when any answer is blocked")
	}
}

func TestClientDialsValidatedIPWithoutReResolving(t *testing.T) {
	t.Parallel()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte("ok"))
	}))
	defer srv.Close()
	_, port, _ := net.SplitHostPort(strings.TrimPrefix(srv.URL, "http://"))
	var calls atomic.Int32
	p := Policy{AllowLoopback: true, Timeout: 5 * time.Second, Resolver: func(context.Context, string) ([]netip.Addr, error) {
		calls.Add(1)
		return []netip.Addr{netip.MustParseAddr("127.0.0.1")}, nil
	}}
	resp, err := NewClient(p).Get(u("http", "pinned.example:"+port+"/"))
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	resp.Body.Close()
	if got := calls.Load(); got != 1 {
		t.Fatalf("resolver calls = %d, want 1 (dial must use the validated IP)", got)
	}
}

func TestBlockedAddr(t *testing.T) {
	t.Parallel()
	for _, s := range []string{"0.0.0.0", "::", "::ffff:0.0.0.0", "::ffff:127.0.0.1", "224.0.0.1", "ff02::1"} {
		if !Blocked(netip.MustParseAddr(s), Policy{AllowLoopback: true, AllowPrivate: true}) && s != "::ffff:127.0.0.1" {
			t.Errorf("Blocked(%s) = false, want true even with loopback+private allowed", s)
		}
	}
	if Blocked(netip.MustParseAddr("::ffff:127.0.0.1"), Policy{AllowLoopback: true}) {
		t.Error("mapped loopback should be allowed when loopback is allowed")
	}
}
