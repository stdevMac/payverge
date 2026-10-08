package server

import (
	"fmt"
	"strings"
)

// counterPrefixMaxLen is the shared server limit for counter_prefix after trim
// (matches FE COUNTER_PREFIX_MAX_LENGTH and historical binding max=5).
const counterPrefixMaxLen = 5

// normalizeAndValidateCounterPrefix trims the raw prefix, then enforces
// non-empty and max length (audit L2-34).
//
// Order matters: TrimSpace first so padded values like "  BAR  " become "BAR"
// and pass max=5. Validating max before trim rejected legitimate prefixes that
// only exceeded the limit via surrounding whitespace.
func normalizeAndValidateCounterPrefix(raw string) (string, error) {
	prefix := strings.TrimSpace(raw)
	if prefix == "" {
		return "", fmt.Errorf("counter_prefix is required")
	}
	if len(prefix) > counterPrefixMaxLen {
		return "", fmt.Errorf("counter_prefix must be at most %d characters", counterPrefixMaxLen)
	}
	return prefix, nil
}
