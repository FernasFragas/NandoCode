package agent

import (
	"strings"

	"github.com/FernasFragas/nandocodego/internal/llm"
)

const incompleteAssistantRetryLimit = 1

func shouldRetryIncompleteAssistantResponse(msg llm.Message, doneReason string) bool {
	if len(msg.ToolCalls) > 0 {
		return false
	}
	if doneReason != "" && doneReason != "stop" {
		return false
	}

	content := strings.TrimSpace(msg.Content)
	if content == "" {
		return strings.TrimSpace(msg.Thinking) != ""
	}
	if wordCount(content) > 180 || len(content) > 1200 {
		return false
	}

	normalized := normalizeAssistantPreamble(content)
	if strings.HasSuffix(strings.TrimRight(content, " \t\r\n"), ":") && containsAny(normalized, preambleOnlyCues()) {
		return true
	}
	if containsAny(normalized, explicitPromiseCues()) && wordCount(content) <= 120 {
		return true
	}
	return false
}

type incompleteRetryInput struct {
	PromptIntent         string
	AttachmentPolicy     string
	OriginalUserText     string
	LastUserContent      string
	LastAssistantContent string
}

func buildIncompleteAssistantRetryPrompt(in incompleteRetryInput) (string, bool) {
	lastUser := strings.TrimSpace(in.LastUserContent)
	if lastUser == "" {
		lastUser = strings.TrimSpace(in.OriginalUserText)
	}
	if strings.EqualFold(strings.TrimSpace(in.PromptIntent), "directory_listing") ||
		strings.EqualFold(strings.TrimSpace(in.AttachmentPolicy), "listing_tree_only") ||
		strings.Contains(lastUser, "Directory tree data:") {
		if listingAnswerLooksSubstantive(in.LastAssistantContent) {
			return "", false
		}
		original := strings.TrimSpace(extractOriginalUserRequest(lastUser))
		if original == "" {
			original = strings.TrimSpace(in.OriginalUserText)
		}
		tree := strings.TrimSpace(extractDirectoryTreeData(lastUser))
		var b strings.Builder
		b.WriteString("Your previous response looked incomplete. Continue by answering this listing request:\n\n")
		if original != "" {
			b.WriteString("Original user request:\n")
			b.WriteString(original)
			b.WriteString("\n\n")
		}
		if tree != "" {
			b.WriteString("Directory tree data:\n")
			b.WriteString(tree)
		}
		return strings.TrimSpace(b.String()), true
	}

	original := strings.TrimSpace(in.OriginalUserText)
	if original == "" {
		original = strings.TrimSpace(extractOriginalUserRequest(lastUser))
	}
	if original == "" {
		original = strings.TrimSpace(lastUser)
	}
	if original == "" {
		return "", false
	}
	return "Your previous response looked incomplete. Answer the original user request below without continuing any unrelated prior task.\n\nOriginal user request:\n" + original, true
}

func extractOriginalUserRequest(s string) string {
	lower := strings.ToLower(s)
	key := "user request:"
	idx := strings.Index(lower, key)
	if idx < 0 {
		return s
	}
	rest := strings.TrimSpace(s[idx+len(key):])
	treeKey := "\n\ndirectory tree data:"
	treeIdx := strings.Index(strings.ToLower(rest), treeKey)
	if treeIdx < 0 {
		return rest
	}
	return strings.TrimSpace(rest[:treeIdx])
}

func extractDirectoryTreeData(s string) string {
	lower := strings.ToLower(s)
	key := "directory tree data:"
	idx := strings.Index(lower, key)
	if idx < 0 {
		return ""
	}
	return strings.TrimSpace(s[idx+len(key):])
}

func listingAnswerLooksSubstantive(content string) bool {
	trimmed := strings.TrimSpace(content)
	if trimmed == "" {
		return false
	}
	if wordCount(trimmed) > 80 {
		return true
	}
	lines := strings.Split(trimmed, "\n")
	pathLike := 0
	for _, line := range lines {
		l := strings.TrimSpace(line)
		if l == "" {
			continue
		}
		if strings.Contains(l, "/") || strings.HasSuffix(l, ".md") || strings.HasSuffix(l, ".txt") {
			pathLike++
		}
	}
	return pathLike >= 3
}

func normalizeAssistantPreamble(s string) string {
	replacer := strings.NewReplacer(
		"\u2018", "'",
		"\u2019", "'",
		"\u201c", "\"",
		"\u201d", "\"",
	)
	s = replacer.Replace(strings.ToLower(s))
	return strings.Join(strings.Fields(s), " ")
}

func containsAny(s string, needles []string) bool {
	for _, needle := range needles {
		if strings.Contains(s, needle) {
			return true
		}
	}
	return false
}

func preambleOnlyCues() []string {
	return []string{
		"based on",
		"comprehensive",
		"here is",
		"here are",
		"here's",
		"i can now",
		"i will",
		"i'll",
		"the fix is",
		"let me",
		"now i have",
		"summary",
		"report",
		"analysis",
	}
}

func explicitPromiseCues() []string {
	return []string{
		"i am going to write",
		"i can now write",
		"i found the issue",
		"i will provide",
		"i will now write",
		"i will write",
		"i'm going to write",
		"i'll now write",
		"i'll write",
		"let me write",
		"now i have the full picture",
		"follows below",
		"as follows",
	}
}

func wordCount(s string) int {
	return len(strings.Fields(s))
}
