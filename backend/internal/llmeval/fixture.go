package llmeval

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/stdevmac/payverge/backend/internal/llm"
)

// osStat is an indirection so tests can reference it without importing os.
var osStat = os.Stat

// FixtureKey is a stable 16-hex digest over the salient request fields. It
// intentionally ignores Tools/Temperature/ResponseSchema so that minor
// grading-only knobs do not invalidate a recorded conversation fixture; the
// model + system + ordered (role,text) messages fully determine the prompt.
func FixtureKey(req llm.GenerateRequest) string {
	var b strings.Builder
	b.WriteString(req.Model)
	b.WriteByte('\n')
	b.WriteString(req.System)
	b.WriteByte('\n')
	for _, m := range req.Messages {
		b.WriteString(string(m.Role))
		b.WriteByte('\x1f')
		b.WriteString(m.Text)
		b.WriteByte('\n')
	}
	sum := sha256.Sum256([]byte(b.String()))
	return hex.EncodeToString(sum[:])[:16]
}

// fixtureFile is the on-disk record: the request key + the canned response.
type fixtureFile struct {
	Key      string      `json:"key"`
	Model    string      `json:"model"`
	Response fixtureResp `json:"response"`
}

type fixtureResp struct {
	Text  string    `json:"text"`
	Model string    `json:"model"`
	Usage llm.Usage `json:"usage"`
}

// WriteFixture serializes resp keyed by req into dir as <key>.json.
func WriteFixture(dir string, req llm.GenerateRequest, resp *llm.Response) error {
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return fmt.Errorf("llmeval: mkdir fixtures: %w", err)
	}
	key := FixtureKey(req)
	ff := fixtureFile{
		Key:   key,
		Model: req.Model,
		Response: fixtureResp{
			Text:  resp.Text,
			Model: resp.Model,
			Usage: resp.Usage,
		},
	}
	raw, err := json.MarshalIndent(ff, "", "  ")
	if err != nil {
		return fmt.Errorf("llmeval: marshal fixture: %w", err)
	}
	return os.WriteFile(filepath.Join(dir, key+".json"), raw, 0o644)
}

// FixtureProvider replays recorded responses keyed by FixtureKey. It satisfies
// llm.Provider and never touches the network.
type FixtureProvider struct {
	byKey map[string]fixtureResp
}

// NewFixtureProvider loads every <key>.json fixture under dir.
func NewFixtureProvider(dir string) (*FixtureProvider, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, fmt.Errorf("llmeval: read fixtures %s: %w", dir, err)
	}
	fp := &FixtureProvider{byKey: make(map[string]fixtureResp)}
	for _, e := range entries {
		if e.IsDir() || filepath.Ext(e.Name()) != ".json" {
			continue
		}
		raw, err := os.ReadFile(filepath.Join(dir, e.Name()))
		if err != nil {
			return nil, fmt.Errorf("llmeval: read fixture %s: %w", e.Name(), err)
		}
		var ff fixtureFile
		if err := json.Unmarshal(raw, &ff); err != nil {
			return nil, fmt.Errorf("llmeval: parse fixture %s: %w", e.Name(), err)
		}
		fp.byKey[ff.Key] = ff.Response
	}
	return fp, nil
}

// Generate returns the recorded response for req, or an error on miss.
func (fp *FixtureProvider) Generate(_ context.Context, req llm.GenerateRequest) (*llm.Response, error) {
	key := FixtureKey(req)
	fr, ok := fp.byKey[key]
	if !ok {
		return nil, fmt.Errorf("llmeval: no fixture for key %s (model=%s); re-record with cmd/llmeval -record", key, req.Model)
	}
	return &llm.Response{Text: fr.Text, Model: fr.Model, Usage: fr.Usage}, nil
}
