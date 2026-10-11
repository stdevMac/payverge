package services

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/stdevmac/payverge/backend/internal/analytics"
	"github.com/stdevmac/payverge/backend/internal/database"
	"github.com/stdevmac/payverge/backend/internal/llm"
	"github.com/stdevmac/payverge/backend/internal/services/director_tools"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestDirectorFreezeWrites_NoCambiesNadaDoesNotQueueAvailability(t *testing.T) {
	db := newSageOpsDB(t)
	biz := seedSageOpsMenu(t, db)
	thread, err := database.CreateDirectorConsoleThread(biz.ID, "86 steak", "es")
	require.NoError(t, err)

	final := `{"summary":"El steak ya está 86 en pedidos de clientes. No cambié nada.","diagnosis":"Steak Plate sigue marcado disponible en el menú pero no es pedible.","evidence":["Date Night y la oferta de 5 dólares siguen activas."],"actions":[],"expected_impact":"El dueño decide.","follow_ups":[]}`
	prov := &fakeProvider{scripted: []*llm.Response{
		newFunctionCallResponse("propose_availability_change", map[string]any{
			"item_name": "Steak Plate", "available": false,
		}),
		newTextResponse(final),
		newTextResponse(final),
	}}
	svc := newSageOpsService(t, db, prov)

	res, err := svc.Ask(context.Background(), DirectorAskRequest{
		BusinessID: biz.ID,
		ThreadID:   &thread.ID,
		Message:    "El steak esta 86. Que hago con la promo y el combo? No cambies nada vos.",
		Locale:     "es",
	})
	require.NoError(t, err)
	assert.Empty(t, res.ProposedActions)
	var count int64
	db.GetGorm().Model(&database.DirectorProposedAction{}).
		Where("business_id = ?", biz.ID).Count(&count)
	assert.Zero(t, count)
	assert.NotContains(t, res.Response.Summary, "demo-steak")
}

func TestDirectorFreezeWrites_PricePreviewDoesNotQueueApply(t *testing.T) {
	db := newSageOpsDB(t)
	biz := seedSageOpsMenu(t, db)
	thread, err := database.CreateDirectorConsoleThread(biz.ID, "preview bowl", "en")
	require.NoError(t, err)

	final := `{"summary":"Harvest Bowl would move from $18.50 to $20.50. New margin $12.00.","diagnosis":"Preview only.","evidence":["Plate cost $8.50."],"actions":[],"expected_impact":"+$2.00 margin per bowl.","follow_ups":[]}`
	prov := &fakeProvider{scripted: []*llm.Response{
		newFunctionCallResponse("propose_price_change", map[string]any{
			"item_name": "Harvest Bowl", "mode": "flat", "value": float64(2), "direction": "up",
		}),
		newTextResponse(final),
		newTextResponse(final),
	}}
	svc := newSageOpsService(t, db, prov)

	res, err := svc.Ask(context.Background(), DirectorAskRequest{
		BusinessID: biz.ID,
		ThreadID:   &thread.ID,
		Message:    "Raise Harvest Bowl by two dollars and show me the new margin — but do not apply it.",
		Locale:     "en",
	})
	require.NoError(t, err)
	assert.Empty(t, res.ProposedActions)
	var count int64
	db.GetGorm().Model(&database.DirectorProposedAction{}).
		Where("business_id = ?", biz.ID).Count(&count)
	assert.Zero(t, count)
}

func TestDirectorGuardScrubsLeakedSchemaOnAsk(t *testing.T) {
	db := newSageOpsDB(t)
	biz := seedSageOpsMenu(t, db)
	leaky := `{"summary":"qty_sold is 0 and weekly_revenue is 0.","diagnosis":"Use get_menu_top_items.","evidence":["get_food_cost_analysis failed"],"actions":[{"title":"Open analytics","description":"See sales","deep_link":"/business/1/dashboard?tab=analytics","priority":"high"}],"expected_impact":"Clarity","follow_ups":[]}`
	prov := &fakeProvider{scripted: []*llm.Response{
		newTextResponse(leaky),
		newTextResponse(leaky),
	}}
	svc := newSageOpsService(t, db, prov)
	res, err := svc.Ask(context.Background(), DirectorAskRequest{
		BusinessID: biz.ID,
		Message:    "How much did we sell this week?",
		Locale:     "en",
	})
	require.NoError(t, err)
	assertNoDirectorWireLeaks(t, res.Response)
}

