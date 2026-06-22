package tools

import (
	"fmt"
	"sort"
)

// Registry stores tools by canonical name and aliases.
type Registry struct {
	tools   map[string]Tool
	aliases map[string]string
}

// NewRegistry creates an empty tool registry.
func NewRegistry() *Registry {
	return &Registry{
		tools:   make(map[string]Tool),
		aliases: make(map[string]string),
	}
}

// Register adds a tool. Duplicate canonical names or aliases are rejected.
func (r *Registry) Register(t Tool) error {
	if r == nil {
		return fmt.Errorf("registry is nil")
	}
	if err := ValidateTool(t); err != nil {
		return err
	}

	name := t.Name()
	if _, exists := r.tools[name]; exists {
		return fmt.Errorf("tool %q already registered", name)
	}
	if existing, exists := r.aliases[name]; exists {
		return fmt.Errorf("tool name %q conflicts with alias for %q", name, existing)
	}

	seenAliases := make(map[string]struct{})
	for _, alias := range t.Aliases() {
		if alias == "" {
			return fmt.Errorf("tool %q has empty alias", name)
		}
		if alias == name {
			return fmt.Errorf("tool %q aliases itself", name)
		}
		if _, duplicate := seenAliases[alias]; duplicate {
			return fmt.Errorf("tool %q has duplicate alias %q", name, alias)
		}
		seenAliases[alias] = struct{}{}
		if _, exists := r.tools[alias]; exists {
			return fmt.Errorf("alias %q conflicts with registered tool", alias)
		}
		if existing, exists := r.aliases[alias]; exists {
			return fmt.Errorf("alias %q already registered for %q", alias, existing)
		}
	}

	r.tools[name] = t
	for alias := range seenAliases {
		r.aliases[alias] = name
	}
	return nil
}

// Lookup returns a tool by canonical name or alias.
func (r *Registry) Lookup(name string) (Tool, bool) {
	if r == nil {
		return nil, false
	}
	if t, ok := r.tools[name]; ok {
		return t, true
	}
	if canonical, ok := r.aliases[name]; ok {
		t, found := r.tools[canonical]
		return t, found
	}
	return nil, false
}

// All returns all tools sorted by canonical name.
func (r *Registry) All() []Tool {
	if r == nil {
		return nil
	}
	out := make([]Tool, 0, len(r.tools))
	for _, t := range r.tools {
		out = append(out, t)
	}
	sort.Slice(out, func(i, j int) bool {
		return out[i].Name() < out[j].Name()
	})
	return out
}
