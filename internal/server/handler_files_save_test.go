package server

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/FernasFragas/Nandocode/internal/agent"
	"github.com/FernasFragas/Nandocode/internal/hooks"
	"github.com/FernasFragas/Nandocode/internal/permissions"
	"github.com/FernasFragas/Nandocode/internal/tools"
	"github.com/FernasFragas/Nandocode/internal/tools/filewrite"
)

func saveFixture(t *testing.T) (*Server, *Session, string, string) {
	t.Helper()
	s, id, root := filesFixture(t)
	p := filepath.Join(root, "note.txt")
	if err := os.WriteFile(p, []byte("old"), 0o644); err != nil {
		t.Fatal(err)
	}
	sess, _ := s.registry.get(id)
	return s, sess, root, p
}

func saveHash(s string) string { h := sha256.Sum256([]byte(s)); return hex.EncodeToString(h[:]) }

func saveRequest(t *testing.T, s *Server, id, path, base, confirm string) (int, map[string]any) {
	t.Helper()
	request := map[string]string{"content": "new", "base_sha256": base}
	if confirm != "" {
		request["confirm"] = confirm
	}
	b, _ := json.Marshal(request)
	r := httptest.NewRequest(http.MethodPut, "/v1/sessions/"+id+"/files?path="+path, bytes.NewReader(b))
	r.RemoteAddr = "127.0.0.1:1234"
	r.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	s.routes().ServeHTTP(w, r)
	var body map[string]any
	_ = json.Unmarshal(w.Body.Bytes(), &body)
	return w.Code, body
}

func TestSaveConflictReturns409(t *testing.T) {
	s, sess, _, p := saveFixture(t)
	code, _ := saveRequest(t, s, sess.id, "note.txt", saveHash("stale"), "once")
	if code != 409 {
		t.Fatalf("status=%d, want 409", code)
	}
	if b, _ := os.ReadFile(p); string(b) != "old" {
		t.Fatalf("conflict overwrote file: %q", b)
	}
}

func TestSaveBlockedInPlanMode(t *testing.T) {
	s, sess, _, _ := saveFixture(t)
	sess.mu.Lock()
	sess.permMode = permissions.ModePlan
	sess.mu.Unlock()
	if code, _ := saveRequest(t, s, sess.id, "note.txt", saveHash("old"), "once"); code != 403 {
		t.Fatalf("status=%d", code)
	}
}

func TestSavePlanModeOverridesAllowRule(t *testing.T) {
	s, sess, _, p := saveFixture(t)
	sess.mu.Lock()
	sess.permMode = permissions.ModePlan
	sess.permRules.AlwaysAllow = append(sess.permRules.AlwaysAllow, permissions.Rule{Pattern: "FileWrite(note.txt)", Source: permissions.SourceSession})
	sess.mu.Unlock()
	if code, _ := saveRequest(t, s, sess.id, "note.txt", saveHash("old"), ""); code != 403 {
		t.Fatalf("status=%d", code)
	}
	if b, _ := os.ReadFile(p); string(b) != "old" {
		t.Fatalf("plan save overwrote file: %q", b)
	}
}

func TestSaveDenyRuleBlocks(t *testing.T) {
	s, sess, _, _ := saveFixture(t)
	sess.mu.Lock()
	sess.permRules.AlwaysDeny = append(sess.permRules.AlwaysDeny, permissions.Rule{Pattern: "FileWrite(note.txt)", Source: permissions.SourceSession})
	sess.mu.Unlock()
	if code, _ := saveRequest(t, s, sess.id, "note.txt", saveHash("old"), "once"); code != 403 {
		t.Fatalf("status=%d", code)
	}
}

func TestSaveAllowRuleSavesWithoutConfirm(t *testing.T) {
	s, sess, _, p := saveFixture(t)
	sess.mu.Lock()
	sess.permRules.AlwaysAllow = append(sess.permRules.AlwaysAllow, permissions.Rule{Pattern: "FileWrite(note.txt)", Source: permissions.SourceSession})
	sess.mu.Unlock()
	code, body := saveRequest(t, s, sess.id, "note.txt", saveHash("old"), "")
	if code != 200 || body["sha256"] != saveHash("new") {
		t.Fatalf("status=%d body=%v", code, body)
	}
	if b, _ := os.ReadFile(p); string(b) != "new" {
		t.Fatalf("file=%q", b)
	}
}

