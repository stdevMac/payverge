package director_tools

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

// allProductionTools lists one instance of every registered tool. Name/
// Description/HumanLabel/Schema are pure and need no provider wiring.
func allProductionTools() []Tool {
	return []Tool{
		&BusinessProfileTool{}, &RevenueSummaryTool{}, &OrderFunnelTool{},
		&MenuTopItemsTool{}, &MenuUnderperformersTool{}, &SlowDaypartsTool{},
		&CRMSegmentTool{}, &ReservationLoadTool{}, &AIWaiterPerformanceTool{},
		&PluginStatusTool{},
		&LiveFloorTool{}, &KitchenStatusTool{}, &PromosTool{},
	}
}

func TestAllTools_DescriptionsAreModelFacing(t *testing.T) {
	for _, tool := range allProductionTools() {
		desc := tool.Description()
		assert.NotEmpty(t, desc, tool.Name())
		assert.NotEqual(t, tool.HumanLabel("en"), desc,
			"%s: model description must differ from the UI pill", tool.Name())
		assert.GreaterOrEqual(t, len(desc), 40,
			"%s: description too short to guide tool choice", tool.Name())
	}
}

func TestDeclarations_UseDescriptionNotPill(t *testing.T) {
	r := NewRegistry()
	r.Register(&RevenueSummaryTool{})
	decls := r.Declarations()
	assert.Len(t, decls, 1)
	assert.Equal(t, (&RevenueSummaryTool{}).Description(), decls[0].Description)
}
