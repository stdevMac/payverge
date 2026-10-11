package services

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestSanitizeStructuredResponse_DoesNotMangleDatesMetricsIDs(t *testing.T) {
	in := DirectorStructuredResponse{
		Summary:        "Revenue from 2026-05-30 to 2026-06-06 grew 12%",
		Diagnosis:      "Week of 2026-06-01: 145 orders; sold 1 234 567 units",
		Evidence:       []string{"Order #100234567 was delayed", "AOV up 8.5%"},
		ExpectedImpact: "Up to 1,000 more covers",
		ActionPlan: []DirectorAction{
			{Title: "Review 2026-06-02 dip", Description: "Order #4567 stalled"},
		},
		FollowUps: []string{"Compare 2026-05-25 vs 2026-06-01?"},
	}
	out := sanitizeStructuredResponse(in)

	for _, s := range []string{
		out.Summary, out.Diagnosis, out.ExpectedImpact,
		out.Evidence[0], out.Evidence[1], out.ActionPlan[0].Title,
		out.ActionPlan[0].Description, out.FollowUps[0],
	} {
		assert.NotContains(t, s, "[redacted-phone]", "no false phone redaction: %q", s)
		assert.NotContains(t, s, "[redacted-email]", "no false email redaction: %q", s)
	}
	assert.Contains(t, out.Summary, "2026-05-30")
	assert.Contains(t, out.Diagnosis, "1 234 567")
	assert.Contains(t, out.Evidence[0], "#100234567")
}

func TestSanitizeStructuredResponse_StillRedactsRealPII(t *testing.T) {
	in := DirectorStructuredResponse{
		Summary:   "Contact owner at jane@example.com",
		Diagnosis: "Call +1 (415) 555 0199 for the supplier",
	}
	out := sanitizeStructuredResponse(in)
	assert.True(t, strings.Contains(out.Summary, "[redacted-email]"))
	assert.True(t, strings.Contains(out.Diagnosis, "[redacted-phone]"))
}