func TestSaveAlwaysAllowAddsFileWriteRule(t *testing.T) {
	s, sess, _, _ := saveFixture(t)
	code, body := saveRequest(t, s, sess.id, "note.txt", saveHash("old"), "always")
	if code != 200 {
		t.Fatalf("status=%d body=%v", code, body)
	}
	sess.mu.Lock()
	rules, mode, app := sess.permRules, sess.permMode, sess.appState.Clone()
	sess.mu.Unlock()
	res := permissions.Resolve(t.Context(), permissions.Request{Mode: mode, Rules: rules, Tool: filewrite.NewFileWriteTool(), ToolName: "FileWrite", Input: filewrite.Input{Path: "note.txt", Content: "later"}, ToolContext: app.ToolContext(t.Context())})
	if res.Decision != permissions.DecisionAllow {
		t.Fatalf("agent FileWrite decision=%v", res)
	}
}

func TestSavePreToolUseHookCanBlock(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("test hook command uses POSIX shell syntax")
	}
	s, sess, root, p := saveFixture(t)
	s.hookSnapshot = hooks.Snapshot{Hooks: []hooks.Hook{{Kind: hooks.KindCommand, Event: hooks.EventPreToolUse, Matcher: "FileWrite(*)", Command: "echo blocked >&2; exit 2", Enabled: true}}}
	s.hookCfg.WorkingDir = root
	if code, _ := saveRequest(t, s, sess.id, "note.txt", saveHash("old"), "once"); code != 403 {
		t.Fatalf("status=%d", code)
	}
	if b, _ := os.ReadFile(p); string(b) != "old" {
		t.Fatalf("hook denial overwrote file: %q", b)
	}
}

func TestSaveBlockedWhileRunWritesSamePath(t *testing.T) {
	s, sess, _, _ := saveFixture(t)
	sess.handleAgentEvent(agent.ToolUseStart{ID: "write-1", Name: "FileWrite", Input: filewrite.Input{Path: "note.txt"}})
	if code, _ := saveRequest(t, s, sess.id, "note.txt", saveHash("old"), "once"); code != 423 {
		t.Fatalf("status=%d", code)
	}
	sess.handleAgentEvent(agent.ToolUseResult{ID: "write-1"})
	if code, _ := saveRequest(t, s, sess.id, "note.txt", saveHash("old"), "once"); code != 200 {
		t.Fatalf("after write status=%d", code)
	}
}

func TestSaveEmitsFileSavedEvent(t *testing.T) {
	s, sess, _, _ := saveFixture(t)
	code, _ := saveRequest(t, s, sess.id, "note.txt", saveHash("old"), "once")
	if code != 200 {
		t.Fatalf("status=%d", code)
	}
	for _, evt := range sess.Replay("") {
		if evt.Type == "file_saved" {
			if evt.Data["path"] != "note.txt" || evt.Data["sha256"] != saveHash("new") || evt.Data["size"] != 3 || evt.Data["decision_source"] != "user_once" {
				t.Fatalf("event=%v", evt.Data)
			}
			return
		}
	}
	t.Fatal("file_saved event missing")
}

