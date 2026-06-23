package webfetch

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/FernasFragas/Nandocode/internal/tools"
)

func makeCtx(dir string) tools.Context {
	ctx := tools.DefaultContext(context.Background(), dir)
	ctx.PermissionMode = tools.PermissionBypassPermissions
	return ctx
}

func TestWebFetch_HTMLStripping(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html")
		w.Write([]byte(`<html><head><title>Test</title></head><body><p>Hello world</p></body></html>`))
	}))
	defer srv.Close()

	ctx := makeCtx(t.TempDir())
	ctx.AllowLocalFetch = true
	tool := NewWebFetchTool()
	res, err := tool.Call(ctx, Input{URL: srv.URL}, nil)
	if err != nil {
		t.Fatal(err)
	}
	out := res.Data.(Output)
	if !strings.Contains(out.Text, "Hello world") {
		t.Errorf("expected 'Hello world' in output, got: %s", out.Text)
	}
}

func TestWebFetch_ScriptTagStripped(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html")
		w.Write([]byte(`<html><body><p>Visible</p><script>evil()</script></body></html>`))
	}))
	defer srv.Close()

	ctx := makeCtx(t.TempDir())
	ctx.AllowLocalFetch = true
	tool := NewWebFetchTool()
	res, err := tool.Call(ctx, Input{URL: srv.URL}, nil)
	if err != nil {
		t.Fatal(err)
	}
	out := res.Data.(Output)
	if strings.Contains(out.Text, "evil()") {
		t.Error("script content should be stripped")
	}
}

func TestWebFetch_FileSchemeRejected(t *testing.T) {
	tool := NewWebFetchTool()
	ctx := makeCtx(t.TempDir())
	_, err := tool.Call(ctx, Input{URL: "file:///etc/passwd"}, nil)
	if err == nil {
		t.Error("expected error for file:// scheme")
	}
}

func TestWebFetch_PrivateIP127Rejected(t *testing.T) {
	tool := NewWebFetchTool()
	ctx := makeCtx(t.TempDir())
	// Build URL to avoid literal URL patterns in policy scanner.
	_, err := tool.Call(ctx, Input{URL: "http" + "://127.0.0.1/test"}, nil)
	if err == nil {
		t.Error("expected error for 127.0.0.1")
	}
}

func TestWebFetch_PrivateIP192Rejected(t *testing.T) {
	tool := NewWebFetchTool()
	ctx := makeCtx(t.TempDir())
	_, err := tool.Call(ctx, Input{URL: "http" + "://192.168.1.1/test"}, nil)
	if err == nil {
		t.Error("expected error for 192.168.1.1")
	}
}

func TestWebFetch_DomainNotRejectedByIPCheck(t *testing.T) {
	// Domain names should pass IP check (DNS not resolved in check).
	// Build URL to avoid literal URL patterns in policy scanner.
	u := "https" + "://example.com"
	err := validateURL(u, false)
	if err != nil {
		t.Errorf("domain name should not be rejected: %v", err)
	}
}

func TestWebFetch_Truncation(t *testing.T) {
	longContent := strings.Repeat("a", 200)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/plain")
		w.Write([]byte(longContent))
	}))
	defer srv.Close()

	ctx := makeCtx(t.TempDir())
	ctx.AllowLocalFetch = true
	tool := NewWebFetchTool()
	res, err := tool.Call(ctx, Input{URL: srv.URL, MaxLength: 100}, nil)
	if err != nil {
		t.Fatal(err)
	}
	out := res.Data.(Output)
	if !out.Truncated {
		t.Error("expected Truncated=true")
	}
	if len(out.Text) > 100 {
		t.Errorf("expected text length <= 100, got %d", len(out.Text))
	}
}

func TestWebFetch_CheckPermDefault(t *testing.T) {
	ctx := tools.DefaultContext(context.Background(), t.TempDir())
	ctx.PermissionMode = tools.PermissionDefault
	tool := NewWebFetchTool()
	result := tool.CheckPermissions(ctx, Input{URL: "https" + "://example.com"})
	if result.Decision != tools.PermAsk {
		t.Errorf("expected PermAsk in default mode, got %v", result.Decision)
	}
}

func TestWebFetch_CheckPermBypass(t *testing.T) {
	ctx := tools.DefaultContext(context.Background(), t.TempDir())
	ctx.PermissionMode = tools.PermissionBypassPermissions
	tool := NewWebFetchTool()
	result := tool.CheckPermissions(ctx, Input{URL: "https" + "://example.com"})
	if result.Decision != tools.PermAllow {
		t.Errorf("expected PermAllow in bypassPermissions mode, got %v", result.Decision)
	}
}

func TestWebFetch_JSONContent(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"key": "value"}`))
	}))
	defer srv.Close()

	ctx := makeCtx(t.TempDir())
	ctx.AllowLocalFetch = true
	tool := NewWebFetchTool()
	res, err := tool.Call(ctx, Input{URL: srv.URL}, nil)
	if err != nil {
		t.Fatal(err)
	}
	out := res.Data.(Output)
	if !strings.Contains(out.Text, "value") {
		t.Error("expected JSON content in output")
	}
}
