package mcp

import "testing"

func TestBuildProcessEnvIncludesOverrides(t *testing.T) {
	t.Parallel()
	env := buildProcessEnv(map[string]string{
		"MCP_ONE": "1",
		"MCP_TWO": "2",
	})
	joined := map[string]bool{}
	for _, kv := range env {
		joined[kv] = true
	}
	if !joined["MCP_ONE=1"] || !joined["MCP_TWO=2"] {
		t.Fatalf("expected overrides in env: %#v", env)
	}
}
