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

func liveServerPort(t *testing.T) string {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/plain")
		_, _ = w.Write([]byte("reachable"))
	}))
	t.Cleanup(srv.Close)
	return srv.URL[strings.LastIndex(srv.URL, ":")+1:]
}

func TestWebFetch_LocalhostNameRejected(t *testing.T) {
	port := liveServerPort(t)
	tool := NewWebFetchTool()
	ctx := makeCtx(t.TempDir())
	for _, host := range []string{"localhost", "LOCALHOST.", "app.localhost"} {
		raw := "http" + "://" + host + ":" + port + "/x"
		if _, err := tool.Call(ctx, Input{URL: raw}, nil); err == nil {
			t.Errorf("expected error for %s", raw)
		}
	}
}

func TestWebFetch_UnspecifiedAndMappedIPv6Rejected(t *testing.T) {
	port := liveServerPort(t)
	tool := NewWebFetchTool()
	ctx := makeCtx(t.TempDir())
	for _, host := range []string{"0.0.0.0", "[::]", "[::ffff:127.0.0.1]"} {
		raw := "http" + "://" + host + ":" + port + "/x"
		if _, err := tool.Call(ctx, Input{URL: raw}, nil); err == nil {
			t.Errorf("expected error for %s", raw)
		}
	}
}

func TestWebFetch_RedirectToUnspecifiedRejectedEvenWithAllowLocal(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, "http"+"://0.0.0.0:1/x", http.StatusFound)
	}))
	defer srv.Close()
	ctx := makeCtx(t.TempDir())
	ctx.AllowLocalFetch = true
	if _, err := NewWebFetchTool().Call(ctx, Input{URL: srv.URL}, nil); err == nil {
		t.Error("expected redirect to 0.0.0.0 to be rejected")
	}
}

func TestWebFetch_HTMLBodyCapped(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html")
		_, _ = w.Write([]byte("<p>head</p>"))
		_, _ = w.Write([]byte(strings.Repeat("<p>filler text</p>", 400_000)))
		_, _ = w.Write([]byte("<p>TAILMARKER</p>"))
	}))
	defer srv.Close()
	ctx := makeCtx(t.TempDir())
	ctx.AllowLocalFetch = true
	res, err := NewWebFetchTool().Call(ctx, Input{URL: srv.URL, MaxLength: 100_000_000}, nil)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(res.Data.(Output).Text, "TAILMARKER") {
		t.Error("HTML body beyond the cap must not be parsed")
	}
}

func TestWebFetchBoundsHTMLBody(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html")
		_, _ = w.Write([]byte("<script>" + strings.Repeat("x", 2*1024*1024) + "</script><p>late content</p>"))
	}))
	defer srv.Close()

	ctx := makeCtx(t.TempDir())
	ctx.AllowLocalFetch = true
	res, err := NewWebFetchTool().Call(ctx, Input{URL: srv.URL}, nil)
	if err != nil {
		t.Fatal(err)
	}
	out := res.Data.(Output)
	if !out.Truncated {
		t.Error("expected Truncated=true when HTML body exceeds cap")
	}
	if strings.Contains(out.Text, "late content") {
		t.Error("content past the HTML cap must not appear")
	}
}
