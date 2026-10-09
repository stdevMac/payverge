// Command llmeval runs a named eval suite against the live OpenRouter provider
// and prints a pass/fail + token-cost table. With -record it instead writes
// fixture files so the offline tests can replay without network.
//
//	OPENROUTER_API_KEY=... go run ./cmd/llmeval -suite smoke_waiter
//	OPENROUTER_API_KEY=... go run ./cmd/llmeval -suite smoke_waiter -record
package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"gopkg.in/yaml.v3"

	"github.com/stdevmac/payverge/backend/internal/llm"
	"github.com/stdevmac/payverge/backend/internal/llm/openrouter"
	"github.com/stdevmac/payverge/backend/internal/llmeval"
)

// suiteFile is the on-disk suite definition (a name + default model + cases).
type suiteFile struct {
	Name         string         `yaml:"name"`
	DefaultModel string         `yaml:"default_model"`
	JudgeModel   string         `yaml:"judge_model"`
	Cases        []llmeval.Case `yaml:"cases"`
}

const suitesDir = "internal/llmeval/suites"

func main() {
	suite := flag.String("suite", "", "suite name under internal/llmeval/suites (without .yaml)")
	record := flag.Bool("record", false, "record live responses as offline fixtures instead of grading")
	flag.Parse()

	if *suite == "" {
		fmt.Fprintln(os.Stderr, "error: -suite is required")
		os.Exit(2)
	}

	sf, err := loadSuiteFile(filepath.Join(suitesDir, *suite+".yaml"))
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(2)
	}

	key := os.Getenv("OPENROUTER_API_KEY")
	if key == "" {
		fmt.Fprintln(os.Stderr, "error: OPENROUTER_API_KEY not set (live run requires it)")
		os.Exit(2)
	}
	provider, err := openrouter.New(openrouter.Config{
		APIKey:  key,
		Referer: os.Getenv("OPENROUTER_APP_URL"),
		Title:   os.Getenv("OPENROUTER_APP_TITLE"),
	})
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: provider: %v\n", err)
		os.Exit(2)
	}

	cases := make([]*llmeval.Case, 0, len(sf.Cases))
	for i := range sf.Cases {
		cases = append(cases, &sf.Cases[i])
	}
	opts := llmeval.Options{Provider: provider, JudgeModel: sf.JudgeModel, DefaultModel: sf.DefaultModel}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()

	if *record {
		if err := recordFixtures(ctx, opts, sf.Name, cases); err != nil {
			fmt.Fprintf(os.Stderr, "error: record: %v\n", err)
			os.Exit(1)
		}
		fmt.Printf("recorded %d fixtures for suite %q\n", len(cases), sf.Name)
		return
	}

	rep := llmeval.RunSuite(ctx, opts, sf.Name, cases)
	fmt.Print(llmeval.RenderReport(rep))
	if !rep.AllPassed() {
		os.Exit(1)
	}
}

func loadSuiteFile(path string) (*suiteFile, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read suite %s: %w", path, err)
	}
	var sf suiteFile
	if err := yaml.Unmarshal(raw, &sf); err != nil {
		return nil, fmt.Errorf("parse suite %s: %w", path, err)
	}
	if sf.Name == "" {
		return nil, fmt.Errorf("suite %s missing name", path)
	}
	return &sf, nil
}

// recordFixtures runs each case live and writes a replay fixture keyed by the
// exact request the runner would build, so offline tests resolve them.
func recordFixtures(ctx context.Context, opts llmeval.Options, suite string, cases []*llmeval.Case) error {
	dir := filepath.Join(suitesDir, "testdata", "fixtures", suite)
	for _, c := range cases {
		req := llmeval.BuildRequest(opts, c)
		resp, err := opts.Provider.Generate(ctx, req)
		if err != nil {
			return fmt.Errorf("case %s: %w", c.ID, err)
		}
		if err := llmeval.WriteFixture(dir, req, resp); err != nil {
			return fmt.Errorf("case %s: write fixture: %w", c.ID, err)
		}
	}
	return nil
}

var _ = llm.RoleUser // ensure llm import is used even if record path changes
