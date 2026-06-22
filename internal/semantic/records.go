package semantic

import (
	"crypto/sha256"
	"encoding/hex"
	"path/filepath"
	"sort"
	"strings"
)

func normalizeRelPath(path string) string {
	path = filepath.ToSlash(strings.TrimSpace(path))
	path = strings.TrimPrefix(path, "./")
	return strings.TrimPrefix(path, "/")
}

func hashText(s string) string {
	sum := sha256.Sum256([]byte(s))
	return hex.EncodeToString(sum[:])
}

func hashBytes(b []byte) string {
	sum := sha256.Sum256(b)
	return hex.EncodeToString(sum[:])
}

func estimateTokens(s string) int {
	if s == "" {
		return 0
	}
	return (len(s) + 3) / 4
}

func deterministicRecordID(kind RecordKind, path, name string, startLine, endLine int, textHash string) string {
	var b strings.Builder
	b.WriteString(string(kind))
	b.WriteByte('\n')
	b.WriteString(normalizeRelPath(path))
	b.WriteByte('\n')
	b.WriteString(strings.TrimSpace(name))
	b.WriteByte('\n')
	b.WriteString(textHash)
	b.WriteByte('\n')
	b.WriteString(intToString(startLine))
	b.WriteByte('\n')
	b.WriteString(intToString(endLine))
	return hashText(b.String())
}

func intToString(v int) string {
	if v == 0 {
		return "0"
	}
	neg := v < 0
	if neg {
		v = -v
	}
	var buf [24]byte
	i := len(buf)
	for v > 0 {
		i--
		buf[i] = byte('0' + (v % 10))
		v /= 10
	}
	if neg {
		i--
		buf[i] = '-'
	}
	return string(buf[i:])
}

func makeRecord(
	kind RecordKind,
	path string,
	language string,
	name string,
	parent string,
	startLine int,
	endLine int,
	contentHash string,
	text string,
) Record {
	path = normalizeRelPath(path)
	text = strings.TrimSpace(text)
	preview := text
	if len(preview) > 280 {
		preview = preview[:280]
	}
	textHash := hashText(text)
	return Record{
		ID:          deterministicRecordID(kind, path, name, startLine, endLine, textHash),
		Kind:        kind,
		Path:        path,
		Language:    language,
		Name:        strings.TrimSpace(name),
		Parent:      strings.TrimSpace(parent),
		StartLine:   startLine,
		EndLine:     endLine,
		ContentHash: contentHash,
		TextHash:    textHash,
		TextPreview: preview,
		EmbedText:   text,
		EstTokens:   estimateTokens(text),
	}
}

func sortRecords(records []Record) {
	kindRank := map[RecordKind]int{
		RecordKindFolder:     0,
		RecordKindFile:       1,
		RecordKindSymbol:     2,
		RecordKindDocSection: 3,
		RecordKindChunk:      4,
	}
	sort.Slice(records, func(i, j int) bool {
		a := records[i]
		b := records[j]
		if a.Path != b.Path {
			return a.Path < b.Path
		}
		ra := kindRank[a.Kind]
		rb := kindRank[b.Kind]
		if ra != rb {
			return ra < rb
		}
		if a.StartLine != b.StartLine {
			return a.StartLine < b.StartLine
		}
		if a.EndLine != b.EndLine {
			return a.EndLine < b.EndLine
		}
		if a.Name != b.Name {
			return a.Name < b.Name
		}
		return a.ID < b.ID
	})
}
