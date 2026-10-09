// Command subjectlift lifts each email template's in-<body> <title>...</title>
// into a sibling {{ define "subject" }}...{{ end }} block prepended before
// {{ define "content" }}, and deletes the <title> from the body. Idempotent.
//
// Run from backend/:  go run ./scripts/emailcodemod/subjectlift
package main

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

var (
	titleRe   = regexp.MustCompile(`(?is)<title>\s*(.*?)\s*</title>\s*`)
	contentRe = regexp.MustCompile(`{{\s*define\s+"content"\s*}}`)
)

func main() {
	root := "email/templates"
	families, err := os.ReadDir(root)
	if err != nil {
		fmt.Fprintln(os.Stderr, "read templates root:", err)
		os.Exit(1)
	}
	changed, skipped := 0, 0
	for _, fam := range families {
		if !fam.IsDir() {
			continue
		}
		dir := filepath.Join(root, fam.Name())
		entries, err := os.ReadDir(dir)
		if err != nil {
			fmt.Fprintln(os.Stderr, "read", dir, err)
			os.Exit(1)
		}
		for _, e := range entries {
			if e.IsDir() || filepath.Ext(e.Name()) != ".html" {
				continue
			}
			path := filepath.Join(dir, e.Name())
			b, err := os.ReadFile(path)
			if err != nil {
				fmt.Fprintln(os.Stderr, "read", path, err)
				os.Exit(1)
			}
			src := string(b)
			if strings.Contains(src, `define "subject"`) {
				skipped++
				continue
			}
			m := titleRe.FindStringSubmatch(src)
			if m == nil {
				skipped++
				continue
			}
			subject := strings.TrimSpace(m[1])
			body := titleRe.ReplaceAllString(src, "")
			loc := contentRe.FindStringIndex(body)
			if loc == nil {
				fmt.Fprintf(os.Stderr, "no content define in %s\n", path)
				os.Exit(1)
			}
			define := fmt.Sprintf("{{ define \"subject\" }}%s{{ end }}\n", subject)
			out := body[:loc[0]] + define + body[loc[0]:]
			if err := os.WriteFile(path, []byte(out), 0o644); err != nil {
				fmt.Fprintln(os.Stderr, "write", path, err)
				os.Exit(1)
			}
			changed++
		}
	}
	fmt.Printf("subjectlift: changed=%d skipped=%d\n", changed, skipped)
}