func TestSaveOutsideWorkspaceRejected(t *testing.T) {
	s, sess, root, _ := saveFixture(t)
	outside := t.TempDir()
	if err := os.WriteFile(filepath.Join(outside, "secret.txt"), []byte("old"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(outside, filepath.Join(root, "escape")); err != nil {
		t.Skip(err)
	}
	for _, p := range []string{"..", "escape/secret.txt"} {
		if code, _ := saveRequest(t, s, sess.id, p, saveHash("old"), "once"); code != 403 {
			t.Errorf("path=%q status=%d", p, code)
		}
	}
}

func TestSaveDefaultRequiresConfirmation(t *testing.T) {
	s, sess, _, p := saveFixture(t)
	code, body := saveRequest(t, s, sess.id, "note.txt", saveHash("old"), "")
	if code != 200 || body["needs_confirm"] != true {
		t.Fatalf("status=%d body=%v", code, body)
	}
	if b, _ := os.ReadFile(p); string(b) != "old" {
		t.Fatalf("unconfirmed save wrote %q", b)
	}
}

func TestSaveAcceptEditsAndBypass(t *testing.T) {
	for _, mode := range []permissions.Mode{permissions.ModeAcceptEdits, permissions.ModeBypass} {
		t.Run(string(mode), func(t *testing.T) {
			s, sess, _, _ := saveFixture(t)
			sess.mu.Lock()
			sess.permMode = mode
			sess.mu.Unlock()
			if code, body := saveRequest(t, s, sess.id, "note.txt", saveHash("old"), ""); code != 200 || body["sha256"] != saveHash("new") {
				t.Fatalf("status=%d body=%v", code, body)
			}
		})
	}
}

func TestSavePostToolUseHookNotice(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("test hook command uses POSIX shell syntax")
	}
	s, sess, root, _ := saveFixture(t)
	s.hookSnapshot = hooks.Snapshot{Hooks: []hooks.Hook{{Kind: hooks.KindCommand, Event: hooks.EventPostToolUse, Matcher: "FileWrite(*)", Command: `echo '{"additional_context":"saved by hook"}'`, Enabled: true}}}
	s.hookCfg.WorkingDir = root
	if code, _ := saveRequest(t, s, sess.id, "note.txt", saveHash("old"), "once"); code != 200 {
		t.Fatalf("status=%d", code)
	}
	for _, evt := range sess.Replay("") {
		if evt.Type == "hook_notice" && evt.Data["message"] == "saved by hook" {
			return
		}
	}
	t.Fatal("PostToolUse output was not emitted")
}

func TestSaveLockBlocksConcurrentAgentWrite(t *testing.T) {
	s, sess, _, p := saveFixture(t)
	release, _ := filewrite.LockPath(t.Context(), p)
	if code, _ := saveRequest(t, s, sess.id, "note.txt", saveHash("old"), "once"); code != 423 {
		t.Fatalf("status=%d", code)
	}
	release()
}

func TestSavePendingPermissionBlocksSamePath(t *testing.T) {
	s, sess, _, _ := saveFixture(t)
	sess.broker.mu.Lock()
	sess.broker.pending["test"] = pendingPermission{toolName: "FileWrite", target: "note.txt"}
	sess.broker.mu.Unlock()
	if code, _ := saveRequest(t, s, sess.id, "note.txt", saveHash("old"), "once"); code != 423 {
		t.Fatalf("status=%d", code)
	}
}

func TestSaveHookChangeReturnsConflict(t *testing.T) {
	s, sess, root, p := saveFixture(t)
	s.hookSnapshot = hooks.Snapshot{Hooks: []hooks.Hook{{Kind: hooks.KindCommand, Event: hooks.EventPreToolUse, Matcher: "FileWrite(*)", Command: "printf changed > note.txt", Enabled: true}}}
	s.hookCfg.WorkingDir = root
	if code, _ := saveRequest(t, s, sess.id, "note.txt", saveHash("old"), "once"); code != 409 {
		t.Fatalf("status=%d", code)
	}
	if b, _ := os.ReadFile(p); string(b) != "changed" {
		t.Fatalf("file=%q", b)
	}
}

func TestSaveRejectsMoreTraversalVariants(t *testing.T) {
	s, sess, root, _ := saveFixture(t)
	outside := t.TempDir()
	secret := filepath.Join(outside, "secret.txt")
	if err := os.WriteFile(secret, []byte("old"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(secret, filepath.Join(root, "link.txt")); err != nil {
		t.Skip(err)
	}
	for _, tc := range []struct {
		path string
		want int
	}{
		{secret, 403},
		{"../" + filepath.Base(outside) + "/secret.txt", 403},
		{"link.txt", 403},
		{"sub/../../x", 403},
		{"/etc/passwd", 403},
		{"missing.txt", 403},
		{"..", 403},
	} {
		if code, _ := saveRequest(t, s, sess.id, tc.path, saveHash("old"), "once"); code != tc.want {
			t.Errorf("path=%q status=%d want %d", tc.path, code, tc.want)
		}
	}
	if b, _ := os.ReadFile(secret); string(b) != "old" {
		t.Fatalf("outside file modified: %q", b)
	}
	if _, err := os.Stat(filepath.Join(root, "missing.txt")); err == nil {
		t.Fatal("save created a new file")
	}
}

func TestSaveDenyRuleBlocksNonCanonicalPaths(t *testing.T) {
	s, sess, root, _ := saveFixture(t)
	if err := os.MkdirAll(filepath.Join(root, "secrets"), 0o755); err != nil {
		t.Fatal(err)
	}
	secret := filepath.Join(root, "secrets", "k.txt")
	if err := os.WriteFile(secret, []byte("old"), 0o644); err != nil {
		t.Fatal(err)
	}
	aliasOK := os.Symlink(filepath.Join(root, "secrets"), filepath.Join(root, "alias")) == nil
	sess.mu.Lock()
	sess.permRules.AlwaysDeny = append(sess.permRules.AlwaysDeny, permissions.Rule{Pattern: "FileWrite(secrets/*)"})
	sess.mu.Unlock()
	variants := []string{"secrets/k.txt", "./secrets/k.txt", "x/../secrets/k.txt", "secrets//k.txt", secret, "secrets/./k.txt"}
	if aliasOK {
		variants = append(variants, "alias/k.txt")
	}
	for _, p := range variants {
		for _, confirm := range []string{"", "once", "always"} {
			if code, _ := saveRequest(t, s, sess.id, p, saveHash("old"), confirm); code != 403 {
				t.Errorf("path=%q confirm=%q status=%d want 403", p, confirm, code)
			}
		}
	}
	if b, _ := os.ReadFile(secret); string(b) != "old" {
		t.Fatalf("deny rule bypassed: %q", b)
	}
}

func TestSaveAlwaysRuleMatchesAgentRelativeAndAbsolute(t *testing.T) {
	s, sess, root, _ := saveFixture(t)
	if code, body := saveRequest(t, s, sess.id, "./note.txt", saveHash("old"), "always"); code != 200 {
		t.Fatalf("status=%d body=%v", code, body)
	}
	sess.mu.Lock()
	rules, app := sess.permRules, sess.appState.Clone()
	sess.mu.Unlock()
	for _, p := range []string{"note.txt", filepath.Join(root, "note.txt")} {
		res := permissions.Resolve(t.Context(), permissions.Request{Mode: permissions.ModeDefault, Rules: rules, Tool: filewrite.NewFileWriteTool(), ToolName: "FileWrite", Input: filewrite.Input{Path: p, Content: "later"}, ToolContext: app.ToolContext(t.Context())})
		if res.Decision != permissions.DecisionAllow {
			t.Errorf("agent FileWrite %q decision=%v", p, res)
		}
	}
	for _, evt := range sess.Replay("") {
		if evt.Type == "file_saved" && evt.Data["path"] != "note.txt" {
			t.Fatalf("file_saved path=%v, want canonical note.txt", evt.Data["path"])
		}
	}
}

func hookCounter(t *testing.T, root, extra string) hooks.Hook {
	t.Helper()
	counter := filepath.Join(t.TempDir(), "count")
	return hooks.Hook{Kind: hooks.KindCommand, Event: hooks.EventPreToolUse, Matcher: "FileWrite(*)", Command: "echo x >> '" + counter + "'" + extra, Enabled: true}
}

func TestSaveHookRunsOncePerLogicalSave(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("test hook command uses POSIX shell syntax")
	}
	s, sess, root, _ := saveFixture(t)
	h := hookCounter(t, root, "")
	counter := h.Command[len("echo x >> '") : len(h.Command)-1]
	s.hookSnapshot = hooks.Snapshot{Hooks: []hooks.Hook{h}}
	s.hookCfg.WorkingDir = root
	if code, body := saveRequest(t, s, sess.id, "note.txt", saveHash("old"), ""); code != 200 || body["needs_confirm"] != true {
		t.Fatalf("status=%d body=%v", code, body)
	}
	if code, _ := saveRequest(t, s, sess.id, "note.txt", saveHash("old"), "once"); code != 200 {
		t.Fatalf("status=%d", code)
	}
	b, _ := os.ReadFile(counter)
	if string(b) != "x\n" {
		t.Fatalf("PreToolUse ran %q times, want once", string(b))
	}
}

func TestSaveHookAllowReportedAsRule(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("test hook command uses POSIX shell syntax")
	}
	s, sess, root, _ := saveFixture(t)
	s.hookSnapshot = hooks.Snapshot{Hooks: []hooks.Hook{{Kind: hooks.KindCommand, Event: hooks.EventPreToolUse, Matcher: "FileWrite(*)", Command: `echo '{"decision":"allow"}'`, Enabled: true}}}
	s.hookCfg.WorkingDir = root
	if code, _ := saveRequest(t, s, sess.id, "note.txt", saveHash("old"), ""); code != 200 {
		t.Fatalf("status=%d", code)
	}
	for _, evt := range sess.Replay("") {
		if evt.Type == "file_saved" {
			if evt.Data["decision_source"] != "rule" {
				t.Fatalf("decision_source=%v, want rule", evt.Data["decision_source"])
			}
			return
		}
	}
	t.Fatal("file_saved missing")
}

func TestActiveWritesClearedWhenRunEnds(t *testing.T) {
	for _, end := range []string{"terminal", "stop"} {
		t.Run(end, func(t *testing.T) {
			s, sess, _, _ := saveFixture(t)
			sess.handleAgentEvent(agent.ToolUseStart{ID: "w1", Name: "FileWrite", Input: filewrite.Input{Path: "note.txt"}})
			if code, _ := saveRequest(t, s, sess.id, "note.txt", saveHash("old"), "once"); code != 423 {
				t.Fatalf("status=%d, want 423", code)
			}
			if end == "terminal" {
				sess.handleAgentEvent(agent.Terminal{Reason: agent.TerminalAborted})
			} else {
				sess.stop()
			}
			if code, _ := saveRequest(t, s, sess.id, "note.txt", saveHash("old"), "once"); code != 200 {
				t.Fatalf("after %s status=%d, want 200", end, code)
			}
		})
	}
}

func TestSavePermissionMatchesResolveExceptBrowserRules(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "note.txt"), []byte("old"), 0o644); err != nil {
		t.Fatal(err)
	}
	tctx := tools.DefaultContext(t.Context(), root)
	ruleSets := map[string]permissions.Rules{
		"none":  {},
		"allow": {AlwaysAllow: []permissions.Rule{{Pattern: "FileWrite(note.txt)"}}},
		"ask":   {AlwaysAsk: []permissions.Rule{{Pattern: "FileWrite(note.txt)"}}},
		"deny":  {AlwaysDeny: []permissions.Rule{{Pattern: "FileWrite(note.txt)"}}},
	}
	prompts := map[string]permissions.PromptFunc{
		"nil": nil,
		"allow": func(context.Context, permissions.Prompt) (permissions.Decision, string, error) {
			return permissions.DecisionAllow, "ok", nil
		},
	}
	modes := []permissions.Mode{permissions.ModeBypass, permissions.ModeDontAsk, permissions.ModeAuto, permissions.ModeAcceptEdits, permissions.ModeDefault, permissions.ModePlan, permissions.ModeBubble}
	for _, mode := range modes {
		for rn, rules := range ruleSets {
			for pn, prompt := range prompts {
				req := permissions.Request{Mode: mode, Rules: rules, Tool: filewrite.NewFileWriteTool(), ToolName: "FileWrite", Input: filewrite.Input{Path: "note.txt", Content: "new"}, ToolContext: tctx, Prompt: prompt}
				base := permissions.Resolve(t.Context(), req)
				got := resolveSavePermission(t.Context(), req, nil)
				want := base
				if mode == permissions.ModePlan && base.Decision == permissions.DecisionAllow {
					want.Decision, want.Stage = permissions.DecisionDeny, permissions.StageMode
				}
				if mode == permissions.ModeAcceptEdits && base.Decision == permissions.DecisionAsk && base.Stage == permissions.StageMode {
					want.Decision, want.Stage = permissions.DecisionAllow, permissions.StageMode
				}
				if got.Decision != want.Decision || got.Stage != want.Stage {
					t.Errorf("%s/%s/%s: got %v/%v want %v/%v", mode, rn, pn, got.Decision, got.Stage, want.Decision, want.Stage)
				}
			}
		}
	}
}

