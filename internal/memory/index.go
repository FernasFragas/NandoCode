package memory

import (
	"bufio"
	"os"
	"strings"
)

// LoadIndex loads MEMORY.md and enforces line/byte caps.
func LoadIndex(path string, maxLines, maxBytes int) (Index, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return Index{}, nil
		}
		return Index{}, err
	}

	content := string(b)
	capped := false
	if maxBytes > 0 && len(b) > maxBytes {
		content = string(b[:maxBytes])
		capped = true
	}

	if maxLines > 0 {
		sc := bufio.NewScanner(strings.NewReader(content))
		lines := make([]string, 0, maxLines)
		for sc.Scan() {
			lines = append(lines, sc.Text())
			if len(lines) >= maxLines {
				if sc.Scan() {
					capped = true
				}
				break
			}
		}
		content = strings.Join(lines, "\n")
	}

	idx := Index{
		Content: strings.TrimSpace(content),
		Capped:  capped,
	}
	if capped {
		idx.Warning = "MEMORY.md exceeded limits; keep one-line entries and move detail to topic files."
	}
	return idx, nil
}
