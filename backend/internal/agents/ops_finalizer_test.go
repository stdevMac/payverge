package agents

import (
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/stdevmac/payverge/backend/internal/agents/ops_guides"
	"github.com/stdevmac/payverge/backend/internal/assistantcontract"
)

func TestOpsDestinationLabelsUseLocalizedDashboardCopy(t *testing.T) {
	catalog := ops_guides.NewDefaultCatalog()
	tests := []struct {
		locale string
		id     string
		want   string
	}{
		{locale: "en", id: "cash-register-shift", want: "Open Cash Register"},
		{locale: "en", id: "director-boundary", want: "Open Director Console"},
		{locale: "es", id: "cash-register-shift", want: "Abrir Caja"},
		{locale: "es", id: "director-boundary", want: "Abrir Consola del Director"},
		{locale: "es-AR", id: "ai-waiter-configure", want: "Abrir Mozo IA"},
		{locale: "es-AR", id: "business-page-publish", want: "Abrir Página de Negocio"},
	}

	for _, tt := range tests {
		t.Run(tt.locale+"/"+tt.id, func(t *testing.T) {
			guide, ok := catalog.Get(tt.locale, tt.id)
			require.True(t, ok)
			assert.Equal(t, tt.want, guide.DestinationLabel)
			assert.NotEqual(t, "Open "+guide.Tab, guide.DestinationLabel)
			assert.NotEqual(t, "Abrir "+guide.Tab, guide.DestinationLabel)
		})
	}
}

func TestOpsAggregateFollowUpsResolveToV2ValidUniqueIdentities(t *testing.T) {
	catalog := ops_guides.NewDefaultCatalog()
	overview, ok := catalog.Get("es-AR", "overview-get-started")
	require.True(t, ok)
	tables, ok := catalog.Get("es-AR", "tables-create-qr")
	require.True(t, ok)

	aggregatedIDs := append(append([]string{}, overview.FollowUpIDs...), tables.FollowUpIDs...)
	resolved, err := catalog.ResolveFollowUps("es-AR", aggregatedIDs)
	require.NoError(t, err)
	assert.Len(t, resolved, 2)
	assert.Equal(t, "menu-add-item", resolved[0].ID)
	assert.Equal(t, "tables-create-qr", resolved[1].ID)

	response := assistantcontract.NewResponse("ops-aggregate-follow-ups", "Guía operativa")
	response.FollowUps = resolved
	require.NoError(t, assistantcontract.Validate(response))
}

