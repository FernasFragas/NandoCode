// Package webfetch implements the WebFetch tool.
package webfetch

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"strings"
	"time"

	"golang.org/x/net/html"

	"github.com/FernasFragas/nandocodego/internal/tools"
)

const (
	defaultMaxLength = 50_000
	fetchTimeout     = 30 * time.Second
	maxRedirects     = 3
	userAgent        = "nandocodego/0.1"
)

// Input is the WebFetch tool input.
type Input struct {
	URL       string `json:"url"`
	MaxLength int    `json:"max_length,omitempty"`
}

// Output is the WebFetch tool output.
type Output struct {
	URL         string `json:"url"`
	StatusCode  int    `json:"status_code"`
	ContentType string `json:"content_type"`
	Text        string `json:"text"`
	Truncated   bool   `json:"truncated"`
}

// NewWebFetchTool creates a WebFetch tool.
func NewWebFetchTool() tools.Tool {
	return tools.BuildTool(tools.Spec{
		Name:        "WebFetch",
		Description: "Fetch a URL and return its text content.",
		Schema:      schema(),
		Unmarshal:   unmarshalInput,
		IsReadOnlyFunc: func(input any) bool {
			return true
		},
		IsConcurrentFunc: func(input any) bool {
			return true
		},
		IsDestructiveFunc: func(input any) bool {
			return false
		},
		CheckPermFunc: checkPermissions,
		CallFunc:      call,
		RenderFunc: func(input any, result tools.Result) tools.RenderHints {
			in, _ := input.(Input)
			return tools.RenderHints{Title: "WebFetch", Summary: in.URL}
		},
	})
}

func schema() map[string]any {
	return tools.ObjectSchema(map[string]any{
		"url":        tools.StringProperty("The URL to fetch (http:// or https:// only)."),
		"max_length": tools.IntegerProperty("Maximum characters to return. Default 50000.", 1),
	}, []string{"url"})
}

func unmarshalInput(raw json.RawMessage) (any, error) {
	var input Input
	if err := json.Unmarshal(raw, &input); err != nil {
		return nil, err
	}
	if strings.TrimSpace(input.URL) == "" {
		return nil, errors.New("url is required")
	}
	return input, nil
}

func checkPermissions(ctx tools.Context, input any) tools.PermissionResult {
	if _, ok := input.(Input); !ok {
		return tools.PermissionResult{Decision: tools.PermDeny, Reason: "invalid WebFetch input"}
	}
	switch ctx.PermissionMode {
	case tools.PermissionBypassPermissions:
		return tools.PermissionResult{Decision: tools.PermAllow, UpdatedInput: input}
	case tools.PermissionPlan, tools.PermissionDontAsk:
		return tools.PermissionResult{Decision: tools.PermDeny, Reason: "WebFetch makes external HTTP requests"}
	default:
		return tools.PermissionResult{Decision: tools.PermAsk, Reason: "WebFetch makes an external HTTP request", UpdatedInput: input}
	}
}

func call(ctx tools.Context, input any, _ chan<- tools.ProgressEvent) (tools.Result, error) {
	in, ok := input.(Input)
	if !ok {
		return tools.Result{}, errors.New("invalid WebFetch input")
	}
	if err := validateURL(in.URL, ctx.AllowLocalFetch); err != nil {
		return tools.Result{}, err
	}
	maxLen := in.MaxLength
	if maxLen <= 0 {
		maxLen = defaultMaxLength
	}
	out, err := fetchURL(ctx.EffectiveContext(), in.URL, maxLen)
	if err != nil {
		return tools.Result{}, err
	}
	return tools.Result{Data: out, Display: buildDisplay(out)}, nil
}

func validateURL(rawURL string, allowLocal bool) error {
	u, err := url.Parse(rawURL)
	if err != nil {
		return fmt.Errorf("invalid URL: %w", err)
	}
	if u.Scheme != "http" && u.Scheme != "https" {
		return fmt.Errorf("unsupported URL scheme %q; only http:// and https:// are allowed", u.Scheme)
	}
	if !allowLocal {
		host := u.Hostname()
		if ip := net.ParseIP(host); ip != nil {
			if isPrivateIP(ip) {
				return fmt.Errorf("URL host %q is a private/loopback address; use AllowLocalFetch to permit", host)
			}
		}
	}
	return nil
}

func isPrivateIP(ip net.IP) bool {
	privateRanges := []string{
		"127.0.0.0/8",
		"10.0.0.0/8",
		"172.16.0.0/12",
		"192.168.0.0/16",
		"::1/128",
		"fc00::/7",
	}
	for _, cidr := range privateRanges {
		_, network, err := net.ParseCIDR(cidr)
		if err != nil {
			continue
		}
		if network.Contains(ip) {
			return true
		}
	}
	return false
}

