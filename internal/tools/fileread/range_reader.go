package fileread

import (
	"bufio"
	"bytes"
	"fmt"
	"io"
	"os"
	"strings"
	"time"
	"unicode/utf8"
)

// ReadRangeRequest describes a bounded line-range read request.
type ReadRangeRequest struct {
	Path      string
	StartLine int
	LineLimit int
	MaxBytes  int
}

// ReadRangeResult is a normalized line-range read result used by FileRead and context packing.
type ReadRangeResult struct {
	Path       string
	Content    string
	StartLine  int
	LineCount  int
	TotalLines int
	TotalBytes int64
	ReadBytes  int
	MTime      time.Time
	Truncated  bool
}

// ReadRange reads a bounded line range from a UTF-8 text file.
func ReadRange(req ReadRangeRequest) (ReadRangeResult, error) {
	info, err := os.Stat(req.Path)
	if err != nil {
		return ReadRangeResult{}, err
	}
	if info.IsDir() {
		return ReadRangeResult{}, fmt.Errorf("%s is a directory", basenameForErrors(req.Path))
	}
	startLine := req.StartLine
	if startLine <= 0 {
		startLine = 1
	}
	lineLimit := req.LineLimit
	if lineLimit <= 0 {
		lineLimit = 200
	}
	maxBytes := req.MaxBytes
	if maxBytes < 0 {
		maxBytes = 0
	}
	res, err := readLineRange(req.Path, startLine, lineLimit, maxBytes)
	if err != nil {
		return ReadRangeResult{}, err
	}
	return ReadRangeResult{
		Path:       req.Path,
		Content:    res.Content,
		StartLine:  res.StartLine,
		LineCount:  res.LineCount,
		TotalLines: res.TotalLines,
		TotalBytes: info.Size(),
		ReadBytes:  res.ReadBytes,
		MTime:      info.ModTime(),
		Truncated:  res.Truncated,
	}, nil
}

func selectLineRangeFromBytes(path string, content []byte, startLine, lineLimit, maxBytes int) (lineRangeResult, error) {
	if !utf8.Valid(content) {
		return lineRangeResult{}, fmt.Errorf("%s is not valid UTF-8 text", basenameForErrors(path))
	}
	totalBytes := len(content)
	res, err := selectLineRangeFromReader(path, bytes.NewReader(content), int64(totalBytes), startLine, lineLimit, maxBytes)
	if err != nil {
		return lineRangeResult{}, err
	}
	return res, nil
}

func selectLineRangeFromReader(path string, r io.Reader, _ int64, startLine, lineLimit, maxBytes int) (lineRangeResult, error) {
	if startLine <= 0 {
		startLine = 1
	}
	if lineLimit <= 0 {
		lineLimit = 1
	}
	if maxBytes < 0 {
		maxBytes = 0
	}

	br := bufio.NewReaderSize(r, 512*1024)
	lines := make([]string, 0, min(lineLimit, 256))
	lineNumber := 0
	selectedBytes := 0
	truncated := false
	selectionDone := false
	selectionEndLine := startLine + lineLimit - 1

	for {
		chunk, err := br.ReadString('\n')
		if err != nil && err != io.EOF {
			return lineRangeResult{}, err
		}
		if err == io.EOF && chunk == "" {
			break
		}
		lineNumber++
		if lineNumber == 1 {
			chunk = strings.TrimPrefix(chunk, "\uFEFF")
		}
		if !utf8.ValidString(chunk) {
			return lineRangeResult{}, fmt.Errorf("%s is not valid UTF-8 text", basenameForErrors(path))
		}
		line := strings.TrimSuffix(chunk, "\n")
		line = strings.TrimSuffix(line, "\r")

		if lineNumber >= startLine && !selectionDone {
			sepBytes := 0
			if len(lines) > 0 {
				sepBytes = 1
			}
			nextBytes := selectedBytes + sepBytes + len(line)
			if maxBytes > 0 && nextBytes > maxBytes {
				allowed := maxBytes - selectedBytes - sepBytes
				if allowed > 0 {
					line = truncateUTF8ToBytes(line, allowed)
					lines = append(lines, line)
					selectedBytes += sepBytes + len(line)
				}
				truncated = true
				selectionDone = true
			} else {
				lines = append(lines, line)
				selectedBytes = nextBytes
			}
		}

		if lineNumber >= selectionEndLine {
			selectionDone = true
		}
		if selectionDone && lineNumber > selectionEndLine {
			truncated = true
		}

		if err == io.EOF {
			break
		}
	}

	if startLine > lineNumber {
		return lineRangeResult{
			Content:    "",
			StartLine:  startLine,
			LineCount:  0,
			TotalLines: lineNumber,
			ReadBytes:  0,
			Truncated:  false,
		}, nil
	}

	content := strings.Join(lines, "\n")
	return lineRangeResult{
		Content:    content,
		StartLine:  startLine,
		LineCount:  len(lines),
		TotalLines: lineNumber,
		ReadBytes:  len(content),
		Truncated:  truncated,
	}, nil
}

func truncateUTF8ToBytes(s string, max int) string {
	if max <= 0 || s == "" {
		return ""
	}
	if len(s) <= max {
		return s
	}
	idx := 0
	for idx < len(s) {
		_, size := utf8.DecodeRuneInString(s[idx:])
		if size <= 0 {
			break
		}
		if idx+size > max {
			break
		}
		idx += size
	}
	return s[:idx]
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
