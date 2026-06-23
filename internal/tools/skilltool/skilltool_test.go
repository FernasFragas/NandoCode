package skilltool

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/FernasFragas/Nandocode/internal/skills"
	"github.com/FernasFragas/Nandocode/internal/tools"
)

type fakeLoader struct {
	items map[string]skills.SkillFile
	body  map[string]string
}

func (f *fakeLoader) List() []skills.SkillFile {
	out := make([]skills.SkillFile, 0, len(f.items))
	for _, v := range f.items {
		out = append(out, v)
	}
	return out
}
func (f *fakeLoader) Lookup(name string) (skills.SkillFile, bool) {
	v, ok := f.items[name]
	return v, ok
}
func (f *fakeLoader) ReadBody(sf skills.SkillFile) (string, error) { return f.body[sf.Name], nil }

func TestCallKnownSkill(t *testing.T) {
	t.Parallel()
	l := &fakeLoader{
		items: map[string]skills.SkillFile{"a": {Name: "a", Source: skills.SourceUser}},
		body:  map[string]string{"a": "body"},
	}
	tool := New(l)
	res, err := tool.Call(tools.Context{}, Input{Name: "a"}, nil)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(res.Display, "body") {
		t.Fatal("missing body")
	}
}

func TestCallUnknownSkill(t *testing.T) {
	t.Parallel()
	l := &fakeLoader{items: map[string]skills.SkillFile{"a": {Name: "a"}}}
	tool := New(l)
	_, err := tool.Call(tools.Context{}, Input{Name: "b"}, nil)
	if err == nil || !strings.Contains(err.Error(), "Available") {
		t.Fatalf("unexpected err: %v", err)
	}
}

func TestCheckPermissionsAllow(t *testing.T) {
	t.Parallel()
	tool := New(&fakeLoader{})
	if tool.CheckPermissions(tools.Context{}, Input{Name: "a"}).Decision != tools.PermAllow {
		t.Fatal("expected allow")
	}
}

func TestSchemaAndUnmarshal(t *testing.T) {
	t.Parallel()
	tool := New(&fakeLoader{})
	if tool.Name() != "Skill" {
		t.Fatal("bad name")
	}
	_, err := tool.UnmarshalInput(json.RawMessage(`{}`))
	if err == nil {
		t.Fatal("expected error")
	}
}
