package mcp

import (
	"fmt"
	"sort"

	"github.com/FernasFragas/Nandocode/internal/tools"
)

type OverlayRegistry struct {
	base     *tools.Registry
	mcp      map[string]tools.Tool
	warnings []string
}

func NewOverlayRegistry(base *tools.Registry, mcpTools []tools.Tool) *OverlayRegistry {
	r := &OverlayRegistry{
		base: base,
		mcp:  map[string]tools.Tool{},
	}
	for _, t := range mcpTools {
		if t == nil {
			continue
		}
		name := t.Name()
		if name == "" {
			continue
		}
		if base != nil {
			if _, exists := base.Lookup(name); exists {
				r.warnings = append(r.warnings, fmt.Sprintf("mcp tool %q collides with built-in tool; built-in wins", name))
				continue
			}
		}
		if _, exists := r.mcp[name]; exists {
			r.warnings = append(r.warnings, fmt.Sprintf("duplicate mcp tool %q ignored", name))
			continue
		}
		r.mcp[name] = t
	}
	return r
}

func (r *OverlayRegistry) Lookup(name string) (tools.Tool, bool) {
	if r == nil {
		return nil, false
	}
	if t, ok := r.mcp[name]; ok {
		return t, true
	}
	if r.base == nil {
		return nil, false
	}
	return r.base.Lookup(name)
}

func (r *OverlayRegistry) List() []tools.Tool {
	if r == nil {
		return nil
	}
	var out []tools.Tool
	if r.base != nil {
		out = append(out, r.base.All()...)
	}
	for _, t := range r.mcp {
		out = append(out, t)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Name() < out[j].Name() })
	return out
}

func (r *OverlayRegistry) Warnings() []string {
	if r == nil {
		return nil
	}
	out := make([]string, len(r.warnings))
	copy(out, r.warnings)
	return out
}
