package server

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"unicode/utf8"

	"github.com/FernasFragas/Nandocode/internal/agent"
	"github.com/FernasFragas/Nandocode/internal/hooks"
	"github.com/FernasFragas/Nandocode/internal/permissions"

	"github.com/FernasFragas/Nandocode/internal/tools"
	"github.com/FernasFragas/Nandocode/internal/tools/dirwalk"
	"github.com/FernasFragas/Nandocode/internal/tools/filewrite"
)

const filesMaxContent = 2 * 1024 * 1024

type saveFilesRequest struct {
	Content    string `json:"content"`
	BaseSHA256 string `json:"base_sha256"`
	Confirm    string `json:"confirm"`
}

// handlePutFiles saves one existing text file after the agent's FileWrite
// permission decision, hook checks, and an in-lock content hash check.
func (s *Server) handlePutFiles(w http.ResponseWriter, r *http.Request, id string) {
	sess, ok := s.registry.get(id)
	if !ok {
		http.NotFound(w, r)
		return
	}
	path := r.URL.Query().Get("path")
	if strings.TrimSpace(path) == "" {
		http.Error(w, "path required", http.StatusBadRequest)
		return
	}
	var input saveFilesRequest
	dec := json.NewDecoder(io.LimitReader(r.Body, filesMaxContent+4096))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&input); err != nil || len(input.Content) > filesMaxContent || !utf8.ValidString(input.Content) || len(input.BaseSHA256) != 64 || (input.Confirm != "" && input.Confirm != "once" && input.Confirm != "always") {
		http.Error(w, "invalid save request", http.StatusBadRequest)
		return
	}
	if _, err := hex.DecodeString(input.BaseSHA256); err != nil {
		http.Error(w, "invalid base_sha256", http.StatusBadRequest)
		return
	}
	sess.mu.Lock()
	app, mode, rules := sess.appState.Clone(), sess.permMode, sess.permRules
	rules.AlwaysAllow = append([]permissions.Rule(nil), rules.AlwaysAllow...)
	rules.AlwaysDeny = append([]permissions.Rule(nil), rules.AlwaysDeny...)
	rules.AlwaysAsk = append([]permissions.Rule(nil), rules.AlwaysAsk...)
	sess.mu.Unlock()
	if strings.TrimSpace(app.ToolSettings.WorkingDir) == "" {
		wd, err := os.Getwd()
		if err != nil {
			http.Error(w, "working directory unavailable", 500)
			return
		}
		app.ToolSettings.WorkingDir = wd
	}
	toolCtx := app.ToolContext(r.Context())
	target, err := tools.ResolvePath(toolCtx, path, tools.PathRead)
	if err != nil {
		writeOpsJSON(w, 403, map[string]any{"reason": "path not allowed"})
		return
	}
	root, rel, ok := openInsideRoots(app.ToolSettings.WorkingDir, app.ToolSettings.AdditionalWorkingDirs, target)
	if !ok {
		writeOpsJSON(w, 403, map[string]any{"reason": "path not allowed"})
		return
	}
	defer root.Close()
	target = filepath.Join(root.Name(), rel)
	if sess.writePending(target, toolCtx) {
		writeOpsJSON(w, 423, map[string]any{"reason": "agent is writing this file"})
		return
	}
	unlock, ok := filewrite.TryLockPath(target)
	if !ok {
		writeOpsJSON(w, 423, map[string]any{"reason": "agent is writing this file"})
		return
	}
	defer unlock()
	if sess.writePending(target, toolCtx) {
		writeOpsJSON(w, 423, map[string]any{"reason": "agent is writing this file"})
		return
	}
	info, err := root.Stat(rel)
	if err != nil || !info.Mode().IsRegular() {
		writeOpsJSON(w, 403, map[string]any{"reason": "not a regular file"})
		return
	}
	prior, err := root.ReadFile(rel)
	if err != nil {
		http.Error(w, "read failed", 500)
		return
	}
	hash := sha256.Sum256(prior)
	if !strings.EqualFold(input.BaseSHA256, hex.EncodeToString(hash[:])) {
		writeOpsJSON(w, 409, map[string]any{"reason": "file changed"})
		return
	}
	if !utf8.Valid(prior) || bytes.IndexByte(prior, 0) >= 0 {
		writeOpsJSON(w, 415, map[string]any{"reason": "binary file"})
		return
	}
	canon := canonicalSavePath(app.ToolSettings.WorkingDir, target)
	toolInput := filewrite.Input{Path: canon, Content: input.Content}
	hookCfg := s.hookCfg
	hookCfg.SessionID, hookCfg.WorkingDir, hookCfg.Model, hookCfg.PermissionMode = id, app.ToolSettings.WorkingDir, app.ActiveModel, mode.String()
	hookRunner := hooks.NewRunner(nil, hooks.NewDispatcher(s.hookSnapshot, sess.llmClient, hookCfg))
	hookIn := agent.Input{Model: app.ActiveModel, PermissionMode: mode}
	req := permissions.Request{Mode: mode, Rules: rules, Tool: filewrite.NewFileWriteTool(), ToolName: "FileWrite", Input: toolInput, ToolContext: toolCtx}
	memoKey := saveMemoKey(canon, input.BaseSHA256, input.Content)
	req.HookDecision = func(ctx context.Context, req permissions.Request) (permissions.Result, bool) {
		if input.Confirm != "" {
			if memo, ok := sess.takeSaveHookMemo(memoKey); ok {
				return memo.result, memo.decided
			}
		}
		res, decided, notices := hookRunner.CheckToolUse(ctx, req, hookIn)
		for _, notice := range notices {
			if n, ok := notice.(agent.HookNotice); ok {
				sess.Emit("hook_notice", map[string]any{"message": n.Message})
			}
		}
		sess.putSaveHookMemo(memoKey, saveHookMemo{result: res, decided: decided})
		return res, decided
	}
	decision := resolveSavePermission(r.Context(), req, s.agentCfg.PermissionObserver, saveTargetForms(app.ToolSettings.WorkingDir, canon, target, path)...)
	if decision.Decision == permissions.DecisionDeny {
		sess.dropSaveHookMemo(memoKey)
		writeOpsJSON(w, 403, map[string]any{"reason": decision.Reason})
		return
	}
	if decision.Decision == permissions.DecisionAsk && input.Confirm == "" {
		writeOpsJSON(w, 200, map[string]any{"needs_confirm": true})
		return
	}
	sess.dropSaveHookMemo(memoKey)
	source := "mode"
	// A hook allow is configured policy like a rule, and the K2 enum has no
	// "hook" value, so it is reported as "rule".
	if decision.Stage == permissions.StageRule || decision.Stage == permissions.StageHook {
		source = "rule"
	}
	if decision.Decision == permissions.DecisionAsk {
		if input.Confirm == "always" {
			source = "user_always"
		} else {
			source = "user_once"
		}
	}
	// Check containment again immediately before the atomic rename. The
	// resolved target is used so a symlink at the requested path is not followed.
	if current, ok := resolveInsideRoots(app.ToolSettings.WorkingDir, app.ToolSettings.AdditionalWorkingDirs, target); !ok || current != target {
		writeOpsJSON(w, 403, map[string]any{"reason": "path not allowed"})
		return
	}
	// Hooks may have changed the file while permission was being checked.
	latest, err := root.ReadFile(rel)
	if err != nil {
		http.Error(w, "read failed", 500)
		return
	}
	latestHash := sha256.Sum256(latest)
	if latestHash != hash {
		writeOpsJSON(w, 409, map[string]any{"reason": "file changed"})
		return
	}
	if err := filewrite.AtomicWrite(target, []byte(input.Content), info.Mode().Perm()); err != nil {
		http.Error(w, "write failed", 500)
		return
	}
	if source == "user_always" {
		sess.allowSavedPath(app.ToolSettings.WorkingDir, canon)
	}
	newHash := sha256.Sum256([]byte(input.Content))
	hashString := hex.EncodeToString(newHash[:])
	post := hookRunner.PostToolUse(r.Context(), hookIn, agent.ToolHookEvent{ToolName: "FileWrite", Input: toolInput, Target: canon, ToolContext: toolCtx, PermissionMode: mode, Model: app.ActiveModel, Result: tools.Result{Display: "file saved"}})
	if post.Warning != "" || post.AdditionalContext != "" || post.Reason != "" {
		sess.Emit("hook_notice", map[string]any{"message": strings.TrimSpace(post.Warning + " " + post.AdditionalContext + " " + post.Reason)})
	}
	emitFileSaved(sess, canon, len(input.Content), hashString, source)
	writeOpsJSON(w, 200, map[string]any{"sha256": hashString})
}

