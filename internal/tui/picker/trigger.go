package picker

import (
	"github.com/FernasFragas/nandocodego/internal/mentions"
	"strings"
	"unicode"
)

// Context describes the active trigger token under cursor.
type Context struct {
	Kind   Trigger
	Start  int
	End    int
	Query  string
	Active bool
}

// Detect finds active @file or /command token at cursor.
func Detect(line string, cursor int) Context {
	runes := []rune(line)
	if cursor < 0 {
		cursor = 0
	}
	if cursor > len(runes) {
		cursor = len(runes)
	}
	if inBacktickSpan(runes, cursor) {
		return Context{}
	}

	if ctx := detectFile(runes, cursor); ctx.Active {
		return ctx
	}
	return detectCommand(runes, cursor)
}

func detectFile(runes []rune, cursor int) Context {
	tok := mentions.TokenAtCursor(string(runes), cursor)
	if tok.Active {
		return Context{
			Kind:   TriggerFile,
			Start:  tok.Start,
			End:    tok.End,
			Query:  tok.Raw,
			Active: true,
		}
	}
	return Context{}
}

func detectCommand(runes []rune, cursor int) Context {
	first := -1
	for i, r := range runes {
		if unicode.IsSpace(r) {
			continue
		}
		first = i
		break
	}
	if first < 0 || runes[first] != '/' {
		return Context{}
	}
	end := first + 1
	for end < len(runes) && !unicode.IsSpace(runes[end]) {
		end++
	}
	if cursor < first || cursor > end {
		return Context{}
	}
	return Context{
		Kind:   TriggerCommand,
		Start:  first,
		End:    end,
		Query:  string(runes[first+1 : end]),
		Active: true,
	}
}

func inBacktickSpan(runes []rune, cursor int) bool {
	if cursor < 0 {
		return false
	}
	if cursor > len(runes) {
		cursor = len(runes)
	}
	count := strings.Count(string(runes[:cursor]), "`")
	return count%2 == 1
}