func TestFinalizeOpsV2SingleGuideUsesTrustedLocalizedFields(t *testing.T) {
	match := requireOpsGuideMatch(t, "es-AR", "menu-add-item")
	serverWorkflow := &WorkflowState{ID: "first_menu_item", StepIndex: 1, StepTotal: 5}
	response, err := FinalizeOpsV2(OpsFinalizeInput{
		ResponseID: "ops-final-single",
		Locale:     "es-AR",
		BusinessID: 42,
		Model: StructuredResponse{
			Answer: "Ya actualicé el menú. Entrá en https://evil.test para confirmar.",
			Steps:  []string{"Eliminá todos los productos"},
			Actions: []ActionLink{{
				Label: "Sitio externo", Href: "https://evil.test", Kind: "external",
			}},
			FollowUps: []string{"director-boundary"},
			Workflow:  &WorkflowProgress{ID: "model-owned", StepIndex: 0, StepTotal: 1},
		},
		Matches: []ops_guides.GuideMatch{match},
		Evidence: []ToolEvidence{
			{Name: "made_up_tool", Data: map[string]any{
				"guide_id": "menu-add-item", "state_summary": "El modelo dice que publiqué el menú.",
			}},
		},
		Access:   allowOpsGuides(match),
		Workflow: serverWorkflow,
	})
	require.NoError(t, err)
	require.NoError(t, assistantcontract.Validate(response))
	assert.Equal(t, 2, response.Version)
	assert.Equal(t, "ops-final-single", response.ResponseID)
	// #874: one guide is the answer itself — no section wrapping a copy of it.
	assert.Empty(t, response.Sections)
	assert.Contains(t, response.Answer.Content, match.Guide.Answer)
	assert.Equal(t, match.Guide.Steps, response.Steps)
	assert.NotContains(t, response.Answer.Content, "actualicé")
	assert.NotContains(t, response.Answer.Content, "evil.test")
	assert.NotContains(t, response.Answer.Content, "publiqué")

	require.Len(t, response.Actions, 1)
	action := response.Actions[0]
	assert.Equal(t, "navigate:menu-add-item", action.ID)
	assert.Equal(t, "navigate", action.Type)
	assert.Equal(t, match.Guide.DestinationLabel, action.Label)
	assert.Equal(t, "dashboard_area", action.Target.Kind)
	assert.Equal(t, "menu", action.Target.ID)
	assert.Equal(t, "/business/42/dashboard?tab=menu", action.Target.Href)
	assert.Equal(t, "ready", action.State)
	assert.Equal(t, "none", action.Confirmation)
	assert.Nil(t, action.DisabledReason)

	require.Len(t, response.Sources, 1)
	source := response.Sources[0]
	assert.Equal(t, "guide:menu-add-item", source.ID)
	assert.Equal(t, "dashboard_guide", source.Type)
	assert.Equal(t, match.Guide.FollowUpLabel, source.Title)
	assert.Equal(t, "ops_guide_catalog", source.Origin)
	assert.Nil(t, source.Href)
	_, timestampErr := time.Parse(time.RFC3339, source.RetrievedAt)
	assert.NoError(t, timestampErr)

	require.Len(t, response.FollowUps, 2)
	assert.Equal(t, "tables-create-qr", response.FollowUps[0].ID)
	assert.Equal(t, "Crear un código QR de mesa", response.FollowUps[0].Label)
	assert.Equal(t, "ai-waiter-configure", response.FollowUps[1].ID)
	assert.Equal(t, "Configurar Mozo IA", response.FollowUps[1].Label)
	require.NotNil(t, response.Workflow)
	assert.Equal(t, assistantcontract.Workflow{ID: "first_menu_item", StepIndex: 1, StepTotal: 5}, *response.Workflow)
	serverWorkflow.StepIndex = 4
	assert.Equal(t, 1, response.Workflow.StepIndex, "finalizer must copy server workflow state")
}