func TestDirectorAsk_SpanishSlowMoversDoesNotLeakWireFields(t *testing.T) {
	db := newSageOpsDB(t)
	biz := seedSageOpsMenuWithMargins(t, db)
	leaky := `{"summary":"Los platos se movieron 0 unidades. qty_sold is 0.","diagnosis":"data_readiness.state is setup; metrics.weekly_revenue is 0.","evidence":["data_readiness","metrics.weekly_revenue is 0","Call get_menu_top_items then get_food_cost_analysis","top-sellers report empty"],"actions":[{"title":"Open analytics","description":"See sales","deep_link":"/business/1/dashboard?tab=analytics","priority":"high"}],"expected_impact":"Clarity","follow_ups":[]}`
	prov := &fakeProvider{scripted: []*llm.Response{
		newTextResponse(leaky),
		newTextResponse(leaky),
	}}
	svc := newSageOpsService(t, db, prov)
	res, err := svc.Ask(context.Background(), DirectorAskRequest{
		BusinessID: biz.ID,
		Message:    "Soy el dueno. Que platos se estan moviendo poco esta semana?",
		Locale:     "es",
	})
	require.NoError(t, err)
	assertNoDirectorWireLeaks(t, res.Response)
	blob := directorProseBlob(res.Response)
	assert.NotContains(t, strings.ToLower(blob), "0 unidades")
	assert.NotContains(t, strings.ToLower(blob), "0 units")
	assert.True(t, strings.Contains(blob, "Harvest Bowl") || strings.Contains(blob, "Steak Plate") || strings.Contains(blob, "House Burger"),
		"slow-movers ask must name a live dish, got %q", blob)
}

func TestDirectorAsk_EnglishMarginWalkthroughUsesToolResults(t *testing.T) {
	db := newSageOpsDB(t)
	biz := seedSageOpsMenuWithMargins(t, db)
	leaky := `{"summary":"I cannot identify best-sellers or thinnest margins without the top-sellers report and food-cost analysis.","diagnosis":"Call get_menu_top_items then get_food_cost_analysis.","evidence":["get_food_cost_analysis failed","data_readiness.state is setup"],"actions":[{"title":"Open analytics","description":"Run the reports","deep_link":"/business/1/dashboard?tab=analytics","priority":"high"}],"expected_impact":"Clarity","follow_ups":[]}`
	prov := &fakeProvider{scripted: []*llm.Response{
		newTextResponse(leaky),
		newTextResponse(leaky),
	}}
	svc := newSageOpsService(t, db, prov)
	res, err := svc.Ask(context.Background(), DirectorAskRequest{
		BusinessID: biz.ID,
		Message:    "Which dishes are our best sellers and which have the thinnest margin? Run the advertised margin analysis.",
		Locale:     "en",
	})
	require.NoError(t, err)
	assertNoDirectorWireLeaks(t, res.Response)
	blob := directorProseBlob(res.Response)
	assert.Contains(t, blob, "House Burger", "thinnest menu-card margin should be House Burger")
	assert.Contains(t, blob, "$3.00", "must cite the $3.00 plate margin")
	assert.NotContains(t, strings.ToLower(blob), "cannot identify")
}

