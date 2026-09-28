package basicrefactor

import (
	"fmt"
	"strconv"
	"strings"
)

func ParseLimit(value string) (int, error) {
	value = strings.TrimSpace(value)
	if value == "" {
		return 0, fmt.Errorf("limit is required")
	}
	n, err := strconv.Atoi(value)
	if err != nil {
		return 0, fmt.Errorf("invalid limit: %w", err)
	}
	if n < 0 {
		return 0, fmt.Errorf("limit must be >= 0")
	}
	return n, nil
}

func ParseOffset(value string) (int, error) {
	value = strings.TrimSpace(value)
	if value == "" {
		return 0, fmt.Errorf("offset is required")
	}
	n, err := strconv.Atoi(value)
	if err != nil {
		return 0, fmt.Errorf("invalid offset: %w", err)
	}
	if n < 0 {
		return 0, fmt.Errorf("offset must be >= 0")
	}
	return n, nil
}
