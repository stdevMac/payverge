package ops_guides

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestCatalog_Invariants(t *testing.T) {
	c := NewDefaultCatalog()
	require.NoError(t, c.ValidateCatalog())
}

func TestCatalog_SpanishLocalesHaveCompleteIndependentCopy(t *testing.T) {
	c := NewDefaultCatalog()

	for _, id := range RequiredGuideIDs {
		en, ok := c.Get("en", id)
		require.True(t, ok, "missing English guide %s", id)

		for _, locale := range []string{"es", "es-AR"} {
			localized, ok := c.Get(locale, id)
			require.True(t, ok, "missing %s guide %s", locale, id)
			require.NotEmpty(t, localized.Phrases, "%s/%s phrases", locale, id)
			require.NotEmpty(t, strings.TrimSpace(localized.Answer), "%s/%s answer", locale, id)
			require.NotEmpty(t, localized.Steps, "%s/%s steps", locale, id)

			assert.NotEqual(t, en.Phrases, localized.Phrases, "%s/%s inherited English phrases", locale, id)
			assert.NotEqual(t, en.Answer, localized.Answer, "%s/%s inherited English answer", locale, id)
			assert.NotEqual(t, en.Steps, localized.Steps, "%s/%s inherited English steps", locale, id)
			assert.NotContains(t, strings.ToLower(localized.Answer), strings.ToLower(en.Answer),
				"%s/%s embeds the English answer", locale, id)
			assert.NotContains(t, localized.Answer, "Abre la pestaña "+localized.Tab,
				"%s/%s uses the generic tab fallback", locale, id)

			for _, localizedStep := range localized.Steps {
				for _, englishStep := range en.Steps {
					assert.NotEqual(t, strings.ToLower(strings.TrimSpace(englishStep)), strings.ToLower(strings.TrimSpace(localizedStep)),
						"%s/%s contains an English step", locale, id)
				}
			}
		}
	}
}

func TestCatalog_SpanishArgentinaImperativesUseVoseo(t *testing.T) {
	c := NewDefaultCatalog()
	neutralImperatives := []string{
		"abre ", "activa ", "agrega ", "añade ", "asigna ", "busca ", "cierra ",
		"completa ", "conecta ", "confirma ", "crea ", "define ", "descarga ",
		"elige ", "envía ", "espera ", "guarda ", "inicia ", "indica ", "marca ",
		"mira ", "organiza ", "pide ", "pulsa ", "revisa ", "selecciona ",
		"toca ", "usa ", "vuelve ",
	}

	for _, id := range RequiredGuideIDs {
		argentine, ok := c.Get("es-AR", id)
		require.True(t, ok, "missing es-AR guide %s", id)
		for _, step := range argentine.Steps {
			lowerStep := strings.ToLower(strings.TrimSpace(step))
			for _, imperative := range neutralImperatives {
				assert.False(t, strings.HasPrefix(lowerStep, imperative),
					"es-AR/%s must use natural voseo in step %q", id, step)
			}
		}
	}
}

func TestCatalog_SpanishArgentinaVisibleCopyHasNoNeutralTuLeakage(t *testing.T) {
	c := NewDefaultCatalog()
	neutralTuForms := []string{
		" tú ", " puedes ", " tienes ", " quieres ", " eres ",
		" debes ", " necesitas ", " por ti ", " para ti ",
	}

	for _, id := range RequiredGuideIDs {
		guide, ok := c.Get("es-AR", id)
		require.True(t, ok, "missing es-AR guide %s", id)
		visibleCopy := " " + strings.ToLower(strings.Join(append([]string{guide.Answer}, guide.Steps...), " ")) + " "
		for _, neutralForm := range neutralTuForms {
			assert.NotContains(t, visibleCopy, neutralForm,
				"es-AR/%s contains neutral tú form %q", id, strings.TrimSpace(neutralForm))
		}
	}
}

