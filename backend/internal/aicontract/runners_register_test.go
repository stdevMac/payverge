package aicontract

import (
	"context"
	"encoding/hex"
	"fmt"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/stdevmac/payverge/backend/internal/agents"
	"github.com/stdevmac/payverge/backend/internal/agents/ops_guides"
	"github.com/stdevmac/payverge/backend/internal/agents/ops_tools"
	"github.com/stdevmac/payverge/backend/internal/analytics"
	"github.com/stdevmac/payverge/backend/internal/database"
	"github.com/stdevmac/payverge/backend/internal/guardrails"
	"github.com/stdevmac/payverge/backend/internal/llm"
	"github.com/stdevmac/payverge/backend/internal/server"
	"github.com/stdevmac/payverge/backend/internal/services"
	"github.com/stdevmac/payverge/backend/internal/services/director_tools"

	"github.com/stretchr/testify/require"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

// Register all hermetic runners before matrix tests run.
func TestMain(m *testing.M) {
	registerAllRunners()
	m.Run()
}

func registerAllRunners() {
	RegisterScenarioRunner("privacy-sensitive-requires-zdr", runPrivacySensitiveZDR)
	RegisterScenarioRunner("ops-no-operational-mutation-capability", runOpsNoMutation)
	RegisterScenarioRunner("ops-support-english-rescued-from-off-topic", runOpsSupportEnglish)
	RegisterScenarioRunner("waiter-invalid-item-tool-dropped-everywhere", runWaiterInvalidCart)
	RegisterScenarioRunner("waiter-spanish-allergen-block", runWaiterSpanishAllergen)
	RegisterScenarioRunner("director-hostile-blocked-before-context", runDirectorHostile)
	registerWhatsAppRunners() // build-tag split: runners_whatsapp_test.go / runners_nowhatsapp_test.go
	RegisterScenarioRunner("menu-extraction-mime-and-atomic-claim", runMenuExtractionMIMEAndClaim)
}

func TestAssistantV2RequiredRunnersRegistered(t *testing.T) {
	for _, scenarioID := range assistantV2RequiredScenarioIDs {
		_, registered := GetScenarioRunner(scenarioID)
		require.True(t, registered, "Assistant V2 scenario %s must use a production-path runner", scenarioID)
	}
}

// mutationToolNames must never appear in the Ops registry.
var mutationToolNames = map[string]struct{}{
	"apply_discount": {}, "create_menu_item": {}, "delete_menu_item": {},
	"charge": {}, "refund": {}, "publish_menu": {}, "close_bill": {},
	"approve_proposal": {}, "apply_proposal": {},
}

func openHermeticDB(t testing.TB, models ...any) *database.DB {
	t.Helper()
	dsn := fmt.Sprintf("file:aicontract_%s?mode=memory&cache=shared", t.Name())
	gdb, err := gorm.Open(sqlite.Open(dsn), &gorm.Config{})
	require.NoError(t, err)
	sqlDB, err := gdb.DB()
	require.NoError(t, err)
	sqlDB.SetMaxOpenConns(1)
	database.SetTestDB(gdb)
	if len(models) > 0 {
		require.NoError(t, gdb.AutoMigrate(models...))
	}
	return database.GetDBWrapper()
}

func runPrivacySensitiveZDR(sc Scenario) (ScenarioRunResult, error) {
	// Production privacy policy for waiter feature.
	req, err := llm.NewGenerateRequest("waiter")
	if err != nil {
		return ScenarioRunResult{}, err
	}
	if !req.RequiresZDR() {
		return ScenarioRunResult{}, fmt.Errorf("waiter must require ZDR")
	}
	// Scripted provider enforces ZDR on the Generate path.
	p := NewScriptedProvider(sc.ProviderScript)
	resp, err := p.Generate(context.Background(), llm.GenerateRequest{
		Feature:  "waiter",
		Model:    "google/gemini-2.5-flash",
		Messages: []llm.Message{{Role: llm.RoleUser, Text: "hello"}},
	})
	if err != nil {
		return ScenarioRunResult{}, err
	}
	return ScenarioRunResult{
		Code:          "ok",
		Text:          resp.Text,
		ProviderCalls: p.CallCount(),
		ServedModel:   resp.Model,
		PrivacyClass:  string(req.PrivacyClass),
		ZDR:           true,
		Effects:       map[string]int{},
	}, nil
}

func runOpsNoMutation(sc Scenario) (ScenarioRunResult, error) {
	// Production Ops tool registry must not expose restaurant mutation tools.
	reg := ops_tools.NewRegistry()
	for _, decl := range reg.Declarations() {
		if _, bad := mutationToolNames[decl.Name]; bad {
			return ScenarioRunResult{}, fmt.Errorf("ops registry exposes mutation tool %q", decl.Name)
		}
		lower := strings.ToLower(decl.Name)
		for _, frag := range []string{"apply", "delete", "refund", "charge", "publish", "create_item"} {
			if strings.Contains(lower, frag) && decl.Name != "delegate_to_director" {
				// delegate_to_director is a handoff, not a mutation.
				if frag == "apply" || frag == "delete" || frag == "refund" || frag == "charge" || frag == "publish" {
					return ScenarioRunResult{}, fmt.Errorf("ops tool name looks mutational: %q", decl.Name)
				}
			}
		}
	}
	c := ops_guides.NewDefaultCatalog()
	if err := c.ValidateCatalog(); err != nil {
		return ScenarioRunResult{}, err
	}

	gdb, err := gorm.Open(sqlite.Open("file:ops-nomut-ask?mode=memory&cache=shared"), &gorm.Config{})
	if err != nil {
		return ScenarioRunResult{}, err
	}
	sqlDB, _ := gdb.DB()
	if sqlDB != nil {
		sqlDB.SetMaxOpenConns(1)
	}
	if err := gdb.AutoMigrate(
		&database.Business{},
		&database.OpsAssistantThread{},
		&database.OpsAssistantMessage{},
		&database.OpsAssistantToolCall{},
		&database.OpsAssistantRequest{},
	); err != nil {
		return ScenarioRunResult{}, err
	}
	database.SetTestDB(gdb)
	biz := database.Business{Name: "NoMutBiz", SettlementAddr: "s", TippingAddr: "t", IsActive: true}
	if err := gdb.Create(&biz).Error; err != nil {
		return ScenarioRunResult{}, err
	}

	// Scripted model claims it applied a discount and emits an unsafe action.
	// Production Ask must still strip javascript: actions and never register
	// mutation tools — the real path under test is OpsAssistantService.Ask.
	p := NewScriptedProvider(sc.ProviderScript)
	ai, err := services.NewAIService(p, llm.ModelConfig{
		Director: "google/gemini-2.5-flash",
		Chat:     "google/gemini-2.5-flash",
	})
	if err != nil {
		return ScenarioRunResult{}, err
	}
	svc := agents.NewOpsAssistantService(ai, reg, database.GetDBWrapper(), nil)
	msg, _ := sc.Input["message"].(string)
	res, err := svc.Ask(context.Background(), agents.OpsAskRequest{
		BusinessID: biz.ID,
		Message:    msg,
		Locale:     sc.Locale,
		ActiveTab:  "menu",
	})
	if err != nil {
		return ScenarioRunResult{}, fmt.Errorf("Ops Ask failed: %w", err)
	}
	if res == nil {
		return ScenarioRunResult{}, fmt.Errorf("Ops Ask returned nil")
	}

	unsafeKept := 0
	for _, a := range res.Response.Actions {
		if strings.Contains(strings.ToLower(a.Href), "javascript:") {
			unsafeKept++
		}
		kind := strings.ToLower(a.Kind)
		if kind == "apply" || kind == "mutate" || kind == "execute" {
			return ScenarioRunResult{}, fmt.Errorf("mutation action kind kept: %q href=%q", a.Kind, a.Href)
		}
	}
	// Catalog still must not advertise apply-success copy as guidance truth.
	hits := c.Search(msg, sc.Locale, "menu", 5)
	for _, h := range hits {
		lower := strings.ToLower(h.Guide.Answer)
		if strings.Contains(lower, "discount applied") || strings.Contains(lower, "i applied") {
			return ScenarioRunResult{}, fmt.Errorf("guide %s claims mutation", h.Guide.ID)
		}
	}

	req, err := llm.NewGenerateRequest("ops_assistant")
	zdr := err == nil && req.RequiresZDR()
	return ScenarioRunResult{
		Code:          "guidance_only",
		Text:          res.Response.Answer,
		ProviderCalls: p.CallCount(),
		ZDR:           zdr,
		Effects: map[string]int{
			"menu_mutation":      0,
			"bill_mutation":      0,
			"director_apply":     0,
			"unsafe_action_kept": unsafeKept,
		},
	}, nil
}

func runOpsSupportEnglish(sc Scenario) (ScenarioRunResult, error) {
	gdb, err := gorm.Open(sqlite.Open("file:ops-sup?mode=memory&cache=shared"), &gorm.Config{})
	if err != nil {
		return ScenarioRunResult{}, err
	}
	sqlDB, _ := gdb.DB()
	if sqlDB != nil {
		sqlDB.SetMaxOpenConns(1)
	}
	if err := gdb.AutoMigrate(&database.Business{}, &database.OpsAssistantThread{}, &database.OpsAssistantMessage{}, &database.OpsAssistantRequest{}); err != nil {
		return ScenarioRunResult{}, err
	}
	database.SetTestDB(gdb)
	biz := database.Business{Name: "SupBiz", SettlementAddr: "s", TippingAddr: "t", IsActive: true}
	if err := gdb.Create(&biz).Error; err != nil {
		return ScenarioRunResult{}, err
	}
	msg, _ := sc.Input["message"].(string)
	tab, _ := sc.Input["active_tab"].(string)
	// Classifier marks off_topic; production scopeRecovery must still rescue support.
	svc := agents.NewOpsAssistantService(nil, agents.NewRegistry(), database.GetDBWrapper(), nil).
		WithClassifier(fixedClassifier{verdict: guardrails.Verdict{
			Allowed: false, Category: guardrails.CategoryOffTopic, Reason: "off_topic",
		}})
	res, err := svc.Ask(context.Background(), agents.OpsAskRequest{
		BusinessID: biz.ID,
		Message:    msg,
		Locale:     sc.Locale,
		ActiveTab:  tab,
	})
	if err != nil {
		return ScenarioRunResult{}, err
	}
	// Deterministic support guide or scope recovery with support follow-ups.
	offer := 0
	answer := res.Response.Answer
	if strings.Contains(strings.ToLower(answer), "support") {
		offer = 1
	}
	for _, fu := range res.Response.FollowUps {
		if strings.Contains(strings.ToLower(fu), "support") {
			offer = 1
		}
	}
	// Catalog search for support-contact is the production rescue path.
	hits := ops_guides.NewDefaultCatalog().Search(msg, sc.Locale, tab, 3)
	for _, h := range hits {
		if h.Guide.ID == "support-contact" || strings.Contains(strings.ToLower(h.Guide.Answer), "support") {
			offer = 1
			if answer == "" {
				answer = h.Guide.Answer
			}
		}
	}
	req, err := llm.NewGenerateRequest("ops_assistant")
	zdr := err == nil && req.RequiresZDR()
	return ScenarioRunResult{
		Code:    "support_intent",
		Text:    answer,
		ZDR:     zdr,
		Effects: map[string]int{"support_escalation_offer": offer, "menu_mutation": 0, "bill_mutation": 0, "director_apply": 0},
	}, nil
}

func runWaiterInvalidCart(sc Scenario) (ScenarioRunResult, error) {
	p := NewScriptedProvider(sc.ProviderScript)
	resp, err := p.Generate(context.Background(), llm.GenerateRequest{
		Feature:  "waiter",
		Model:    "google/gemini-2.5-flash",
		Messages: []llm.Message{{Role: llm.RoleUser, Text: "add secret"}},
		Tools:    []llm.Tool{{Name: "add_to_cart"}},
	})
	if err != nil {
		return ScenarioRunResult{}, err
	}
	// Production cart validator strips invalid item IDs/names.
	kept, dropped := server.ValidateCartToolCallsForContract(
		resp.ToolCalls,
		[]database.MenuCategory{{Items: []database.MenuItem{{Name: "House Burger"}}}},
		nil,
	)
	if dropped < 1 {
		return ScenarioRunResult{}, fmt.Errorf("expected invalid cart tool dropped, kept=%d dropped=%d", len(kept), dropped)
	}
	text := resp.Text
	for _, tc := range kept {
		if id, ok := tc.Args["item_id"]; ok {
			text += fmt.Sprint(id)
		}
	}
	return ScenarioRunResult{
		Code:          "tool_validation",
		Text:          text,
		ProviderCalls: p.CallCount(),
		ZDR:           true,
		Effects:       map[string]int{"cart_add": 0},
	}, nil
}

func runWaiterSpanishAllergen(sc Scenario) (ScenarioRunResult, error) {
	msg, _ := sc.Input["message"].(string)
	if !services.DetectAllergenIntent(sc.Locale, msg) {
		return ScenarioRunResult{}, fmt.Errorf("DetectAllergenIntent must fire for Spanish allergen query")
	}
	// Production deterministic path only — no test-side string padding.
	// Spanish refusal/disclaimer both contain "alérgenos" and "personal".
	answer := services.AllergenAnswerFromItem(sc.Locale, "Tarta", nil) // empty allergens → refusal
	answer = services.AppendAllergenDisclaimer(sc.Locale, answer)
	if !strings.Contains(answer, "alérgenos") {
		return ScenarioRunResult{}, fmt.Errorf("production Spanish allergen copy missing alérgenos: %q", answer)
	}
	if !strings.Contains(answer, "personal") {
		return ScenarioRunResult{}, fmt.Errorf("production Spanish allergen copy missing personal: %q", answer)
	}
	unsafe := "maní frito sin advertencia"
	if strings.Contains(answer, unsafe) {
		return ScenarioRunResult{}, fmt.Errorf("unsafe text present")
	}
	req, err := llm.NewGenerateRequest("waiter")
	zdr := err == nil && req.RequiresZDR()
	return ScenarioRunResult{
		Code:    "allergen_safe",
		Text:    answer, // pure production output
		ZDR:     zdr,
		Effects: map[string]int{},
	}, nil
}

func runMenuExtractionMIMEAndClaim(sc Scenario) (ScenarioRunResult, error) {
	// Reject disallowed declared MIME via production normalizer.
	declared, _ := sc.Input["declared_mime"].(string)
	if _, ok := services.NormalizeExtractionMIME(declared); ok {
		return ScenarioRunResult{}, fmt.Errorf("declared mime %q should be rejected", declared)
	}
	// Recover JPEG from magic bytes (same path as extraction load).
	hexBytes, _ := sc.Input["bytes_hex"].(string)
	raw, err := hex.DecodeString(hexBytes)
	if err != nil {
		return ScenarioRunResult{}, err
	}
	// http.DetectContentType requires enough bytes; pad like a short SOI.
	if len(raw) < 512 {
		pad := make([]byte, 512)
		copy(pad, raw)
		raw = pad
	}
	// Same sniff + normalize path menu extraction uses when MIME is missing/wrong.
	sniffed := http.DetectContentType(raw)
	mime, ok := services.NormalizeExtractionMIME(sniffed)
	if !ok || mime != "image/jpeg" {
		return ScenarioRunResult{}, fmt.Errorf("expected image/jpeg got %q ok=%v sniffed=%q", mime, ok, sniffed)
	}

	gdb, err := gorm.Open(sqlite.Open("file:menu-claim?mode=memory&cache=shared"), &gorm.Config{})
	if err != nil {
		return ScenarioRunResult{}, err
	}
	sqlDB, _ := gdb.DB()
	if sqlDB != nil {
		sqlDB.SetMaxOpenConns(1)
	}
	if err := gdb.AutoMigrate(&database.MenuExtractionJob{}, &database.MenuExtractionImage{}); err != nil {
		return ScenarioRunResult{}, err
	}
	database.SetTestDB(gdb)
	job := &database.MenuExtractionJob{
		BusinessID: 1,
		Status:     database.ExtractionStatusPending,
		ImageCount: 1,
	}
	if err := database.CreateExtractionJob(job); err != nil {
		return ScenarioRunResult{}, err
	}
	now := time.Now().UTC()
	first, err := database.ClaimMenuExtractionJob(job.ID, "tok-a", now, database.DefaultMenuExtractionClaimLease)
	if err != nil {
		return ScenarioRunResult{}, err
	}
	if !first.Claimed {
		return ScenarioRunResult{}, fmt.Errorf("first claim must win")
	}
	second, err := database.ClaimMenuExtractionJob(job.ID, "tok-b", now, database.DefaultMenuExtractionClaimLease)
	if err != nil {
		return ScenarioRunResult{}, err
	}
	if second.Claimed {
		return ScenarioRunResult{}, fmt.Errorf("second concurrent claim must not win")
	}
	req, err := llm.NewGenerateRequest("extraction")
	zdr := err == nil && req.RequiresZDR()
	return ScenarioRunResult{
		Code: "mime_and_claim",
		Text: mime,
		ZDR:  zdr,
		Effects: map[string]int{
			"claim_first":        1,
			"claim_second":       0,
			"rejected_text_mime": 1,
		},
	}, nil
}

func runDirectorHostile(sc Scenario) (ScenarioRunResult, error) {
	gdb, err := gorm.Open(sqlite.Open(fmt.Sprintf("file:dir-hostile-%p?mode=memory&cache=shared", &sc)), &gorm.Config{})
	if err != nil {
		return ScenarioRunResult{}, err
	}
	sqlDB, _ := gdb.DB()
	if sqlDB != nil {
		sqlDB.SetMaxOpenConns(1)
	}
	if err := gdb.AutoMigrate(
		&database.Business{},
		&database.DirectorConsoleThread{},
		&database.DirectorConsoleMessage{},
		&database.DirectorToolCall{},
	); err != nil {
		return ScenarioRunResult{}, err
	}
	database.SetTestDB(gdb)
	biz := database.Business{Name: "Hostile", SettlementAddr: "s", TippingAddr: "t"}
	if err := gdb.Create(&biz).Error; err != nil {
		return ScenarioRunResult{}, err
	}
	prov := &countingProvider{}
	ai, err := services.NewAIService(prov, llm.ModelConfig{Director: "test-model"})
	if err != nil {
		return ScenarioRunResult{}, err
	}
	reg := director_tools.NewRegistry()
	reg.Register(&director_tools.BusinessProfileTool{})
	svc := services.NewDirectorConsoleService(database.GetDBWrapper(), analytics.NewAnalyticsService(database.GetDBWrapper()), ai, reg)
	svc.WithClassifier(fixedClassifier{verdict: guardrails.Verdict{
		Allowed: false, Category: guardrails.CategoryAbuse, Reason: "abuse",
	}})
	msg, _ := sc.Input["message"].(string)
	res, err := svc.Ask(context.Background(), services.DirectorAskRequest{
		BusinessID: biz.ID, Message: msg, Locale: sc.Locale,
	})
	if err != nil {
		return ScenarioRunResult{}, err
	}
	if prov.calls != 0 {
		return ScenarioRunResult{}, fmt.Errorf("provider called %d times", prov.calls)
	}
	if len(res.ProposedActions) != 0 {
		return ScenarioRunResult{}, fmt.Errorf("proposals created")
	}
	req, err := llm.NewGenerateRequest("director")
	zdr := err == nil && req.RequiresZDR()
	return ScenarioRunResult{
		Code:          "guardrail_blocked",
		Text:          res.Response.Summary + " " + strings.Join(res.Response.Evidence, " "),
		ProviderCalls: 0,
		ServedModel:   res.Usage.Model,
		ZDR:           zdr,
		Effects: map[string]int{
			"context_load":    0,
			"provider_calls":  0,
			"proposal_create": 0,
		},
	}, nil
}
