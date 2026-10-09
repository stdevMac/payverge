package logger

import "strings"

// RedactEmail masks the local part of an email address, keeping only the
// first character so logs can still be correlated without leaking full PII.
// Returns "***" for empty or unparseable input.
func RedactEmail(email string) string {
	parts := strings.SplitN(email, "@", 2)
	if len(parts) != 2 || len(parts[0]) == 0 {
		return "***"
	}
	return string(parts[0][0]) + "***@" + parts[1]
}

// RedactEmails redacts each address and joins them with commas for log lines.
func RedactEmails(emails []string) string {
	if len(emails) == 0 {
		return ""
	}
	parts := make([]string, len(emails))
	for i, email := range emails {
		parts[i] = RedactEmail(email)
	}
	return strings.Join(parts, ",")
}
