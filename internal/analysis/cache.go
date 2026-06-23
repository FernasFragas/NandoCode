package analysis

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/FernasFragas/Nandocode/internal/paths"
)

type SummaryCacheEntry struct {
	Path      string    `json:"path"`
	ContentID string    `json:"content_id"`
	Summary   string    `json:"summary"`
	UpdatedAt time.Time `json:"updated_at"`
}

func SummaryContentID(content string) string {
	sum := sha256.Sum256([]byte(content))
	return hex.EncodeToString(sum[:])
}

func LoadSummaryFromCache(path, contentID string) (SummaryCacheEntry, bool, error) {
	cachePath := summaryCachePath(path)
	b, err := os.ReadFile(cachePath)
	if err != nil {
		if os.IsNotExist(err) {
			return SummaryCacheEntry{}, false, nil
		}
		return SummaryCacheEntry{}, false, err
	}
	var e SummaryCacheEntry
	if err := json.Unmarshal(b, &e); err != nil {
		return SummaryCacheEntry{}, false, err
	}
	if e.ContentID != contentID || strings.TrimSpace(e.Summary) == "" {
		return SummaryCacheEntry{}, false, nil
	}
	return e, true, nil
}

func SaveSummaryToCache(path, contentID, summary string) error {
	e := SummaryCacheEntry{
		Path:      filepath.ToSlash(strings.TrimSpace(path)),
		ContentID: strings.TrimSpace(contentID),
		Summary:   strings.TrimSpace(summary),
		UpdatedAt: time.Now().UTC(),
	}
	b, err := json.MarshalIndent(e, "", "  ")
	if err != nil {
		return err
	}
	cachePath := summaryCachePath(path)
	if err := os.MkdirAll(filepath.Dir(cachePath), 0o755); err != nil {
		return err
	}
	return os.WriteFile(cachePath, b, 0o644)
}

func summaryCachePath(path string) string {
	sum := sha256.Sum256([]byte(filepath.ToSlash(strings.TrimSpace(path))))
	id := hex.EncodeToString(sum[:])
	return filepath.Join(paths.CacheDir(), "analysis", "summaries", id+".json")
}
