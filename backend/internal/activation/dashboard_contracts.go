package activation

import (
	"encoding/json"
	"io"
)

type DashboardStatus string

const DashboardDesiredOnly DashboardStatus = "desired_only"

type DashboardProvider string

const (
	DashboardProviderPostHog  DashboardProvider = "posthog"
	DashboardProviderPostgres DashboardProvider = "postgres"
)

// DashboardContract is desired provider configuration. It does not assert
// that a provider dashboard exists.
type DashboardContract struct {
	ID          string            `json:"id"`
	Title       string            `json:"title"`
	Provider    DashboardProvider `json:"provider"`
	Status      DashboardStatus   `json:"status"`
	Denominator Denominator       `json:"denominator"`
	Events      []Name            `json:"events,omitempty"`
	Breakdowns  []string          `json:"breakdowns,omitempty"`
	Query       string            `json:"query,omitempty"`
}

type DashboardPlan struct {
	SchemaVersion int                 `json:"schema_version"`
	Status        DashboardStatus     `json:"status"`
	Contracts     []DashboardContract `json:"contracts"`
}

func DashboardContracts() []DashboardContract {
	return []DashboardContract{
		{ID: "acquisition", Title: "Acquisition — completed registrations by source", Provider: DashboardProviderPostHog, Status: DashboardDesiredOnly, Denominator: ServerAllEligible, Events: []Name{RegistrationCompleted}, Breakdowns: []string{"acquisition_source"}},
		{ID: "setup_client", Title: "Setup funnel — consent-qualified starts", Provider: DashboardProviderPostHog, Status: DashboardDesiredOnly, Denominator: ConsentQualifiedClient, Events: []Name{RegistrationStarted, OnboardingStepViewed, OnboardingStepClicked}},
		{ID: "setup_server", Title: "Setup funnel — all eligible server milestones", Provider: DashboardProviderPostHog, Status: DashboardDesiredOnly, Denominator: ServerAllEligible, Events: []Name{RegistrationCompleted, WorkspaceCreated, MenuItemCreated, TableCreated, QRPreviewed, PaymentConfigured, StaffInvited, SetupCompleted}},
		{ID: "product_activation", Title: "Product activation", Provider: DashboardProviderPostHog, Status: DashboardDesiredOnly, Denominator: ServerAllEligible, Events: []Name{TestOrderCompleted, ActivationAchieved}},
		{ID: "revenue_activation", Title: "Revenue activation", Provider: DashboardProviderPostHog, Status: DashboardDesiredOnly, Denominator: ServerAllEligible, Events: []Name{FirstPaidBill}},
		{ID: "support", Title: "Support demand — escalations by source and status", Provider: DashboardProviderPostgres, Status: DashboardDesiredOnly, Denominator: ServerAllEligible, Query: `SELECT date_trunc('week', created_at) AS week, source, status, COUNT(*) AS escalations FROM escalations GROUP BY 1, 2, 3 ORDER BY 1, 2, 3`},
		{ID: "retention", Title: "Weekly retained activated businesses", Provider: DashboardProviderPostgres, Status: DashboardDesiredOnly, Denominator: ServerAllEligible, Query: `SELECT date_trunc('week', activity.occurred_at) AS week, COUNT(DISTINCT activity.business_id) AS retained_businesses FROM (SELECT orders.business_id, COALESCE(orders.approved_at, orders.created_at) AS occurred_at FROM orders JOIN business_activation_states states ON states.business_id = orders.business_id WHERE states.activation_achieved_at IS NOT NULL AND orders.status = 'completed' AND COALESCE(orders.approved_at, orders.created_at) >= states.activation_achieved_at UNION ALL SELECT bills.business_id, COALESCE(bills.settled_at, bills.closed_at, bills.updated_at, bills.created_at) AS occurred_at FROM bills JOIN business_activation_states states ON states.business_id = bills.business_id WHERE states.activation_achieved_at IS NOT NULL AND bills.status IN ('paid', 'closed') AND COALESCE(bills.settled_at, bills.closed_at, bills.updated_at, bills.created_at) >= states.activation_achieved_at) activity GROUP BY 1 ORDER BY 1`},
	}
}

// ExportDashboardPlan writes desired configuration only. It performs no
// provider requests and accepts no credentials.
func ExportDashboardPlan(destination io.Writer) error {
	encoder := json.NewEncoder(destination)
	encoder.SetIndent("", "  ")
	return encoder.Encode(DashboardPlan{
		SchemaVersion: 1,
		Status:        DashboardDesiredOnly,
		Contracts:     DashboardContracts(),
	})
}
