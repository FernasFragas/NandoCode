package hooks

import "time"

type Kind string

const (
	KindCommand Kind = "command"
	KindPrompt  Kind = "prompt"
	KindHTTP    Kind = "http"
	KindAgent   Kind = "agent"
)

func (k Kind) Valid() bool {
	switch k {
	case KindCommand, KindPrompt, KindHTTP, KindAgent:
		return true
	default:
		return false
	}
}

func (k Kind) Executable() bool {
	return k == KindCommand || k == KindPrompt || k == KindHTTP || k == KindAgent
}

type Source string

const (
	SourceUser    Source = "user"
	SourceProject Source = "project"
)

type Hook struct {
	Kind         Kind              `json:"kind"`
	Event        Event             `json:"event"`
	Matcher      string            `json:"matcher,omitempty"`
	Command      string            `json:"command,omitempty"`
	Prompt       string            `json:"prompt,omitempty"`
	URL          string            `json:"url,omitempty"`
	Method       string            `json:"method,omitempty"`
	TimeoutSec   int               `json:"timeout_sec,omitempty"`
	ParallelSafe bool              `json:"parallel_safe,omitempty"`
	Env          map[string]string `json:"env,omitempty"`

	Source  Source `json:"-"`
	Enabled bool   `json:"-"`
}

func (h Hook) Timeout(defaultTimeout time.Duration) time.Duration {
	if h.TimeoutSec <= 0 {
		return defaultTimeout
	}
	return time.Duration(h.TimeoutSec) * time.Second
}

type ConfigFile struct {
	Hooks []Hook `json:"hooks"`
}

type DisabledHook struct {
	Hook   Hook
	Reason string
}

type Snapshot struct {
	Hooks    []Hook
	Disabled []DisabledHook
	Warnings []string
}

type LoadOptions struct {
	UserPath    string
	ProjectPath string
}

type Config struct {
	SessionID      string
	Model          string
	PermissionMode string
	WorkingDir     string
	DefaultTimeout time.Duration
}

func DefaultConfig() Config {
	return Config{DefaultTimeout: 5 * time.Second}
}
