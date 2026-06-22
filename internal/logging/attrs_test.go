package logging

import "testing"

func TestSafeStringAttrRedacts(t *testing.T) {
	attr := SafeStringAttr("token", "sk-abc123xyz")
	if attr.Value.String() != "sk-***" {
		t.Fatalf("got %q", attr.Value.String())
	}
}
