package logger

import (
	"bufio"
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"sort"
	"strconv"
	"strings"
	"testing"
)

// TestNotificationLogsDoNotInterpolateRawPII is a source gate (in the spirit
// of TestAutoMigrateSourceGate) covering the whole backend, not just the
// handful of files named in issue #552: it walks every *.go file under
// backend/internal/ and backend/cmd/app/ and fails if any stdlib log.Printf /
// logger.Logger.* / logrus.With*|Fields call interpolates a raw email,
// phone, or push-subscription-endpoint value.
//
// A "log call" is joined across lines by paren-depth so multi-line calls
// (format string on one line, args on the next; logrus.WithFields{...}
// blocks) are checked as one statement, not line-by-line — a raw
// `user.Email` passed as a continuation-line argument is caught even though
// the line itself doesn't contain "log.Printf(".
//
// Allowed: pass the value through logger.RedactEmail / RedactEmails /
// emailDomainForLog / maskEmail / maskEmailForDisplay / endpointHost, or log
// a count (recipient_count) instead of the raw value(s).
//
// backend/perf/seed/** is intentionally out of scope: it is a dev-only
// `package main` seeding tool, not compiled into cmd/app.
var (
	// logTriggerRe finds the opening call of a logging statement so we know
	// where to start paren-depth tracking for multi-line joins.
	logTriggerRe = regexp.MustCompile(`log\.Print(f|ln)?\(|logger\.Logger\.\w+\(|logrus\.\w+\(`)

	// quotedStringRe strips string-literal *contents* (keeping empty quotes)
	// so neither paren-depth counting nor PII matching is confused by
	// parentheses or field-name-shaped text inside a human-readable message.
	quotedStringRe = regexp.MustCompile("\"(?:[^\"\\\\]|\\\\.)*\"|`[^`]*`")

	// barePIIRe matches a Go identifier/selector that is (or plausibly is) a
	// raw email address, phone number, or push endpoint, once string
	// literals have been stripped from the line. The [Ee]/[Pp] case
	// alternation catches both bare lowercase locals (`email`, `phone`) and
	// camelCase ones (`onboardingEmail`, `customerPhone`) as well as
	// exported field selectors (`user.Email`, `reservation.CustomerPhone`).
	barePIIRe = regexp.MustCompile(`\.Endpoint\b|\b[A-Za-z0-9_]*[Ee]mail\b|\b[A-Za-z0-9_]*[Pp]hone\b`)

	// rawToFieldRe matches a logrus.Fields{"to": to} recipient-slice leak.
	// Checked against the raw (unstripped) buffer since it depends on the
	// literal quoted "to" key.
	rawToFieldRe = regexp.MustCompile(`"to"\s*:\s*to\b`)

	// redactAllowRe suppresses matches on lines that already route the value
	// through a known-safe redaction helper, or that log a count instead.
	redactAllowRe = regexp.MustCompile(`RedactEmail|RedactEmails|emailDomainForLog|maskEmailForDisplay|maskEmail|endpointHost|recipient_count`)
)

// pathHasTriggerSubstring is a cheap pre-filter so most of the ~2,200
// backend .go files never touch the regexp engine.
func pathHasTriggerSubstring(line string) bool {
	return strings.Contains(line, "log.Print") ||
		strings.Contains(line, "logger.Logger.") ||
		strings.Contains(line, "logrus.")
}

func TestNotificationLogsDoNotInterpolateRawPII(t *testing.T) {
	root := notificationSourceGateBackendRoot(t)

	var files []string
	for _, sub := range []string{"internal", "cmd/app"} {
		dir := filepath.Join(root, sub)
		walkErr := filepath.Walk(dir, func(path string, info os.FileInfo, err error) error {
			if err != nil {
				return err
			}
			if info.IsDir() {
				return nil
			}
			if strings.HasSuffix(path, ".go") {
				files = append(files, path)
			}
			return nil
		})
		if walkErr != nil {
			t.Fatalf("walk %s: %v", dir, walkErr)
		}
	}
	sort.Strings(files)

	var violations []string
	for _, path := range files {
		rel, _ := filepath.Rel(root, path)
		violations = append(violations, scanFileForRawPII(t, path, rel)...)
	}

	if len(violations) > 0 {
		t.Fatalf("log statements interpolate raw PII (%d):\n  %s\n\nPass the value through logger.RedactEmail (or RedactEmails / emailDomainForLog / maskEmail / maskEmailForDisplay / endpointHost), or log a count (e.g. recipient_count) instead.",
			len(violations), strings.Join(violations, "\n  "))
	}
}

// scanFileForRawPII reads one file and returns "path:line: snippet" entries
// for every logging statement (joined across lines by paren depth) that
// interpolates a raw PII-shaped value without going through an allowed
// redaction helper.
func scanFileForRawPII(t *testing.T, path, rel string) []string {
	t.Helper()

	f, err := os.Open(path)
	if err != nil {
		t.Fatalf("open %s: %v", rel, err)
	}
	defer func() { _ = f.Close() }()

	scanner := bufio.NewScanner(f)
	buf := make([]byte, 0, 64*1024)
	scanner.Buffer(buf, 1024*1024)

	var lines []string
	for scanner.Scan() {
		lines = append(lines, scanner.Text())
	}
	if err := scanner.Err(); err != nil {
		t.Fatalf("scan %s: %v", rel, err)
	}

	var violations []string
	n := len(lines)
	for i := 0; i < n; i++ {
		line := lines[i]
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, "//") {
			continue
		}
		if !pathHasTriggerSubstring(line) || !logTriggerRe.MatchString(line) {
			continue
		}

		// Join continuation lines until the call's parens balance back out,
		// counting depth on the quote-stripped text so a literal '(' or ')'
		// inside a human-readable message can't desync the count.
		rawBuf := line
		strippedBuf := quotedStringRe.ReplaceAllString(line, `""`)
		depth := strings.Count(strippedBuf, "(") - strings.Count(strippedBuf, ")")
		endLine := i
		for depth > 0 && endLine+1 < n {
			endLine++
			next := lines[endLine]
			rawBuf += "\n" + next
			strippedNext := quotedStringRe.ReplaceAllString(next, `""`)
			strippedBuf += "\n" + strippedNext
			depth += strings.Count(strippedNext, "(") - strings.Count(strippedNext, ")")
		}

		isViolation := barePIIRe.MatchString(strippedBuf) || rawToFieldRe.MatchString(rawBuf)
		if isViolation && !redactAllowRe.MatchString(rawBuf) {
			snippet := strings.Join(strings.Fields(rawBuf), " ")
			if len(snippet) > 200 {
				snippet = snippet[:200] + "..."
			}
			violations = append(violations, rel+":"+strconv.Itoa(i+1)+": "+snippet)
		}

		i = endLine
	}
	return violations
}

func notificationSourceGateBackendRoot(t *testing.T) string {
	t.Helper()
	_, thisFile, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("runtime.Caller failed")
	}
	root := filepath.Clean(filepath.Join(filepath.Dir(thisFile), "..", ".."))
	if _, err := os.Stat(filepath.Join(root, "go.mod")); err != nil {
		t.Fatalf("backend root %s has no go.mod: %v", root, err)
	}
	return root
}
