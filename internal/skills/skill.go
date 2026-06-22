package skills

import (
	"bufio"
	"fmt"
	"io"
	"strings"
	"time"

	"gopkg.in/yaml.v3"
)

type Source int

const (
	SourceBundled Source = iota
	SourceUser
	SourceProject
	SourceMCP
)

func (s Source) String() string {
	switch s {
	case SourceBundled:
		return "bundled"
	case SourceUser:
		return "user"
	case SourceProject:
		return "project"
	case SourceMCP:
		return "mcp"
	default:
		return "unknown"
	}
}

type SkillFile struct {
	Name        string
	Description string
	Version     string
	Author      string
	Tags        []string
	Source      Source
	Path        string
	EmbedPath   string
	ModTime     time.Time
	Body        string
}

func (s SkillFile) IsFilesystem() bool { return strings.TrimSpace(s.Path) != "" }
func (s SkillFile) IsEmbedded() bool   { return strings.TrimSpace(s.EmbedPath) != "" }

type frontmatter struct {
	Name        string   `yaml:"name"`
	Description string   `yaml:"description"`
	Version     string   `yaml:"version"`
	Author      string   `yaml:"author"`
	Tags        []string `yaml:"tags"`
}

func parseFrontmatter(r io.Reader) (SkillFile, string, error) {
	sc := bufio.NewScanner(r)
	maxLines := 50
	maxBytes := 4096
	sc.Buffer(make([]byte, 0, 4096), maxBytes)

	var lines []string
	bytesRead := 0
	sawOpening := false
	for i := 0; i < maxLines && sc.Scan(); i++ {
		line := sc.Text()
		lines = append(lines, line)
		bytesRead += len(line) + 1
		if bytesRead > maxBytes {
			break
		}
		if i == 0 && strings.TrimSpace(lines[0]) != "---" {
			return SkillFile{}, "", fmt.Errorf("missing opening frontmatter delimiter")
		}
		if i == 0 {
			sawOpening = true
		}
		if i > 0 && strings.TrimSpace(lines[i]) == "---" {
			header := strings.Join(lines[1:i], "\n")
			var fm frontmatter
			if err := yaml.Unmarshal([]byte(header), &fm); err != nil {
				return SkillFile{}, "", fmt.Errorf("invalid yaml frontmatter: %w", err)
			}
			if strings.TrimSpace(fm.Name) == "" {
				return SkillFile{}, "", fmt.Errorf("frontmatter field name is required")
			}
			if strings.TrimSpace(fm.Description) == "" {
				return SkillFile{}, "", fmt.Errorf("frontmatter field description is required")
			}
			rest := strings.Join(lines[i+1:], "\n")
			for sc.Scan() {
				if rest == "" {
					rest = sc.Text()
				} else {
					rest += "\n" + sc.Text()
				}
			}
			if err := sc.Err(); err != nil {
				return SkillFile{}, "", err
			}
			sf := SkillFile{
				Name:        strings.TrimSpace(fm.Name),
				Description: strings.TrimSpace(fm.Description),
				Version:     strings.TrimSpace(fm.Version),
				Author:      strings.TrimSpace(fm.Author),
				Tags:        append([]string(nil), fm.Tags...),
			}
			return sf, rest, nil
		}
	}
	if err := sc.Err(); err != nil {
		return SkillFile{}, "", err
	}
	if !sawOpening {
		return SkillFile{}, "", fmt.Errorf("missing opening frontmatter delimiter")
	}
	return SkillFile{}, "", fmt.Errorf("no closing frontmatter delimiter found")
}