func TestSaveAbsoluteRuleBlocksEveryPathForm(t *testing.T) {
	for _, kind := range []string{"deny", "ask"} {
		t.Run(kind, func(t *testing.T) {
			s, sess, root, _ := saveFixture(t)
			if err := os.MkdirAll(filepath.Join(root, "secrets"), 0o755); err != nil {
				t.Fatal(err)
			}
			secret := filepath.Join(root, "secrets", "k.txt")
			if err := os.WriteFile(secret, []byte("old"), 0o644); err != nil {
				t.Fatal(err)
			}
			aliasOK := os.Symlink(filepath.Join(root, "secrets"), filepath.Join(root, "alias")) == nil
			// Rules use "/" separators on every OS; "\\" is the glob escape.
			rule := permissions.Rule{Pattern: "FileWrite(" + filepath.ToSlash(filepath.Join(root, "secrets")) + "/*)"}
			sess.mu.Lock()
			if kind == "deny" {
				sess.permRules.AlwaysDeny = append(sess.permRules.AlwaysDeny, rule)
			} else {
				sess.permRules.AlwaysAsk = append(sess.permRules.AlwaysAsk, rule)
				sess.permMode = permissions.ModeBypass
			}
			sess.mu.Unlock()
			variants := []string{"secrets/k.txt", "./secrets/k.txt", "x/../secrets/k.txt", secret, "secrets//k.txt"}
			if aliasOK {
				variants = append(variants, "alias/k.txt")
			}
			for _, p := range variants {
				code, body := saveRequest(t, s, sess.id, p, saveHash("old"), "")
				if kind == "deny" && code != 403 {
					t.Errorf("deny path=%q status=%d", p, code)
				}
				if kind == "ask" && (code != 200 || body["needs_confirm"] != true) {
					t.Errorf("ask path=%q status=%d body=%v", p, code, body)
				}
			}
			if b, _ := os.ReadFile(secret); string(b) != "old" {
				t.Fatalf("file modified: %q", b)
			}
		})
	}
}