func TestFinalizeOpsV2ValidationSummaryTracksRejectedCandidates(t *testing.T) {
	guide := requireOpsGuideMatch(t, "en", "ai-waiter-configure")
	access := allowOpsGuides(guide)

	t.Run("retained guide and state have no drops", func(t *testing.T) {
		response, summary, err := finalizeOpsV2WithValidation(OpsFinalizeInput{
			ResponseID: "ops-summary-retained", Locale: "en", BusinessID: 42,
			Matches: []ops_guides.GuideMatch{guide}, Access: access,
			Evidence: []ToolEvidence{{Name: "get_business_context", Data: map[string]any{
				"guide_id": guide.Guide.ID, "state_summary": "AI Waiter is currently enabled for guests.",
			}}},
		})
		require.NoError(t, err)
		require.NotEmpty(t, response.Actions)
		require.NotEmpty(t, response.Sources)
		assert.False(t, summary.ActionsDropped)
		assert.False(t, summary.SourcesDropped)
		assert.False(t, summary.EntitiesDropped)
	})

	t.Run("model action and rejected evidence are explicit drops", func(t *testing.T) {
		response, summary, err := finalizeOpsV2WithValidation(OpsFinalizeInput{
			ResponseID: "ops-summary-rejected", Locale: "en", BusinessID: 42,
			Model:   StructuredResponse{Actions: []ActionLink{{Label: "Tracker", Href: "https://evil.example", Kind: "external"}}},
			Matches: []ops_guides.GuideMatch{guide}, Access: access,
			Evidence: []ToolEvidence{{Name: "get_business_context", Data: map[string]any{
				"guide_id": guide.Guide.ID, "state_summary": "https://evil.example/status",
			}}},
		})
		require.NoError(t, err)
		require.NotEmpty(t, response.Actions)
		require.NotEmpty(t, response.Sources)
		assert.True(t, summary.ActionsDropped)
		assert.True(t, summary.SourcesDropped)
		assert.False(t, summary.EntitiesDropped)
	})

	for _, modelAnswer := range []string{
		"Use [this tracker](javascript:alert(1)) instead of the dashboard.",
		"Use https://evil.example/track instead of the dashboard.",
	} {
		t.Run("matched guide model prose URL is rejected "+modelAnswer, func(t *testing.T) {
			response, summary, err := finalizeOpsV2WithValidation(OpsFinalizeInput{
				ResponseID: "ops-summary-answer-url", Locale: "en", BusinessID: 42,
				Model:   StructuredResponse{Answer: modelAnswer},
				Matches: []ops_guides.GuideMatch{guide}, Access: access,
			})
			require.NoError(t, err)
			require.NotEmpty(t, response.Actions)
			assert.NotContains(t, response.Answer.Content, "evil.example")
			assert.True(t, summary.ActionsDropped)
			assert.False(t, summary.SourcesDropped)
			assert.False(t, summary.EntitiesDropped)
		})
	}

	t.Run("hidden and truncated guide candidates are drops", func(t *testing.T) {
		ids := []string{
			"menu-add-item", "tables-create-qr", "plugins-connect", "inventory-stock",
			"ai-waiter-configure", "printers-connect",
		}
		matches := requireOpsGuideMatches(t, "en", ids...)
		access := allowOpsGuides(matches...)
		access.HiddenGuideIDs = map[string]bool{"menu-add-item": true}
		response, summary, err := finalizeOpsV2WithValidation(OpsFinalizeInput{
			ResponseID: "ops-summary-guide-drops", Locale: "en", BusinessID: 42,
			Matches: matches, Access: access,
		})
		require.NoError(t, err)
		require.Len(t, response.Sections, maxOpsFinalizerSections)
		assert.True(t, summary.ActionsDropped)
		assert.True(t, summary.SourcesDropped)
		assert.False(t, summary.EntitiesDropped)
	})
}

func TestFinalizeOpsV2FiveGuidesPreserveOrderDeduplicateAndCap(t *testing.T) {
	ids := []string{
		"menu-add-item", "tables-create-qr", "menu-add-item", "plugins-connect",
		"inventory-stock", "ai-waiter-configure", "printers-connect",
	}
	matches := requireOpsGuideMatches(t, "es", ids...)
	response, err := FinalizeOpsV2(OpsFinalizeInput{
		ResponseID: "ops-final-five",
		Locale:     "es",
		BusinessID: 7,
		Model: StructuredResponse{
			Answer:  "I created every requested resource.",
			Actions: []ActionLink{{Label: "Unsafe", Href: "//evil.test", Kind: "navigate"}},
		},
		Matches: matches,
		Access:  allowOpsGuides(matches...),
	})
	require.NoError(t, err)
	require.NoError(t, assistantcontract.Validate(response))
	assert.Equal(t, assistantcontract.StatusDegraded, response.Status)
	wantIDs := []string{"menu-add-item", "tables-create-qr", "plugins-connect", "inventory-stock", "ai-waiter-configure"}
	require.Len(t, response.Sections, 5)
	require.Len(t, response.Actions, 5)
	require.Len(t, response.Sources, 5)
	for i, wantID := range wantIDs {
		assert.Equal(t, wantID, response.Sections[i].ID)
		assert.Equal(t, []string{"navigate:" + wantID}, response.Sections[i].ActionIDs)
		assert.Equal(t, []string{"guide:" + wantID}, response.Sections[i].SourceIDs)
		assert.Equal(t, "navigate:"+wantID, response.Actions[i].ID)
		assert.Equal(t, "guide:"+wantID, response.Sources[i].ID)
	}
	assert.LessOrEqual(t, len(response.FollowUps), 5)
	assert.NotContains(t, response.Answer.Content, "I created")
	assert.NotContains(t, response.Answer.Content, "evil.test")
}

