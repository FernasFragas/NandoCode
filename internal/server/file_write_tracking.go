package server

import (
	"crypto/sha256"
	"encoding/hex"
	"path/filepath"

	"github.com/FernasFragas/Nandocode/internal/agent"
	"github.com/FernasFragas/Nandocode/internal/permissions"
	"github.com/FernasFragas/Nandocode/internal/tools"
)

func (s *Session) trackFileWriteStart(evt agent.ToolUseStart) {
	if evt.Name != "FileWrite" && evt.Name != "FileEdit" && evt.Name != "Write" && evt.Name != "Edit" {
		return
	}
	target := permissions.ExtractTarget(evt.Input)
	s.mu.Lock()
	app := s.appState.Clone()
	s.mu.Unlock()
	path, err := tools.ResolvePath(app.ToolContext(s.ctx), target, tools.PathWrite)
	if err != nil {
		return
	}
	s.mu.Lock()
	if s.activeWrites == nil {
		s.activeWrites = make(map[string]string)
	}
	s.activeWrites[evt.ID] = path
	s.mu.Unlock()
}

func (s *Session) trackFileWriteResult(id string) {
	s.mu.Lock()
	delete(s.activeWrites, id)
	s.mu.Unlock()
}

func (s *Session) writePending(path string, ctx tools.Context) bool {
	s.mu.Lock()
	for _, active := range s.activeWrites {
		if sameWritePath(active, path) {
			s.mu.Unlock()
			return true
		}
	}
	s.mu.Unlock()
	s.broker.mu.Lock()
	defer s.broker.mu.Unlock()
	for _, pending := range s.broker.pending {
		if pending.toolName != "FileWrite" && pending.toolName != "FileEdit" && pending.toolName != "Write" && pending.toolName != "Edit" {
			continue
		}
		candidate, err := tools.ResolvePath(ctx, pending.target, tools.PathWrite)
		if err == nil && sameWritePath(candidate, path) {
			return true
		}
	}
	return false
}

func sameWritePath(a, b string) bool {
	if x, err := filepath.EvalSymlinks(a); err == nil {
		a = x
	}
	if x, err := filepath.EvalSymlinks(b); err == nil {
		b = x
	}
	return filepath.Clean(a) == filepath.Clean(b)
}

// clearFileWrites forgets in-flight write tracking. A run that ends (finished,
// cancelled or closed) may never send a ToolUseResult for a started call, and
// a stale entry would answer 423 to saves forever.
func (s *Session) clearFileWrites() {
	s.mu.Lock()
	s.activeWrites = nil
	s.mu.Unlock()
}

func saveHashHex(content string) string {
	h := sha256.Sum256([]byte(content))
	return hex.EncodeToString(h[:])
}

func (s *Session) putSaveHookMemo(key string, m saveHookMemo) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.saveHooks == nil || len(s.saveHooks) > 32 {
		s.saveHooks = map[string]saveHookMemo{}
	}
	s.saveHooks[key] = m
}

func (s *Session) takeSaveHookMemo(key string) (saveHookMemo, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	m, ok := s.saveHooks[key]
	delete(s.saveHooks, key)
	return m, ok
}

func (s *Session) dropSaveHookMemo(key string) {
	s.mu.Lock()
	delete(s.saveHooks, key)
	s.mu.Unlock()
}

// allowSavedPath records the session "always allow" for a saved file in every
// form an agent FileWrite may use for it: the canonical workspace-relative
// path, and the absolute path (resolved and unresolved workspace root). Each
// rule is exact and glob-escaped by permissions.SessionAllowRule.
func (s *Session) allowSavedPath(workDir, canon string) {
	forms := []string{canon}
	if !filepath.IsAbs(canon) {
		forms = append(forms, filepath.Join(workDir, filepath.FromSlash(canon)))
		if root, err := filepath.EvalSymlinks(workDir); err == nil {
			forms = append(forms, filepath.Join(root, filepath.FromSlash(canon)))
		}
	}
	seen := map[string]bool{}
	for _, f := range forms {
		if !seen[f] {
			seen[f] = true
			s.AllowSession("FileWrite", f)
		}
	}
}
