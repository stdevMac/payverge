package services

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
)

func upsellPayload() *directorContext {
	p := &directorContext{}
	p.Plugins.MissingRecommendations = []map[string]string{
		{"title": "Trustpilot", "deep_link": "/business/1/dashboard?tab=plugins"},
		{"title": "Mailchimp", "deep_link": "/business/1/dashboard?tab=plugins"},
		{"title": "Square", "deep_link": "/business/1/dashboard?tab=plugins"},
	}
	return p
}

func TestUpsell_StaffingQuestionGetsZeroPlugins(t *testing.T) {
	svc := &DirectorConsoleService{}
	out := DirectorStructuredResponse{
		ActionPlan: []DirectorAction{
			{Title: "Add a closer", Description: "x", DeepLink: "/business/1/dashboard?tab=staff", Priority: "high"},
			{Title: "Stagger shifts", Description: "y", DeepLink: "/business/1/dashboard?tab=staff", Priority: "medium"},
		},
	}
	got := svc.normalizeStructuredOutput(1, "en", "How should I schedule staff for the dinner rush?", out, upsellPayload())
	for _, a := range got.ActionPlan {
		assert.NotContains(t, a.Title, "Trustpilot")
		assert.NotContains(t, a.Title, "Mailchimp")
	}
}

func TestUpsell_SparsePlanGetsUpToTwoPlugins(t *testing.T) {
	svc := &DirectorConsoleService{}
	out := DirectorStructuredResponse{
		ActionPlan: []DirectorAction{{Title: "Only one", Description: "x", DeepLink: "/business/1/dashboard?tab=analytics", Priority: "high"}},
	}
	got := svc.normalizeStructuredOutput(1, "en", "anything", out, upsellPayload())
	pluginCount := 0
	for _, a := range got.ActionPlan {
		if strings.Contains(a.Title, "Trustpilot") || strings.Contains(a.Title, "Mailchimp") || strings.Contains(a.Title, "Square") {
			pluginCount++
		}
	}
	assert.LessOrEqual(t, pluginCount, 2, "never more than 2 plugin upsells")
	assert.GreaterOrEqual(t, pluginCount, 1, "sparse plan should be padded with at least one")
}

func TestUpsell_GrowthQuestionGetsPlugins(t *testing.T) {
	svc := &DirectorConsoleService{}
	out := DirectorStructuredResponse{
		ActionPlan: []DirectorAction{
			{Title: "A", Description: "x", DeepLink: "/business/1/dashboard?tab=analytics", Priority: "high"},
			{Title: "B", Description: "y", DeepLink: "/business/1/dashboard?tab=menu", Priority: "medium"},
		},
	}
	got := svc.normalizeStructuredOutput(1, "en", "What plugins help me grow reviews and marketing?", out, upsellPayload())
	pluginCount := 0
	for _, a := range got.ActionPlan {
		if strings.Contains(a.Title, "Trustpilot") || strings.Contains(a.Title, "Mailchimp") || strings.Contains(a.Title, "Square") {
			pluginCount++
		}
	}
	assert.GreaterOrEqual(t, pluginCount, 1)
	assert.LessOrEqual(t, pluginCount, 2)
}
