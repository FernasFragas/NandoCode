package analysis

import "testing"

func TestSummaryCacheSaveLoad(t *testing.T) {
	t.Setenv("NANDOCODEGO_CACHE_HOME", t.TempDir())
	path := "internal/tui/app.go"
	contentID := SummaryContentID("package tui")
	if err := SaveSummaryToCache(path, contentID, "summary text"); err != nil {
		t.Fatalf("save cache: %v", err)
	}
	entry, ok, err := LoadSummaryFromCache(path, contentID)
	if err != nil {
		t.Fatalf("load cache: %v", err)
	}
	if !ok {
		t.Fatal("expected cache hit")
	}
	if entry.Summary != "summary text" {
		t.Fatalf("summary mismatch: %q", entry.Summary)
	}
}

func TestSummaryCacheMissOnContentID(t *testing.T) {
	t.Setenv("NANDOCODEGO_CACHE_HOME", t.TempDir())
	path := "internal/tui/app.go"
	if err := SaveSummaryToCache(path, SummaryContentID("a"), "summary text"); err != nil {
		t.Fatalf("save cache: %v", err)
	}
	_, ok, err := LoadSummaryFromCache(path, SummaryContentID("b"))
	if err != nil {
		t.Fatalf("load cache: %v", err)
	}
	if ok {
		t.Fatal("expected cache miss")
	}
}