type filesEntry struct {
	Name string `json:"name"`
	Kind string `json:"kind"`
	Size int64  `json:"size"`
}

// handleGetFiles serves one workspace directory or file per request. ResolvePath
// checks the workspace boundary after following symlinks.
func (s *Server) handleGetFiles(w http.ResponseWriter, r *http.Request, id string) {
	sess, ok := s.registry.get(id)
	if !ok {
		http.NotFound(w, r)
		return
	}
	sess.mu.Lock()
	app := sess.appState.Clone()
	sess.mu.Unlock()
	if strings.TrimSpace(app.ToolSettings.WorkingDir) == "" {
		wd, err := os.Getwd()
		if err != nil {
			http.Error(w, "unable to determine working directory", http.StatusInternalServerError)
			return
		}
		app.ToolSettings.WorkingDir = wd
	}
	path := r.URL.Query().Get("path")
	if path == "" {
		path = "."
	}
	target, err := tools.ResolvePath(app.ToolContext(r.Context()), path, tools.PathRead)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			http.Error(w, "path not found", http.StatusNotFound)
		} else {
			http.Error(w, "path not allowed", http.StatusForbidden)
		}
		return
	}
	// Re-resolve and re-check the final path, then read through an os.Root, so
	// a symlink swapped after ResolvePath cannot redirect the read.
	root, rel, ok := openInsideRoots(app.ToolSettings.WorkingDir, app.ToolSettings.AdditionalWorkingDirs, target)
	if !ok {
		http.Error(w, "path not allowed", http.StatusForbidden)
		return
	}
	defer root.Close()
	target = filepath.Join(root.Name(), rel)
	info, err := root.Stat(rel)
	if err != nil {
		http.Error(w, "path not found", http.StatusNotFound)
		return
	}
	if info.IsDir() {
		walked, _, err := dirwalk.Walk(r.Context(), target, dirwalk.Options{
			Excludes: append(dirwalk.DefaultExcludes(), ".DS_Store", "__pycache__", "bin"),
			MaxFiles: 500, MaxDepth: 1, Source: dirwalk.SourceFilesystem,
		})
		if err != nil {
			http.Error(w, "walk error", http.StatusInternalServerError)
			return
		}
		entries := []filesEntry{}
		for _, e := range walked {
			if e.Skipped || strings.Contains(e.RelPath, string(filepath.Separator)) {
				continue
			}
			kind := "file"
			if e.IsDir {
				kind = "dir"
			}
			entries = append(entries, filesEntry{Name: filepath.Base(e.RelPath), Kind: kind, Size: e.SizeBytes})
		}
		writeFilesJSON(w, map[string]any{"kind": "dir", "entries": entries})
		return
	}
	if !info.Mode().IsRegular() {
		http.Error(w, "not a regular file", http.StatusUnsupportedMediaType)
		return
	}
	if info.Size() > filesMaxContent {
		writeFilesJSON(w, map[string]any{"kind": "file", "too_large": true, "size": info.Size()})
		return
	}
	f, err := root.Open(rel)
	if err != nil {
		http.Error(w, "path not found", http.StatusNotFound)
		return
	}
	defer f.Close()
	content, err := io.ReadAll(io.LimitReader(f, filesMaxContent+1))
	if err != nil {
		http.Error(w, "read error", http.StatusInternalServerError)
		return
	}
	if len(content) > filesMaxContent {
		writeFilesJSON(w, map[string]any{"kind": "file", "too_large": true, "size": len(content)})
		return
	}
	probe := content
	if len(probe) > 8192 {
		probe = probe[:8192]
	}
	if bytes.IndexByte(probe, 0) >= 0 {
		http.Error(w, "binary file", http.StatusUnsupportedMediaType)
		return
	}
	if !utf8.Valid(content) {
		http.Error(w, "file is not valid UTF-8", http.StatusUnsupportedMediaType)
		return
	}
	hash := sha256.Sum256(content)
	writeFilesJSON(w, map[string]any{"kind": "file", "content": string(content), "sha256": hex.EncodeToString(hash[:]), "size": len(content), "lang": filesLanguage(target)})
}

