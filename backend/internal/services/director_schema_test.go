package services

import (
	"context"
	"testing"

	"github.com/stdevmac/payverge/backend/internal/database"
	"github.com/stdevmac/payverge/backend/internal/llm"
	"github.com/stdevmac/payverge/backend/internal/services/director_tools"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestDirectorResponseSchema_MirrorsStruct(t *testing.T) {
	sch := directorResponseSchema()
	require.Equal(t, llm.TypeObject, sch.Type)
	for _, key := range []string{"summary", "diagnosis", "evidence", "actions", "expected_impact", "follow_ups"} {
		_, ok := sch.Properties[key]
		assert.Truef(t, ok, "schema missing property %q", key)
	}
	actions := sch.Properties["actions"]
	require.NotNil(t, actions)
	require.Equal(t, llm.TypeArray, actions.Type)
	require.NotNil(t, actions.Items)
	for _, key := range []string{"title", "description", "deep_link", "priority"} {
		_, ok := actions.Items.Properties[key]
		assert.Truef(t, ok, "action item schema missing %q", key)
	}
}

func TestRunDirectorLoop_FinalTurnUsesSchemaNoTools(t *testing.T) {
	db := newServiceTestDB(t)
	business := createTestBusinessForService(t, db, "Schema Restaurant")
	thread, err := database.CreateDirectorConsoleThread(business.ID, "schema test", "en")
	require.NoError(t, err)

	reg := director_tools.NewRegistry()
	reg.Register(&director_tools.BusinessProfileTool{})

	clean := `{"summary":"ok","diagnosis":"ok","evidence":[],"actions":[],"expected_impact":"","follow_ups":[]}`
	cap := &capturingDirectorProvider{scripted: []*llm.Response{
		newFunctionCallResponse("get_business_profile", map[string]any{}),
		newTextResponse(clean), // tool-allowed probe after tool execution
		newTextResponse(clean), // schema finalization
	}}

	res, err := RunDirectorLoop(context.Background(), LoopConfig{
		Registry: reg, Provider: cap, Model: "test",
		SystemPrompt: "sys", UserPrompt: "hi",
		Env: director_tools.ToolEnv{BusinessID: business.ID, ThreadID: thread.ID, Locale: "en", DB: db},
	})
	require.NoError(t, err)
	require.Equal(t, "ok", res.Final.Summary)
	require.Len(t, cap.requests, 3)

	final := cap.requests[len(cap.requests)-1]
	assert.NotNil(t, final.ResponseSchema, "final turn must set ResponseSchema")
	assert.Empty(t, final.Tools, "final turn must omit Tools")
}

func TestParseDirectorJSON_FencedAndProse(t *testing.T) {
	clean := `{"summary":"a","diagnosis":"b","evidence":[],"actions":[],"expected_impact":"","follow_ups":[]}`
	fenced := "```json\n" + clean + "\n```"
	prose := "Here is the plan:\n" + clean + "\nThanks!"
	for _, in := range []string{clean, fenced, prose} {
		got, err := parseDirectorJSON(in)
		require.NoError(t, err)
		assert.Equal(t, "a", got.Summary)
	}
}

func TestRunDirectorLoop_RequestCarriesFeatureMaxTokensTemperature(t *testing.T) {
	db := newServiceTestDB(t)
	business := createTestBusinessForService(t, db, "C1 Restaurant")
	thread, err := database.CreateDirectorConsoleThread(business.ID, "c1", "en")
	require.NoError(t, err)
	reg := director_tools.NewRegistry()
	final := `{"summary":"ok","diagnosis":"ok","evidence":[],"actions":[],"expected_impact":"","follow_ups":[]}`
	cap := &capturingDirectorProvider{scripted: []*llm.Response{newTextResponse(final), newTextResponse(final)}}
	_, err = RunDirectorLoop(context.Background(), LoopConfig{
		Registry: reg, Provider: cap, Model: "test", SystemPrompt: "sys", UserPrompt: "hi",
		Env: director_tools.ToolEnv{BusinessID: business.ID, ThreadID: thread.ID, Locale: "en", DB: db},
	})
	require.NoError(t, err)
	require.NotEmpty(t, cap.requests)
	for i, r := range cap.requests {
		assert.Equalf(t, "director", r.Feature, "request %d Feature", i)
		assert.Equalf(t, 4096, r.MaxTokens, "request %d MaxTokens", i)
		require.NotNilf(t, r.Temperature, "request %d Temperature", i)
		assert.InDeltaf(t, 0.2, float64(*r.Temperature), 0.001, "request %d Temperature value", i)
	}
}