func fetchURL(ctx context.Context, rawURL string, maxLen int) (Output, error) {
	redirectCount := 0
	client := &http.Client{
		Timeout: fetchTimeout,
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			redirectCount++
			if redirectCount > maxRedirects {
				return fmt.Errorf("too many redirects (max %d)", maxRedirects)
			}
			return nil
		},
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, rawURL, nil)
	if err != nil {
		return Output{}, fmt.Errorf("failed to create request: %w", err)
	}
	req.Header.Set("User-Agent", userAgent)
	req.Header.Set("Accept", "text/html,application/json,text/plain,*/*")

	resp, err := client.Do(req)
	if err != nil {
		return Output{}, fmt.Errorf("fetch failed: %w", err)
	}
	defer resp.Body.Close()

	ct := resp.Header.Get("Content-Type")
	baseCT := strings.ToLower(strings.SplitN(ct, ";", 2)[0])
	baseCT = strings.TrimSpace(baseCT)

	var text string
	switch {
	case strings.Contains(baseCT, "text/html"):
		text = stripHTML(resp.Body)
	case strings.Contains(baseCT, "application/json"), strings.Contains(baseCT, "text/plain"), strings.Contains(baseCT, "text/"):
		raw, readErr := io.ReadAll(io.LimitReader(resp.Body, int64(maxLen*4)))
		if readErr != nil {
			return Output{}, fmt.Errorf("failed to read response body: %w", readErr)
		}
		text = string(raw)
	default:
		return Output{}, fmt.Errorf("unsupported content type %q", baseCT)
	}

	truncated := false
	if len(text) > maxLen {
		text = text[:maxLen]
		truncated = true
	}

	return Output{
		URL:         rawURL,
		StatusCode:  resp.StatusCode,
		ContentType: ct,
		Text:        text,
		Truncated:   truncated,
	}, nil
}

// stripHTML extracts readable text from HTML using the x/net/html tokenizer.
// It skips <script>, <style>, and <head> content and inserts paragraph breaks.
func stripHTML(r io.Reader) string {
	z := html.NewTokenizer(r)
	var b strings.Builder
	var skipDepth int
	var inSkip bool
	skipTags := map[string]bool{"script": true, "style": true, "head": true}
	breakTags := map[string]bool{"p": true, "div": true, "li": true, "h1": true, "h2": true, "h3": true, "h4": true, "h5": true, "h6": true, "br": true}

	for {
		tt := z.Next()
		if tt == html.ErrorToken {
			break
		}
		switch tt {
		case html.StartTagToken, html.SelfClosingTagToken:
			name, _ := z.TagName()
			tag := string(name)
			if skipTags[tag] {
				if tt == html.StartTagToken {
					inSkip = true
					skipDepth++
				}
			}
			if breakTags[tag] && !inSkip {
				if b.Len() > 0 {
					last := b.String()
					if !strings.HasSuffix(last, "\n\n") {
						b.WriteString("\n\n")
					}
				}
			}
		case html.EndTagToken:
			name, _ := z.TagName()
			tag := string(name)
			if skipTags[tag] && inSkip {
				skipDepth--
				if skipDepth <= 0 {
					skipDepth = 0
					inSkip = false
				}
			}
			if breakTags[tag] && !inSkip {
				if !strings.HasSuffix(b.String(), "\n\n") {
					b.WriteString("\n\n")
				}
			}
		case html.TextToken:
			if inSkip {
				continue
			}
			text := string(z.Text())
			// Collapse whitespace
			words := strings.Fields(text)
			if len(words) > 0 {
				if b.Len() > 0 && !strings.HasSuffix(b.String(), "\n") && !strings.HasSuffix(b.String(), " ") {
					b.WriteString(" ")
				}
				b.WriteString(strings.Join(words, " "))
			}
		}
	}

	// Collapse excessive blank lines
	result := b.String()
	for strings.Contains(result, "\n\n\n") {
		result = strings.ReplaceAll(result, "\n\n\n", "\n\n")
	}
	return strings.TrimSpace(result)
}

func buildDisplay(out Output) string {
	var b strings.Builder
	fmt.Fprintf(&b, "URL: %s\nStatus: %d\nContent-Type: %s\n\n", out.URL, out.StatusCode, out.ContentType)
	b.WriteString(out.Text)
	if out.Truncated {
		fmt.Fprintf(&b, "\n[truncated at %d chars]\n", defaultMaxLength)
	}
	return b.String()
}