func TestCatalog_SpanishKitchenConditionalDescribesOptionalGuestOrders(t *testing.T) {
	c := NewDefaultCatalog()
	es, ok := c.Get("es", "kitchen-order-flow")
	require.True(t, ok)
	require.Len(t, es.Steps, 4)
	assert.Contains(t, es.Steps[1], "si vas a aceptar pedidos de clientes")
	assert.NotContains(t, es.Steps[1], "si aceptarás")

	esAR, ok := c.Get("es-AR", "kitchen-order-flow")
	require.True(t, ok)
	require.Len(t, esAR.Steps, 4)
	assert.Contains(t, esAR.Steps[1], "si querés aceptar pedidos de clientes")
	assert.NotContains(t, esAR.Steps[1], "si aceptarás")
}

func TestCatalog_SpanishArgentinaUsesCanonicalMozoIALabel(t *testing.T) {
	c := NewDefaultCatalog()
	guide, ok := c.Get("es-AR", "ai-waiter-configure")
	require.True(t, ok)
	assert.Contains(t, guide.Answer, "Mozo IA")
	assert.NotContains(t, guide.Answer, "Camarero IA")
	require.NotEmpty(t, guide.Steps)
	assert.Equal(t, "Abrí Mozo IA", guide.Steps[0])
	assert.Equal(t, "Configurar Mozo IA", guide.FollowUpLabel)
	assert.Equal(t, "¿Cómo configuro Mozo IA?", guide.FollowUpPrompt)
	assert.Equal(t, "Abrir Mozo IA", guide.DestinationLabel)
}

func TestCatalog_SpanishMenuUsesCanonicalAddItemCTA(t *testing.T) {
	c := NewDefaultCatalog()
	tests := []struct {
		locale string
		want   string
	}{
		{locale: "es", want: "Pulsa Agregar Elemento"},
		{locale: "es-AR", want: "Tocá Agregar Elemento"},
	}
	for _, tt := range tests {
		t.Run(tt.locale, func(t *testing.T) {
			guide, ok := c.Get(tt.locale, "menu-add-item")
			require.True(t, ok)
			require.Len(t, guide.Steps, 5)
			assert.Equal(t, tt.want, guide.Steps[2])
			assert.NotContains(t, guide.Steps[2], "Agregar producto")
		})
	}
}

func TestCatalog_SpanishArgentinaBillsAnswerUsesVoseo(t *testing.T) {
	c := NewDefaultCatalog()
	guide, ok := c.Get("es-AR", "bills-open-close")
	require.True(t, ok)
	assert.Equal(t,
		"En Cuentas podés revisar consumos abiertos, registrar pagos y cerrar las mesas cuando el total esté saldado.",
		guide.Answer)
}

func TestCatalog_LocalesPreserveStableGuideShells(t *testing.T) {
	c := NewDefaultCatalog()
	for _, id := range RequiredGuideIDs {
		en, ok := c.Get("en", id)
		require.True(t, ok, "missing English guide %s", id)
		for _, locale := range []string{"es", "es-AR"} {
			localized, ok := c.Get(locale, id)
			require.True(t, ok, "missing %s guide %s", locale, id)
			assert.Equal(t, en.ID, localized.ID, "%s/%s ID", locale, id)
			assert.Equal(t, en.Tab, localized.Tab, "%s/%s tab", locale, id)
			assert.Equal(t, en.Destination, localized.Destination, "%s/%s destination", locale, id)
			assert.Equal(t, en.RequiredPermission, localized.RequiredPermission, "%s/%s permission", locale, id)
			assert.Equal(t, en.WorkflowID, localized.WorkflowID, "%s/%s workflow", locale, id)
			assert.Equal(t, en.FollowUpIDs, localized.FollowUpIDs, "%s/%s follow-ups", locale, id)
		}
	}
}

