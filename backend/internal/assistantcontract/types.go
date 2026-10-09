package assistantcontract

type Status string

const (
	StatusComplete           Status = "complete"
	StatusNeedsClarification Status = "needs_clarification"
	StatusBlocked            Status = "blocked"
	StatusDegraded           Status = "degraded"
)

type AnswerFormat string

const (
	FormatMarkdown  AnswerFormat = "markdown"
	FormatPlainText AnswerFormat = "plain_text"
)

type Answer struct {
	Format  AnswerFormat `json:"format"`
	Content string       `json:"content"`
}

type Section struct {
	ID        string   `json:"id"`
	Title     string   `json:"title"`
	Answer    string   `json:"answer"`
	Steps     []string `json:"steps"`
	ActionIDs []string `json:"action_ids"`
	SourceIDs []string `json:"source_ids"`
	EntityIDs []string `json:"entity_ids"`
}

type ActionTarget struct {
	Kind     string  `json:"kind"`
	ID       string  `json:"id"`
	Href     string  `json:"href"`
	Quantity *int    `json:"quantity,omitempty"`
	Notes    *string `json:"notes,omitempty"`
}

type Action struct {
	ID             string       `json:"id"`
	Type           string       `json:"type"`
	Label          string       `json:"label"`
	Target         ActionTarget `json:"target"`
	State          string       `json:"state"`
	Confirmation   string       `json:"confirmation"`
	DisabledReason *string      `json:"disabled_reason"`
	ExpiresAt      *string      `json:"expires_at"`
}

type Source struct {
	ID          string  `json:"id"`
	Type        string  `json:"type"`
	Title       string  `json:"title"`
	Href        *string `json:"href"`
	Origin      string  `json:"origin"`
	RetrievedAt string  `json:"retrieved_at"`
}

type Entity struct {
	ID           string `json:"id"`
	Type         string `json:"type"`
	DisplayName  string `json:"display_name"`
	Availability string `json:"availability"`
	SourceID     string `json:"source_id"`
}

type FollowUp struct {
	ID     string `json:"id"`
	Label  string `json:"label"`
	Prompt string `json:"prompt"`
}

type Workflow struct {
	ID        string `json:"id"`
	StepIndex int    `json:"step_index"`
	StepTotal int    `json:"step_total"`
}

type Notice struct {
	ID      string `json:"id"`
	Kind    string `json:"kind"`
	Message string `json:"message"`
}

type Response struct {
	Version    int        `json:"version"`
	ResponseID string     `json:"response_id"`
	Answer     Answer     `json:"answer"`
	Sections   []Section  `json:"sections"`
	Steps      []string   `json:"steps"`
	Actions    []Action   `json:"actions"`
	Sources    []Source   `json:"sources"`
	Entities   []Entity   `json:"entities"`
	FollowUps  []FollowUp `json:"follow_ups"`
	Workflow   *Workflow  `json:"workflow"`
	Notices    []Notice   `json:"notices"`
	Status     Status     `json:"status"`
}

func NewResponse(responseID, content string) Response {
	return Response{
		Version:    2,
		ResponseID: responseID,
		Answer: Answer{
			Format:  FormatMarkdown,
			Content: content,
		},
		Sections:  []Section{},
		Steps:     []string{},
		Actions:   []Action{},
		Sources:   []Source{},
		Entities:  []Entity{},
		FollowUps: []FollowUp{},
		Notices:   []Notice{},
		Status:    StatusComplete,
	}
}