func TestDirectorAsk_AdvertisedMarginWalkthroughShowsCurrentAndNewMargin(t *testing.T) {
	db := newSageOpsDB(t)
	biz := seedSageOpsMenu(t, db)
	liveShaped := `{"summary":"Two of our best-selling dishes look like they have the thinnest margins: Harvest Bowl and Steak Plate. A small price adjustment would add +$1.50 and +$3.00 in revenue impact. Food cost percentage is not available.","diagnosis":"Food cost percentage is not available, so I cannot show current margin or a new-margin preview.","evidence":["Harvest Bowl +$1.50 revenue impact","Steak Plate +$3.00 revenue impact"],"actions":[{"title":"Review pricing","description":"Consider raising prices","deep_link":"/business/1/dashboard?tab=menu","priority":"high"}],"expected_impact":"+$1.50 / +$3.00 revenue impact.","follow_ups":[]}`
	prov := &fakeProvider{scripted: []*llm.Response{
		newTextResponse(liveShaped),
		newTextResponse(liveShaped),
	}}
	svc := newSageOpsService(t, db, prov)
	res, err := svc.Ask(context.Background(), DirectorAskRequest{
		BusinessID: biz.ID,
		Message:    "Two of our best-selling dishes look like they have the thinnest margins. Show a small price adjustment on both with the new margin before anything goes live.",
		Locale:     "en",
	})
	require.NoError(t, err)
	assertNoDirectorWireLeaks(t, res.Response)
	blob := directorProseBlob(res.Response)
	assert.NotContains(t, strings.ToLower(blob), "food cost percentage is not available")
	assert.Contains(t, blob, "Harvest Bowl")
	assert.Contains(t, blob, "Steak Plate")
	assert.Contains(t, blob, "$10.00", "current Harvest Bowl margin")
	assert.Contains(t, blob, "$13.00", "current Steak Plate margin")
	assert.Contains(t, blob, "$11.50", "new Harvest Bowl margin after +$1.50")
	assert.Contains(t, blob, "$14.50", "new Steak Plate margin after +$1.50")
	assert.Contains(t, strings.ToLower(blob), "current margin")
	assert.Contains(t, strings.ToLower(blob), "new margin")
	assert.Contains(t, strings.ToLower(blob), "food cost")
	assert.Contains(t, blob, "%")
	assert.Contains(t, strings.ToLower(blob), "nothing is queued")
	assert.NotContains(t, strings.ToLower(blob), "if this preview is applied")
	assert.NotContains(t, strings.ToLower(blob), "after a $1.50 price increase")
}

func TestDirectorAsk_LiveShapedWalkthroughAsksForPriceGetsPreview(t *testing.T) {
	// Exact 2026-08-21 live failure: current Steak numbers only, Spritz
	// "plate cost not available", then the model asks what price to consider.
	db := newSageOpsDB(t)
	biz := seedSageOpsMenuItems(t, db, []database.MenuItem{
		{ID: "demo-steak", Name: "Steak Plate", Price: 42.00, Cogs: 7.52, IsAvailable: true},
		{ID: "demo-spritz", Name: "Demo Spritz", Price: 14.00, IsAvailable: true},
		{ID: "demo-bowl", Name: "Harvest Bowl", Price: 18.50, Cogs: 8.50, IsAvailable: true},
	})
	liveShaped := `{"summary":"Steak Plate 158 sold / $42 / plate $7.52 / margin $34.48. Demo Spritz 163 sold / $14. Plate cost information is not available.","diagnosis":"I can show the new margin once you tell me what price to consider.","evidence":["Steak Plate 158 sold / $42 / plate $7.52 / margin $34.48","Demo Spritz 163 sold / $14"],"actions":[{"title":"Update plate cost for Demo Spritz","description":"Add a plate cost so we can compute margin.","deep_link":"/business/1/dashboard?tab=menu","priority":"high"}],"expected_impact":"Clarity.","follow_ups":["What price should I consider?"]}`
	prov := &fakeProvider{scripted: []*llm.Response{
		newTextResponse(liveShaped),
		newTextResponse(liveShaped),
	}}
	svc := newSageOpsService(t, db, prov)
	res, err := svc.Ask(context.Background(), DirectorAskRequest{
		BusinessID: biz.ID,
		Message:    "two best-selling dishes look like they have the thinnest margin, show me the new margin before anything goes live",
		Locale:     "en",
	})
	require.NoError(t, err)
	assertNoDirectorWireLeaks(t, res.Response)
	assert.Empty(t, res.ProposedActions)
	blob := directorProseBlob(res.Response)
	assert.NotContains(t, strings.ToLower(blob), "what price to consider")
	assert.NotContains(t, strings.ToLower(blob), "what price should")
	assert.NotContains(t, strings.ToLower(blob), "plate cost information is not available")
	assert.NotContains(t, blob, "preview_margin_change")
	assert.Contains(t, blob, "Steak Plate")
	assert.Contains(t, blob, "$34.48", "current Steak margin")
	assert.Contains(t, blob, "$35.98", "new Steak margin after +$1.50")
	assert.Contains(t, blob, "$10.00", "current Harvest Bowl margin")
	assert.Contains(t, blob, "$11.50", "new Harvest Bowl margin after +$1.50")
	assert.Contains(t, strings.ToLower(blob), "current margin")
	assert.Contains(t, strings.ToLower(blob), "new margin")
	assert.Contains(t, strings.ToLower(blob), "food cost")
	assert.Contains(t, blob, "%")
	assert.Contains(t, strings.ToLower(blob), "nothing is queued")
	assert.NotContains(t, strings.ToLower(blob), "if this preview is applied")
	assert.NotContains(t, strings.ToLower(blob), "after a $1.50 price increase")
}

