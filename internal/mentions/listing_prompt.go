package mentions

import "strings"

func buildListingScopedPrompt(userInput string, dirs []ResolvedDirectory) string {
	var b strings.Builder
	b.WriteString("User request:\n")
	b.WriteString(strings.TrimSpace(userInput))
	b.WriteString("\n\nDirectory tree data:\n")
	for i := range dirs {
		tree := strings.TrimSpace(dirs[i].Tree)
		if tree == "" {
			continue
		}
		b.WriteString(tree)
		if i < len(dirs)-1 {
			b.WriteString("\n\n")
		}
	}
	return b.String()
}