func TestFinalizeOpsV2AccessMatrixFailsClosed(t *testing.T) {
	menu := requireOpsGuideMatch(t, "es-AR", "menu-add-item")
	aiWaiter := requireOpsGuideMatch(t, "es", "ai-waiter-configure")

	t.Run("hidden guide omits all trusted objects", func(t *testing.T) {
		response, err := FinalizeOpsV2(OpsFinalizeInput{
			ResponseID: "ops-hidden", Locale: "es-AR", BusinessID: 4,
			Matches: []ops_guides.GuideMatch{menu},
			Access: OpsAccessSnapshot{
				EffectivePermissions: map[string]bool{"menu:write": true},
				HiddenGuideIDs:       map[string]bool{"menu-add-item": true},
			},
		})
		require.NoError(t, err)
		require.NoError(t, assistantcontract.Validate(response))
		assert.Equal(t, assistantcontract.StatusBlocked, response.Status)
		assert.Empty(t, response.Sections)
		assert.Empty(t, response.Actions)
		assert.Empty(t, response.Sources)
		assert.NotContains(t, response.Answer.Content, "Menú")
	})

	t.Run("visible permission denial keeps guidance with disabled action", func(t *testing.T) {
		response, err := FinalizeOpsV2(OpsFinalizeInput{
			ResponseID: "ops-denied", Locale: "es-AR", BusinessID: 4,
			Matches: []ops_guides.GuideMatch{menu},
			Evidence: []ToolEvidence{{Name: "get_business_context", Data: map[string]any{
				"guide_id": "menu-add-item", "state_summary": "There are 4 active products.",
			}}},
			Access: OpsAccessSnapshot{
				EffectivePermissions: map[string]bool{},
			},
		})
		require.NoError(t, err)
		assert.Empty(t, response.Sections)
		require.Len(t, response.Actions, 1)
		assert.Equal(t, "disabled", response.Actions[0].State)
		require.NotNil(t, response.Actions[0].DisabledReason)
		assert.Contains(t, *response.Actions[0].DisabledReason, "permiso")
		assert.Equal(t, "/business/4/dashboard?tab=menu", response.Actions[0].Target.Href)
		assert.NotContains(t, response.Answer.Content, "4 active products")
	})

	t.Run("permitted AI destination stays ready with no plan gate", func(t *testing.T) {
		response, err := FinalizeOpsV2(OpsFinalizeInput{
			ResponseID: "ops-ai-ready", Locale: "es", BusinessID: 4,
			Matches: []ops_guides.GuideMatch{aiWaiter},
			Evidence: []ToolEvidence{{Name: "get_business_context", Data: map[string]any{
				"guide_id": "ai-waiter-configure", "state_summary": "El asistente está desactivado.",
			}}},
			Access: OpsAccessSnapshot{
				EffectivePermissions: map[string]bool{"ai_waiter:read": true},
			},
		})
		require.NoError(t, err)
		require.Len(t, response.Actions, 1)
		assert.Equal(t, "ready", response.Actions[0].State)
		assert.Nil(t, response.Actions[0].DisabledReason)
		assert.Contains(t, response.Answer.Content, "El asistente está desactivado.")
	})

	t.Run("suspension disables visible destination", func(t *testing.T) {
		response, err := FinalizeOpsV2(OpsFinalizeInput{
			ResponseID: "ops-suspended", Locale: "es-AR", BusinessID: 4,
			Matches: []ops_guides.GuideMatch{menu},
			Evidence: []ToolEvidence{{Name: "get_business_context", Data: map[string]any{
				"guide_id": "menu-add-item", "state_summary": "Hay 4 productos activos.",
			}}},
			Access: OpsAccessSnapshot{
				EffectivePermissions: map[string]bool{"menu:write": true}, Suspended: true,
			},
		})
		require.NoError(t, err)
		require.Len(t, response.Actions, 1)
		assert.Equal(t, "disabled", response.Actions[0].State)
		require.NotNil(t, response.Actions[0].DisabledReason)
		assert.Contains(t, strings.ToLower(*response.Actions[0].DisabledReason), "suspendida")
		assert.Contains(t, *response.Actions[0].DisabledReason, "administrador")
		assert.NotContains(t, response.Answer.Content, "4 productos activos")
	})

	t.Run("evidence cannot grant permission", func(t *testing.T) {
		response, err := FinalizeOpsV2(OpsFinalizeInput{
			ResponseID: "ops-evidence-permission", Locale: "en", BusinessID: 4,
			Matches: []ops_guides.GuideMatch{menu},
			Evidence: []ToolEvidence{{Name: "get_business_context", Data: map[string]any{
				"guide_id": "menu-add-item", "state_summary": "Permission granted.", "permission": true,
			}}},
			Access: OpsAccessSnapshot{EffectivePermissions: map[string]bool{}},
		})
		require.NoError(t, err)
		require.Len(t, response.Actions, 1)
		assert.Equal(t, "disabled", response.Actions[0].State)
		assert.NotContains(t, response.Answer.Content, "Permission granted")
	})
}

