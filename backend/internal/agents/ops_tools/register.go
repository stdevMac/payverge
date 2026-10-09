package ops_tools

import "github.com/stdevmac/payverge/backend/internal/agents"

// NewRegistry registers all Ops Assistant tools.
func NewRegistry() *agents.Registry {
	r := agents.NewRegistry()
	r.Register(&BusinessContextTool{})
	r.Register(&SetupStatusTool{})
	r.Register(&ActiveTabHelpTool{})
	r.Register(&NavigateToTabTool{})
	r.Register(&NavigateToSettingsTool{})
	r.Register(&ListAccessibleTabsTool{})
	r.Register(&ExplainLockedFeatureTool{})
	r.Register(&SearchOperatorHelpTool{})
	r.Register(&PluginStatusTool{})
	r.Register(&BusinessProfileAdapter{})
	r.Register(&WorkflowTool{})
	r.Register(&DelegateToDirectorTool{})
	r.Register(&SupportEscalationTool{})
	return r
}