func writeFilesJSON(w http.ResponseWriter, body any) {
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(body)
}

func filesLanguage(path string) string {
	switch strings.ToLower(filepath.Ext(path)) {
	case ".go":
		return "go"
	case ".js", ".mjs", ".cjs":
		return "javascript"
	case ".json":
		return "json"
	case ".html", ".htm":
		return "html"
	case ".css":
		return "css"
	case ".yaml", ".yml":
		return "yaml"
	case ".toml":
		return "toml"
	case ".sh", ".bash", ".zsh":
		return "shell"
	case ".md", ".markdown":
		return "markdown"
	default:
		return "text"
	}
}

// withinAnyRoot reports whether the already-resolved path lies inside one of
// the already-resolved roots.
func withinAnyRoot(resolved string, roots []string) bool {
	_, _, ok := containingRoot(resolved, roots)
	return ok
}

// containingRoot returns the first already-resolved root holding the
// already-resolved path, and the path relative to it.
func containingRoot(resolved string, roots []string) (string, string, bool) {
	for _, root := range roots {
		rel, err := filepath.Rel(root, resolved)
		if err == nil && rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
			return root, rel, true
		}
	}
	return "", "", false
}

func resolveInsideRoots(workDir string, extra []string, target string) (string, bool) {
	resolved, _, _, ok := locateInsideRoots(workDir, extra, target)
	return resolved, ok
}

// openInsideRoots is resolveInsideRoots plus an os.Root for the containing
// workspace root, so reads through it cannot leave that root even if a
// symlink is swapped after the check. The caller closes the root.
func openInsideRoots(workDir string, extra []string, target string) (*os.Root, string, bool) {
	_, root, rel, ok := locateInsideRoots(workDir, extra, target)
	if !ok {
		return nil, "", false
	}
	r, err := os.OpenRoot(root)
	if err != nil {
		return nil, "", false
	}
	return r, rel, true
}

func locateInsideRoots(workDir string, extra []string, target string) (resolved, root, rel string, ok bool) {
	resolved, err := filepath.EvalSymlinks(target)
	if err != nil {
		return "", "", "", false
	}
	var roots []string
	for _, r := range append([]string{workDir}, extra...) {
		if strings.TrimSpace(r) == "" {
			continue
		}
		if abs, err := filepath.Abs(r); err == nil {
			if ev, err := filepath.EvalSymlinks(abs); err == nil {
				roots = append(roots, filepath.Clean(ev))
			}
		}
	}
	root, rel, ok = containingRoot(resolved, roots)
	if !ok {
		return "", "", "", false
	}
	return resolved, root, rel, true
}