func TestDirectorAsk_BestSellingAdjectivePhrasingRunsMarginWalkthrough(t *testing.T) {
	// The advertised copy is "your best-selling dishes are carrying your
	// thinnest margins", so operators paraphrase the ask with the adjective
	// ("best-selling", "best selling", "bestselling"). The grounding gate used
	// to accept only the noun forms, so these asks skipped grounding entirely
	// and shipped the model's "food cost percentage is not available" reply.
	for _, q := range []string{
		"What are our two best-selling dishes, and what is the margin on each?",
		"Which of our best selling dishes have the worst margins?",
		"Show me the margin on our bestselling plates.",
		"Which are our top-selling plates right now?",
	} {
		assert.True(t, directorWantsMenuPerformance(q), "menu-performance gate should accept %q", q)
	}

	db := newSageOpsDB(t)
	biz := seedSageOpsMenu(t, db)
	liveShaped := `{"summary":"Two of our best-selling dishes look like they have the thinnest margins: Harvest Bowl and Steak Plate. A small price adjustment would add +$1.50 and +$3.00 in revenue impact. Food cost percentage is not available.","diagnosis":"Food cost percentage is not available, so I cannot show current margin or a new-margin preview.","evidence":["Harvest Bowl +$1.50 revenue impact","Steak Plate +$3.00 revenue impact"],"actions":[{"title":"Review pricing","description":"Consider raising prices","deep_link":"/business/1/dashboard?tab=menu","priority":"high"}],"expected_impact":"+$1.50 / +$3.00 revenue impact.","follow_ups":[]}`
	prov := &fakeProvider{scripted: []*llm.Response{
		newTextResponse(liveShaped),
		newTextResponse(liveShaped),
	}}
	svc := newSageOpsService(t, db, prov)
	res, err := svc.Ask(context.Background(), DirectorAskRequest{
		BusinessID: biz.ID,
		Message:    "What are our two best-selling dishes, and what is the margin on each?",
		Locale:     "en",
	})
	require.NoError(t, err)
	assertNoDirectorWireLeaks(t, res.Response)
	blob := directorProseBlob(res.Response)
	assert.NotContains(t, strings.ToLower(blob), "food cost percentage is not available")
	assert.Contains(t, blob, "Harvest Bowl")
	assert.Contains(t, blob, "Steak Plate")
	assert.Contains(t, blob, "$10.00", "Harvest Bowl margin")
	assert.Contains(t, blob, "$13.00", "Steak Plate margin")
}

func TestDirectorAsk_BestSellerUsesSiblingThreadInsteadOfNoData(t *testing.T) {
	db := newSageOpsDB(t)
	biz := seedSageOpsMenuWithMargins(t, db)
	_, err := database.CreateDirectorConsoleThread(biz.ID, "Two of our best-selling dishes look like they have the thinnest margins: Harvest Bowl", "en")
	require.NoError(t, err)

	canned := `{"summary":"No hay datos suficientes","diagnosis":"No tengo cifras ni un período que pueda mostrar para este consejo de ingresos.","evidence":["weekly_revenue is 0","data_readiness.state is setup"],"actions":[{"title":"Abrí Analítica","description":"Ver ventas","deep_link":"/business/1/dashboard?tab=analytics","priority":"medium"}],"expected_impact":"","follow_ups":[]}`
	prov := &fakeProvider{scripted: []*llm.Response{
		newTextResponse(canned),
		newTextResponse(canned),
	}}
	svc := newSageOpsService(t, db, prov)
	res, err := svc.Ask(context.Background(), DirectorAskRequest{
		BusinessID: biz.ID,
		Message:    "what's our best seller",
		Locale:     "es",
	})
	require.NoError(t, err)
	assertNoDirectorWireLeaks(t, res.Response)
	blob := directorProseBlob(res.Response)
	assert.NotContains(t, blob, "No hay datos suficientes")
	assert.NotContains(t, blob, "No tengo cifras")
	assert.Contains(t, blob, "Harvest Bowl")
}

