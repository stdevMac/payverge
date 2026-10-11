package agents

import (
	"context"
	"sync"
	"testing"

	"github.com/stdevmac/payverge/backend/internal/llm"
)

type stubTool struct{ name string }

func (s stubTool) Name() string             { return s.name }
func (s stubTool) HumanLabel(string) string { return s.name }
func (s stubTool) Description() string      { return "stub" }
func (s stubTool) Schema() *llm.JSONSchema  { return &llm.JSONSchema{Type: "object"} }
func (s stubTool) Run(context.Context, map[string]any, ToolEnv) (ToolResult, error) {
	return ToolResult{Summary: "ok"}, nil
}

func TestRegistryDeclarations_concurrentRLock(t *testing.T) {
	reg := NewRegistry()
	reg.Register(stubTool{name: "alpha"})
	reg.Register(stubTool{name: "beta"})

	var wg sync.WaitGroup
	for i := 0; i < 32; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			decls := reg.Declarations()
			if len(decls) != 2 {
				t.Errorf("declarations=%d want 2", len(decls))
			}
		}()
	}
	wg.Wait()
}
