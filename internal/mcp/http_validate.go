package mcp

import (
	"context"
	"net/http"
	"time"

	"github.com/FernasFragas/Nandocode/internal/netguard"
)

// policy keeps MCP's historical rules: loopback is reachable, http only to
// loopback, private ranges only when allowPrivate is set. The shared guard
// additionally blocks 0.0.0.0, unmapped IPv6 forms, DNS rebinding and
// redirects to blocked hosts.
func policy(allowPrivate bool, timeout time.Duration) netguard.Policy {
	return netguard.Policy{
		AllowPrivate:     allowPrivate,
		AllowLoopback:    true,
		HTTPLoopbackOnly: true,
		Timeout:          timeout,
	}
}

// ValidateHTTPDestination checks whether an HTTP endpoint is allowed.
// It delegates to internal/netguard.
func ValidateHTTPDestination(rawURL string, allowPrivate bool) error {
	return netguard.ValidateURL(rawURL, policy(allowPrivate, 0))
}

// NewSafeHTTPClient returns a client that dials only validated IPs and
// re-validates redirects. It delegates to internal/netguard.
func NewSafeHTTPClient(allowPrivate bool, timeout time.Duration) *http.Client {
	return netguard.NewClient(policy(allowPrivate, timeout))
}

func validateDialHost(ctx context.Context, host string, allowPrivate bool) error {
	return netguard.ValidateDialHost(ctx, host, policy(allowPrivate, 0))
}