func TestDirectorAsk_TonightSalesDoesNotClaimPaymentsDisabled(t *testing.T) {
	db := newSageOpsDB(t)
	biz := seedSageOpsMenu(t, db)
	now := time.Now()
	require.NoError(t, db.GetGorm().Create(&database.Bill{
		BusinessID:     biz.ID,
		BillNumber:     "761",
		Subtotal:       3608,
		TotalAmount:    3608,
		PaidAmount:     1804,
		Status:         database.BillStatusPartial,
		SettlementAddr: "settlement-761",
		TippingAddr:    "tipping-761",
		CreatedAt:      now,
		UpdatedAt:      now,
	}).Error)

	canned := `{"summary":"El sistema de pagos no está habilitado, así que no se pueden registrar ventas.","diagnosis":"Payments are not enabled. Not enough data.","evidence":["payments_enabled is false"],"actions":[{"title":"Connect payments","description":"Enable a processor","deep_link":"/business/1/dashboard?tab=plugins","priority":"high"}],"expected_impact":"","follow_ups":[]}`
	prov := &fakeProvider{scripted: []*llm.Response{
		newTextResponse(canned),
		newTextResponse(canned),
	}}
	svc := newSageOpsService(t, db, prov)
	res, err := svc.Ask(context.Background(), DirectorAskRequest{
		BusinessID: biz.ID,
		Message:    "how much did we sell tonight",
		Locale:     "en",
	})
	require.NoError(t, err)
	blob := strings.ToLower(directorProseBlob(res.Response))
	assert.NotContains(t, blob, "sistema de pagos no")
	assert.NotContains(t, blob, "payments are not enabled")
	assert.NotContains(t, blob, "not enough data")
	assert.Contains(t, res.Response.Summary, "18.04")
}

func TestDirectorLiveOpsQuestionDoesNotAddPluginPadding(t *testing.T) {
	q := "qué mesas están libres?"
	assert.True(t, directorIsLiveOpsQuestion(q))
	assert.False(t, directorWantsPluginUpsell(q))
	assert.True(t, directorIsLiveOpsQuestion("quién está en cocina ahora"))
	assert.True(t, directorIsLiveOpsQuestion("cierra la caja"))
	assert.True(t, directorIsLiveOpsQuestion("mandá un mozo a la 12"))
	assert.True(t, directorIsLiveOpsQuestion("what's our best seller"))
	assert.True(t, directorIsLiveOpsQuestion("how much did we sell tonight"))
	assert.True(t, directorIsLiveOpsQuestion("thinnest margin"))
	assert.True(t, directorIsLiveOpsQuestion("Que platos se estan moviendo poco esta semana"))
	assert.True(t, directorFreezeWrites("No cambies nada vos."))
	assert.True(t, directorFreezeWrites("show me the new margin — but do not apply it."))
	assert.True(t, directorFreezeWrites("show me the new margin before anything goes live"))
	assert.True(t, directorFreezeWrites("mostrame el margen nuevo antes de que esté en vivo"))
	assert.True(t, directorFreezeWrites("mostrame el margen nuevo antes de que este en vivo"))
	assert.True(t, directorFreezeWrites("mostrame el margen nuevo antes de que vaya en vivo"))
	assert.True(t, directorFreezeWrites("mostrame el margen nuevo antes de que esté live"))
	assert.False(t, directorFreezeWrites("raise Harvest Bowl by two dollars"))
	assert.True(t, directorWantsMarginPreview("Show a small price adjustment on both with the new margin before anything goes live."))
	assert.True(t, directorWantsMarginPreview("two best-selling dishes look like they have the thinnest margin, show me the new margin before anything goes live"))
	assert.True(t, directorWantsMarginPreview("mostrame el margen nuevo antes de que esté en vivo"))
	assert.False(t, directorWantsMarginPreview("Which dishes are our best sellers and which have the thinnest margin?"))
	assert.False(t, directorHasNewMarginDollars(DirectorStructuredResponse{
		Summary:   "Steak Plate margin $34.48. I can show the new margin once you tell me what price to consider.",
		Diagnosis: "Plate cost information is not available for Demo Spritz.",
	}, "two best-selling dishes look like they have the thinnest margin, show me the new margin before anything goes live"))

	namedQ := "Raise Harvest Bowl by two dollars and show me the new margin before anything goes live"
	bump, ok := directorNamedPriceBump(namedQ)
	assert.True(t, ok)
	assert.InDelta(t, 2.00, bump, 0.001)
	assert.InDelta(t, 2.00, directorPreviewBump(namedQ), 0.001)
	plus, plusOK := directorNamedPriceBump("preview Harvest Bowl +$2 before anything goes live")
	assert.True(t, plusOK)
	assert.InDelta(t, 2.00, plus, 0.001)
	byCash, byCashOK := directorNamedPriceBump("raise Harvest Bowl by $2 before anything goes live")
	assert.True(t, byCashOK)
	assert.InDelta(t, 2.00, byCash, 0.001)
	esBump, esOK := directorNamedPriceBump("aumentalo dos dólares antes de que esté en vivo")
	assert.True(t, esOK)
	assert.InDelta(t, 2.00, esBump, 0.001)
	assert.True(t, directorHasNewMarginDollars(DirectorStructuredResponse{
		Summary: "Harvest Bowl new margin $12.00 after +$2.00.",
	}, namedQ), "a named +$2 preview must not be treated as missing just because it omitted 'current margin $'")
}

