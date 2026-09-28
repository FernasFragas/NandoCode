package addinputvalidation

import "testing"

func TestBuildUserPreservesName(t *testing.T) {
	user, err := BuildUser("nando")
	if err != nil {
		t.Fatal(err)
	}
	if user.Name != "nando" {
		t.Fatalf("got %q want %q", user.Name, "nando")
	}
}

func TestBuildUserRejectsBlankName(t *testing.T) {
	if _, err := BuildUser("   "); err == nil {
		t.Fatal("expected blank name error")
	}
}
