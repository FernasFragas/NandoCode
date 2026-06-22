package semantic

import (
	"fmt"
	"regexp"
	"strings"
)

var markdownHeadingRe = regexp.MustCompile(`^\s{0,3}(#{1,6})\s+(.+?)\s*$`)

type heading struct {
	line  int
	level int
	text  string
}

func extractMarkdownSectionRecords(relPath string, body []byte, contentHash string) []Record {
	text := string(body)
	lines := strings.Split(text, "\n")
	headings := make([]heading, 0, 32)
	for i, raw := range lines {
		matches := markdownHeadingRe.FindStringSubmatch(strings.TrimRight(raw, "\r"))
		if len(matches) != 3 {
			continue
		}
		level := len(matches[1])
		title := strings.TrimSpace(strings.Trim(matches[2], "#"))
		if title == "" {
			continue
		}
		headings = append(headings, heading{
			line:  i + 1,
			level: level,
			text:  title,
		})
	}
	if len(headings) == 0 {
		return nil
	}

	out := make([]Record, 0, len(headings))
	for i, h := range headings {
		start := h.line
		end := len(lines)
		if i+1 < len(headings) {
			end = headings[i+1].line - 1
		}
		parent := ""
		for j := i - 1; j >= 0; j-- {
			if headings[j].level < h.level {
				parent = headings[j].text
				break
			}
		}
		section := snippetForRange(text, start, end)
		embed := fmt.Sprintf("markdown section %s in %s\n%s", h.text, normalizeRelPath(relPath), section)
		out = append(out, makeRecord(
			RecordKindDocSection,
			relPath,
			"markdown",
			h.text,
			parent,
			start,
			end,
			contentHash,
			embed,
		))
	}
	return out
}

func chunkFallbackRecords(relPath string, language string, body []byte, contentHash string, maxChunkBytes int, overlapLines int) []Record {
	if maxChunkBytes <= 0 {
		maxChunkBytes = 2048
	}
	if overlapLines < 0 {
		overlapLines = 0
	}

	text := string(body)
	lines := splitPreserveLineBreaks(text)
	if len(lines) == 0 {
		return nil
	}

	type span struct {
		start int
		end   int
		text  string
	}
	spans := make([]span, 0, len(lines)/8+1)
	seen := make(map[string]struct{}, len(lines)/4+1)

	for idx := 0; idx < len(lines); {
		start := idx
		size := 0
		end := idx
		for end < len(lines) {
			next := len(lines[end])
			if size > 0 && size+next > maxChunkBytes {
				break
			}
			size += next
			end++
		}
		if end == start {
			end = start + 1
		}
		chunk := strings.TrimSpace(strings.Join(lines[start:end], ""))
		if chunk != "" {
			key := intToString(start+1) + ":" + intToString(end)
			if _, ok := seen[key]; !ok {
				seen[key] = struct{}{}
				spans = append(spans, span{
					start: start + 1,
					end:   end,
					text:  chunk,
				})
			}
		}
		if end >= len(lines) {
			break
		}
		if overlapLines > 0 && end-overlapLines > start {
			idx = end - overlapLines
		} else {
			idx = end
		}
	}

	out := make([]Record, 0, len(spans))
	for i, s := range spans {
		name := fmt.Sprintf("chunk-%03d", i+1)
		embed := fmt.Sprintf("text chunk %s in %s\n%s", name, normalizeRelPath(relPath), s.text)
		out = append(out, makeRecord(
			RecordKindChunk,
			relPath,
			language,
			name,
			"",
			s.start,
			s.end,
			contentHash,
			embed,
		))
	}
	return out
}

func snippetForRange(text string, startLine, endLine int) string {
	if startLine <= 0 || endLine <= 0 || endLine < startLine {
		return ""
	}
	lines := strings.Split(text, "\n")
	if startLine > len(lines) {
		return ""
	}
	if endLine > len(lines) {
		endLine = len(lines)
	}
	segment := strings.TrimSpace(strings.Join(lines[startLine-1:endLine], "\n"))
	if len(segment) > 2000 {
		return segment[:2000]
	}
	return segment
}

func splitPreserveLineBreaks(text string) []string {
	if text == "" {
		return nil
	}
	lines := strings.SplitAfter(text, "\n")
	if len(lines) == 0 {
		return nil
	}
	return lines
}