func TestDirectorAsk_PreviewMarginChangeToolRunUsesNamedTwoDollarBump(t *testing.T) {
	db := newSageOpsDB(t)
	biz := seedSageOpsMenu(t, db)
	thread, err := database.CreateDirectorConsoleThread(biz.ID, "named two dollar preview", "en")
	require.NoError(t, err)

	// Owner named +$2. The model reply omits "current margin $", which used to
	// make grounding clobber the preview with the default +$1.50 ($11.50).
	final := `{"summary":"Harvest Bowl new margin $12.00. Preview only.","diagnosis":"Named +$2 preview.","evidence":["Harvest Bowl new margin $12.00"],"actions":[],"expected_impact":"Nothing is queued.","follow_ups":[]}`
	prov := &fakeProvider{scripted: []*llm.Response{
		newFunctionCallResponse("preview_margin_change", map[string]any{
			"item_name": "Harvest Bowl", "value": float64(2),
		}),
		newTextResponse(final),
		newTextResponse(final),
	}}
	svc := newSageOpsService(t, db, prov)

	res, err := svc.Ask(context.Background(), DirectorAskRequest{
		BusinessID: biz.ID,
		ThreadID:   &thread.ID,
		Message:    "Raise Harvest Bowl by two dollars and show me the new margin before anything goes live",
		Locale:     "en",
	})
	require.NoError(t, err)
	assert.Empty(t, res.ProposedActions)
	var count int64
	db.GetGorm().Model(&database.DirectorProposedAction{}).
		Where("business_id = ?", biz.ID).Count(&count)
	assert.Zero(t, count)

	var calls []database.DirectorToolCall
	err = database.GetDB().Where("thread_id = ?", thread.ID).Order("id asc").Find(&calls).Error
	require.NoError(t, err)
	require.NotEmpty(t, calls, "preview_margin_change must actually Run")
	assert.Equal(t, "preview_margin_change", calls[0].ToolName)
	assert.True(t, calls[0].Success)
	assert.Contains(t, calls[0].ArgsJSON, "2")

	blob := directorProseBlob(res.Response)
	assert.Contains(t, blob, "$12.00", "named +$2 preview is $12.00, not the default +$1.50")
	assert.NotContains(t, blob, "$11.50")
	assert.NotContains(t, blob, "preview_margin_change")
	assert.Contains(t, strings.ToLower(blob), "nothing is queued")
	assertHarvestBowlPriceUnchanged(t, biz.ID)
}

