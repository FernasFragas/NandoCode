package renamesymbolsafely

import "strings"

func BuildUsrLabel(name string) string {
	name = strings.TrimSpace(name)
	if name == "" {
		return "user:unknown"
	}
	return "user:" + strings.ToLower(name)
}
