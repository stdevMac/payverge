package director_tools

import (
	"context"
	"fmt"
	"time"

	"github.com/stdevmac/payverge/backend/internal/database"
	"github.com/stdevmac/payverge/backend/internal/llm"
)

// BusinessProfileTool returns the calling business's identity fields —
// name, IANA timezone and assistant alias. The model uses these to phrase
// responses (e.g., "Sage" instead of generic "I", the right time-of-day)
// without needing to chase any other tool first. It takes no arguments.
type BusinessProfileTool struct{}

// Name is the snake_case function identifier sent to the model.
func (t *BusinessProfileTool) Name() string { return "get_business_profile" }

// HumanLabel is the short pill label surfaced in the UI while the tool runs.
func (t *BusinessProfileTool) HumanLabel(locale string) string {
	switch locale {
	case "es":
		return "Leyendo perfil del negocio"
	case "fr":
		return "Lecture du profil de l'entreprise"
	case "ar":
		return "قراءة ملف النشاط التجاري"
	default:
		return "Reading business profile"
	}
}

// Description is the model-facing tool description sent to the provider.
func (t *BusinessProfileTool) Description() string {
	return "Returns the business's identity and configuration: name, IANA timezone, the venue's current LOCAL time and UTC offset, and assistant alias. Call when you need basic facts about the business itself (its name or local time) — not for metrics. Use local_time verbatim; never convert a timestamp yourself. Takes no arguments."
}

// Schema declares the argument shape the model sees. The tool takes no
// arguments — the business context is already pinned to env.BusinessID.
func (t *BusinessProfileTool) Schema() *llm.JSONSchema {
	return &llm.JSONSchema{
		Type:       llm.TypeObject,
		Properties: map[string]*llm.JSONSchema{},
	}
}

// Run fetches the business and returns its profile fields.
func (t *BusinessProfileTool) Run(_ context.Context, _ map[string]any, env ToolEnv) (ToolResult, error) {
	if env.DB == nil {
		return ToolResult{}, fmt.Errorf("get_business_profile: nil DB in tool env")
	}

	business, err := env.DB.GetBusinessByID(env.BusinessID)
	if err != nil {
		return ToolResult{}, fmt.Errorf("get_business_profile: %w", err)
	}

	tz := business.Timezone
	if tz == "" {
		tz = "UTC"
	}
	assistant := business.AiSettings.AiName
	if assistant == "" {
		assistant = "Sage"
	}

	// #902: the description promised local time but the tool returned only an
	// IANA name, so the model did the conversion itself off the UTC snapshot
	// stamp and reported 20:00 UTC as "8:00 PM New York". Resolve it here.
	local := time.Now().In(database.ResolveLocation(tz))

	summary := fmt.Sprintf("%s · %s · %s local", business.Name, tz, local.Format("3:04 PM"))

	return ToolResult{
		Summary: summary,
		Data: map[string]any{
			"name":           business.Name,
			"timezone":       tz,
			"local_time":     local.Format("3:04 PM"),
			"local_datetime": local.Format(time.RFC3339),
			"local_weekday":  local.Weekday().String(),
			"utc_offset":     local.Format("-07:00"),
			"assistant_name": assistant,
		},
	}, nil
}