func TestDirectorAsk_SpanishAntesDeQueEsteEnVivoDoesNotPersistPropose(t *testing.T) {
	db := newSageOpsDB(t)
	biz := seedSageOpsMenu(t, db)
	thread, err := database.CreateDirectorConsoleThread(biz.ID, "margen preview es", "es")
	require.NoError(t, err)

	final := `{"summary":"Harvest Bowl pasaría de $18.50 a $20.00. Margen nuevo $11.50.","diagnosis":"Solo vista previa, no se encoló nada.","evidence":["Costo de plato $8.50."],"actions":[],"expected_impact":"Nada se encoló.","follow_ups":[]}`
	prov := &fakeProvider{scripted: []*llm.Response{
		newFunctionCallResponse("propose_price_change", map[string]any{
			"item_name": "Harvest Bowl", "mode": "flat", "value": float64(1.5), "direction": "up",
		}),
		newTextResponse(final),
		newTextResponse(final),
	}}
	svc := newSageOpsService(t, db, prov)

	res, err := svc.Ask(context.Background(), DirectorAskRequest{
		BusinessID: biz.ID,
		ThreadID:   &thread.ID,
		Message:    "Mostrame el margen nuevo de Harvest Bowl antes de que esté en vivo",
		Locale:     "es",
	})
	require.NoError(t, err)
	assert.Empty(t, res.ProposedActions)
	var count int64
	db.GetGorm().Model(&database.DirectorProposedAction{}).
		Where("business_id = ?", biz.ID).Count(&count)
	assert.Zero(t, count, "Spanish preview-only phrase must freeze propose_price_change")
	assertHarvestBowlPriceUnchanged(t, biz.ID)
}

func TestDirectorAskStreaming_PreviewMarginChangeHumanLabelDoesNotLeakToolName(t *testing.T) {
	for _, tc := range []struct {
		locale string
		want   string
		final  string
		msg    string
	}{
		{
			locale: "en",
			want:   "Previewing new margin",
			final:  `{"summary":"Harvest Bowl current margin $10.00, new margin $11.50. Preview only, nothing is queued.","diagnosis":"Preview only — nothing is queued or staged.","evidence":["Harvest Bowl current margin $10.00, new margin $11.50"],"actions":[],"expected_impact":"Nothing is queued.","follow_ups":[]}`,
			msg:    "Show me the new Harvest Bowl margin before anything goes live",
		},
		{
			locale: "es",
			want:   "Calculando el margen nuevo",
			final:  `{"summary":"Harvest Bowl margen actual $10.00, margen nuevo $11.50. Solo vista previa, no se encoló nada.","diagnosis":"Solo vista previa — no se encoló ni se guardó nada.","evidence":["Harvest Bowl margen actual $10.00, margen nuevo $11.50"],"actions":[],"expected_impact":"No se encoló nada.","follow_ups":[]}`,
			msg:    "Mostrame el margen nuevo de Harvest Bowl antes de que esté en vivo",
		},
	} {
		t.Run(tc.locale, func(t *testing.T) {
			db := newSageOpsDB(t)
			biz := seedSageOpsMenu(t, db)
			thread, err := database.CreateDirectorConsoleThread(biz.ID, "sse preview "+tc.locale, tc.locale)
			require.NoError(t, err)

			prov := &fakeProvider{scripted: []*llm.Response{
				newFunctionCallResponse("preview_margin_change", map[string]any{
					"item_name": "Harvest Bowl",
				}),
				newTextResponse(tc.final),
				newTextResponse(tc.final),
			}}
			svc := newSageOpsService(t, db, prov)
			sink := &bufferSink{}
			_, err = svc.AskStreaming(context.Background(), DirectorAskRequest{
				BusinessID: biz.ID,
				ThreadID:   &thread.ID,
				Message:    tc.msg,
				Locale:     tc.locale,
			}, sink)
			require.NoError(t, err)

			var label string
			for _, ev := range sink.events {
				if ev.Type != "tool.call.started" {
					continue
				}
				if name, _ := ev.Payload["name"].(string); name == "preview_margin_change" {
					label, _ = ev.Payload["human_label"].(string)
					break
				}
			}
			require.NotEmpty(t, label, "tool.call.started must emit a human_label")
			assert.Equal(t, tc.want, label)
			assert.NotContains(t, label, "preview_margin_change")
		})
	}
}

func assertHarvestBowlPriceUnchanged(t *testing.T, businessID uint) {
	t.Helper()
	_, cats, err := database.GetMenuByBusinessID(businessID)
	require.NoError(t, err)
	for _, cat := range cats {
		for _, item := range cat.Items {
			if item.Name == "Harvest Bowl" {
				assert.InDelta(t, 18.50, item.Price, 0.001, "preview must not write Harvest Bowl")
				return
			}
		}
	}
	t.Fatal("Harvest Bowl missing after preview")
}

