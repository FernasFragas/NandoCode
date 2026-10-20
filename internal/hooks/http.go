package hooks

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/FernasFragas/Nandocode/internal/netguard"
)

// hookPolicy keeps the historical hook rules (loopback reachable, http only to
// loopback, private ranges denied) on top of the shared guard.
func hookPolicy(timeout time.Duration) netguard.Policy {
	return netguard.Policy{AllowLoopback: true, HTTPLoopbackOnly: true, Timeout: timeout}
}

func runHTTPHook(ctx context.Context, h Hook, env Envelope, cfg Config) Result {
	if err := netguard.ValidateURL(h.URL, hookPolicy(0)); err != nil {
		return Result{Warning: fmt.Sprintf("http hook blocked for %q: %v", h.URL, err)}
	}
	method := strings.ToUpper(strings.TrimSpace(h.Method))
	if method == "" {
		method = http.MethodPost
	}
	timeout := h.Timeout(cfg.DefaultTimeout)
	if timeout > 10*time.Second {
		timeout = 10 * time.Second
	}
	body, _ := json.Marshal(env)
	callCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	req, err := http.NewRequestWithContext(callCtx, method, h.URL, bytes.NewReader(body))
	if err != nil {
		return Result{Warning: fmt.Sprintf("http hook request failed: %v", err)}
	}
	req.Header.Set("Content-Type", "application/json")
	for k, v := range h.Env {
		if strings.TrimSpace(k) == "" {
			continue
		}
		req.Header.Set(k, v)
	}
	client := netguard.NewClient(hookPolicy(timeout))
	resp, err := client.Do(req)
	if err != nil {
		return Result{Warning: fmt.Sprintf("http hook call failed: %v", err)}
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 400 {
		return Result{Warning: fmt.Sprintf("http hook status %d", resp.StatusCode)}
	}
	var out struct {
		Decision Decision `json:"decision"`
		Reason   string   `json:"reason"`
		Warning  string   `json:"warning"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		return Result{}
	}
	return Result{Decision: out.Decision, Reason: sanitize(out.Reason), Warning: sanitize(out.Warning)}
}