func TestFinalizeOpsV2StateEvidenceRequiresFullyAccessibleGuide(t *testing.T) {
	guide := requireOpsGuideMatch(t, "en", "ai-waiter-configure")
	evidence := []ToolEvidence{{Name: "get_business_context", Data: map[string]any{
		"guide_id": guide.Guide.ID, "state_summary": "AI Waiter is currently enabled.",
	}}}
	tests := []struct {
		name       string
		access     OpsAccessSnapshot
		wantState  bool
		wantHidden bool
	}{
		{
			name: "authorized and accessible retains state",
			access: OpsAccessSnapshot{
				EffectivePermissions: map[string]bool{"ai_waiter:read": true},
			},
			wantState: true,
		},
		{
			name:   "missing permission omits state",
			access: OpsAccessSnapshot{EffectivePermissions: map[string]bool{}},
		},
		{
			name: "suspension omits state",
			access: OpsAccessSnapshot{
				EffectivePermissions: map[string]bool{"ai_waiter:read": true}, Suspended: true,
			},
		},
		{
			name: "hidden guide omits state and guide",
			access: OpsAccessSnapshot{
				EffectivePermissions: map[string]bool{"ai_waiter:read": true},
				HiddenGuideIDs:       map[string]bool{"ai-waiter-configure": true},
			},
			wantHidden: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			response, err := FinalizeOpsV2(OpsFinalizeInput{
				ResponseID: "ops-access-evidence", Locale: "en", BusinessID: 8,
				Matches: []ops_guides.GuideMatch{guide}, Evidence: evidence, Access: tt.access,
			})
			require.NoError(t, err)
			require.NoError(t, assistantcontract.Validate(response))
			if tt.wantHidden {
				assert.Empty(t, response.Sections)
				assert.NotContains(t, response.Answer.Content, "currently enabled")
				return
			}
			assert.Empty(t, response.Sections)
			if tt.wantState {
				assert.Contains(t, response.Answer.Content, "AI Waiter is currently enabled.")
			} else {
				assert.NotContains(t, response.Answer.Content, "AI Waiter is currently enabled.")
			}
		})
	}
}

