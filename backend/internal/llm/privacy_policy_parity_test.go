package llm

import (
	"testing"

	"github.com/stretchr/testify/require"
)

// retentionParityDocuments the documented retention contract for every production
// feature (Wave 5 policy table). The test fails if a production feature is
// added without a documented retention/deletion entry here.
var retentionParity = map[string]struct {
	Class           PrivacyClass
	Retention       string
	PermanentDelete string
	ClosurePurge    bool
	RequireZDR      bool
}{
	"waiter":          {Class: PrivacyCustomerSensitive, Retention: "90d", PermanentDelete: "janitor+business_closure", ClosurePurge: true, RequireZDR: true},
	"waiter_whatsapp": {Class: PrivacyCustomerSensitive, Retention: "90d", PermanentDelete: "janitor+business_closure", ClosurePurge: true, RequireZDR: true},
	"ops_assistant":   {Class: PrivacyBusinessConfidential, Retention: "active_lifetime;archived_30d", PermanentDelete: "owner_delete+janitor+closure", ClosurePurge: true, RequireZDR: true},
	"director":        {Class: PrivacyBusinessConfidential, Retention: "account_lifetime", PermanentDelete: "owner_delete+closure", ClosurePurge: true, RequireZDR: true},
	"guardrail":       {Class: PrivacyCustomerSensitive, Retention: "ephemeral", PermanentDelete: "n/a", ClosurePurge: false, RequireZDR: true},
	"wizard":          {Class: PrivacyBusinessConfidential, Retention: "account_lifetime", PermanentDelete: "owner_delete+closure", ClosurePurge: true, RequireZDR: true},
	"extraction":      {Class: PrivacyBusinessConfidential, Retention: "account_lifetime", PermanentDelete: "owner_delete+closure", ClosurePurge: true, RequireZDR: true},
	"image":           {Class: PrivacyBusinessConfidential, Retention: "account_lifetime", PermanentDelete: "asset_delete+closure", ClosurePurge: true, RequireZDR: true},
	"marketing":       {Class: PrivacyBusinessConfidential, Retention: "draft_lifetime", PermanentDelete: "record_delete+closure", ClosurePurge: true, RequireZDR: true},
}

func TestPrivacyPolicyParity_AllProductionFeaturesDocumented(t *testing.T) {
	for _, feature := range KnownProductionFeatures() {
		if feature == "eval" || feature == "test" {
			continue
		}
		doc, ok := retentionParity[feature]
		require.True(t, ok, "production feature %q missing from retentionParity / public policy", feature)
		policy, ok := PrivacyPolicyForFeature(feature)
		require.True(t, ok, feature)
		require.Equal(t, doc.Class, policy.Class, feature)
		require.Equal(t, doc.RequireZDR, policy.RequireZDR, feature)
		require.NotEmpty(t, doc.Retention, feature)
		require.NotEmpty(t, doc.PermanentDelete, feature)
	}
}
