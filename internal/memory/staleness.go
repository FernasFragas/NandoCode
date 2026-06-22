package memory

import (
	"fmt"
	"time"
)

func dayStart(t time.Time) time.Time {
	y, m, d := t.Date()
	return time.Date(y, m, d, 0, 0, 0, 0, t.Location())
}

// AgeDescription returns a human-friendly relative age label.
func AgeDescription(now, updatedAt time.Time) string {
	n := dayStart(now)
	u := dayStart(updatedAt)
	days := int(n.Sub(u).Hours() / 24)
	switch {
	case days <= 0:
		return "today"
	case days == 1:
		return "yesterday"
	default:
		return fmt.Sprintf("%d days ago", days)
	}
}

// StalenessWarning returns action-cue warning for memories older than yesterday.
func StalenessWarning(now, updatedAt time.Time) string {
	n := dayStart(now)
	u := dayStart(updatedAt)
	days := int(n.Sub(u).Hours() / 24)
	if days <= 1 {
		return ""
	}
	return fmt.Sprintf("Before recommending from memory: confirm this is still current. This memory was last updated %s.", AgeDescription(now, updatedAt))
}