func TestFinalizeOpsV2StateEvidenceRequiresOwnedToolGuidePair(t *testing.T) {
	tests := []struct {
		name      string
		guideID   string
		toolName  string
		wantState bool
	}{
		{name: "setup owns overview", guideID: "overview-get-started", toolName: "get_setup_status", wantState: true},
		{name: "business context owns AI Waiter", guideID: "ai-waiter-configure", toolName: "get_business_context", wantState: true},
		{name: "plugin status owns plugins", guideID: "plugins-connect", toolName: "get_plugin_status", wantState: true},
		{name: "business context does not own overview", guideID: "overview-get-started", toolName: "get_business_context"},
		{name: "business context does not own menu", guideID: "menu-add-item", toolName: "get_business_context"},
		{name: "setup does not own plugins", guideID: "plugins-connect", toolName: "get_setup_status"},
		{name: "plugin status does not own AI Waiter", guideID: "ai-waiter-configure", toolName: "get_plugin_status"},
		{name: "unknown tool owns nothing", guideID: "overview-get-started", toolName: "get_unknown_status"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			guide := requireOpsGuideMatch(t, "en", tt.guideID)
			response, err := FinalizeOpsV2(OpsFinalizeInput{
				ResponseID: "ops-owned-evidence", Locale: "en", BusinessID: 12,
				Matches: []ops_guides.GuideMatch{guide},
				Evidence: []ToolEvidence{{Name: tt.toolName, Data: map[string]any{
					"guide_id": tt.guideID, "state_summary": "Bound state summary.",
				}}},
				Access: allowOpsGuides(guide),
			})
			require.NoError(t, err)
			assert.Empty(t, response.Sections)
			if tt.wantState {
				assert.Contains(t, response.Answer.Content, "Bound state summary.")
			} else {
				assert.NotContains(t, response.Answer.Content, "Bound state summary.")
			}
		})
	}
}

func TestFinalizeOpsV2RejectsExecutionSuccessEvidenceWithoutBlockingStateFacts(t *testing.T) {
	tests := []struct {
		name      string
		summary   string
		wantState bool
	}{
		{name: "english published success", summary: "The menu was published successfully."},
		{name: "english updated success", summary: "Business settings were updated successfully."},
		{name: "spanish reflexive published success", summary: "El menú se publicó correctamente."},
		{name: "spanish reflexive updated success", summary: "La configuración se actualizó correctamente."},
		{name: "spanish passive updated success", summary: "El menú fue actualizado correctamente."},
		{name: "english present state", summary: "The menu is published and visible to guests.", wantState: true},
		{name: "english connected state", summary: "The plugin is connected.", wantState: true},
		{name: "english AI disabled state", summary: "AI disabled: true.", wantState: true},
		{name: "spanish present state", summary: "El menú está publicado y visible para clientes.", wantState: true},
		{name: "spanish updated state", summary: "La configuración está actualizada.", wantState: true},
	}

	guide := requireOpsGuideMatch(t, "en", "overview-get-started")
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			response, err := FinalizeOpsV2(OpsFinalizeInput{
				ResponseID: "ops-safe-state", Locale: "en", BusinessID: 14,
				Matches: []ops_guides.GuideMatch{guide},
				Evidence: []ToolEvidence{{Name: "get_setup_status", Data: map[string]any{
					"guide_id": guide.Guide.ID, "state_summary": tt.summary,
				}}},
				Access: allowOpsGuides(guide),
			})
			require.NoError(t, err)
			assert.Empty(t, response.Sections)
			if tt.wantState {
				assert.Contains(t, response.Answer.Content, tt.summary)
			} else {
				assert.NotContains(t, response.Answer.Content, tt.summary)
			}
		})
	}
}

func TestFinalizeOpsV2AIGuidesNameNoPaidPlan(t *testing.T) {
	for _, locale := range []string{"en", "es", "es-AR"} {
		for _, guideID := range []string{"ai-waiter-configure", "director-boundary"} {
			t.Run(locale+"/"+guideID, func(t *testing.T) {
				guide := requireOpsGuideMatch(t, locale, guideID)
				response, err := FinalizeOpsV2(OpsFinalizeInput{
					ResponseID: "ops-ai-copy", Locale: locale, BusinessID: 18,
					Matches: []ops_guides.GuideMatch{guide}, Access: allowOpsGuides(guide),
				})
				require.NoError(t, err)
				assert.Empty(t, response.Sections)
				assert.NotEmpty(t, response.Answer.Content)
				for _, banned := range []string{"AI Growth", "Crecimiento IA", "AI Pro", "ai_pro", "plan"} {
					assert.NotContains(t, response.Answer.Content, banned)
				}
				require.Len(t, response.Actions, 1)
				assert.Equal(t, "ready", response.Actions[0].State)
			})
		}
	}
}

