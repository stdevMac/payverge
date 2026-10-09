package money

import (
	"fmt"
	"strconv"
	"strings"
)

// ParseDollarStringToCents converts a dollar-amount string (e.g. "99.99") to
// integer cents (9999). It rejects amounts with more than two decimal places,
// empty strings, and non-numeric input.
func ParseDollarStringToCents(amount string) (int64, error) {
	raw := strings.TrimSpace(amount)
	if raw == "" {
		return 0, fmt.Errorf("amount is required")
	}

	sign := int64(1)
	if strings.HasPrefix(raw, "-") {
		sign = -1
		raw = strings.TrimPrefix(raw, "-")
	} else if strings.HasPrefix(raw, "+") {
		raw = strings.TrimPrefix(raw, "+")
	}

	parts := strings.Split(raw, ".")
	if len(parts) > 2 {
		return 0, fmt.Errorf("invalid amount format")
	}

	whole := parts[0]
	if whole == "" {
		whole = "0"
	}
	if !isDigitsOnly(whole) {
		return 0, fmt.Errorf("invalid amount format")
	}

	fraction := ""
	if len(parts) == 2 {
		fraction = parts[1]
		if fraction == "" || !isDigitsOnly(fraction) {
			return 0, fmt.Errorf("invalid amount format")
		}
		if len(fraction) > 2 {
			return 0, fmt.Errorf("dollar amount must use cents precision")
		}
	}
	fraction = fraction + strings.Repeat("0", 2-len(fraction))

	cents, err := strconv.ParseInt(whole+fraction, 10, 64)
	if err != nil {
		return 0, fmt.Errorf("invalid amount format")
	}
	return sign * cents, nil
}

// Float64ToCents converts a float64 dollar amount to integer cents by
// formatting to two decimal places first (avoiding IEEE-754 drift).
func Float64ToCents(amount float64) (int64, error) {
	return ParseDollarStringToCents(fmt.Sprintf("%.2f", amount))
}

func isDigitsOnly(value string) bool {
	for _, c := range value {
		if c < '0' || c > '9' {
			return false
		}
	}
	return len(value) > 0
}
