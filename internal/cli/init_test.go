package cli

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestRunInitCreatesConfig(t *testing.T) {
	t.Setenv("NANDOCODEGO_CONFIG_HOME", t.TempDir())
	cmd := NewRootCmd()
	var out bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&out)
	cmd.SetArgs([]string{"init"})
	if err := cmd.ExecuteContext(context.Background()); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(os.Getenv("NANDOCODEGO_CONFIG_HOME"), "config.toml")
	if _, err := os.Stat(path); err != nil {
		t.Fatalf("expected config file: %v", err)
	}
	if !strings.Contains(out.String(), "Created config at") {
		t.Fatalf("unexpected output: %s", out.String())
	}
}

func TestRunInitDoesNotOverwrite(t *testing.T) {
	td := t.TempDir()
	t.Setenv("NANDOCODEGO_CONFIG_HOME", td)
	path := filepath.Join(td, "config.toml")
	if err := os.WriteFile(path, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	cmd := NewRootCmd()
	var out bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&out)
	cmd.SetArgs([]string{"init"})
	if err := cmd.ExecuteContext(context.Background()); err != nil {
		t.Fatal(err)
	}
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(b) != "x" {
		t.Fatal("file should not be overwritten")
	}
}
