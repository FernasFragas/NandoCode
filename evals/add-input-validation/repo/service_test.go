package addinputvalidation

import "testing"

func TestBuildUserPreservesName(t *testing.T) {
	user := BuildUser("nando")
	if user.Name != "nando" {
		t.Fatalf("got %q want %q", user.Name, "nando")
	}
}
