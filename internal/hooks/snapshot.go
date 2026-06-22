package hooks

import (
	"encoding/json"
	"fmt"
	"os"
	"strings"

	"github.com/FernasFragas/nandocodego/internal/mcp"
)

func LoadSnapshot(opts LoadOptions) Snapshot {
	var snap Snapshot
	if strings.TrimSpace(opts.UserPath) != "" {
		snap.merge(loadFile(opts.UserPath, SourceUser, true))
	}
	if strings.TrimSpace(opts.ProjectPath) != "" {
		snap.merge(loadFile(opts.ProjectPath, SourceProject, false))
	}
	return snap
}

func (s *Snapshot) merge(other Snapshot) {
	s.Hooks = append(s.Hooks, other.Hooks...)
	s.Disabled = append(s.Disabled, other.Disabled...)
	s.Warnings = append(s.Warnings, other.Warnings...)
}

func loadFile(path string, source Source, executable bool) Snapshot {
	b, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return Snapshot{}
		}
		return Snapshot{Warnings: []string{fmt.Sprintf("%s: %v", path, err)}}
	}
	var cfg ConfigFile
	if err := json.Unmarshal(b, &cfg); err != nil {
		return Snapshot{Warnings: []string{fmt.Sprintf("%s: invalid JSON: %v", path, err)}}
	}
	var snap Snapshot
	for _, h := range cfg.Hooks {
		h.Source = source
		if err := validateHook(h); err != nil {
			snap.Disabled = append(snap.Disabled, DisabledHook{Hook: h, Reason: err.Error()})
			continue
		}
		if !executable {
			snap.Disabled = append(snap.Disabled, DisabledHook{Hook: h, Reason: "project hooks are disabled until workspace trust exists"})
			continue
		}
		if h.Kind == KindHTTP {
			if err := mcp.ValidateHTTPDestination(h.URL, false); err != nil {
				reason := fmt.Sprintf("http hook destination rejected: %v", err)
				snap.Disabled = append(snap.Disabled, DisabledHook{Hook: h, Reason: reason})
				snap.Warnings = append(snap.Warnings, fmt.Sprintf("%s: %s %s %s", path, h.Kind, h.Event, reason))
				continue
			}
		}
		if !h.Kind.Executable() {
			snap.Disabled = append(snap.Disabled, DisabledHook{Hook: h, Reason: fmt.Sprintf("%s hooks are disabled in Phase 9", h.Kind)})
			continue
		}
		h.Enabled = true
		snap.Hooks = append(snap.Hooks, h)
	}
	return snap
}

func validateHook(h Hook) error {
	if !h.Kind.Valid() {
		return fmt.Errorf("invalid hook kind %q", h.Kind)
	}
	if !h.Event.Valid() {
		return fmt.Errorf("invalid hook event %q", h.Event)
	}
	switch h.Kind {
	case KindCommand:
		if strings.TrimSpace(h.Command) == "" {
			return fmt.Errorf("command hook requires command")
		}
	case KindPrompt:
		if strings.TrimSpace(h.Prompt) == "" {
			return fmt.Errorf("prompt hook requires prompt")
		}
	case KindHTTP:
		if strings.TrimSpace(h.URL) == "" {
			return fmt.Errorf("http hook requires url")
		}
	case KindAgent:
		if strings.TrimSpace(h.Prompt) == "" {
			return fmt.Errorf("agent hook requires prompt")
		}
	}
	return nil
}
