package handlers

import "strconv"

// ClampLimit parses the "limit" query param, falls back to def when
// missing/unparseable/non-positive, and caps the result at max.
//
// Centralizing this prevents callers from passing arbitrarily large
// limits that would let a client exhaust DB/memory by asking for
// millions of rows.
func ClampLimit(raw string, def, max int) int {
	n, err := strconv.Atoi(raw)
	if err != nil || n <= 0 {
		n = def
	}
	if n > max {
		n = max
	}
	return n
}
