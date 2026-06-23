package memory

import (
	"time"

	"github.com/FernasFragas/Nandocode/internal/llm"
)

// Type is the memory taxonomy type.
type Type string

const (
	TypeUser      Type = "user"
	TypeFeedback  Type = "feedback"
	TypeProject   Type = "project"
	TypeReference Type = "reference"
)

// Entry is metadata for one active memory file.
type Entry struct {
	Filename    string
	Path        string
	Name        string
	Description string
	Type        Type
	UpdatedAt   time.Time
	SizeBytes   int64
}

// LoadedEntry is an Entry with full file body loaded.
type LoadedEntry struct {
	Entry
	Content          string
	StalenessWarning string
}

// Config controls memory behavior.
type Config struct {
	Enabled        bool
	Model          string
	RecallMode     string
	MaxSelected    int
	RecallTimeout  time.Duration
	ExtractTimeout time.Duration
	IndexMaxLines  int
	IndexMaxBytes  int
	NoExtract      bool
}

// DefaultConfig returns conservative defaults for Phase 8.
func DefaultConfig(model string) Config {
	return Config{
		Enabled:        true,
		Model:          model,
		RecallMode:     "fast",
		MaxSelected:    5,
		RecallTimeout:  3 * time.Second,
		ExtractTimeout: 5 * time.Second,
		IndexMaxLines:  200,
		IndexMaxBytes:  25_000,
	}
}

// ScanResult groups valid entries and recoverable scan warnings.
type ScanResult struct {
	Entries   []Entry
	Warnings  []string
	Duration  time.Duration
	FileCount int
}

// Index contains capped MEMORY.md content for prompt injection.
type Index struct {
	Content string
	Capped  bool
	Warning string
}

// Query contains recall input context.
type Query struct {
	LatestUser string
	Messages   []llm.Message
}

// RecallResult contains selected entries and recoverable warnings.
type RecallResult struct {
	Selected []Entry
	Warnings []string
}

// Draft is a pending extracted memory proposal.
type Draft struct {
	Filename string
	Content  string
}

// SectionInput bundles data used to build memory prompt section.
type SectionInput struct {
	MemoryDir string
	Index     Index
	Recalled  []LoadedEntry
}