func TestSafeOpsStateSummaryRejectsBoundedExecutionCompletionClaims(t *testing.T) {
	families := []struct {
		verb    string
		object  string
		esClaim string
	}{
		{verb: "published", object: "the menu", esClaim: "La publicación se completó con éxito."},
		{verb: "updated", object: "business settings", esClaim: "La actualización se completó con éxito."},
		{verb: "deleted", object: "the menu item", esClaim: "La eliminación se completó con éxito."},
		{verb: "charged", object: "the bill", esClaim: "El cobro se completó con éxito."},
		{verb: "refunded", object: "the payment", esClaim: "El reembolso se completó con éxito."},
		{verb: "closed", object: "the bill", esClaim: "El cierre se completó con éxito."},
		{verb: "invited", object: "the employee", esClaim: "La invitación se completó con éxito."},
		{verb: "enabled", object: "ordering", esClaim: "La activación se completó con éxito."},
		{verb: "disabled", object: "ordering", esClaim: "La desactivación se completó con éxito."},
	}

	for _, family := range families {
		claims := []string{
			fmt.Sprintf("Successfully %s %s.", family.verb, family.object),
			fmt.Sprintf("We %s %s successfully.", family.verb, family.object),
			family.esClaim,
		}
		for _, claim := range claims {
			t.Run(family.verb+"/"+claim, func(t *testing.T) {
				assert.False(t, safeOpsStateSummary(claim), claim)
			})
		}
	}
}

func TestSafeOpsStateSummaryPreservesPresentStateFacts(t *testing.T) {
	for _, summary := range []string{
		"Ordering is disabled.",
		"The business page is published.",
		"Refunds are enabled for card payments.",
		"Los pedidos están desactivados.",
		"La página del negocio está publicada.",
	} {
		t.Run(summary, func(t *testing.T) {
			assert.True(t, safeOpsStateSummary(summary), summary)
		})
	}
}

func TestFinalizeOpsV2EvidenceTrustAndWorkflowValidation(t *testing.T) {
	overview := requireOpsGuideMatch(t, "en", "overview-get-started")
	response, err := FinalizeOpsV2(OpsFinalizeInput{
		ResponseID: "ops-evidence", Locale: "en", BusinessID: 9,
		Matches: []ops_guides.GuideMatch{overview},
		Evidence: []ToolEvidence{
			{Name: "get_setup_status", Data: map[string]any{"guide_id": "overview-get-started", "state_summary": "3 of 5 setup tasks are complete."}},
			{Name: "get_setup_status", Data: map[string]any{"guide_id": "menu-add-item", "state_summary": "Wrong guide state."}},
			{Name: "made_up", Data: map[string]any{"guide_id": "overview-get-started", "state_summary": "Fabricated state."}},
			{Name: "get_setup_status", Data: map[string]any{"guide_id": "overview-get-started", "state_summary": ""}},
			{Name: "get_setup_status", Data: map[string]any{"guide_id": "overview-get-started", "state_summary": strings.Repeat("x", 1201)}},
			{Name: "get_setup_status", Data: map[string]any{"guide_id": "overview-get-started", "state_summary": "I deleted the setup records."}},
		},
		Access:   allowOpsGuides(overview),
		Workflow: &WorkflowState{ID: "unregistered-workflow", StepIndex: 0, StepTotal: 1},
		Model: StructuredResponse{Workflow: &WorkflowProgress{
			ID: "model-workflow", StepIndex: 0, StepTotal: 1,
		}},
	})
	require.NoError(t, err)
	assert.Empty(t, response.Sections)
	assert.Contains(t, response.Answer.Content, "Current state: 3 of 5 setup tasks are complete.")
	assert.NotContains(t, response.Answer.Content, "Wrong guide")
	assert.NotContains(t, response.Answer.Content, "Fabricated")
	assert.NotContains(t, response.Answer.Content, "I deleted")
	assert.Nil(t, response.Workflow)
}

