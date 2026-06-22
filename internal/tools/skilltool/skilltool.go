package skilltool

import (
	"encoding/json"
	"fmt"
	"sort"
	"strings"

	"github.com/FernasFragas/nandocodego/internal/skills"
	"github.com/FernasFragas/nandocodego/internal/tools"
)

type Loader interface {
	List() []skills.SkillFile
	Lookup(name string) (skills.SkillFile, bool)
	ReadBody(sf skills.SkillFile) (string, error)
}

type Tool struct {
	loader Loader
}

type Input struct {
	Name string `json:"name"`
}

func New(loader Loader) *Tool { return &Tool{loader: loader} }

func (t *Tool) Name() string { return "Skill" }
func (t *Tool) Description() string {
	return "Load a named skill as behavioral context for this session."
}
func (t *Tool) Aliases() []string { return nil }
func (t *Tool) JSONSchema() map[string]any {
	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"name": map[string]any{"type": "string", "description": "The name of the skill to load"},
		},
		"required": []string{"name"},
	}
}
func (t *Tool) UnmarshalInput(raw json.RawMessage) (any, error) {
	var in Input
	if err := json.Unmarshal(raw, &in); err != nil {
		return nil, err
	}
	if strings.TrimSpace(in.Name) == "" {
		return nil, fmt.Errorf("name is required")
	}
	return in, nil
}
func (t *Tool) IsEnabled(ctx tools.Context) bool { return true }
func (t *Tool) IsReadOnly(input any) bool        { return true }
func (t *Tool) IsConcurrencySafe(input any) bool { return false }
func (t *Tool) IsDestructive(input any) bool     { return true }
func (t *Tool) CheckPermissions(ctx tools.Context, input any) tools.PermissionResult {
	return tools.PermissionResult{Decision: tools.PermAllow, UpdatedInput: input}
}
func (t *Tool) Call(ctx tools.Context, input any, progress chan<- tools.ProgressEvent) (tools.Result, error) {
	if t.loader == nil {
		return tools.Result{}, fmt.Errorf("skill loader is not configured")
	}
	in, ok := input.(Input)
	if !ok {
		return tools.Result{}, fmt.Errorf("invalid input")
	}
	sf, found := t.loader.Lookup(in.Name)
	if !found {
		all := t.loader.List()
		names := make([]string, 0, len(all))
		for _, s := range all {
			names = append(names, s.Name)
		}
		sort.Strings(names)
		return tools.Result{}, fmt.Errorf("unknown skill %q. Available: %s", in.Name, strings.Join(names, ", "))
	}
	body, err := t.loader.ReadBody(sf)
	if err != nil {
		return tools.Result{}, err
	}
	display := fmt.Sprintf("Skill loaded: %s\nSource: %s\n\nThe following behavioral context has been adopted for this session:\n\n%s", sf.Name, sf.Source.String(), body)
	return tools.Result{Display: display}, nil
}
func (t *Tool) Render(input any, result tools.Result) tools.RenderHints {
	return tools.RenderHints{Title: "Skill", Summary: "skill context loaded"}
}
