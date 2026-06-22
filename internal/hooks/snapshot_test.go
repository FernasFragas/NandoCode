package hooks

import (
	"os"
	"path/filepath"
	"testing"
)

func writeHookConfig(t *testing.T, content string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "hooks.json")
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
	return path
}

func TestLoadSnapshotDisablesProjectHooks(t *testing.T) {
	dir := t.TempDir()
	userPath := filepath.Join(dir, "user-hooks.json")
	projectPath := filepath.Join(dir, "project-hooks.json")
	userConfig := `{"hooks":[{"kind":"command","event":"PreToolUse","matcher":"Bash(rm -rf*)","command":"exit 0"}]}`
	projectConfig := `{"hooks":[{"kind":"command","event":"PreToolUse","matcher":"Bash(git commit*)","command":"exit 0"}]}`
	if err := os.WriteFile(userPath, []byte(userConfig), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(projectPath, []byte(projectConfig), 0o600); err != nil {
		t.Fatal(err)
	}

	snap := LoadSnapshot(LoadOptions{UserPath: userPath, ProjectPath: projectPath})
	if len(snap.Hooks) != 1 {
		t.Fatalf("expected 1 executable hook, got %d", len(snap.Hooks))
	}
	if snap.Hooks[0].Source != SourceUser {
		t.Fatalf("expected user source, got %q", snap.Hooks[0].Source)
	}
	if len(snap.Disabled) != 1 {
		t.Fatalf("expected 1 disabled project hook, got %d", len(snap.Disabled))
	}
	if snap.Disabled[0].Hook.Source != SourceProject {
		t.Fatalf("expected project source, got %q", snap.Disabled[0].Hook.Source)
	}
}

func TestSnapshotIsFrozenAfterLoad(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "hooks.json")
	if err := os.WriteFile(path, []byte(`{"hooks":[{"kind":"command","event":"PreToolUse","command":"exit 0"}]}`), 0o600); err != nil {
		t.Fatal(err)
	}
	snap := LoadSnapshot(LoadOptions{UserPath: path})
	if err := os.WriteFile(path, []byte(`{"hooks":[]}`), 0o600); err != nil {
		t.Fatal(err)
	}
	if len(snap.Hooks) != 1 {
		t.Fatalf("snapshot changed after source edit")
	}
}

func TestLoadSnapshotRejectsInvalidKindAndEvent(t *testing.T) {
	path := writeHookConfig(t, `{"hooks":[
		{"kind":"bogus","event":"PreToolUse","command":"exit 0"},
		{"kind":"command","event":"BogusEvent","command":"exit 0"}
	]}`)
	snap := LoadSnapshot(LoadOptions{UserPath: path})
	if len(snap.Hooks) != 0 {
		t.Fatalf("expected no executable hooks, got %d", len(snap.Hooks))
	}
	if len(snap.Disabled) != 2 {
		t.Fatalf("expected two disabled hooks, got %d", len(snap.Disabled))
	}
}
