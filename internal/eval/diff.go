package eval

import (
	"fmt"
	"sort"
	"strings"
)

func DiffManifests(before, after Manifest) ([]FileChange, DiffSize, string, error) {
	seen := make(map[string]struct{}, len(before.Files)+len(after.Files))
	for path := range before.Files {
		seen[path] = struct{}{}
	}
	for path := range after.Files {
		seen[path] = struct{}{}
	}
	var paths []string
	for path := range seen {
		paths = append(paths, path)
	}
	sort.Strings(paths)

	var changes []FileChange
	var size DiffSize
	var patch strings.Builder
	for _, rel := range paths {
		b, okBefore := before.Files[rel]
		a, okAfter := after.Files[rel]
		switch {
		case !okBefore && okAfter:
			changes = append(changes, FileChange{Path: rel, Change: ChangeAdded})
			size.Files++
			if a.Binary {
				size.BinaryFiles++
			}
			content := a.Content
			size.BytesAdded += len(content)
			size.LinesAdded += lineCount(content)
			fmt.Fprintf(&patch, "--- /dev/null\n+++ %s\n", rel)
			for _, line := range splitLines(normalizeText(content)) {
				if line == "" {
					continue
				}
				patch.WriteString("+" + line + "\n")
			}
		case okBefore && !okAfter:
			changes = append(changes, FileChange{Path: rel, Change: ChangeDeleted})
			size.Files++
			if b.Binary {
				size.BinaryFiles++
			}
			content := b.Content
			size.BytesDeleted += len(content)
			size.LinesDeleted += lineCount(content)
			fmt.Fprintf(&patch, "--- %s\n+++ /dev/null\n", rel)
			for _, line := range splitLines(normalizeText(content)) {
				if line == "" {
					continue
				}
				patch.WriteString("-" + line + "\n")
			}
		case okBefore && okAfter:
			if b.Digest == a.Digest && b.Executable == a.Executable {
				continue
			}
			changeType := ChangeModified
			if b.Digest == a.Digest && b.Executable != a.Executable {
				changeType = ChangeModeChange
			}
			changes = append(changes, FileChange{Path: rel, Change: changeType})
			size.Files++
			beforeContent := b.Content
			afterContent := a.Content
			if b.Binary || a.Binary {
				size.BinaryFiles++
			}
			linesAdded, linesDeleted, bytesAdded, bytesDeleted := modifiedCounts(beforeContent, afterContent)
			size.LinesAdded += linesAdded
			size.LinesDeleted += linesDeleted
			size.BytesAdded += bytesAdded
			size.BytesDeleted += bytesDeleted
			fmt.Fprintf(&patch, "--- %s\n+++ %s\n", rel, rel)
			beforeText := splitLines(normalizeText(beforeContent))
			afterText := splitLines(normalizeText(afterContent))
			for _, line := range beforeText {
				if line == "" {
					continue
				}
				patch.WriteString("-" + line + "\n")
			}
			for _, line := range afterText {
				if line == "" {
					continue
				}
				patch.WriteString("+" + line + "\n")
			}
		}
	}
	return changes, size, patch.String(), nil
}

func lineCount(content []byte) int {
	return len(splitLines(normalizeText(content)))
}

func modifiedCounts(before, after []byte) (linesAdded, linesDeleted, bytesAdded, bytesDeleted int) {
	beforeLines := splitLines(normalizeText(before))
	afterLines := splitLines(normalizeText(after))
	start := 0
	for start < len(beforeLines) && start < len(afterLines) && beforeLines[start] == afterLines[start] {
		start++
	}
	endBefore := len(beforeLines) - 1
	endAfter := len(afterLines) - 1
	for endBefore >= start && endAfter >= start && beforeLines[endBefore] == afterLines[endAfter] {
		endBefore--
		endAfter--
	}
	if endAfter >= start {
		linesAdded = endAfter - start + 1
		for i := start; i <= endAfter; i++ {
			bytesAdded += len(afterLines[i])
		}
	}
	if endBefore >= start {
		linesDeleted = endBefore - start + 1
		for i := start; i <= endBefore; i++ {
			bytesDeleted += len(beforeLines[i])
		}
	}
	return linesAdded, linesDeleted, bytesAdded, bytesDeleted
}

func splitLines(text string) []string {
	if text == "" {
		return nil
	}
	text = strings.TrimSuffix(text, "\n")
	if text == "" {
		return nil
	}
	return strings.Split(text, "\n")
}
