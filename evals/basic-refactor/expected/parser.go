package basicrefactor

import (
	"fmt"
	"strconv"
	"strings"
)

func ParseLimit(value string) (int, error) {
	return parseNonNegative(value, "limit")
}

func ParseOffset(value string) (int, error) {
	return parseNonNegative(value, "offset")
}

func parseNonNegative(value string, field string) (int, error) {
	value = strings.TrimSpace(value)
	if value == "" {
		return 0, fmt.Errorf("%s is required", field)
	}
	n, err := strconv.Atoi(value)
	if err != nil {
		return 0, fmt.Errorf("invalid %s: %w", field, err)
	}
	if n < 0 {
		return 0, fmt.Errorf("%s must be >= 0", field)
	}
	return n, nil
}
