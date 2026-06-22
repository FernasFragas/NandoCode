package cli

import "testing"

func TestRootCommandHasServer(t *testing.T) {
	cmd := NewRootCmd()
	if _, _, err := cmd.Find([]string{"server"}); err != nil {
		t.Fatal(err)
	}
}
