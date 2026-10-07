package director_tools

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/stdevmac/payverge/backend/internal/llm"
)

type fakeTool struct {
	name string
}

func (f *fakeTool) Name() string               { return f.name }
func (f *fakeTool) HumanLabel(_ string) string { return f.name }
func (f *fakeTool) Description() string        { return "fake tool that does " + f.name }
func (f *fakeTool) Schema() *llm.JSONSchema    { return &llm.JSONSchema{Type: llm.TypeObject} }
func (f *fakeTool) Run(_ context.Context, _ map[string]any, _ ToolEnv) (ToolResult, error) {
	return ToolResult{Summary: "ok", Data: map[string]any{"name": f.name}}, nil
}

func TestRegistry_RegisterAndGet(t *testing.T) {
	r := NewRegistry()
	r.Register(&fakeTool{name: "alpha"})
	r.Register(&fakeTool{name: "beta"})

	tool, ok := r.Get("alpha")
	assert.True(t, ok)
	assert.Equal(t, "alpha", tool.Name())

	_, ok = r.Get("nope")
	assert.False(t, ok)

	decls := r.Declarations()
	assert.Len(t, decls, 2)
}

func TestRegistry_RejectsDuplicate(t *testing.T) {
	r := NewRegistry()
	r.Register(&fakeTool{name: "x"})
	assert.Panics(t, func() { r.Register(&fakeTool{name: "x"}) })
}
