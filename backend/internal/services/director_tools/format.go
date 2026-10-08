package director_tools

import (
	"fmt"
	"math"
)

// humanizeMoney formats a dollar amount with thousands separators and two
// decimal places, suitable for both human-facing summaries and the
// structured data we hand back to the model. The leading dollar sign is
// intentionally NOT included — callers prefix their own currency symbol
// or include the value inline ("revenue: $1,234.56").
//
// Examples:
//
//	humanizeMoney(0)        -> "0.00"
//	humanizeMoney(1234.5)   -> "1,234.50"
//	humanizeMoney(-99.999)  -> "-100.00"
func humanizeMoney(amount float64) string {
	if math.IsNaN(amount) || math.IsInf(amount, 0) {
		return "0.00"
	}

	neg := amount < 0
	abs := math.Abs(amount)
	whole := int64(abs)
	cents := int64(math.Round((abs - float64(whole)) * 100))
	if cents == 100 {
		whole++
		cents = 0
	}

	wholeStr := insertThousandsSeparators(whole)
	out := fmt.Sprintf("%s.%02d", wholeStr, cents)
	if neg {
		return "-" + out
	}
	return out
}

// insertThousandsSeparators stringifies a non-negative integer with commas
// every three digits ("1234567" -> "1,234,567").
func insertThousandsSeparators(n int64) string {
	if n < 1000 {
		return fmt.Sprintf("%d", n)
	}
	s := fmt.Sprintf("%d", n)
	// Walk from the right inserting commas every three chars.
	var out []byte
	for i, c := range []byte(s) {
		if i > 0 && (len(s)-i)%3 == 0 {
			out = append(out, ',')
		}
		out = append(out, c)
	}
	return string(out)
}