func TestCatalog_FollowUpsResolveLocalizedTargetCopy(t *testing.T) {
	c := NewDefaultCatalog()
	tests := []struct {
		locale      string
		wantLabel   string
		wantPrompt  string
		wantDestCTA string
	}{
		{locale: "en", wantLabel: "Add a menu item", wantPrompt: "How do I add a menu item?", wantDestCTA: "Open Menu"},
		{locale: "fr", wantLabel: "Add a menu item", wantPrompt: "How do I add a menu item?", wantDestCTA: "Open Menu"},
		{locale: "es", wantLabel: "Agregar un producto al menú", wantPrompt: "¿Cómo agrego un producto al menú?", wantDestCTA: "Abrir Menú"},
		{locale: "es-MX", wantLabel: "Agregar un producto al menú", wantPrompt: "¿Cómo agrego un producto al menú?", wantDestCTA: "Abrir Menú"},
		{locale: "es-AR", wantLabel: "Agregar un producto al menú", wantPrompt: "¿Cómo agrego un producto al menú?", wantDestCTA: "Abrir Menú"},
	}

	for _, tt := range tests {
		t.Run(tt.locale, func(t *testing.T) {
			followUps, err := c.ResolveFollowUps(tt.locale, []string{"menu-add-item"})
			require.NoError(t, err)
			require.Len(t, followUps, 1)
			assert.Equal(t, "menu-add-item", followUps[0].ID)
			assert.Equal(t, tt.wantLabel, followUps[0].Label)
			assert.Equal(t, tt.wantPrompt, followUps[0].Prompt)

			target, ok := c.Get(tt.locale, "menu-add-item")
			require.True(t, ok)
			assert.Equal(t, target.FollowUpLabel, followUps[0].Label)
			assert.Equal(t, target.FollowUpPrompt, followUps[0].Prompt)
			assert.Equal(t, tt.wantDestCTA, target.DestinationLabel)
		})
	}
}

func TestCatalog_FollowUpsDeduplicatePreservingFirstSeenOrder(t *testing.T) {
	c := NewDefaultCatalog()
	got, err := c.ResolveFollowUps("es", []string{
		"tables-create-qr",
		"menu-add-item",
		"tables-create-qr",
		"support-contact",
		"menu-add-item",
	})
	require.NoError(t, err)
	require.Len(t, got, 3)
	assert.Equal(t, "tables-create-qr", got[0].ID)
	assert.Equal(t, "menu-add-item", got[1].ID)
	assert.Equal(t, "support-contact", got[2].ID)
}

func TestCatalog_FollowUpsKeepCanonicalSpanishArgentinaMozoIACopy(t *testing.T) {
	c := NewDefaultCatalog()
	got, err := c.ResolveFollowUps("es-AR", []string{"ai-waiter-configure"})
	require.NoError(t, err)
	require.Len(t, got, 1)
	assert.Equal(t, "Configurar Mozo IA", got[0].Label)
	assert.Equal(t, "¿Cómo configuro Mozo IA?", got[0].Prompt)
	assert.NotContains(t, got[0].Label+" "+got[0].Prompt, "Camarero IA")
}

func TestCatalog_FollowUpsRejectUnknownIDs(t *testing.T) {
	c := NewDefaultCatalog()
	got, err := c.ResolveFollowUps("es", []string{"menu-add-item", "unknown-guide"})
	require.Error(t, err)
	assert.Empty(t, got)
	assert.Contains(t, err.Error(), "unknown-guide")
}

