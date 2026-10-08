// Command colorsweep rebrands inline colors in email content templates:
// button backgrounds (background[-color]:#111827) become brand teal #1a6b6a,
// every other #111827 becomes charcoal #1c1917,
// secondary/label gray #6b7280 becomes muted #6b6b6b,
// table-divider gray #e5e7eb becomes warm divider #ece9e2. Idempotent.
//
// Run from backend/:  go run ./scripts/emailcodemod/colorsweep
package main

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

var buttonBgRe = regexp.MustCompile(`(background(?:-color)?:\s*)#111827`)

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
			out := buttonBgRe.ReplaceAllString(src, "${1}#1a6b6a")
			out = strings.ReplaceAll(out, "#111827", "#1c1917")
			out = strings.ReplaceAll(out, "#6b7280", "#6b6b6b")
			out = strings.ReplaceAll(out, "#e5e7eb", "#ece9e2")
			if out != src {
				if err := os.WriteFile(path, []byte(out), 0o644); err != nil {
					fmt.Fprintln(os.Stderr, "write", path, err)
					os.Exit(1)
				}
				changed++
			} else {
				skipped++
			}
		}
	}
	fmt.Printf("colorsweep: changed=%d skipped=%d\n", changed, skipped)
}
