// Package main is a small CLI that compares two benchstat outputs and prints
// the top 5 movers as a markdown issue body. Usage:
//
//	go run ./perf/drift/compare old.txt new.txt > drift.md
package main

import (
	"bufio"
	"fmt"
	"math"
	"os"
	"sort"
	"strconv"
	"strings"
)

// benchResult holds a single bench's measurements.
type benchResult struct {
	Name        string
	NsPerOp     float64
	BytesPerOp  float64
	AllocsPerOp float64
}

func main() {
	if len(os.Args) != 3 {
		fmt.Fprintln(os.Stderr, "usage: compare <old.txt> <new.txt>")
		os.Exit(2)
	}

	old, err := parseBenches(os.Args[1])
	if err != nil {
		fmt.Fprintf(os.Stderr, "parse old: %v\n", err)
		os.Exit(1)
	}
	cur, err := parseBenches(os.Args[2])
	if err != nil {
		fmt.Fprintf(os.Stderr, "parse new: %v\n", err)
		os.Exit(1)
	}

	type delta struct {
		Name string
		Pct  float64
	}
	var deltas []delta
	for name, b := range cur {
		o, ok := old[name]
		if !ok || o.NsPerOp == 0 {
			continue
		}
		pct := (b.NsPerOp - o.NsPerOp) / o.NsPerOp * 100
		if math.IsNaN(pct) || math.IsInf(pct, 0) {
			continue
		}
		deltas = append(deltas, delta{Name: name, Pct: pct})
	}

	// Sort by absolute percent change, descending.
	sort.Slice(deltas, func(i, j int) bool {
		return math.Abs(deltas[i].Pct) > math.Abs(deltas[j].Pct)
	})

	w := bufio.NewWriter(os.Stdout)
	defer w.Flush()

	fmt.Fprintln(w, "# Perf drift")
	fmt.Fprintln(w)
	fmt.Fprintln(w, "Top 5 movers vs. baseline (sorted by absolute % change in ns/op):")
	fmt.Fprintln(w)
	fmt.Fprintln(w, "| Bench | Old ns/op | New ns/op | % Δ |")
	fmt.Fprintln(w, "|---|---:|---:|---:|")
	for i, d := range deltas {
		if i == 5 {
			break
		}
		o := old[d.Name]
		c := cur[d.Name]
		sign := "+"
		if d.Pct < 0 {
			sign = ""
		}
		fmt.Fprintf(w, "| `%s` | %.0f | %.0f | %s%.1f%% |\n",
			d.Name, o.NsPerOp, c.NsPerOp, sign, d.Pct)
	}
	fmt.Fprintln(w)
	fmt.Fprintln(w, "_Compares two `go test -bench` runs: the first argument is the baseline, ")
	fmt.Fprintln(w, "the second the candidate._")
}

// parseBenches reads raw `go test -bench` output (NOT benchstat output — we
// want the per-iter line itself) and returns a map of bench name to averaged
// measurements. If the same bench appears multiple times (count>1), values
// are averaged.
//
// Expected line shape:
//
//	BenchmarkFoo-8  1000   12345 ns/op  6789 B/op  10 allocs/op
//
// Anything not matching that shape is skipped.
func parseBenches(path string) (map[string]benchResult, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	type accum struct {
		ns, bytes, allocs float64
		n                 int
	}
	bag := map[string]*accum{}

	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if !strings.HasPrefix(line, "Benchmark") {
			continue
		}
		fields := strings.Fields(line)
		// At minimum: name, count, ns/op number, "ns/op" label
		if len(fields) < 4 {
			continue
		}
		name := fields[0]
		// Strip the trailing GOMAXPROCS suffix (-8) so old/new can match if
		// they ran on different CPU counts. Cap at the last hyphen + digits.
		if idx := strings.LastIndex(name, "-"); idx > 0 {
			suf := name[idx+1:]
			if _, err := strconv.Atoi(suf); err == nil {
				name = name[:idx]
			}
		}

		a := bag[name]
		if a == nil {
			a = &accum{}
			bag[name] = a
		}

		// Walk pairs (value, label) to extract ns/op, B/op, allocs/op regardless
		// of column order. -benchmem typically emits ns/op, B/op, allocs/op.
		for i := 2; i+1 < len(fields); i += 2 {
			val, err := strconv.ParseFloat(fields[i], 64)
			if err != nil {
				continue
			}
			switch fields[i+1] {
			case "ns/op":
				a.ns += val
			case "B/op":
				a.bytes += val
			case "allocs/op":
				a.allocs += val
			}
		}
		a.n++
	}
	if err := scanner.Err(); err != nil {
		return nil, err
	}

	out := map[string]benchResult{}
	for name, a := range bag {
		if a.n == 0 {
			continue
		}
		out[name] = benchResult{
			Name:        name,
			NsPerOp:     a.ns / float64(a.n),
			BytesPerOp:  a.bytes / float64(a.n),
			AllocsPerOp: a.allocs / float64(a.n),
		}
	}
	return out, nil
}
