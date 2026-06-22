package semantic

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

func renderRetrievedContext(root string, hits []SearchHit, maxBytes int) (string, int, int, []SearchHit, []string) {
	if maxBytes <= 0 {
		maxBytes = DefaultMaxContextBytes
	}
	var b strings.Builder
	warnings := []string{}
	staleDropped := 0
	kept := make([]SearchHit, 0, len(hits))

	b.WriteString(`<semantic_context`)
	b.WriteString(` records="`)
	b.WriteString(intToString(len(hits)))
	b.WriteString(`">`)
	b.WriteString("\n")

	for _, hit := range hits {
		full := filepath.Join(root, filepath.FromSlash(hit.Record.Path))
		stale, err := IsRecordStale(hit.Record, full)
		if err != nil {
			warnings = append(warnings, fmt.Sprintf("stale check failed for %s: %v", hit.Record.Path, err))
			staleDropped++
			continue
		}
		if stale {
			staleDropped++
			continue
		}

		snippet, err := readLineRange(full, hit.Record.StartLine, hit.Record.EndLine, 2000)
		if err != nil {
			warnings = append(warnings, fmt.Sprintf("read evidence failed for %s: %v", hit.Record.Path, err))
			continue
		}
		block := renderEvidenceBlock(hit, snippet)
		if b.Len()+len(block)+len("\n</semantic_context>\n") > maxBytes {
			break
		}
		b.WriteString(block)
		kept = append(kept, hit)
	}
	b.WriteString("</semantic_context>\n")
	return b.String(), b.Len(), staleDropped, kept, warnings
}

func renderEvidenceBlock(hit SearchHit, snippet string) string {
	var b strings.Builder
	b.WriteString(`<evidence path="`)
	b.WriteString(escapeAttr(hit.Record.Path))
	b.WriteString(`" kind="`)
	b.WriteString(escapeAttr(string(hit.Record.Kind)))
	b.WriteString(`" name="`)
	b.WriteString(escapeAttr(hit.Record.Name))
	b.WriteString(`" lines="`)
	b.WriteString(intToString(hit.Record.StartLine))
	b.WriteString("-")
	b.WriteString(intToString(hit.Record.EndLine))
	b.WriteString(`" score="`)
	b.WriteString(fmt.Sprintf("%.3f", hit.Score))
	b.WriteString(`">`)
	b.WriteString("\n")
	b.WriteString(escapeText(snippet))
	b.WriteString("\n</evidence>\n")
	return b.String()
}

func readLineRange(path string, startLine, endLine, maxChars int) (string, error) {
	if startLine <= 0 {
		startLine = 1
	}
	if endLine < startLine || endLine <= 0 {
		endLine = startLine + 120
	}
	f, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer f.Close()

	var b strings.Builder
	scanner := bufio.NewScanner(f)
	scanner.Buffer(make([]byte, 0, 64*1024), 4*1024*1024)
	line := 0
	for scanner.Scan() {
		line++
		if line < startLine {
			continue
		}
		if line > endLine {
			break
		}
		txt := scanner.Text()
		if b.Len() > 0 {
			b.WriteByte('\n')
		}
		if maxChars > 0 && b.Len()+len(txt) > maxChars {
			remain := maxChars - b.Len()
			if remain > 0 {
				b.WriteString(txt[:remain])
			}
			break
		}
		b.WriteString(txt)
	}
	if err := scanner.Err(); err != nil {
		return "", err
	}
	return b.String(), nil
}

func escapeAttr(s string) string {
	s = strings.ReplaceAll(s, "&", "&amp;")
	s = strings.ReplaceAll(s, `"`, "&quot;")
	s = strings.ReplaceAll(s, "<", "&lt;")
	s = strings.ReplaceAll(s, ">", "&gt;")
	return s
}

func escapeText(s string) string {
	s = strings.ReplaceAll(s, "&", "&amp;")
	s = strings.ReplaceAll(s, "<", "&lt;")
	s = strings.ReplaceAll(s, ">", "&gt;")
	return s
}
