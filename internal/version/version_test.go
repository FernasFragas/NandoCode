package version

import (
	"runtime"
	"strings"
	"testing"
)

func TestInfo(t *testing.T) {
	got := Info()
	if !strings.Contains(got, "nandocodego") {
		t.Fatalf("Info() = %q", got)
	}
	if !strings.Contains(got, Version) {
		t.Fatalf("Info() = %q, want version %q", got, Version)
	}
}

func TestString(t *testing.T) {
	got := String()
	if !strings.Contains(got, "nandocodego") {
		t.Fatalf("String() = %q", got)
	}
	if !strings.Contains(got, Version) {
		t.Fatalf("String() = %q, want version %q", got, Version)
	}
}

func TestCommitPreferredOverCommitSHA(t *testing.T) {
	oldCommit := Commit
	oldCommitSHA := CommitSHA
	t.Cleanup(func() {
		Commit = oldCommit
		CommitSHA = oldCommitSHA
	})

	Commit = "new"
	CommitSHA = "old"
	if got := String(); !strings.Contains(got, "new") {
		t.Fatalf("String() = %q, want preferred Commit", got)
	}
}

func TestFullInfo(t *testing.T) {
	got := FullInfo()
	for _, want := range []string{"nandocodego", "Commit:", "Built:", runtime.GOOS + "/" + runtime.GOARCH} {
		if !strings.Contains(got, want) {
			t.Fatalf("FullInfo() = %q, missing %q", got, want)
		}
	}
}