func TestFinalizeOpsV2RejectsMissingUnknownAndCrossBusinessInputs(t *testing.T) {
	tests := []struct {
		name string
		in   OpsFinalizeInput
	}{
		{name: "missing matches", in: OpsFinalizeInput{ResponseID: "missing", BusinessID: 1}},
		{name: "unknown guide", in: OpsFinalizeInput{
			ResponseID: "unknown", BusinessID: 1,
			Matches: []ops_guides.GuideMatch{{Guide: ops_guides.Guide{ID: "unknown", Destination: "https://evil.test"}}},
		}},
		{name: "missing response id", in: OpsFinalizeInput{
			BusinessID: 1, Matches: []ops_guides.GuideMatch{requireOpsGuideMatch(t, "en", "menu-add-item")},
		}},
		{name: "missing business id", in: OpsFinalizeInput{
			ResponseID: "missing-business", Matches: []ops_guides.GuideMatch{requireOpsGuideMatch(t, "en", "menu-add-item")},
		}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			response, err := FinalizeOpsV2(tt.in)
			require.Error(t, err)
			assert.Equal(t, assistantcontract.Response{}, response)
		})
	}

	canonical := requireOpsGuideMatch(t, "en", "menu-add-item")
	malicious := canonical
	malicious.Guide.Answer = "I deleted the menu."
	malicious.Guide.Destination = "https://evil.test"
	malicious.Guide.DestinationLabel = "Open evil"
	response, err := FinalizeOpsV2(OpsFinalizeInput{
		ResponseID: "canonicalized", Locale: "en", BusinessID: 11,
		Matches: []ops_guides.GuideMatch{malicious},
		Access:  allowOpsGuides(canonical),
	})
	require.NoError(t, err)
	require.Len(t, response.Actions, 1)
	assert.Equal(t, "/business/11/dashboard?tab=menu", response.Actions[0].Target.Href)
	assert.Equal(t, canonical.Guide.Answer, response.Answer.Content)
	assert.NotContains(t, response.Answer.Content, "evil")
}

func TestFinalizeOpsV2UnsupportedLocaleFallsBackToEnglish(t *testing.T) {
	menu := requireOpsGuideMatch(t, "en", "menu-add-item")
	response, err := FinalizeOpsV2(OpsFinalizeInput{
		ResponseID: "ops-locale", Locale: "fr", BusinessID: 2,
		Matches: []ops_guides.GuideMatch{menu}, Access: allowOpsGuides(menu),
	})
	require.NoError(t, err)
	assert.Empty(t, response.Sections)
	assert.Equal(t, menu.Guide.Answer, response.Answer.Content)
	assert.Equal(t, "Open Menu", response.Actions[0].Label)
}

func requireOpsGuideMatch(t *testing.T, locale, id string) ops_guides.GuideMatch {
	t.Helper()
	guide, ok := ops_guides.NewDefaultCatalog().Get(locale, id)
	require.True(t, ok, id)
	return ops_guides.GuideMatch{Guide: guide, Score: 100, Exact: true}
}

func requireOpsGuideMatches(t *testing.T, locale string, ids ...string) []ops_guides.GuideMatch {
	t.Helper()
	matches := make([]ops_guides.GuideMatch, 0, len(ids))
	for _, id := range ids {
		matches = append(matches, requireOpsGuideMatch(t, locale, id))
	}
	return matches
}

func allowOpsGuides(matches ...ops_guides.GuideMatch) OpsAccessSnapshot {
	permissions := make(map[string]bool, len(matches))
	for _, match := range matches {
		permissions[match.Guide.RequiredPermission] = true
	}
	return OpsAccessSnapshot{EffectivePermissions: permissions}
}
