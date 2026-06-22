package tasks

import (
	"bufio"
	"encoding/json"
	"os"
	"path/filepath"
	"sync"
	"time"
)

type OutputLine struct {
	Ts     time.Time `json:"ts"`
	Stream string    `json:"stream,omitempty"`
	Text   string    `json:"text,omitempty"`
	Kind   string    `json:"kind,omitempty"`
	Code   int       `json:"code,omitempty"`
}

type OutputWriter struct {
	f   *os.File
	enc *json.Encoder
	mu  sync.Mutex
}

func NewOutputWriter(path string) (*OutputWriter, error) {
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return nil, err
	}
	f, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o600)
	if err != nil {
		return nil, err
	}
	return &OutputWriter{f: f, enc: json.NewEncoder(f)}, nil
}

func (w *OutputWriter) WriteText(stream, text string) error {
	w.mu.Lock()
	defer w.mu.Unlock()
	return w.enc.Encode(OutputLine{Ts: time.Now().UTC(), Stream: stream, Text: text})
}

func (w *OutputWriter) WriteExit(code int) error {
	w.mu.Lock()
	defer w.mu.Unlock()
	return w.enc.Encode(OutputLine{Ts: time.Now().UTC(), Kind: "exit", Code: code})
}

func (w *OutputWriter) Close() error {
	if w == nil || w.f == nil {
		return nil
	}
	w.mu.Lock()
	defer w.mu.Unlock()
	return w.f.Close()
}

func TailLines(path string, n int) ([]OutputLine, error) {
	if n <= 0 {
		n = 20
	}
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	sc := bufio.NewScanner(f)
	all := make([]OutputLine, 0, n)
	for sc.Scan() {
		var line OutputLine
		if err := json.Unmarshal(sc.Bytes(), &line); err != nil {
			continue
		}
		all = append(all, line)
		if len(all) > n {
			all = all[1:]
		}
	}
	return all, sc.Err()
}
