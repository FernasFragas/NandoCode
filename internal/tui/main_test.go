package tui

import (
	"os"
	"testing"
)

// TestMain isolates every test in this package from the real user state and
// data homes, which several app tests would otherwise write into.
func TestMain(m *testing.M) {
	dir, err := os.MkdirTemp("", "nandocode-tui-test-")
	if err != nil {
		panic(err)
	}
	os.Setenv("NANDOCODEGO_STATE_HOME", dir)
	os.Setenv("NANDOCODEGO_DATA_HOME", dir)
	code := m.Run()
	os.RemoveAll(dir)
	os.Exit(code)
}
