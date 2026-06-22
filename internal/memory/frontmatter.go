package memory

import (
	"bufio"
	"fmt"
	"io"
	"strings"
	"time"

	"gopkg.in/yaml.v3"
)

type fmHeader struct {
	Name        string `yaml:"name"`
	Description string `yaml:"description"`
	Type        Type   `yaml:"type"`
}

func ParseFrontmatter(filename string, r io.Reader, modTime time.Time, size int64) (Entry, error) {
	sc := bufio.NewScanner(r)
	lines := make([]string, 0, 32)
	for sc.Scan() {
		lines = append(lines, sc.Text())
		if len(lines) >= 30 {
			break
		}
	}
	if err := sc.Err(); err != nil {
		return Entry{}, err
	}
	if len(lines) < 3 || strings.TrimSpace(lines[0]) != "---" {
		return Entry{}, fmt.Errorf("missing YAML frontmatter")
	}
	end := -1
	for i := 1; i < len(lines); i++ {
		if strings.TrimSpace(lines[i]) == "---" {
			end = i
			break
		}
	}
	if end == -1 {
		return Entry{}, fmt.Errorf("unterminated YAML frontmatter")
	}
	var hdr fmHeader
	block := strings.Join(lines[1:end], "\n")
	if err := yaml.Unmarshal([]byte(block), &hdr); err != nil {
		return Entry{}, fmt.Errorf("invalid YAML frontmatter: %w", err)
	}
	if strings.TrimSpace(hdr.Name) == "" || strings.TrimSpace(hdr.Description) == "" {
		return Entry{}, fmt.Errorf("frontmatter requires non-empty name and description")
	}
	switch hdr.Type {
	case TypeUser, TypeFeedback, TypeProject, TypeReference:
	default:
		return Entry{}, fmt.Errorf("invalid memory type: %q", hdr.Type)
	}

	return Entry{
		Filename:    filename,
		Name:        hdr.Name,
		Description: hdr.Description,
		Type:        hdr.Type,
		UpdatedAt:   modTime,
		SizeBytes:   size,
	}, nil
}
