package semantic

import (
	"context"
	"errors"
	"time"
)

const (
	// SchemaVersion is the manifest schema version for local semantic indexes.
	SchemaVersion = 1
)

var (
	ErrDisabled           = errors.New("semantic index disabled")
	ErrIndexMissing       = errors.New("semantic index missing")
	ErrIndexStale         = errors.New("semantic index stale")
	ErrModelMissing       = errors.New("semantic embedding model missing")
	ErrDeadline           = errors.New("semantic retrieval deadline exceeded")
	ErrDimensionsMismatch = errors.New("semantic embed dimensions mismatch")
	ErrSchemaMismatch     = errors.New("semantic index schema mismatch")
	ErrCorruptIndex       = errors.New("semantic index corrupt")
)

type RecordKind string

const (
	RecordKindFile       RecordKind = "file"
	RecordKindFolder     RecordKind = "folder"
	RecordKindSymbol     RecordKind = "symbol"
	RecordKindDocSection RecordKind = "doc_section"
	RecordKindChunk      RecordKind = "chunk"
)

type Stage string

const (
	StageScanStart        Stage = "scan_start"
	StageScanProgress     Stage = "scan_progress"
	StageExtractProgress  Stage = "extract_progress"
	StageEmbedProgress    Stage = "embed_progress"
	StageWriteStart       Stage = "write_start"
	StageWriteDone        Stage = "write_done"
	StageRetrieveStart    Stage = "retrieve_start"
	StageRetrieveQueryEmb Stage = "retrieve_query_embed"
	StageRetrieveSearch   Stage = "retrieve_search"
	StageRetrieveRender   Stage = "retrieve_render"
	StageRetrieveDone     Stage = "retrieve_done"
)

type Config struct {
	Enabled               bool          `koanf:"enabled"`
	Mode                  string        `koanf:"mode"`
	AutoBuild             bool          `koanf:"auto_build"`
	Model                 string        `koanf:"model"`
	Dimensions            int           `koanf:"dimensions"`
	MaxChunkTokens        int           `koanf:"max_chunk_tokens"`
	ChunkOverlapTokens    int           `koanf:"chunk_overlap_tokens"`
	MaxFileBytes          int64         `koanf:"max_file_bytes"`
	MaxRecords            int           `koanf:"max_records"`
	BatchSize             int           `koanf:"batch_size"`
	TopKRecords           int           `koanf:"top_k_records"`
	TopKFiles             int           `koanf:"top_k_files"`
	MaxContextBytes       int           `koanf:"max_context_bytes"`
	LightTopKRecords      int           `koanf:"light_top_k_records"`
	LightTopKFiles        int           `koanf:"light_top_k_files"`
	LightMaxContextBytes  int           `koanf:"light_max_context_bytes"`
	LightDeadlineMS       int           `koanf:"light_deadline_ms"`
	FullTopKRecords       int           `koanf:"full_top_k_records"`
	FullTopKFiles         int           `koanf:"full_top_k_files"`
	FullMaxContextBytes   int           `koanf:"full_max_context_bytes"`
	FullDeadlineMS        int           `koanf:"full_deadline_ms"`
	DeepTopKRecords       int           `koanf:"deep_top_k_records"`
	DeepTopKFiles         int           `koanf:"deep_top_k_files"`
	DeepMaxContextBytes   int           `koanf:"deep_max_context_bytes"`
	DeepDeadlineMS        int           `koanf:"deep_deadline_ms"`
	HybridLexicalWeight   float64       `koanf:"hybrid_lexical_weight"`
	FrecencyWeight        float64       `koanf:"frecency_weight"`
	PromptRefreshMaxFiles int           `koanf:"prompt_refresh_max_files"`
	PromptRefreshTimeout  time.Duration `koanf:"-"`
	KeepAlive             string        `koanf:"keep_alive"`
	QueryKeepAlive        string        `koanf:"query_keep_alive"`
	BuildKeepAlive        string        `koanf:"build_keep_alive"`
	StorePreviews         bool          `koanf:"store_previews"`
}

type Event struct {
	Stage   Stage
	Message string
	Root    string
	Path    string
	// FilesTotal is the total number of non-directory filesystem entries discovered for the scan.
	// A value of 0 means the total is unknown.
	FilesTotal   int
	FilesSeen    int
	FilesDone    int
	FilesIndexed int
	FilesSkipped int
	RecordsDone  int
	RecordsTotal int
	BatchesDone  int
	TotalBatches int
	Duration     time.Duration
}

type EventSink interface {
	Publish(Event)
}

type Service interface {
	Status(ctx context.Context, root string) (Status, error)
	Build(ctx context.Context, req BuildRequest) (BuildReport, error)
	Refresh(ctx context.Context, req RefreshRequest) (BuildReport, error)
	Clear(ctx context.Context, root string) error
	Retrieve(ctx context.Context, req RetrieveRequest) (RetrieveResult, error)
}

type EmbedRequest struct {
	Model      string
	Input      []string
	Dimensions int
	Truncate   *bool
	KeepAlive  string
}

type EmbedResult struct {
	Vectors    [][]float32
	Dimensions int
}

