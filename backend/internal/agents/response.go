package agents

// ActionLink is a CTA rendered in chat UIs.
type ActionLink struct {
	Label          string `json:"label"`
	Href           string `json:"href"`
	Target         string `json:"target,omitempty"` // legacy model output; normalized to href
	Kind           string `json:"kind"`             // navigate | external | handoff
	Disabled       bool   `json:"disabled"`
	DisabledReason string `json:"disabled_reason,omitempty"`
}

// WorkflowProgress tracks multi-step guided flows.
type WorkflowProgress struct {
	ID        string `json:"id"`
	StepIndex int    `json:"step_index"`
	StepTotal int    `json:"step_total"`
}

// StructuredResponse is the Ops Assistant structured reply.
type StructuredResponse struct {
	Answer    string            `json:"answer"`
	Steps     []string          `json:"steps"`
	Actions   []ActionLink      `json:"actions"`
	FollowUps []string          `json:"follow_ups"`
	Workflow  *WorkflowProgress `json:"workflow,omitempty"`
}
