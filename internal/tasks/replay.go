package tasks

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
	"strings"

	"github.com/FernasFragas/nandocodego/internal/llm"
)

type replayRecord struct {
	Kind    string      `json:"kind"`
	Message llm.Message `json:"message"`
}

func ReplayMessagesFromOutput(path string, max int) ([]llm.Message, error) {
	if strings.TrimSpace(path) == "" {
		return nil, fmt.Errorf("output path is required")
	}
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	var out []llm.Message
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		var line OutputLine
		if err := json.Unmarshal(sc.Bytes(), &line); err != nil {
			continue
		}
		raw := strings.TrimSpace(line.Text)
		if raw == "" {
			continue
		}
		var rec replayRecord
		if err := json.Unmarshal([]byte(raw), &rec); err != nil {
			continue
		}
		if rec.Kind != "message" || rec.Message.Role == "" {
			continue
		}
		out = append(out, rec.Message)
	}
	if err := sc.Err(); err != nil {
		return nil, err
	}
	if max > 0 && len(out) > max {
		out = out[len(out)-max:]
	}
	return out, nil
}

