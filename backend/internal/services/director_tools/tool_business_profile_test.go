package director_tools

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestBusinessProfileTool_Name(t *testing.T) {
	tool := &BusinessProfileTool{}
	assert.Equal(t, "get_business_profile", tool.Name())
}

func TestBusinessProfileTool_HumanLabel(t *testing.T) {
	tool := &BusinessProfileTool{}
	assert.NotEmpty(t, tool.HumanLabel("en"))
	assert.NotEmpty(t, tool.HumanLabel("es"))
}

func TestBusinessProfileTool_Schema(t *testing.T) {
	tool := &BusinessProfileTool{}
	schema := tool.Schema()
	require.NotNil(t, schema)
	// No arguments — caller passes {} and we read everything off ToolEnv.
}

func TestBusinessProfileTool_Run(t *testing.T) {
	db := newTestDB(t)
	bizID := createTestBusiness(t, db, "The Iron Skillet")

	env := ToolEnv{
		BusinessID: bizID,
		Locale:     "en",
		DB:         db,
	}

	tool := &BusinessProfileTool{}
	result, err := tool.Run(context.Background(), map[string]any{}, env)
	require.NoError(t, err)

	assert.Equal(t, "The Iron Skillet", result.Data["name"])
	assert.Equal(t, "America/New_York", result.Data["timezone"])
	assert.NotContains(t, result.Data, "subscription_plan", "no plans exist; the profile must not report one")
	assert.Equal(t, "Sage", result.Data["assistant_name"])
	assert.NotEmpty(t, result.Summary)
}

func TestBusinessProfileTool_Run_MissingBusiness(t *testing.T) {
	db := newTestDB(t)

	env := ToolEnv{
		BusinessID: 9999, // does not exist
		Locale:     "en",
		DB:         db,
	}

	tool := &BusinessProfileTool{}
	_, err := tool.Run(context.Background(), map[string]any{}, env)
	require.Error(t, err)
}

// TestBusinessProfileTool_ReturnsVenueLocalTime — #902. The tool's description
// promised local time but only ever returned an IANA name, so a model asked
// "what time is it" fell back to the snapshot's UTC stamp and reported
// 20:00 UTC as "8:00 PM New York". It must hand back the resolved clock.
func TestBusinessProfileTool_ReturnsVenueLocalTime(t *testing.T) {
	db := newTestDB(t)
	bizID := createTestBusiness(t, db, "Still Water Lounge")

	result, err := (&BusinessProfileTool{}).Run(context.Background(), map[string]any{}, ToolEnv{
		BusinessID: bizID,
		Locale:     "en",
		DB:         db,
	})
	require.NoError(t, err)

	ny, err := time.LoadLocation("America/New_York")
	require.NoError(t, err)
	now := time.Now().In(ny)

	assert.Equal(t, now.Format("3:04 PM"), result.Data["local_time"],
		"the venue clock must be resolved here, not left for the model to derive")
	assert.Equal(t, now.Weekday().String(), result.Data["local_weekday"])
	assert.Equal(t, now.Format("-07:00"), result.Data["utc_offset"])
	assert.NotEqual(t, time.Now().UTC().Format("3:04 PM"), result.Data["local_time"],
		"New York is never on UTC; a UTC clock here is the reported bug")
	assert.Contains(t, result.Summary, "local")

	// The tool must not start advertising something it does not return.
	assert.Contains(t, (&BusinessProfileTool{}).Description(), "local_time")
}
