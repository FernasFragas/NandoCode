package tui

import "strings"

// ParseSlashCommand parses a slash command input into command and args.
func ParseSlashCommand(input string) (string, []string) {
	input = strings.TrimSpace(input)
	if !strings.HasPrefix(input, "/") {
		return "", nil
	}

	parts := strings.Fields(input[1:])
	if len(parts) == 0 {
		return "", nil
	}

	command := parts[0]
	args := parts[1:]
	return command, args
}
