package suites_test

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"gopkg.in/yaml.v3"

	"github.com/stdevmac/payverge/backend/internal/llm"
	"github.com/stdevmac/payverge/backend/internal/llm/openrouter"
	"github.com/stdevmac/payverge/backend/internal/llmeval"
)

var prodSuites = []string{
	"allergen_redteam",
	"injection_redteam",
	"locale_adherence",
	"exact_names",
	"json_adherence",
	"director_quality",
}

func TestSuitesWellFormed(t *testing.T) {
	for _, name := range prodSuites {
		name := name
		t.Run(name, func(t *testing.T) {
			raw := mustRead(t, name+".yaml")
			var sf suiteFile
			if err := yaml.Unmarshal(raw, &sf); err != nil {
				t.Fatalf("parse %s: %v", name, err)
			}
			if sf.Name != name {
				t.Fatalf("suite name = %q, want %q", sf.Name, name)
			}
			if sf.DefaultModel == "" {
				t.Fatalf("suite %s missing default_model", name)
			}
			if len(sf.Cases) == 0 {
				t.Fatalf("suite %s has no cases", name)
			}
			seen := map[string]bool{}
			for i, c := range sf.Cases {
				if c.ID == "" {
					t.Fatalf("%s case %d missing id", name, i)
				}
				if seen[c.ID] {
					t.Fatalf("%s duplicate case id %q", name, c.ID)
				}
				seen[c.ID] = true
				if len(c.Assertions) == 0 {
					t.Fatalf("%s case %q has no assertions", name, c.ID)
				}
			}
		})
	}
}

func TestDumpFixtureKeys(t *testing.T) {
	if testing.Short() {
		t.Skip("key dumper is a manual authoring aid")
	}
	for _, name := range prodSuites {
		raw, err := osReadFileMaybe(name + ".yaml")
		if err != nil {
			t.Logf("skip %s (%v)", name, err)
			continue
		}
		var sf suiteFile
		if err := yaml.Unmarshal(raw, &sf); err != nil {
			t.Logf("skip %s parse: %v", name, err)
			continue
		}
		opts := llmeval.Options{DefaultModel: sf.DefaultModel}
		for i := range sf.Cases {
			c := sf.Cases[i]
			key := llmeval.FixtureKey(llmeval.BuildRequest(opts, &c))
			t.Logf("%s/%s -> testdata/fixtures/%s/%s.json", name, c.ID, name, key)
		}
	}
}

func osReadFileMaybe(name string) ([]byte, error) { return os.ReadFile(name) }

// suiteFile mirrors the cmd/llmeval suite shape (name + default model + cases).
type suiteFile struct {
	Name         string         `yaml:"name"`
	DefaultModel string         `yaml:"default_model"`
	JudgeModel   string         `yaml:"judge_model"`
	Cases        []llmeval.Case `yaml:"cases"`
}

// recordingJudgeProvider wraps the fixture provider: on a fixture miss (the
// judge call, which the offline recorder in cmd/llmeval does not capture) it
// falls through to a live provider and WRITES the response as a fixture, so a
// single online run seeds judge-verdict fixtures. Enabled via LLMEVAL_RECORD_JUDGE=1.
type recordingJudgeProvider struct {
	fixtures *llmeval.FixtureProvider
	live     llm.Provider
	dir      string
}

func (r *recordingJudgeProvider) Generate(ctx context.Context, req llm.GenerateRequest) (*llm.Response, error) {
	resp, err := r.fixtures.Generate(ctx, req)
	if err == nil {
		return resp, nil
	}
	if r.live == nil {
		return nil, err
	}
	live, lerr := r.live.Generate(ctx, req)
	if lerr != nil {
		return nil, lerr
	}
	if werr := llmeval.WriteFixture(r.dir, req, live); werr != nil {
		return nil, werr
	}
	return live, nil
}

// TestEvalOffline replays all suites (smoke + prod) against recorded fixtures
// with zero network. This is the CI-hermetic entry point (contract C9).
func TestEvalOffline(t *testing.T) {
	allOffline := append([]string{"smoke_waiter", "smoke_director"}, prodSuites...)
	for _, name := range allOffline {
		name := name
		t.Run(name, func(t *testing.T) {
			raw := mustRead(t, name+".yaml")
			var sf suiteFile
			if err := yaml.Unmarshal(raw, &sf); err != nil {
				t.Fatalf("parse suite: %v", err)
			}
			fixtureDir := filepath.Join("testdata", "fixtures", name)
			provider, err := llmeval.NewFixtureProvider(fixtureDir)
			if err != nil {
				t.Fatalf("fixtures: %v", err)
			}
			var prov llm.Provider = provider
			if os.Getenv("LLMEVAL_RECORD_JUDGE") == "1" {
				if key := os.Getenv("OPENROUTER_API_KEY"); key != "" {
					live, lerr := openrouter.New(openrouter.Config{APIKey: key})
					if lerr == nil {
						prov = &recordingJudgeProvider{fixtures: provider, live: live, dir: fixtureDir}
					}
				}
			}
			cases := make([]*llmeval.Case, 0, len(sf.Cases))
			for i := range sf.Cases {
				cases = append(cases, &sf.Cases[i])
			}
			opts := llmeval.Options{Provider: prov, JudgeModel: sf.JudgeModel, DefaultModel: sf.DefaultModel}
			rep := llmeval.RunSuite(context.Background(), opts, sf.Name, cases)
			if !rep.AllPassed() {
				t.Fatalf("suite %s not all-passed (%d/%d):\n%s", name, rep.Passed, rep.Total, llmeval.RenderReport(rep))
			}
		})
	}
}

func mustRead(t *testing.T, name string) []byte {
	t.Helper()
	raw, err := os.ReadFile(name)
	if err != nil {
		t.Fatalf("read %s: %v", name, err)
	}
	return raw
}