func newSageOpsDB(t *testing.T) *database.DB {
	t.Helper()
	db := newServiceTestDB(t)
	require.NoError(t, db.GetGorm().AutoMigrate(
		&database.Payment{},
		&database.Bill{},
		&database.AlternativePayment{},
		&database.Menu{},
		&database.DirectorProposedAction{},
		&database.Offer{},
		&database.Bundle{},
		&database.Table{},
		&database.Staff{},
		&database.Order{},
	))
	require.NoError(t, db.GetGorm().Exec(`
		CREATE TABLE IF NOT EXISTS bill_items (
			id TEXT PRIMARY KEY,
			bill_id INTEGER NOT NULL,
			menu_item_id TEXT DEFAULT '',
			name TEXT NOT NULL,
			price REAL NOT NULL,
			quantity INTEGER NOT NULL,
			options TEXT,
			item_type TEXT DEFAULT 'menu_item',
			bundle_id INTEGER,
			parent_bundle_id INTEGER,
			source_offer_id INTEGER,
			order_id INTEGER,
			subtotal REAL NOT NULL,
			created_at DATETIME
		)
	`).Error)
	return db
}

func seedSageOpsMenu(t *testing.T, db *database.DB) database.Business {
	t.Helper()
	return seedSageOpsMenuItems(t, db, []database.MenuItem{
		{ID: "demo-bowl", Name: "Harvest Bowl", Price: 18.50, Cogs: 8.50, IsAvailable: true},
		{ID: "demo-steak", Name: "Steak Plate", Price: 24.00, Cogs: 11.00, IsAvailable: true},
	})
}

func seedSageOpsMenuWithMargins(t *testing.T, db *database.DB) database.Business {
	t.Helper()
	return seedSageOpsMenuItems(t, db, []database.MenuItem{
		{ID: "demo-bowl", Name: "Harvest Bowl", Price: 18.50, Cogs: 8.50, IsAvailable: true},
		{ID: "demo-steak", Name: "Steak Plate", Price: 24.00, Cogs: 11.00, IsAvailable: true},
		{ID: "demo-burger", Name: "House Burger", Price: 12.00, Cogs: 9.00, IsAvailable: true},
	})
}

func seedSageOpsMenuItems(t *testing.T, db *database.DB, items []database.MenuItem) database.Business {
	t.Helper()
	biz := createTestBusinessForService(t, db, "Sage Ops Lounge")
	cats := []database.MenuCategory{{
		ID:    "cat-mains",
		Name:  "Mains",
		Items: items,
	}}
	raw, err := json.Marshal(cats)
	require.NoError(t, err)
	require.NoError(t, db.GetGorm().Create(&database.Menu{
		BusinessID: biz.ID, Categories: string(raw), IsActive: true, Version: 1,
	}).Error)
	return biz
}

func assertNoDirectorWireLeaks(t *testing.T, resp DirectorStructuredResponse) {
	t.Helper()
	blob := directorProseBlob(resp)
	for _, leaked := range directorBannedWirePhrases {
		assert.NotContains(t, blob, leaked, "director reply must not contain %q", leaked)
	}
}

func newSageOpsService(t *testing.T, db *database.DB, prov llm.Provider) *DirectorConsoleService {
	t.Helper()
	ai, err := NewAIService(prov, llm.ModelConfig{Director: "test-model"})
	require.NoError(t, err)
	reg := director_tools.NewRegistry()
	reg.Register(&director_tools.BusinessProfileTool{})
	reg.Register(&director_tools.ProposePriceChangeTool{})
	reg.Register(&director_tools.ProposeAvailabilityChangeTool{})
	reg.Register(&director_tools.LiveFloorTool{})
	reg.Register(&director_tools.KitchenStatusTool{})
	reg.Register(&director_tools.PromosTool{})
	reg.Register(&director_tools.FoodCostAnalysisTool{})
	reg.Register(&director_tools.PreviewMarginChangeTool{})
	reg.Register(&director_tools.MenuTopItemsTool{})
	reg.Register(&director_tools.RevenueSummaryTool{})
	return NewDirectorConsoleService(db, analytics.NewAnalyticsService(db), ai, reg)
}
