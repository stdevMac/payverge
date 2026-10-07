package formatters

import (
	"regexp"
	"strings"
)

// #825: table and counter display names are stored WITH the type word
// embedded ("Table 1", "Counter 3") for demo and default-provisioned venues,
// but operators can also rename a table to a bare custom label ("Patio A").
// Every printed document prepended the localized type word again, so the
// demo venue's tickets read "Table Table 1".
//
// This is the Go mirror of frontend/src/lib/tableLabel.ts formatEntityName,
// which already fixed the same doubling on the operator Recent-jobs list.
// Keep the two in step.

// englishSeedRe matches demo/default seed names: "Table 9", "Counter 3".
var englishSeedRe = regexp.MustCompile(`(?i)^(table|counter)\s+(\d+)$`)

var englishTypeWords = []string{"table", "counter"}

// EntityValue returns the identity half of a location name, with the type
// word removed, for documents that print the type word in their own label
// column ("Table" | "1"). A custom name that merely starts with the type word
// as a longer word ("Tablecloth Corner") keeps every character.
//
//	EntityValue("Table", "Table 1")           -> "1"
//	EntityValue("Mesa",  "Table 9")           -> "9"      (localized label)
//	EntityValue("Table", "Patio A")           -> "Patio A"
//	EntityValue("Mesa",  "Tablecloth Corner") -> "Tablecloth Corner"
//	EntityValue("Table", "Table")             -> ""       (no identity)
func EntityValue(label, name string) string {
	trimmed := strings.TrimSpace(name)
	if trimmed == "" {
		return ""
	}
	labelTrim := strings.TrimSpace(label)
	lower := strings.ToLower(trimmed)

	if labelTrim != "" {
		lowerLabel := strings.ToLower(labelTrim)
		if strings.HasPrefix(lower, lowerLabel) {
			rest := strings.TrimSpace(trimmed[len(labelTrim):])
			// "Tablecloth Corner" starts with "Table" but the label is not a
			// whole word there — only strip on a word boundary.
			if rest != trimmed && (rest == "" || len(rest) < len(trimmed)-len(labelTrim) || isSeparated(trimmed, len(labelTrim))) {
				return rest
			}
		}
	}

	if seed := englishSeedRe.FindStringSubmatch(trimmed); seed != nil {
		// English seed name under a localized label: keep the number.
		return seed[2]
	}

	for _, word := range englishTypeWords {
		if lower == word {
			return ""
		}
	}

	return trimmed
}

// isSeparated reports whether the rune at idx in s is whitespace, i.e. the
// label consumed a whole word rather than a prefix of a longer one.
func isSeparated(s string, idx int) bool {
	if idx >= len(s) {
		return true
	}
	switch s[idx] {
	case ' ', '\t', '-', '_':
		return true
	}
	return false
}

// EntityName returns the full display name for documents that print the
// location on one line, with the type word applied exactly once.
//
//	EntityName("Table", "Table 1") -> "Table 1"
//	EntityName("Mesa",  "Table 9") -> "Mesa 9"
//	EntityName("Table", "T-4")     -> "Table T-4"
//	EntityName("Table", "")        -> "Table"
func EntityName(label, name string) string {
	labelTrim := strings.TrimSpace(label)
	value := EntityValue(labelTrim, name)
	if value == "" {
		return labelTrim
	}
	if labelTrim == "" {
		return value
	}
	return labelTrim + " " + value
}