func TestCatalog_FollowUpVisibleCopyNeverExposesInternalIDs(t *testing.T) {
	c := NewDefaultCatalog()

	for _, locale := range []string{"en", "es", "es-AR"} {
		for _, id := range RequiredGuideIDs {
			guide, ok := c.Get(locale, id)
			require.True(t, ok, "%s/%s", locale, id)
			require.NotEmpty(t, guide.FollowUpLabel, "%s/%s label", locale, id)
			require.NotEmpty(t, guide.FollowUpPrompt, "%s/%s prompt", locale, id)
			require.NotEmpty(t, guide.DestinationLabel, "%s/%s destination label", locale, id)

			visibleCopy := strings.ToLower(strings.Join([]string{
				guide.FollowUpLabel,
				guide.FollowUpPrompt,
				guide.DestinationLabel,
			}, " "))
			for _, internalID := range RequiredGuideIDs {
				assert.NotContains(t, visibleCopy, internalID,
					"%s/%s exposes internal guide ID %s", locale, id, internalID)
			}
		}

		director, err := c.ResolveFollowUps(locale, []string{"director-boundary"})
		require.NoError(t, err)
		require.Len(t, director, 1)
		assert.NotContains(t, director[0].Label, "director-boundary")
		assert.NotContains(t, director[0].Prompt, "director-boundary")
	}
}

func TestCatalog_SearchMenuFromBillsTab(t *testing.T) {
	c := NewDefaultCatalog()
	// Active tab is bills — must still find menu guidance.
	hits := c.Search("How do I add menu items?", "en", "bills", 5)
	require.NotEmpty(t, hits)
	assert.Equal(t, "menu-add-item", hits[0].Guide.ID)

	hitsES := c.Search("¿Cómo agrego platos?", "es", "bills", 5)
	require.NotEmpty(t, hitsES)
	assert.Equal(t, "menu-add-item", hitsES[0].Guide.ID)
}

func TestCatalog_AIGuideAnswersNameNoPaidPlan(t *testing.T) {
	catalog := NewDefaultCatalog()
	for _, locale := range []string{"en", "es", "es-AR"} {
		for _, guideID := range []string{"ai-waiter-configure", "director-boundary"} {
			t.Run(locale+"/"+guideID, func(t *testing.T) {
				guide, ok := catalog.Get(locale, guideID)
				require.True(t, ok)
				require.NotEmpty(t, guide.Answer)
				for _, banned := range []string{"AI Growth", "Crecimiento IA", "AI Pro", "ai_pro", "plan"} {
					assert.NotContains(t, guide.Answer, banned)
				}
			})
		}
	}
}

func TestCatalog_SearchSupportIntent(t *testing.T) {
	c := NewDefaultCatalog()
	for _, q := range []string{"can you connect me with support?", "conectame con soporte"} {
		hits := c.Search(q, "en", "bills", 5)
		require.NotEmpty(t, hits, q)
		assert.Equal(t, "support-contact", hits[0].Guide.ID, q)
	}
}

func TestCatalog_OffTopicDoesNotForceGuide(t *testing.T) {
	c := NewDefaultCatalog()
	hits := c.Search("what is the capital of france?", "en", "overview", 5)
	// May be empty or low-confidence; must not claim support-contact or menu-add-item as exact.
	for _, h := range hits {
		assert.False(t, h.Exact && (h.Guide.ID == "support-contact" || h.Guide.ID == "menu-add-item"))
		assert.Less(t, h.Score, 50.0)
	}
}

func TestOpsAssistantHasNoOperationalMutationCapability(t *testing.T) {
	// Source-level boundary: Ops guide destinations and catalog text must not
	// claim restaurant mutations. Support escalation is the only allowed side effect.
	c := NewDefaultCatalog()
	require.NoError(t, c.ValidateCatalog())
	forbidden := []string{"i will create", "i will delete", "i charged", "i refunded", "i published", "i applied"}
	for _, g := range c.byLocale["en"] {
		blob := g.Answer + " " + g.Destination
		for _, f := range forbidden {
			assert.NotContains(t, blob, f, "guide %s", g.ID)
		}
		// Destinations must be trusted tab/route tokens only.
		if g.Destination != "" {
			assert.True(t,
				hasPrefix(g.Destination, "tab:") || hasPrefix(g.Destination, "route:"),
				"guide %s destination %s", g.ID, g.Destination)
		}
	}
}

func hasPrefix(s, p string) bool {
	return len(s) >= len(p) && s[:len(p)] == p
}