type Embedder interface {
	Embed(ctx context.Context, req EmbedRequest) (EmbedResult, error)
}

type Store interface {
	LoadManifest(ctx context.Context, root string) (Manifest, error)
	LoadRecords(ctx context.Context, manifest Manifest) ([]Record, error)
	LoadVectors(ctx context.Context, manifest Manifest) (VectorSet, error)
	Replace(ctx context.Context, manifest Manifest, records []Record, vectors VectorSet) error
	Clear(ctx context.Context, root string) error
}

type Manifest struct {
	SchemaVersion int       `json:"schema_version"`
	WorkspaceRoot string    `json:"workspace_root"`
	WorkspaceID   string    `json:"workspace_id"`
	Model         string    `json:"model"`
	Dimensions    int       `json:"dimensions"`
	RecordCount   int       `json:"record_count"`
	FileCount     int       `json:"file_count"`
	CreatedAt     time.Time `json:"created_at"`
	UpdatedAt     time.Time `json:"updated_at"`
	StorePreviews bool      `json:"store_previews"`
}

type Record struct {
	ID          string     `json:"id"`
	Kind        RecordKind `json:"kind"`
	Path        string     `json:"path"`
	Language    string     `json:"language,omitempty"`
	Name        string     `json:"name,omitempty"`
	Parent      string     `json:"parent,omitempty"`
	StartLine   int        `json:"start_line,omitempty"`
	EndLine     int        `json:"end_line,omitempty"`
	ContentHash string     `json:"content_hash,omitempty"`
	TextHash    string     `json:"text_hash,omitempty"`
	TextPreview string     `json:"text_preview,omitempty"`
	EmbedText   string     `json:"embed_text,omitempty"`
	EstTokens   int        `json:"est_tokens,omitempty"`
	Generated   bool       `json:"generated,omitempty"`
	Skipped     bool       `json:"skipped,omitempty"`
	SkipReason  string     `json:"skip_reason,omitempty"`
}

type VectorSet struct {
	Dimensions int
	Vectors    [][]float32
}

type SkippedFile struct {
	Path   string `json:"path"`
	Reason string `json:"reason"`
}

type Status struct {
	Root             string
	Exists           bool
	Compatible       bool
	Enabled          bool
	Model            string
	Dimensions       int
	SchemaVersion    int
	FileCount        int
	RecordCount      int
	VectorCount      int
	StaleFileCount   int
	SkippedFileCount int
	UpdatedAt        time.Time
	IndexPath        string
	Warnings         []string
}

type BuildRequest struct {
	Root      string
	Config    Config
	Force     bool
	EventSink EventSink
}

type RefreshRequest struct {
	Root      string
	Config    Config
	MaxFiles  int
	Timeout   time.Duration
	EventSink EventSink
}

type BuildReport struct {
	Root           string
	Model          string
	Dimensions     int
	FilesSeen      int
	FilesIndexed   int
	FilesSkipped   int
	RecordsIndexed int
	EmbedBatches   int
	Duration       time.Duration
	IndexPath      string
	Skipped        []SkippedFile
	Warnings       []string
}

type RetrieveRequest struct {
	Root                 string
	Query                string
	ExplicitPaths        []string
	CurrentTurnPaths     []string
	Deadline             time.Duration
	RouteAction          string
	RouteReason          string
	RouteProfile         string
	UseCurrentPathWeight bool
	MaxRecords           int
	MaxFiles             int
	MaxContextBytes      int
	Observer             RetrieveObserverFunc
}

type RetrieveStage string

const (
	RetrieveStageManifest RetrieveStage = "manifest"
	RetrieveStageRecords  RetrieveStage = "records"
	RetrieveStageVectors  RetrieveStage = "vectors"
	RetrieveStageEmbed    RetrieveStage = "embed"
	RetrieveStageScore    RetrieveStage = "score"
	RetrieveStageRender   RetrieveStage = "render"
	RetrieveStageTotal    RetrieveStage = "total"
)

type RetrieveStageEvent struct {
	Stage    RetrieveStage
	Duration time.Duration
	CacheHit bool
}

type RetrieveObserverFunc func(RetrieveStageEvent)

func (f RetrieveObserverFunc) Observe(evt RetrieveStageEvent) {
	if f == nil {
		return
	}
	f(evt)
}

type SearchHit struct {
	Record Record
	Score  float64
	Reason string
}

type RetrievedFile struct {
	Path    string
	Score   float64
	Records []SearchHit
}

type RetrieveResult struct {
	Used            bool
	FallbackReason  string
	Records         []SearchHit
	Files           []RetrievedFile
	RenderedContext string
	ContextBytes    int
	StaleDropped    int
	Warnings        []string
}

func IsFallbackError(err error) bool {
	return errors.Is(err, ErrDisabled) ||
		errors.Is(err, ErrIndexMissing) ||
		errors.Is(err, ErrIndexStale) ||
		errors.Is(err, ErrModelMissing) ||
		errors.Is(err, ErrDeadline) ||
		errors.Is(err, ErrDimensionsMismatch)
}
