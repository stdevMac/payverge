//go:build integration_postgres

package database

import (
	"fmt"
	"sort"
	"strings"
)

// DiffFingerprints returns "" when a and b are equal. Otherwise it returns a
// human-readable description of the FIRST differing dimension (by name) with a
// line-oriented unified-style diff of that dimension's values.
func DiffFingerprints(a, b map[string]string) string {
	keys := make(map[string]struct{}, len(a)+len(b))
	for k := range a {
		keys[k] = struct{}{}
	}
	for k := range b {
		keys[k] = struct{}{}
	}
	sorted := make([]string, 0, len(keys))
	for k := range keys {
		sorted = append(sorted, k)
	}
	sort.Strings(sorted)

	for _, dim := range sorted {
		av, aok := a[dim]
		bv, bok := b[dim]
		if aok && bok && av == bv {
			continue
		}
		var bld strings.Builder
		fmt.Fprintf(&bld, "dimension %q differs\n", dim)
		if !aok {
			fmt.Fprintf(&bld, "  only in b (missing in a)\n")
		}
		if !bok {
			fmt.Fprintf(&bld, "  only in a (missing in b)\n")
		}
		bld.WriteString(unifiedLineDiff(av, bv))
		return bld.String()
	}
	return ""
}

// unifiedLineDiff is a minimal line-oriented diff (not a full LCS). Equal lines
// are omitted; only insertions/deletions are emitted so test failures stay
// actionable when dimensions contain hundreds of catalog rows.
func unifiedLineDiff(a, b string) string {
	aLines := splitLines(a)
	bLines := splitLines(b)
	var bld strings.Builder
	bld.WriteString("--- a\n+++ b\n")
	i, j := 0, 0
	changed := 0
	for i < len(aLines) || j < len(bLines) {
		if i < len(aLines) && j < len(bLines) && aLines[i] == bLines[j] {
			i++
			j++
			continue
		}
		// Prefer delete when a's current line appears later in b (insertion in b).
		if i < len(aLines) && !lineIn(bLines, j, aLines[i], 8) {
			fmt.Fprintf(&bld, "-%s\n", aLines[i])
			i++
			changed++
			continue
		}
		if j < len(bLines) && !lineIn(aLines, i, bLines[j], 8) {
			fmt.Fprintf(&bld, "+%s\n", bLines[j])
			j++
			changed++
			continue
		}
		// Both present nearby but out of order — emit both.
		if i < len(aLines) {
			fmt.Fprintf(&bld, "-%s\n", aLines[i])
			i++
			changed++
		}
		if j < len(bLines) {
			fmt.Fprintf(&bld, "+%s\n", bLines[j])
			j++
			changed++
		}
	}
	if changed == 0 {
		bld.WriteString("(no line-level delta; empty-vs-missing or whitespace-only)\n")
	}
	return bld.String()
}

func splitLines(s string) []string {
	if s == "" {
		return nil
	}
	return strings.Split(s, "\n")
}

func lineIn(lines []string, start int, target string, window int) bool {
	end := start + window
	if end > len(lines) {
		end = len(lines)
	}
	for k := start; k < end; k++ {
		if lines[k] == target {
			return true
		}
	}
	return false
}
