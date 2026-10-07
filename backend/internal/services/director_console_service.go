package services

import (
	"context"
	"embed"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/stdevmac/payverge/backend/internal/agents/ops_guides"
	"github.com/stdevmac/payverge/backend/internal/analytics"
	"github.com/stdevmac/payverge/backend/internal/database"
	"github.com/stdevmac/payverge/backend/internal/guardrails"
	"github.com/stdevmac/payverge/backend/internal/llm"
	"github.com/stdevmac/payverge/backend/internal/llmeval/langid"
	"github.com/stdevmac/payverge/backend/internal/locales"
	"github.com/stdevmac/payverge/backend/internal/pii"
	"github.com/stdevmac/payverge/backend/internal/services/director_actions"
	"github.com/stdevmac/payverge/backend/internal/services/director_tools"

	"gorm.io/gorm"
)

//go:embed prompts/director_console/*.md
var directorPromptFS embed.FS

var (
	directorAllowedTabs = map[string]struct{}{
		"overview":      {},
		"analytics":     {},
		"menu":          {},
		"tables":        {},
		"bills":         {},
		"kitchen":       {},
		"counter":       {},
		"crm":           {},
		"reservations":  {},
		"delivery":      {},
		"plugins":       {},
		"staff":         {},
		"settings":      {},
		"business-page": {},
		"ai-waiter":     {},
		// S3-Loop: Marketing Library / studio deep links from Director.
		"marketing": {},
		// #579: inventory ingredients (stock items) are grounded to the
		// Inventory tab — the same target the proactive-insight alert card's
		// "Abrir" CTA already uses (buildProactiveInsightsWithReports).
		"inventory": {},
	}
)

type DirectorConsoleService struct {
	db           *database.DB
	analytics    *analytics.AnalyticsService
	aiService    *AIService
	toolRegistry *director_tools.Registry
	classifier   guardrails.InputClassifier
	costGate     dollarBudgetGate
}

// dollarBudgetGate is the per-business daily USD ceiling the Director consults
// before running its multi-iteration, tool-calling model loop. Satisfied by
// *llm.AICostGate in prod (wired in main.go). A nil gate disables enforcement
// (uncapped) so tests and non-metered deployments keep working.
type dollarBudgetGate interface {
	OverBudget(businessID uint) bool
}

type DirectorAskRequest struct {
	BusinessID uint
	Message    string
	ThreadID   *uint
	Locale     string
	ActiveTab  string
	// Regenerate re-answers the last user turn without appending a new user
	// message (L4-15). Requires ThreadID; deletes the trailing assistant row.
	Regenerate bool
}

type DirectorAction struct {
	Title       string `json:"title"`
	Description string `json:"description"`
	DeepLink    string `json:"deep_link"`
	Priority    string `json:"priority"`
}

type DirectorStructuredResponse struct {
	Summary        string           `json:"summary"`
	Diagnosis      string           `json:"diagnosis"`
	Evidence       []string         `json:"evidence"`
	ActionPlan     []DirectorAction `json:"actions"`
	ExpectedImpact string           `json:"expected_impact"`
	FollowUps      []string         `json:"follow_ups"`
}

type DirectorUsage struct {
	Model     string `json:"model"`
	LatencyMs int64  `json:"latency_ms"`
}

type DirectorAskResult struct {
	Thread           database.DirectorConsoleThread  `json:"thread"`
	AssistantMessage database.DirectorConsoleMessage `json:"assistant_message"`
	Response         DirectorStructuredResponse      `json:"response"`
	Usage            DirectorUsage                   `json:"usage"`
	// ToolCallIDs are the IDs of the director_tool_calls rows that were
	// persisted while answering this request. The SSE handler forwards them
	// in the response.complete event so the client can render a final
	// tool-trace summary linked to the assistant message.
	ToolCallIDs []uint `json:"tool_call_ids,omitempty"`
	// ProposedActions are server-truth write proposals the director staged while
	// answering this ask (assembled from persisted pending rows, not model text).
	ProposedActions []director_actions.ProposedAction `json:"proposed_actions,omitempty"`
}

type DirectorThreadDTO struct {
	ID            uint      `json:"id"`
	BusinessID    uint      `json:"business_id"`
	Title         string    `json:"title"`
	Locale        string    `json:"locale"`
	LastMessageAt time.Time `json:"last_message_at"`
	CreatedAt     time.Time `json:"created_at"`
	UpdatedAt     time.Time `json:"updated_at"`
	// Pinned surfaces the thread's pin state to the client so the actions menu
	// can toggle Pin/Unpin correctly and render a pin indicator. Previously
	// missing from the DTO, which made the client read `undefined` — unpin was
	// unreachable and the toggle always sent `true`.
	Pinned bool `json:"pinned"`
	// ArchivedAt is set only on archived threads (the sidebar's Archived
	// section). Nil/absent for active threads.
	ArchivedAt *time.Time `json:"archived_at,omitempty"`
}

// ListThreadsOptions carries optional pagination + archived-filter parameters
// for the thread sidebar. The zero value lists active threads with the
// default cap.
type ListThreadsOptions struct {
	Archived bool
	Limit    int
	Offset   int
}

// ListThreadsResult wraps the page plus the total count so the sidebar can show
// "load older" honestly.
type ListThreadsResult struct {
	Threads []DirectorThreadDTO
	Total   int64
}

type DirectorMessageDTO struct {
	ID                 uint                           `json:"id"`
	ThreadID           uint                           `json:"thread_id"`
	BusinessID         uint                           `json:"business_id"`
	Role               database.DirectorMessageRole   `json:"role"`
	Locale             string                         `json:"locale"`
	Content            string                         `json:"content"`
	StructuredResponse *DirectorStructuredResponse    `json:"structured_response,omitempty"`
	ModelName          string                         `json:"model_name,omitempty"`
	LatencyMs          int64                          `json:"latency_ms,omitempty"`
	FeedbackVote       *database.DirectorFeedbackVote `json:"feedback_vote,omitempty"`
	FeedbackAt         *time.Time                     `json:"feedback_at,omitempty"`
	CreatedAt          time.Time                      `json:"created_at"`
}

type directorContext struct {
	GeneratedAt string `json:"generated_at"`
	Business    struct {
		ID       uint   `json:"id"`
		Name     string `json:"name"`
		Timezone string `json:"timezone"`
		// Currency is the venue's own display currency (ARS, EUR, USD…).
		// Without it the model reads every figure below as dollars and tells
		// an Argentine owner they collected "$40,600" (#870).
		Currency      string `json:"currency"`
		AssistantName string `json:"assistant_name"`
	} `json:"business"`
	Question string `json:"question"`
	Metrics  struct {
		WeeklyRevenue      float64 `json:"weekly_revenue"`
		WeeklyTransactions int     `json:"weekly_transactions"`
		AverageTicket      float64 `json:"average_ticket"`
		WeeklyTips         float64 `json:"weekly_tips"`
		ActiveBills        int     `json:"active_bills"`
		// TodayRevenue/TodayCollected are recognized payments today.
		// TodayFloorRemaining is leftover due on live open/partial checks and
		// is never sold as today's sales (#703).
		TodayRevenue        float64 `json:"today_revenue"`
		TodayCollected      float64 `json:"today_collected"`
		TodayFloorRemaining float64 `json:"today_floor_remaining"`
	} `json:"metrics"`
	Orders struct {
		Pending   int64 `json:"pending"`
		Approved  int64 `json:"approved"`
		InKitchen int64 `json:"in_kitchen"`
		Ready     int64 `json:"ready"`
		Delivered int64 `json:"delivered"`
		Cancelled int64 `json:"cancelled"`
	} `json:"orders"`
	Reservations map[string]interface{} `json:"reservations"`
	CRM          map[string]interface{} `json:"crm"`
	AIWaiter     map[string]interface{} `json:"ai_waiter"`
	// Marketing is the S3-Loop closed-loop aggregate (posts this period, recent
	// titles, optional freeform channel tags). Aggregate counts only — no signed
	// asset URLs. Absent schedule fields forever.
	Marketing map[string]interface{} `json:"marketing"`
	Plugins   struct {
		Enabled                []map[string]interface{} `json:"enabled"`
		ReportSchedules        []map[string]interface{} `json:"report_schedules"`
		MissingRecommendations []map[string]string      `json:"missing_recommendations"`
	} `json:"plugins"`
	TopItems      []analytics.ItemStats `json:"top_items"`
	ActiveTab     string                `json:"active_tab"`
	DataReadiness directorDataReadiness `json:"data_readiness"`
	// MetricsSummary is operator-facing prose so the model does not echo
	// snake_case field names like weekly_revenue / qty_sold.
	MetricsSummary string `json:"metrics_summary"`
	// MenuPerformance is operator-facing plate-cost / sibling-thread prose.
	// It exists so best-seller and margin asks are grounded even when the
	// weekly snapshot is $0 — never dump qty_sold / data_readiness here.
	MenuPerformance string                 `json:"menu_performance,omitempty"`
	LiveOps         map[string]interface{} `json:"live_ops,omitempty"`
	Kitchen         map[string]interface{} `json:"kitchen,omitempty"`
	Promos          map[string]interface{} `json:"promos,omitempty"`
	// Availability is the 86 board: which menu items are sold out right now.
	// AvailabilitySummary is the operator-facing prose form so the model does
	// not echo sold_out_items back as a JSON field name (#870).
	Availability        directorAvailability `json:"availability"`
	AvailabilitySummary string               `json:"availability_summary"`
	// Delivery is the venue's own Delivery settings: toggles, the marketplace
	// partners the operator already configured, and the fee. DeliverySummary is
	// the prose form. Without these the model had no delivery evidence at all
	// and told a venue running PedidosYa + Rappi that it had no delivery
	// partners and should go add one in Plugins (#871).
	Delivery        directorDeliveryConfig `json:"delivery"`
	DeliverySummary string                 `json:"delivery_summary"`
	// LocalClock is the venue's own wall clock and today's operating window,
	// already converted into the business timezone. Without it the model had
	// only a UTC stamp next to a timezone name and reported 20:00 UTC as
	// "8:00 PM New York" while claiming the hours were unknown (#902).
	LocalClock        directorLocalClock `json:"local_clock"`
	LocalClockSummary string             `json:"local_clock_summary"`
}

// directorSoldOutNameCap bounds how many 86'd item names ride in the prompt.
// A venue that 86'd forty things needs the count and a sample, not forty
// names eating the context budget.
const directorSoldOutNameCap = 25

// directorInventoryStatusOutOfStock mirrors the serve-time stamp
// database.GetInventorySummary writes onto MenuItem.InventoryStatus.
const directorInventoryStatusOutOfStock = "out_of_stock"

// directorAvailability is the Director's 86 board.
//
// Known reports whether the availability read actually succeeded. It matters
// because "no sold-out items" and "we could not read stock" are different
// answers, and only the first one is safe to say out loud.
type directorAvailability struct {
	Known         bool     `json:"known"`
	MenuItemCount int      `json:"menu_item_count"`
	SoldOutCount  int      `json:"sold_out_count"`
	SoldOutItems  []string `json:"sold_out_items"`
	Truncated     bool     `json:"truncated"`
}

// buildDirectorAvailability projects the 86 board from the menu the readiness
// signal already loaded plus the batched inventory block-list.
//
// An item counts as 86'd when the operator flipped it unavailable, when
// inventory stamped it out_of_stock, or when the shared orderability read says
// it is unrecommendable (hard-block mode with a depleted ingredient). Those are
// exactly the three ways an item stops being orderable on the guest menu, so
// the Director's board matches what the storefront shows.
func buildDirectorAvailability(categories []database.MenuCategory, blocked map[string]bool, blockedKnown bool) directorAvailability {
	board := directorAvailability{Known: blockedKnown, SoldOutItems: []string{}}
	for _, cat := range categories {
		for _, item := range cat.Items {
			board.MenuItemCount++
			soldOut := !item.IsAvailable ||
				item.InventoryStatus == directorInventoryStatusOutOfStock ||
				blocked[item.ID]
			if !soldOut {
				continue
			}
			board.SoldOutCount++
			if len(board.SoldOutItems) < directorSoldOutNameCap {
				board.SoldOutItems = append(board.SoldOutItems, SanitizePromptField(item.Name, 80))
			} else {
				board.Truncated = true
			}
		}
	}
	return board
}

// directorAvailabilityProse renders the 86 board as operator prose. English
// like the rest of the grounding block — the model translates into the ask's
// locale; the snapshot's job is to be unambiguous, not localized.
func directorAvailabilityProse(board directorAvailability) string {
	if !board.Known {
		return "86 board: stock could not be read for this venue right now, so do not claim anything is or is not sold out."
	}
	if board.SoldOutCount == 0 {
		return "86 board: nothing on the menu is sold out right now."
	}
	names := strings.Join(board.SoldOutItems, ", ")
	if board.Truncated {
		return fmt.Sprintf("86 board (sold out right now): %d of %d menu items, including %s.", board.SoldOutCount, board.MenuItemCount, names)
	}
	return fmt.Sprintf("86 board (sold out right now): %d of %d menu items — %s.", board.SoldOutCount, board.MenuItemCount, names)
}

// directorMoney renders a money figure with its currency code so the model
// never has to guess whether "$" means dollars or pesos (#870).
func directorMoney(currency string, amount float64) string {
	if currency == "" {
		currency = "USD"
	}
	return fmt.Sprintf("%s %.2f", currency, amount)
}

// directorPayloadCurrency reads the venue currency off the grounding snapshot,
// defaulting to USD when the snapshot predates the field or is absent.
func directorPayloadCurrency(payload *directorContext) string {
	if payload == nil || strings.TrimSpace(payload.Business.Currency) == "" {
		return "USD"
	}
	return strings.TrimSpace(payload.Business.Currency)
}

type directorDataReadiness struct {
	State          string `json:"state"` // "setup" | "established"
	MenuItemCount  int    `json:"menu_item_count"`
	LifetimeOrders int64  `json:"lifetime_orders"`
	// HasRecognizedRevenue is true when Overview-style money exists: weekly
	// recognized collections, today's collected payments, or remaining due on
	// live open/partial checks. It only changes the verdict while
	// LifetimeOrders is below the threshold.
	HasRecognizedRevenue bool  `json:"has_recognized_revenue"`
	CustomerCount        int64 `json:"customer_count"`
	PaymentsEnabled      bool  `json:"payments_enabled"`
}

// directorSetupOrderThreshold is the lifetime-order count below which a
// business is treated as still setting up (any growth metric would be invented).
const directorSetupOrderThreshold int64 = 5

// classifyReadinessState gates "setup" mode: a business with essentially no
// transactional activity yet (so any growth metric would be invented).
// hasOperationalActivity is true when Overview-style money exists — recognized
// collections, remaining due on open checks, or an in-progress bill. A live
// floor must never read as "setup" / "payments off" (#730).
func classifyReadinessState(lifetimeOrders int64, hasOperationalActivity bool) string {
	if lifetimeOrders < directorSetupOrderThreshold && !hasOperationalActivity {
		return "setup"
	}
	return "established"
}

// directorPaymentsEnabled is true when the venue can take money: a payment
// plugin is on, OR Overview already shows sales or an open/partial bill.
// Kitchen tickets alone (lifetimeOrders) do not mean payments work (#730).
func directorPaymentsEnabled(pluginOn bool, activeBills int, todayRevenue, weeklyRevenue float64) bool {
	return pluginOn || activeBills > 0 || todayRevenue > 0 || weeklyRevenue > 0
}

func directorAsInt(v interface{}) int {
	switch n := v.(type) {
	case int:
		return n
	case int32:
		return int(n)
	case int64:
		return int(n)
	case uint:
		return int(n)
	case uint64:
		return int(n)
	case float32:
		return int(n)
	case float64:
		return int(n)
	default:
		return 0
	}
}

func directorAsFloat(v interface{}) float64 {
	switch n := v.(type) {
	case float64:
		return n
	case float32:
		return float64(n)
	case int:
		return float64(n)
	case int32:
		return float64(n)
	case int64:
		return float64(n)
	case uint:
		return float64(n)
	case uint64:
		return float64(n)
	default:
		return 0
	}
}

// directorPluginIsEnabled reads the loosely-typed is_enabled field from
// GetBusinessPlugins map rows. SQLite hands us int64; Postgres usually bool.
func directorPluginIsEnabled(v interface{}) bool {
	switch x := v.(type) {
	case bool:
		return x
	case int:
		return x != 0
	case int32:
		return x != 0
	case int64:
		return x != 0
	case uint:
		return x != 0
	case uint64:
		return x != 0
	case float64:
		return x != 0
	case string:
		return x == "1" || strings.EqualFold(x, "true") || strings.EqualFold(x, "t")
	default:
		return false
	}
}

type directorModelOutput struct {
	Summary        string           `json:"summary"`
	Diagnosis      string           `json:"diagnosis"`
	Evidence       []string         `json:"evidence"`
	ActionPlan     []DirectorAction `json:"actions"`
	ExpectedImpact string           `json:"expected_impact"`
	FollowUps      []string         `json:"follow_ups"`
}

func NewDirectorConsoleService(db *database.DB, analyticsService *analytics.AnalyticsService, aiService *AIService, toolRegistry *director_tools.Registry) *DirectorConsoleService {
	return &DirectorConsoleService{
		db:           db,
		analytics:    analyticsService,
		aiService:    aiService,
		toolRegistry: toolRegistry,
		classifier:   guardrails.AllowAll{},
	}
}

// WithClassifier injects the input scope/abuse classifier. Defaults to
// guardrails.AllowAll (fail-open) when never called.
// WithCostGate installs the per-business daily USD ceiling. Passing nil (or
// never calling this) leaves the Director uncapped. Consulted in askInternal
// before the model loop runs.
func (s *DirectorConsoleService) WithCostGate(g dollarBudgetGate) *DirectorConsoleService {
	s.costGate = g
	return s
}

// budgetExceeded reports whether the business has hit its daily AI dollar
// ceiling. No gate installed => false (uncapped).
func (s *DirectorConsoleService) budgetExceeded(businessID uint) bool {
	return s.costGate != nil && s.costGate.OverBudget(businessID)
}

func (s *DirectorConsoleService) WithClassifier(c guardrails.InputClassifier) *DirectorConsoleService {
	if c != nil {
		s.classifier = c
	}
	return s
}

// Ask is the non-streaming entry point used by POST /ai/director/ask. It
// drives the same loop as AskStreaming but discards intermediate progress
// events into an in-memory bufferSink so the caller only sees the final
// structured response.
func (s *DirectorConsoleService) Ask(ctx context.Context, req DirectorAskRequest) (*DirectorAskResult, error) {
	return s.askInternal(ctx, req, &bufferSink{})
}

// AskStreaming is the SSE entry point used by POST /ai/director/ask/stream.
// The provided Sink receives tool.call.started / tool.call.completed / error
// events as the function-calling loop progresses; the final structured
// response is returned via DirectorAskResult exactly like Ask.
func (s *DirectorConsoleService) AskStreaming(ctx context.Context, req DirectorAskRequest, sink Sink) (*DirectorAskResult, error) {
	if sink == nil {
		sink = &bufferSink{}
	}
	return s.askInternal(ctx, req, sink)
}

// askInternal holds the shared body that both Ask and AskStreaming run. The
// sink is the only behavioral difference: bufferSink for the JSON HTTP path,
// the SSE sink for the streaming path.
func (s *DirectorConsoleService) askInternal(ctx context.Context, req DirectorAskRequest, sink Sink) (*DirectorAskResult, error) {
	if strings.TrimSpace(req.Message) == "" {
		return nil, fmt.Errorf("message is required")
	}

	// L4-15: a regenerate only ever replaces an answer that already exists, so it
	// must be rejected BEFORE resolveThread — which would otherwise create a brand
	// new (and permanently empty) thread row for a request that cannot succeed.
	if req.Regenerate && (req.ThreadID == nil || *req.ThreadID == 0) {
		return nil, fmt.Errorf("regenerate requires thread_id")
	}

	business, err := s.db.GetBusinessByID(req.BusinessID)
	if err != nil {
		return nil, fmt.Errorf("failed to load business: %w", err)
	}

	// Resolve the answer locale. When the client sends no explicit locale, fall
	// back to the business owner's UI language (not the "en" default) so an
	// es-AR owner's briefing arrives in Spanish. An explicit request locale
	// always wins so a mid-conversation language switch is honored.
	req.Locale = resolveDirectorAskLocale(req.Locale, business)

	thread, threadExisted, err := s.resolveThread(req, business.Name)
	if err != nil {
		return nil, err
	}

	// Stamp the thread with the current ask locale so a thread whose UI language
	// changed mid-conversation renders consistently. Messages keep their own
	// per-message locale. Best-effort: a stamp failure must not block the answer.
	//
	// #820: a PINNED thread is exempt. The pinned card is the seeded briefing,
	// and its locale labels the briefing copy the card renders — not whatever
	// language the latest follow-up happened to use. Live thread 10 is the proof:
	// an English "Demo Director Briefing" body carrying locale es-AR, because one
	// Spanish follow-up asked inside the card relabeled it. The demo reseed keys
	// on the thread TITLE, so the mislabel then survived every reseed. Each
	// answer still carries its own per-message locale, so nothing is rendered in
	// the wrong language by leaving the card's label alone.
	if thread.Locale != req.Locale && !thread.Pinned {
		if updErr := s.db.GetGorm().Model(&database.DirectorConsoleThread{}).
			Where("id = ? AND business_id = ?", thread.ID, req.BusinessID).
			Update("locale", req.Locale).Error; updErr == nil {
			thread.Locale = req.Locale
		}
	}

	var (
		userMessage   *database.DirectorConsoleMessage
		supersededIDs []uint
	)
	if req.Regenerate {
		// L4-15: reuse the last user turn and MARK the trailing assistant as
		// superseded — nothing is deleted here. The old answer is the user's only
		// copy, so it survives until a genuine replacement exists; the swap then
		// happens atomically below (delete + insert in one transaction).
		msgs, listErr := database.ListDirectorConsoleMessages(req.BusinessID, thread.ID, 50)
		if listErr != nil {
			return nil, listErr
		}
		if len(msgs) == 0 {
			return nil, fmt.Errorf("regenerate requires a prior user message")
		}
		// Walk from the end: collect consecutive trailing assistants (usually one),
		// then reuse the nearest user message.
		var lastUser *database.DirectorConsoleMessage
		for i := len(msgs) - 1; i >= 0; i-- {
			m := msgs[i]
			if m.Role == database.DirectorMessageRoleAssistant && lastUser == nil {
				supersededIDs = append(supersededIDs, m.ID)
				continue
			}
			if m.Role == database.DirectorMessageRoleUser {
				cp := m
				lastUser = &cp
				break
			}
		}
		if lastUser == nil {
			return nil, fmt.Errorf("regenerate requires a prior user message")
		}
		if strings.TrimSpace(req.Message) == "" {
			req.Message = lastUser.Content
		}
		userMessage = lastUser
	} else {
		// #820: when a retry lands in an existing thread whose trailing turn is
		// the identical UNANSWERED question, reuse that pending row instead of
		// stacking a duplicate user bubble. Best-effort: any lookup failure
		// falls through to a normal insert.
		if threadExisted {
			if last, lastErr := database.LatestDirectorConsoleMessage(req.BusinessID, thread.ID); lastErr == nil &&
				last != nil && last.Role == database.DirectorMessageRoleUser &&
				last.Content == strings.TrimSpace(req.Message) {
				userMessage = last
			}
		}
		if userMessage == nil {
			userMessage = &database.DirectorConsoleMessage{
				ThreadID:   thread.ID,
				BusinessID: req.BusinessID,
				Role:       database.DirectorMessageRoleUser,
				Locale:     req.Locale,
				Content:    strings.TrimSpace(req.Message),
			}
			if err := database.SaveDirectorConsoleMessage(userMessage); err != nil {
				return nil, err
			}
		}
	}

	var (
		structured     DirectorStructuredResponse
		modelName      string
		latencyMs      int64
		toolCallIDs    []uint
		proposalIDs    []uint
		contextPayload *directorContext
	)

	// Guardrails run BEFORE context/tool/provider work. Hostile verdicts must
	// never load business evidence or call the answering model.
	verdict, classifyErr := s.classifier.Classify(ctx, guardrails.ClassifyRequest{
		Surface:    guardrails.SurfaceDirector,
		BusinessID: req.BusinessID,
		Locale:     req.Locale,
		Text:       req.Message,
	})
	if s.budgetExceeded(req.BusinessID) {
		// Daily AI dollar ceiling reached — serve a deterministic notice and
		// make NO model call, so the flagship Director surface can't run up
		// unmetered LLM spend past the same cap every other AI lane enforces.
		structured = sanitizeStructuredResponse(buildBudgetExceededResponse(req.Locale))
		modelName = "budget"
	} else if classifyErr == nil && !verdict.Allowed {
		switch verdict.Category {
		case guardrails.CategoryAbuse, guardrails.CategoryInjection:
			structured = sanitizeStructuredResponse(buildHostileBlockResponse(req.Locale, verdict.Category))
			modelName = "guardrail"
		case guardrails.CategoryOffTopic:
			structured = sanitizeStructuredResponse(buildScopeRedirectResponse(req.BusinessID, req.Locale))
			modelName = "guardrail"
		default:
			structured = sanitizeStructuredResponse(buildHostileBlockResponse(req.Locale, verdict.Category))
			modelName = "guardrail"
		}
	} else {
		// Context is loaded only after the guardrail allows the turn.
		var ctxErr error
		contextPayload, ctxErr = s.buildContext(req, business)
		if ctxErr != nil {
			contextPayload = nil
		}
		// Only load conversation memory when we actually run the loop —
		// guardrail-blocked requests skip the (bounded) thread-history read.
		// The answer being regenerated is still on disk (it is only swapped out
		// once a replacement exists), so it must be excluded from memory —
		// otherwise the model would see its own previous answer as a prior turn.
		priorTurns := s.loadPriorTurns(req.BusinessID, thread.ID, userMessage.ID, supersededIDs...)
		snap := s.collectMenuPerformance(ctx, req.BusinessID, req.Locale, thread.ID)
		if contextPayload != nil && snap.usable() {
			contextPayload.MenuPerformance = snap.operatorProse(req.Locale)
		}
		structured, modelName, latencyMs, toolCallIDs, proposalIDs = s.runLoopOrFallback(ctx, req, business, thread, contextPayload, priorTurns, sink)
		structured = s.groundMenuPerformanceIfNeeded(req.Message, req.Locale, req.BusinessID, structured, snap)
		structured = s.groundOverviewSalesIfNeeded(req.Message, req.Locale, req.BusinessID, structured, contextPayload)
		// Deterministic output guard: scrub any customer PII the model echoed and
		// fail to a safe canned answer if it leaked its own system prompt. Runs on
		// the model-output path only; canned/fallback responses are server-built.
		structured = s.guardDirectorOutput(contextPayload, req.BusinessID, req.Locale, structured)
	}

	structuredJSON, _ := json.Marshal(structured)
	assistantMessage := &database.DirectorConsoleMessage{
		ThreadID:           thread.ID,
		BusinessID:         req.BusinessID,
		Role:               database.DirectorMessageRoleAssistant,
		Locale:             req.Locale,
		Content:            structured.Summary,
		StructuredResponse: string(structuredJSON),
		ModelName:          modelName,
		LatencyMs:          latencyMs,
	}

	// L4-15: the superseded answer is destroyed ONLY here, in the same
	// transaction that persists its replacement. A canned or degraded result
	// (budget cap, guardrail block, loop failure, partial timeout) is appended
	// alongside the previous answer instead — losing a real answer to a failed
	// retry is strictly worse than showing an extra notice.
	if len(supersededIDs) > 0 && directorAnswerSupersedes(modelName) {
		if err := database.ReplaceDirectorConsoleTrailingAssistant(supersededIDs, assistantMessage); err != nil {
			return nil, err
		}
	} else if err := database.SaveDirectorConsoleMessage(assistantMessage); err != nil {
		return nil, err
	}

	if len(toolCallIDs) > 0 {
		// Best-effort back-link; failure here doesn't invalidate the assistant
		// response that already shipped.
		_ = database.AttachMessageIDToToolCalls(toolCallIDs, assistantMessage.ID)
	}

	var proposedActions []director_actions.ProposedAction
	if len(proposalIDs) > 0 {
		_ = database.AttachMessageIDToProposals(proposalIDs, assistantMessage.ID)
		proposedActions = s.assembleProposedActions(req.BusinessID, proposalIDs)
	}

	return &DirectorAskResult{
		Thread:           *thread,
		AssistantMessage: *assistantMessage,
		Response:         structured,
		Usage: DirectorUsage{
			Model:     modelName,
			LatencyMs: latencyMs,
		},
		ToolCallIDs:     toolCallIDs,
		ProposedActions: proposedActions,
	}, nil
}

func threadToDTO(thread database.DirectorConsoleThread) DirectorThreadDTO {
	return DirectorThreadDTO{
		ID:            thread.ID,
		BusinessID:    thread.BusinessID,
		Title:         thread.Title,
		Locale:        thread.Locale,
		LastMessageAt: thread.LastMessageAt,
		CreatedAt:     thread.CreatedAt,
		UpdatedAt:     thread.UpdatedAt,
		Pinned:        thread.Pinned,
		ArchivedAt:    thread.ArchivedAt,
	}
}

// uniqueDirectorThreadDTOs keeps one row per real thread id. Never key on
// title — a second "how much did we sell tonight" is a different conversation.
// List queries are already unique-by-id; this is only a duplicate-id safety
// net. ListThreadsPaged.Total stays the pre-dedup database count (#730).
func uniqueDirectorThreadDTOs(in []DirectorThreadDTO) []DirectorThreadDTO {
	if len(in) < 2 {
		return in
	}
	out := make([]DirectorThreadDTO, 0, len(in))
	seenID := make(map[uint]struct{}, len(in))
	for _, t := range in {
		if t.ID == 0 {
			continue
		}
		if _, ok := seenID[t.ID]; ok {
			continue
		}
		seenID[t.ID] = struct{}{}
		out = append(out, t)
	}
	return out
}

// ListThreads returns the first page of active threads (legacy behavior).
func (s *DirectorConsoleService) ListThreads(businessID uint) ([]DirectorThreadDTO, error) {
	threads, err := database.ListDirectorConsoleThreads(businessID, 100)
	if err != nil {
		return nil, err
	}

	result := make([]DirectorThreadDTO, 0, len(threads))
	for _, thread := range threads {
		result = append(result, threadToDTO(thread))
	}

	return uniqueDirectorThreadDTOs(result), nil
}

// ListThreadsPaged returns a bounded page of threads (active or archived) plus
// the total count for the requested filter, so the sidebar can offer a "load
// older" affordance and a separate Archived section with restore.
func (s *DirectorConsoleService) ListThreadsPaged(businessID uint, opts ListThreadsOptions) (ListThreadsResult, error) {
	threads, total, err := database.ListDirectorConsoleThreadsPaged(businessID, opts.Archived, opts.Limit, opts.Offset)
	if err != nil {
		return ListThreadsResult{}, err
	}

	dtos := make([]DirectorThreadDTO, 0, len(threads))
	for _, thread := range threads {
		dtos = append(dtos, threadToDTO(thread))
	}
	return ListThreadsResult{Threads: uniqueDirectorThreadDTOs(dtos), Total: total}, nil
}

func (s *DirectorConsoleService) ListThreadMessages(businessID uint, threadID uint, limit int) ([]DirectorMessageDTO, error) {
	if _, err := database.GetDirectorConsoleThreadByID(businessID, threadID); err != nil {
		return nil, err
	}

	messages, err := database.ListDirectorConsoleMessages(businessID, threadID, limit)
	if err != nil {
		return nil, err
	}

	result := make([]DirectorMessageDTO, 0, len(messages))
	for _, message := range messages {
		dto := DirectorMessageDTO{
			ID:           message.ID,
			ThreadID:     message.ThreadID,
			BusinessID:   message.BusinessID,
			Role:         message.Role,
			Locale:       message.Locale,
			Content:      message.Content,
			ModelName:    message.ModelName,
			LatencyMs:    message.LatencyMs,
			FeedbackVote: message.FeedbackVote,
			FeedbackAt:   message.FeedbackAt,
			CreatedAt:    message.CreatedAt,
		}

		if strings.TrimSpace(message.StructuredResponse) != "" {
			var structured DirectorStructuredResponse
			if err := json.Unmarshal([]byte(message.StructuredResponse), &structured); err == nil {
				clean := sanitizeStructuredResponse(structured)
				dto.StructuredResponse = &clean
			}
		}

		result = append(result, dto)
	}

	return result, nil
}

func (s *DirectorConsoleService) SubmitFeedback(businessID uint, messageID uint, vote database.DirectorFeedbackVote) (*DirectorMessageDTO, error) {
	updated, err := database.UpdateDirectorConsoleMessageFeedback(businessID, messageID, vote)
	if err != nil {
		return nil, err
	}

	dto := &DirectorMessageDTO{
		ID:           updated.ID,
		ThreadID:     updated.ThreadID,
		BusinessID:   updated.BusinessID,
		Role:         updated.Role,
		Locale:       updated.Locale,
		Content:      updated.Content,
		ModelName:    updated.ModelName,
		LatencyMs:    updated.LatencyMs,
		FeedbackVote: updated.FeedbackVote,
		FeedbackAt:   updated.FeedbackAt,
		CreatedAt:    updated.CreatedAt,
	}

	if strings.TrimSpace(updated.StructuredResponse) != "" {
		var structured DirectorStructuredResponse
		if err := json.Unmarshal([]byte(updated.StructuredResponse), &structured); err == nil {
			clean := sanitizeStructuredResponse(structured)
			dto.StructuredResponse = &clean
		}
	}

	return dto, nil
}

// directorThreadReuseWindow bounds verbatim re-ask thread reuse (#820). A
// retry or scripted repeat of the exact same question within this window
// appends to the existing conversation; the same question on a later service
// day is a genuinely new conversation and gets its own thread (#730).
const directorThreadReuseWindow = 6 * time.Hour

// directorThreadTitleRuneLimit bounds the sidebar thread title.
const directorThreadTitleRuneLimit = 70

// directorThreadTitle derives the sidebar title for a new thread from the
// question.
//
// #870: the cut is by RUNE, never by byte. Slicing a UTF-8 string at a byte
// offset splits multi-byte runes in half, and every accented Spanish question
// long enough to be truncated ("¿Qué me recomendás para el turno de la
// noche…?") has a good chance of landing the cut inside an accent. The
// resulting string is not valid UTF-8, Postgres rejects it on the TEXT column
// ("invalid byte sequence for encoding UTF8"), CreateDirectorConsoleThread
// fails, and the whole ask 500s — the Spanish-only briefing 500 QA saw. English
// asks are pure ASCII and never reproduce it.
func directorThreadTitle(message, businessName string) string {
	title := strings.TrimSpace(message)
	if runes := []rune(title); len(runes) > directorThreadTitleRuneLimit {
		title = strings.TrimSpace(string(runes[:directorThreadTitleRuneLimit]))
	}
	if title == "" {
		title = fmt.Sprintf("%s strategy", businessName)
	}
	return title
}

// resolveThread returns the thread an ask belongs to plus whether that thread
// already existed (true) or was created for this ask (false).
func (s *DirectorConsoleService) resolveThread(req DirectorAskRequest, businessName string) (*database.DirectorConsoleThread, bool, error) {
	if req.ThreadID != nil && *req.ThreadID > 0 {
		thread, err := database.GetDirectorConsoleThreadByID(req.BusinessID, *req.ThreadID)
		if err != nil {
			return nil, false, err
		}
		return thread, true, nil
	}

	title := directorThreadTitle(req.Message, businessName)

	// #820: an ask without a thread_id that repeats a recent question verbatim
	// is a retry (the client only learns the thread_id from the terminal SSE
	// complete event, so a dropped stream always retries thread-less) or a
	// scripted repeat. Reuse the existing conversation instead of minting a
	// duplicate sidebar row per attempt. Best-effort: a lookup failure falls
	// through to creation.
	if reused, err := database.FindReusableDirectorConsoleThread(
		req.BusinessID, title, req.Message, time.Now().Add(-directorThreadReuseWindow)); err == nil && reused != nil {
		return reused, true, nil
	}

	thread, err := database.CreateDirectorConsoleThread(req.BusinessID, title, req.Locale)
	if err != nil {
		return nil, false, err
	}
	return thread, false, nil
}

func (s *DirectorConsoleService) buildContext(req DirectorAskRequest, business *database.Business) (*directorContext, error) {
	loc := database.ResolveBusinessLocation(business)
	weeklyReport, err := s.analytics.GetPaymentPeriodSummary(req.BusinessID, "week", loc)
	if err != nil {
		return nil, err
	}

	// Limit applied at SQL level; 10 keeps Sage from reporting a 3-item menu.
	topItems, _ := s.analytics.GetPopularItems(req.BusinessID, 10, "week", loc)

	activeBillCount, _ := s.db.GetActiveBillCountByBusinessID(req.BusinessID)
	ordersByStatus := s.countOrdersByStatus(req.BusinessID)

	now := time.Now()
	reservationStats, _ := database.GetReservationStats(req.BusinessID, now.AddDate(0, 0, -7), now.AddDate(0, 0, 7))
	crmStats := s.buildCRMStats(req.BusinessID)
	aiWaiterStats := s.buildAIWaiterStats(req.BusinessID)
	marketingStats := s.buildMarketingStats(req.BusinessID)
	enabledPlugins, missingPlugins := s.buildPluginInsights(req.BusinessID)
	schedules, _ := s.db.GetReportSchedulesByBusinessID(req.BusinessID)

	// #902: generated_at used to be a bare UTC stamp. Rendered in the business
	// timezone it still names the same instant, but it can no longer be read
	// off as the venue's wall clock four hours out.
	payload := &directorContext{
		GeneratedAt:  now.In(loc).Format(time.RFC3339),
		Question:     req.Message,
		Reservations: reservationStats,
		CRM:          crmStats,
		AIWaiter:     aiWaiterStats,
		Marketing:    marketingStats,
		TopItems:     topItems,
		// SEC-AI-01: client-supplied; only canonical dashboard tab keys reach
		// the prompt payload.
		ActiveTab: ops_guides.NormalizeActiveTab(req.ActiveTab),
	}

	payload.Business.ID = req.BusinessID
	payload.Business.Name = business.Name
	payload.Business.Timezone = business.Timezone
	payload.Business.Currency = deliveryDisplayCurrency(business)
	payload.Business.AssistantName = SanitizePromptField(business.AiSettings.AiName, 40)
	if payload.Business.AssistantName == "" {
		payload.Business.AssistantName = "Sage"
	}

	payload.Metrics.WeeklyRevenue = weeklyReport.TotalRevenue
	payload.Metrics.WeeklyTransactions = weeklyReport.TransactionCount
	payload.Metrics.AverageTicket = weeklyReport.AverageTicket
	payload.Metrics.WeeklyTips = weeklyReport.TotalTips
	payload.Metrics.ActiveBills = int(activeBillCount)

	todayStart := database.ServiceDayStart(time.Now(), loc, business.ServiceDayStartMinute)
	todayCollected := 0.0
	if todayWindow, todayErr := s.analytics.GetPaymentWindowSummary(req.BusinessID, todayStart, todayStart.AddDate(0, 0, 1)); todayErr == nil && todayWindow != nil {
		todayCollected = todayWindow.TotalRevenue
	}
	floorRemaining := 0.0
	if remaining, openErr := s.analytics.LiveOpenCheckRemaining(req.BusinessID); openErr == nil {
		floorRemaining = remaining
	}
	payload.Metrics.TodayRevenue = todayCollected
	payload.Metrics.TodayCollected = todayCollected
	payload.Metrics.TodayFloorRemaining = floorRemaining

	payload.Orders = ordersByStatus
	payload.Plugins.Enabled = enabledPlugins
	payload.Plugins.MissingRecommendations = missingPlugins
	payload.Plugins.ReportSchedules = make([]map[string]interface{}, 0, len(schedules))
	for _, schedule := range schedules {
		payload.Plugins.ReportSchedules = append(payload.Plugins.ReportSchedules, map[string]interface{}{
			"frequency":    schedule.Frequency,
			"day_of_week":  schedule.DayOfWeek,
			"hour":         schedule.Hour,
			"minute":       schedule.Minute,
			"timezone":     schedule.Timezone,
			"is_active":    schedule.IsActive,
			"next_send_at": schedule.NextSendAt,
		})
	}

	// Compute data_readiness signal from already-gathered values.
	lifetimeOrders := ordersByStatus.Pending + ordersByStatus.Approved +
		ordersByStatus.InKitchen + ordersByStatus.Ready +
		ordersByStatus.Delivered + ordersByStatus.Cancelled

	// #870: the 86 board. The menu read below already happens for the readiness
	// signal, so the sold-out projection costs one extra batched inventory
	// summary (the same one the AI waiter's availability path uses) and no
	// per-item queries. Without it the model has no availability evidence at
	// all and confidently answers "there are no sold-out items" while the
	// kitchen is 86'ing the bife.
	menuItemCount := 0
	var menuCategories []database.MenuCategory
	if _, cats, menuErr := database.GetMenuByBusinessID(req.BusinessID); menuErr == nil {
		menuCategories = cats
		for _, cat := range cats {
			menuItemCount += len(cat.Items)
		}
	}
	blockedItemIDs, blockedErr := database.UnrecommendableMenuItemIDs(req.BusinessID)
	availability := buildDirectorAvailability(menuCategories, blockedItemIDs, blockedErr == nil)
	payload.Availability = availability
	payload.AvailabilitySummary = directorAvailabilityProse(availability)

	// #871: delivery evidence. One indexed single-row read; delivery is not a
	// plugin, so nothing else in this snapshot can tell the model whether the
	// venue delivers or which marketplaces it already runs.
	var deliveryDB *gorm.DB
	if s.db != nil {
		deliveryDB = s.db.GetGorm()
	}
	payload.Delivery = loadDirectorDeliveryConfig(deliveryDB, req.BusinessID)
	payload.DeliverySummary = directorDeliveryProse(payload.Delivery, deliveryDisplayCurrency(business))

	// #902: the venue clock. Today's rows only — one indexed read on
	// (business_id, day_of_week), not the whole week — and the weekday is taken
	// in the business timezone so a venue whose local day differs from UTC's
	// does not read yesterday's schedule.
	todayHours, _ := database.GetBusinessOperatingHoursByDay(req.BusinessID, int(now.In(loc).Weekday()))
	payload.LocalClock = buildDirectorLocalClock(business.Timezone, loc, todayHours, now)
	payload.LocalClockSummary = directorLocalClockProse(payload.LocalClock)

	pluginOn := false
	if s.db != nil {
		if on, err := database.HasEnabledPaymentPlugin(s.db.GetGorm(), req.BusinessID); err == nil {
			pluginOn = on
		}
	}
	if !pluginOn {
		for _, pl := range enabledPlugins {
			if toStringValue(pl["category"]) == database.PluginCategoryPayment {
				pluginOn = true
				break
			}
		}
	}
	paymentsEnabled := directorPaymentsEnabled(pluginOn, int(activeBillCount), todayCollected, weeklyReport.TotalRevenue)

	customerCount, _ := crmStats["customers"].(int64)
	hasRevenue := weeklyReport.TotalRevenue > 0 || todayCollected > 0 || floorRemaining > 0 || activeBillCount > 0

	payload.DataReadiness = directorDataReadiness{
		State:                classifyReadinessState(lifetimeOrders, hasRevenue),
		MenuItemCount:        menuItemCount,
		LifetimeOrders:       lifetimeOrders,
		HasRecognizedRevenue: hasRevenue,
		CustomerCount:        customerCount,
		PaymentsEnabled:      paymentsEnabled,
	}

	toolEnv := director_tools.ToolEnv{
		BusinessID: req.BusinessID,
		Locale:     req.Locale,
		DB:         s.db,
		Analytics:  s.analytics,
		Location:   loc,
	}
	if floor, ferr := (&director_tools.LiveFloorTool{}).Run(context.Background(), nil, toolEnv); ferr == nil {
		payload.LiveOps = floor.Data
	}
	if kitchen, kerr := (&director_tools.KitchenStatusTool{}).Run(context.Background(), nil, toolEnv); kerr == nil {
		payload.Kitchen = kitchen.Data
	}
	if promos, perr := (&director_tools.PromosTool{}).Run(context.Background(), nil, toolEnv); perr == nil {
		payload.Promos = promos.Data
	}

	openChecks := 0
	openTotal := 0.0
	if payload.LiveOps != nil {
		openChecks = directorAsInt(payload.LiveOps["open_checks"])
		openTotal = directorAsFloat(payload.LiveOps["open_checks_total"])
	}
	// #870: every money figure below is in the business's own currency. The
	// prose used to hardcode a bare "$", so an Argentine venue's ARS checks were
	// read back to the owner as dollars.
	currency := directorPayloadCurrency(payload)
	payload.MetricsSummary = fmt.Sprintf(
		"All money figures are in %s. Collected today: %s. Remaining on open checks: %s (%d active bills). This week: %s across %d transactions (average ticket %s). Open checks: %d totaling %s.",
		currency,
		directorMoney(currency, payload.Metrics.TodayCollected),
		directorMoney(currency, payload.Metrics.TodayFloorRemaining),
		payload.Metrics.ActiveBills,
		directorMoney(currency, payload.Metrics.WeeklyRevenue),
		payload.Metrics.WeeklyTransactions,
		directorMoney(currency, payload.Metrics.AverageTicket),
		openChecks,
		directorMoney(currency, openTotal),
	)

	return payload, nil
}

func (s *DirectorConsoleService) countOrdersByStatus(businessID uint) struct {
	Pending   int64 `json:"pending"`
	Approved  int64 `json:"approved"`
	InKitchen int64 `json:"in_kitchen"`
	Ready     int64 `json:"ready"`
	Delivered int64 `json:"delivered"`
	Cancelled int64 `json:"cancelled"`
} {
	type orderStatusCount struct {
		Status database.OrderStatus
		Count  int64
	}

	var counts []orderStatusCount
	_ = s.db.GetGorm().
		Model(&database.Order{}).
		Select("status, count(*) as count").
		Where("business_id = ?", businessID).
		Group("status").
		Scan(&counts).Error

	result := struct {
		Pending   int64 `json:"pending"`
		Approved  int64 `json:"approved"`
		InKitchen int64 `json:"in_kitchen"`
		Ready     int64 `json:"ready"`
		Delivered int64 `json:"delivered"`
		Cancelled int64 `json:"cancelled"`
	}{}

	for _, row := range counts {
		switch row.Status {
		case database.OrderStatusPending:
			result.Pending = row.Count
		case database.OrderStatusApproved:
			result.Approved = row.Count
		case database.OrderStatusInKitchen:
			result.InKitchen = row.Count
		case database.OrderStatusOrderReady:
			result.Ready = row.Count
		case database.OrderStatusOrderDelivered:
			result.Delivered = row.Count
		case database.OrderStatusOrderCancelled:
			result.Cancelled = row.Count
		}
	}

	return result
}

// marketingPostsContextPeriodDays matches the briefing lookback used by the
// Director front door (server/director_briefing_handler.go). Keep in sync.
const marketingPostsContextPeriodDays = 7

// buildMarketingStats surfaces mark_posted activity for the model context so
// Sage can mention recent posts and deep-link to the Marketing tab. Counts and
// freeform channel tags only — never signed image URLs or guest PII.
func (s *DirectorConsoleService) buildMarketingStats(businessID uint) map[string]interface{} {
	stats := map[string]interface{}{
		"posts_this_period": int64(0),
		"period_days":       marketingPostsContextPeriodDays,
		"recent_titles":     []string{},
		"channels":          []string{},
		"library_tab":       "marketing",
	}
	if s.db == nil {
		return stats
	}
	since := time.Now().UTC().AddDate(0, 0, -marketingPostsContextPeriodDays)
	period, err := s.db.RecentMarketingPosts(businessID, since, 5)
	if err != nil {
		return stats
	}
	stats["posts_this_period"] = period.Count
	stats["period_days"] = period.PeriodDays
	if len(period.RecentTitles) > 0 {
		stats["recent_titles"] = period.RecentTitles
	}
	if len(period.Channels) > 0 {
		stats["channels"] = period.Channels
	}
	return stats
}

func (s *DirectorConsoleService) buildCRMStats(businessID uint) map[string]interface{} {
	stats := map[string]interface{}{
		"customers":              int64(0),
		"marketing_opt_in_ratio": 0.0,
		"avg_total_spent":        0.0,
		"repeat_customer_ratio":  0.0,
	}

	type crmAggregate struct {
		Customers          int64
		MarketingOptIns    int64
		RepeatCustomers    int64
		AvgTotalSpentValue float64
	}

	var aggregate crmAggregate
	err := s.db.GetGorm().Model(&database.CustomerBusiness{}).
		Select(`
			COUNT(*) AS customers,
			COALESCE(SUM(CASE WHEN opt_in_marketing = true THEN 1 ELSE 0 END), 0) AS marketing_opt_ins,
			COALESCE(SUM(CASE WHEN visit_count > 1 THEN 1 ELSE 0 END), 0) AS repeat_customers,
			COALESCE(AVG(total_spent), 0) AS avg_total_spent_value
		`).
		Where("business_id = ? AND is_active = true", businessID).
		Scan(&aggregate).Error
	if err != nil {
		return stats
	}

	stats["customers"] = aggregate.Customers
	stats["avg_total_spent"] = aggregate.AvgTotalSpentValue
	if aggregate.Customers > 0 {
		stats["marketing_opt_in_ratio"] = float64(aggregate.MarketingOptIns) / float64(aggregate.Customers) * 100
		stats["repeat_customer_ratio"] = float64(aggregate.RepeatCustomers) / float64(aggregate.Customers) * 100
	}

	return stats
}

func (s *DirectorConsoleService) buildAIWaiterStats(businessID uint) map[string]interface{} {
	stats := map[string]interface{}{
		"total_conversations":  int64(0),
		"total_messages":       int64(0),
		"upsell_success_rate":  0.0,
		"active_conversations": int64(0),
	}

	var totalConversations int64
	var totalMessages int64
	var successfulUpsells int64
	var activeConversations int64
	gormDB := s.db.GetGorm()

	gormDB.Model(&database.AiWaiterConversation{}).
		Where("business_id = ?", businessID).
		Count(&totalConversations)
	gormDB.Model(&database.AiWaiterMessage{}).
		Joins("JOIN ai_waiter_conversations ON ai_waiter_messages.conversation_id = ai_waiter_conversations.id").
		Where("ai_waiter_conversations.business_id = ?", businessID).
		Count(&totalMessages)
	gormDB.Model(&database.AiWaiterConversation{}).
		Where("business_id = ? AND cart_items_added > 0", businessID).
		Count(&successfulUpsells)
	gormDB.Model(&database.AiWaiterConversation{}).
		Where("business_id = ? AND status = ?", businessID, "active").
		Count(&activeConversations)

	rate := 0.0
	if totalConversations > 0 {
		rate = float64(successfulUpsells) / float64(totalConversations) * 100
	}

	stats["total_conversations"] = totalConversations
	stats["total_messages"] = totalMessages
	stats["upsell_success_rate"] = rate
	stats["active_conversations"] = activeConversations

	return stats
}

func (s *DirectorConsoleService) buildPluginInsights(businessID uint) ([]map[string]interface{}, []map[string]string) {
	enabled := make([]map[string]interface{}, 0)
	enabledNames := map[string]struct{}{}

	businessPlugins, err := database.GetBusinessPlugins(businessID)
	if err == nil {
		for _, plugin := range businessPlugins {
			if !directorPluginIsEnabled(plugin["is_enabled"]) {
				continue
			}

			name := toStringValue(plugin["name"])
			if name == "" {
				continue
			}
			enabledNames[name] = struct{}{}

			configValue := map[string]interface{}{}
			rawConfig := toStringValue(plugin["config"])
			if rawConfig != "" {
				_ = json.Unmarshal([]byte(rawConfig), &configValue)
			}

			enabled = append(enabled, map[string]interface{}{
				"name":         name,
				"display_name": toStringValue(plugin["display_name"]),
				"category":     toStringValue(plugin["category"]),
				"config":       configValue,
			})
		}
	}

	sort.Slice(enabled, func(i, j int) bool {
		left := toStringValue(enabled[i]["display_name"])
		right := toStringValue(enabled[j]["display_name"])
		return strings.ToLower(left) < strings.ToLower(right)
	})

	missing := make([]map[string]string, 0)
	platformPlugins, err := database.GetAllPlugins(true)
	if err != nil {
		return enabled, missing
	}

	for _, plugin := range platformPlugins {
		if _, ok := enabledNames[plugin.Name]; ok {
			continue
		}
		if plugin.ComingSoon {
			continue
		}
		if plugin.Category != database.PluginCategoryAnalytics && plugin.Category != database.PluginCategoryReporting {
			continue
		}

		tab := "plugins"
		missing = append(missing, map[string]string{
			"name":      plugin.Name,
			"title":     plugin.DisplayName,
			"category":  plugin.Category,
			"deep_link": buildTabDeepLink(strconv.Itoa(int(businessID)), tab),
		})
		if len(missing) >= 5 {
			break
		}
	}

	return enabled, missing
}

func buildDirectorSystemPrompt(assistantName string, businessID uint, localeCode string) string {
	locale := resolvePromptLocale(localeCode)
	contentLocale := locale
	content, err := directorPromptFS.ReadFile(fmt.Sprintf("prompts/director_console/%s.md", locale.PromptFamily))
	if err != nil {
		content, err = directorPromptFS.ReadFile("prompts/director_console/en.md")
		contentLocale = locales.Default()
	}
	if err != nil {
		return fmt.Sprintf("You are %s, an owner-focused business copilot for restaurant operations and growth. Output strict JSON and use dashboard deep links only in format /business/%d/dashboard?tab=<tab>. %s", assistantName, businessID, directorLanguageInstruction(contentLocale))
	}

	replacer := strings.NewReplacer(
		"{{ASSISTANT_NAME}}", assistantName,
		"{{BUSINESS_ID}}", strconv.Itoa(int(businessID)),
		"{{LANGUAGE_INSTRUCTION}}", directorLanguageInstruction(contentLocale),
	)

	return replacer.Replace(strings.TrimSpace(string(content)))
}

// resolveDirectorAskLocale picks the canonical prompt locale for a Director
// Console answer. An explicit, resolvable request locale always wins. When the
// request omits a locale (blank), the business owner's UI language is used so
// the briefing/answer matches the operator's language instead of defaulting to
// English.
func resolveDirectorAskLocale(requestLocale string, business *database.Business) string {
	if strings.TrimSpace(requestLocale) != "" {
		return resolvePromptLocale(requestLocale).Canonical
	}
	return resolvePromptLocale(DetermineBusinessOwnerLanguage(business)).Canonical
}

func directorLanguageInstruction(locale locales.Locale) string {
	switch locale.PromptFamily {
	case "es_ar":
		return fmt.Sprintf("Respond in %s with Argentine Spanish phrasing for restaurant operators.", locale.NativeName)
	case "es":
		return fmt.Sprintf("Respond in %s, using clear, professional Spanish.", locale.NativeName)
	default:
		return fmt.Sprintf("Respond in %s.", locale.NativeName)
	}
}

func directorUsesSpanishCopy(localeCode string) bool {
	promptFamily := resolvePromptLocale(localeCode).PromptFamily
	return promptFamily == "es" || strings.HasPrefix(promptFamily, "es_")
}

// directorRewriteSuffix appends a localized "(opens <tab>)" hint so a rewritten
// deep link's button destination matches the visible action text.
func directorRewriteSuffix(title, tab, locale string) string {
	if directorUsesSpanishCopy(locale) {
		return fmt.Sprintf("%s (abre %s)", title, tab)
	}
	return fmt.Sprintf("%s (opens %s)", title, tab)
}

// directorWantsPluginUpsell does a light intent check: only growth/plugin/
// marketing/integration questions invite plugin recommendations. Anything else
// (staffing, kitchen, a slow daypart) gets none unless the action plan is sparse.
func directorFreezeWrites(message string) bool {
	q := strings.ToLower(strings.TrimSpace(message))
	if q == "" {
		return false
	}
	for _, phrase := range []string{
		"no cambies nada",
		"no lo cambies",
		"no cambies",
		"do not change",
		"don't change",
		"do not apply",
		"don't apply",
		"no lo apliques",
		"no apliques",
		"sin aplicar",
		"preview only",
		"before anything goes live",
		"before it goes live",
		"before anything is live",
		"antes de que esté en vivo",
		"antes de que este en vivo",
		"antes de que vaya en vivo",
		"antes de que salga en vivo",
		"antes de que entre en vivo",
		"antes de que esté live",
		"antes de que este live",
	} {
		if strings.Contains(q, phrase) {
			return true
		}
	}
	return false
}

func directorIsLiveOpsQuestion(question string) bool {
	q := strings.ToLower(question)
	for _, kw := range []string{
		"mesa", "mesas", "table", "tables",
		"cocina", "kitchen",
		"mozo", "waiter", "server",
		"caja", "cash drawer", "cash register", "cierra la caja", "close the cash",
		"best seller", "best-seller", "más vendid", "mas vendid",
		"sell tonight", "sold tonight", "vendimos", "esta noche",
		"how much did we sell", "cuánto vend", "cuanto vend",
		"waiting", "esperando",
		"offer", "bundle", "promo", "combo", "date night",
		"86",
		"margin", "margen", "thinnest", "slow mover", "slow-mover",
		"moviendo poco", "food cost", "food-cost",
	} {
		if strings.Contains(q, kw) {
			return true
		}
	}
	return false
}

func directorWantsPluginUpsell(question string) bool {
	q := strings.ToLower(question)
	for _, kw := range []string{
		"plugin", "integrat", "grow", "growth", "marketing", "review", "reseña",
		"crecer", "crecimiento", "integraci", "automat", "automatiz",
	} {
		if strings.Contains(q, kw) {
			return true
		}
	}
	return false
}

// directorLangidMinConfidence mirrors Lane H0's grader floor (grader.go
// langidMinConfidence = 0.34) so the Director post-check and the eval grader
// agree on what counts as a confident detection. Below this floor the
// detection is INCONCLUSIVE and we accept the answer (no mismatch).
const directorLangidMinConfidence = 0.34

// languageMismatch reports whether the answer's detected language confidently
// differs from the requested locale's base language. langid.Detect returns a
// confidence in [0,1]; a confidence below directorLangidMinConfidence (0.34,
// matching Lane H0's grader) is treated as "no mismatch" (accept).
func (s *DirectorConsoleService) languageMismatch(localeCode, summary string) bool {
	if strings.TrimSpace(summary) == "" {
		return false
	}
	detected, conf := langid.Detect(summary)
	if conf < directorLangidMinConfidence {
		return false
	}
	want := pillLocale(resolvePromptLocale(localeCode).Canonical)
	return detected != want
}

// regenerateForLanguage re-runs the loop exactly once with a corrective
// instruction prepended to the user prompt, forcing the target language. It
// returns ok=false if the regeneration errored (caller keeps the first answer).
func (s *DirectorConsoleService) regenerateForLanguage(ctx context.Context, req DirectorAskRequest, business *database.Business, thread *database.DirectorConsoleThread, payload *directorContext, contextJSON []byte, systemPrompt string, priorTurns []llm.Message, sink Sink) (DirectorStructuredResponse, string, int64, []uint, []uint, bool) {
	corrective := directorUserLanguageAnchor(req.Locale)
	userPrompt := fmt.Sprintf(
		"IMPORTANT: Your previous answer was in the wrong language. %s\n\nOwner question:\n%s\n\nBusiness context JSON:\n%s\n\n%s",
		corrective, strings.TrimSpace(req.Message), string(contextJSON), corrective,
	)
	cfg := LoopConfig{
		Registry: s.toolRegistry,
		Sink:     sink,
		Env: director_tools.ToolEnv{
			BusinessID: req.BusinessID, ThreadID: thread.ID, Locale: req.Locale,
			DB: s.db, Analytics: s.analytics, Location: database.ResolveBusinessLocation(business),
			FreezeWrites: directorFreezeWrites(req.Message),
		},
		Model:               s.aiService.DirectorModel(),
		MaxIterations:       defaultDirectorLoopMaxIterations,
		WallClock:           defaultDirectorLoopWallClock,
		PerIterationTimeout: defaultDirectorPerIterationTimeout,
		Provider:            s.aiService.Provider(),
		Fallbacks:           s.aiService.DirectorFallbacks(),
		SystemPrompt:        systemPrompt,
		UserPrompt:          userPrompt,
		PriorTurns:          priorTurns,
	}
	res, err := RunDirectorLoop(ctx, cfg)
	if err != nil {
		return DirectorStructuredResponse{}, "", 0, nil, nil, false
	}
	return res.Final, res.Model, res.LatencyMs, res.ToolCallIDs, res.ProposalIDs, true
}

// directorUserLanguageAnchor is appended to the END of the user turn. Trailing
// placement + NativeName naming counters the English anchoring of the context
// JSON and English tool results (audit §4-E). en/es/es_ar carry tailored copy;
// all other prompt families use a neutral NativeName line.
func directorUserLanguageAnchor(localeCode string) string {
	locale := resolvePromptLocale(localeCode)
	switch locale.PromptFamily {
	case "es_ar":
		return fmt.Sprintf("Respondé únicamente en %s. Todos los valores de texto del JSON deben estar en %s.", locale.NativeName, locale.NativeName)
	case "es":
		return fmt.Sprintf("Responde únicamente en %s. Todos los valores de texto del JSON deben estar en %s.", locale.NativeName, locale.NativeName)
	default:
		return fmt.Sprintf("Answer in %s only. Every JSON string value must be in %s.", locale.NativeName, locale.NativeName)
	}
}

type directorFallbackText struct {
	ActionTitle              string
	ActionDescription        string
	PluginTitleFormat        string
	PluginDescription        string
	DefaultActionTitle       string
	DefaultActionDescription string
}

func directorFallbackTextFor(localeCode string) directorFallbackText {
	switch resolvePromptLocale(localeCode).PromptFamily {
	case "es_ar":
		return directorFallbackText{
			ActionTitle:              "Acción",
			ActionDescription:        "Aplicá esta recomendación desde el dashboard.",
			PluginTitleFormat:        "Evaluá %s",
			PluginDescription:        "Activá y configurá este plugin para mejorar la cobertura de reportes y la automatización.",
			DefaultActionTitle:       "Revisá tendencia de ingresos y conversión",
			DefaultActionDescription: "Inspeccioná cambios semanales y detectá franjas horarias de bajo rendimiento.",
		}
	case "es":
		return directorFallbackText{
			ActionTitle:              "Acción",
			ActionDescription:        "Aplica esta recomendación desde el dashboard.",
			PluginTitleFormat:        "Evalúa %s",
			PluginDescription:        "Activa y configura este plugin para mejorar la cobertura de reportes y la automatización.",
			DefaultActionTitle:       "Revisar tendencia de ingresos y conversión",
			DefaultActionDescription: "Inspecciona cambios semanales y detecta franjas horarias de bajo rendimiento.",
		}
	default:
		return directorFallbackText{
			ActionTitle:              "Action",
			ActionDescription:        "Apply this recommendation from the dashboard.",
			PluginTitleFormat:        "Evaluate %s",
			PluginDescription:        "Enable and configure this plugin to improve reporting coverage and automation.",
			DefaultActionTitle:       "Review revenue and conversion trend",
			DefaultActionDescription: "Inspect weekly changes and isolate low-performing service windows.",
		}
	}
}

// loadPriorTurns reads the thread's recent messages and compacts them into
// loop turns, excluding the just-saved current user message. Best-effort:
// a read failure simply yields no memory.
// excludeIDs drops additional rows from memory — used by regenerate for the
// answer that is about to be replaced but is still persisted (L4-15).
func (s *DirectorConsoleService) loadPriorTurns(businessID, threadID, currentUserMsgID uint, excludeIDs ...uint) []llm.Message {
	msgs, err := database.ListDirectorConsoleMessageSummaries(businessID, threadID, directorMemoryMessageLimit)
	if err != nil {
		return nil
	}
	return buildPriorTurns(msgs, currentUserMsgID, directorMemoryMaxTurns, excludeIDs...)
}

// directorSyntheticModelNames are the model-name sentinels askInternal stamps
// when no genuine model answer was produced: the daily-dollar budget notice,
// guardrail blocks, the no-provider / loop-error fallbacks, and the partial
// progress emitted on a loop timeout.
var directorSyntheticModelNames = map[string]struct{}{
	"":                {},
	"budget":          {},
	"guardrail":       {},
	"fallback":        {},
	"fallback_error":  {},
	"partial_timeout": {},
}

// directorAnswerSupersedes reports whether an answer stamped with modelName is
// a genuine replacement, i.e. whether a regenerate may destroy the answer it
// replaces (L4-15). Canned and degraded results never may.
func directorAnswerSupersedes(modelName string) bool {
	_, synthetic := directorSyntheticModelNames[strings.TrimSpace(modelName)]
	return !synthetic
}

// runLoopOrFallback wraps RunDirectorLoop with the deterministic fallback
// the Director Console must always produce on error. Both Ask and
// AskStreaming feed in their own Sink — bufferSink for the JSON HTTP path,
// a streaming SSE sink for the streaming path — so the loop's per-step
// progress events go to the right consumer.
func (s *DirectorConsoleService) runLoopOrFallback(ctx context.Context, req DirectorAskRequest, business *database.Business, thread *database.DirectorConsoleThread, payload *directorContext, priorTurns []llm.Message, sink Sink) (DirectorStructuredResponse, string, int64, []uint, []uint) {
	var fallback DirectorStructuredResponse
	if payload != nil && payload.DataReadiness.State == "setup" {
		fallback = sanitizeStructuredResponse(s.buildSetupResponse(req.BusinessID, req.Locale, payload))
	} else {
		fallback = sanitizeStructuredResponse(s.buildFallbackResponse(req.BusinessID, req.Locale, payload))
	}

	if s.aiService == nil || s.aiService.Provider() == nil || s.toolRegistry == nil {
		return fallback, "fallback", 0, nil, nil
	}

	if sink == nil {
		sink = &bufferSink{}
	}

	contextJSON, _ := json.Marshal(payload)
	assistantName := SanitizePromptField(business.AiSettings.AiName, 40)
	if assistantName == "" {
		assistantName = "Sage"
	}

	systemPrompt := buildDirectorSystemPrompt(assistantName, req.BusinessID, req.Locale)
	userPrompt := fmt.Sprintf(
		"Owner question:\n%s\n\nBusiness context JSON:\n%s\n\n%s",
		strings.TrimSpace(req.Message),
		string(contextJSON),
		directorUserLanguageAnchor(req.Locale),
	)

	cfg := LoopConfig{
		Registry: s.toolRegistry,
		Sink:     sink,
		Env: director_tools.ToolEnv{
			BusinessID:   req.BusinessID,
			ThreadID:     thread.ID,
			Locale:       req.Locale,
			DB:           s.db,
			Analytics:    s.analytics,
			Location:     database.ResolveBusinessLocation(business),
			FreezeWrites: directorFreezeWrites(req.Message),
		},
		Model:               s.aiService.DirectorModel(),
		MaxIterations:       defaultDirectorLoopMaxIterations,
		WallClock:           defaultDirectorLoopWallClock,
		PerIterationTimeout: defaultDirectorPerIterationTimeout,
		Provider:            s.aiService.Provider(),
		Fallbacks:           s.aiService.DirectorFallbacks(),
		SystemPrompt:        systemPrompt,
		UserPrompt:          userPrompt,
		PriorTurns:          priorTurns,
	}

	loopStart := time.Now()
	res, err := RunDirectorLoop(ctx, cfg)
	if err != nil {
		var timedOut *DirectorLoopTimeoutError
		if errors.As(err, &timedOut) {
			partial := s.buildPartialProgressResponse(req.BusinessID, req.Locale, payload, timedOut.CompletedSummaries)
			return sanitizeStructuredResponse(partial), "partial_timeout", time.Since(loopStart).Milliseconds(), nil, nil
		}
		return fallback, "fallback_error", 0, nil, nil
	}

	if directorFreezeWrites(req.Message) && len(res.ProposalIDs) > 0 {
		_ = database.DismissDirectorProposals(res.ProposalIDs)
		res.ProposalIDs = nil
	}

	normalized := s.normalizeStructuredOutput(req.BusinessID, req.Locale, req.Message, res.Final, payload)

	if s.languageMismatch(req.Locale, normalized.Summary) {
		if regen, regenModel, regenLatency, regenToolIDs, regenProposalIDs, ok := s.regenerateForLanguage(ctx, req, business, thread, payload, contextJSON, systemPrompt, priorTurns, sink); ok {
			regenNorm := s.normalizeStructuredOutput(req.BusinessID, req.Locale, req.Message, regen, payload)
			// Best-effort: dismiss the first-run proposals since the regen supersedes them.
			if len(res.ProposalIDs) > 0 {
				_ = database.DismissDirectorProposals(res.ProposalIDs)
			}
			return sanitizeStructuredResponse(regenNorm), regenModel, regenLatency, append(res.ToolCallIDs, regenToolIDs...), regenProposalIDs
		}
	}

	return sanitizeStructuredResponse(normalized), res.Model, res.LatencyMs, res.ToolCallIDs, res.ProposalIDs
}

func (s *DirectorConsoleService) normalizeStructuredOutput(businessID uint, locale string, question string, output DirectorStructuredResponse, payload *directorContext) DirectorStructuredResponse {
	output.Summary = strings.TrimSpace(output.Summary)
	output.Diagnosis = strings.TrimSpace(output.Diagnosis)
	output.ExpectedImpact = strings.TrimSpace(output.ExpectedImpact)

	if output.Summary == "" {
		if directorUsesSpanishCopy(locale) {
			output.Summary = "Esto es lo que encontré."
		} else {
			output.Summary = "Here is what I found."
		}
	}

	fallbackText := directorFallbackTextFor(locale)
	cleanActions := make([]DirectorAction, 0, len(output.ActionPlan)+2)
	for _, action := range output.ActionPlan {
		tab := extractTabFromDeepLink(action.DeepLink)
		title := fallbackString(action.Title, fallbackText.ActionTitle)
		if _, ok := directorAllowedTabs[tab]; !ok {
			tab = "analytics"
			title = directorRewriteSuffix(title, tab, locale)
		}
		priority := strings.ToLower(strings.TrimSpace(action.Priority))
		if priority != "high" && priority != "medium" && priority != "low" {
			priority = "medium"
		}
		cleanActions = append(cleanActions, DirectorAction{
			Title:       title,
			Description: fallbackString(action.Description, fallbackText.ActionDescription),
			DeepLink:    buildTabDeepLink(strconv.Itoa(int(businessID)), tab),
			Priority:    priority,
		})
	}

	modelActionCount := len(cleanActions)
	if payload != nil && !directorIsLiveOpsQuestion(question) && (modelActionCount < 2 || directorWantsPluginUpsell(question)) {
		pluginsAdded := 0
		for _, plugin := range payload.Plugins.MissingRecommendations {
			if pluginsAdded >= 2 {
				break
			}
			title := plugin["title"]
			if title == "" {
				continue
			}
			cleanActions = append(cleanActions, DirectorAction{
				Title:       fmt.Sprintf(fallbackText.PluginTitleFormat, title),
				Description: fallbackText.PluginDescription,
				DeepLink:    plugin["deep_link"],
				Priority:    "low",
			})
			pluginsAdded++
		}
	}

	if len(cleanActions) == 0 {
		cleanActions = append(cleanActions, DirectorAction{
			Title:       fallbackText.DefaultActionTitle,
			Description: fallbackText.DefaultActionDescription,
			DeepLink:    buildTabDeepLink(strconv.Itoa(int(businessID)), "analytics"),
			Priority:    "high",
		})
	}

	output.ActionPlan = cleanActions

	if len(output.FollowUps) == 0 {
		output.FollowUps = defaultFollowUps(locale)
	}

	return output
}

// buildPartialProgressResponse is returned when the loop hits its wall clock
// after completing some tool work. It is honest ("this took too long, here is
// what I found") and surfaces whatever the completed tools summarized, instead
// of the generic growth plan the hard-error fallback returns. Localized en/es.
func (s *DirectorConsoleService) buildPartialProgressResponse(businessID uint, locale string, payload *directorContext, completed []string) DirectorStructuredResponse {
	evidence := make([]string, 0, len(completed))
	for _, c := range completed {
		if strings.TrimSpace(c) != "" {
			evidence = append(evidence, strings.TrimSpace(c))
		}
	}

	action := DirectorAction{
		DeepLink: buildTabDeepLink(strconv.Itoa(int(businessID)), "analytics"),
		Priority: "high",
	}

	if directorUsesSpanishCopy(locale) {
		if len(evidence) == 0 {
			evidence = []string{"No alcancé a completar el análisis antes del límite de tiempo."}
		}
		action.Title = "Volvé a preguntar para un análisis completo"
		action.Description = "Revisá analytics mientras tanto; reintentá la pregunta para el plan completo."
		return DirectorStructuredResponse{
			Summary:        "Esto tardó más de lo esperado. Esto es lo que alcancé a encontrar:",
			Diagnosis:      "El análisis no terminó dentro del límite de tiempo; los datos de abajo son parciales.",
			Evidence:       evidence,
			ActionPlan:     []DirectorAction{action},
			ExpectedImpact: "Reintentá la pregunta para obtener un diagnóstico y plan completos.",
			FollowUps:      defaultFollowUps(locale),
		}
	}

	if len(evidence) == 0 {
		evidence = []string{"I could not finish the analysis before the time limit."}
	}
	action.Title = "Re-ask for a full analysis"
	action.Description = "Review analytics in the meantime; resend the question for the complete plan."
	return DirectorStructuredResponse{
		Summary:        "This took longer than expected. Here is what I found so far:",
		Diagnosis:      "The analysis did not finish within the time limit; the data below is partial.",
		Evidence:       evidence,
		ActionPlan:     []DirectorAction{action},
		ExpectedImpact: "Re-ask the question to get a complete diagnosis and plan.",
		FollowUps:      defaultFollowUps(locale),
	}
}

func (s *DirectorConsoleService) buildFallbackResponse(businessID uint, locale string, payload *directorContext) DirectorStructuredResponse {
	weekly := 0.0
	tx := 0
	openChecks := 0
	openTotal := 0.0
	if payload != nil {
		weekly = payload.Metrics.WeeklyRevenue
		tx = payload.Metrics.WeeklyTransactions
		if payload.LiveOps != nil {
			openChecks = directorAsInt(payload.LiveOps["open_checks"])
			openTotal = directorAsFloat(payload.LiveOps["open_checks_total"])
		}
	}
	if directorUsesSpanishCopy(locale) {
		response := DirectorStructuredResponse{
			Summary:   "Aquí tienes un plan práctico para impulsar crecimiento y estabilidad operativa.",
			Diagnosis: fmt.Sprintf("Esta semana hay $%.2f en %d transacciones. Cuentas abiertas: %d por $%.2f.", weekly, tx, openChecks, openTotal),
			Evidence: []string{
				fmt.Sprintf("Ingresos de la semana: $%.2f en %d transacciones.", weekly, tx),
				fmt.Sprintf("Cuentas abiertas ahora: %d por $%.2f.", openChecks, openTotal),
			},
			ActionPlan: []DirectorAction{
				{
					Title:       "Auditar rendimiento por franja horaria",
					Description: "Detecta horas lentas y ajusta promociones, personal y oferta para recuperar demanda.",
					DeepLink:    buildTabDeepLink(strconv.Itoa(int(businessID)), "analytics"),
					Priority:    "high",
				},
				{
					Title:       "Optimizar recomendaciones y upsells",
					Description: "Activa acciones de ticket promedio en menú y experiencia asistida para aumentar ingresos por pedido.",
					DeepLink:    buildTabDeepLink(strconv.Itoa(int(businessID)), "menu"),
					Priority:    "medium",
				},
				{
					Title:       "Ajustar reportes automáticos",
					Description: "Revisa plugins de reportes y horarios para asegurar decisiones basadas en datos frescos.",
					DeepLink:    buildTabDeepLink(strconv.Itoa(int(businessID)), "plugins"),
					Priority:    "medium",
				},
			},
			ExpectedImpact: "Aplicando estas acciones deberías mejorar ritmo de ventas y conversión en ventanas de baja demanda.",
			FollowUps: []string{
				"¿Qué día y hora quieres que prioricemos para mejorar primero?",
				"¿Quieres un plan de 7 días para aumentar ticket promedio?",
				"¿Prefieres enfocarnos en adquisición o en retención esta semana?",
			},
		}
		return response
	}

	return DirectorStructuredResponse{
		Summary:   "Here is a practical plan to improve growth and operational consistency.",
		Diagnosis: fmt.Sprintf("This week: $%.2f across %d transactions. Open checks: %d totaling $%.2f.", weekly, tx, openChecks, openTotal),
		Evidence: []string{
			fmt.Sprintf("This week's recognized revenue is $%.2f across %d transactions.", weekly, tx),
			fmt.Sprintf("Open checks right now: %d totaling $%.2f.", openChecks, openTotal),
		},
		ActionPlan: []DirectorAction{
			{
				Title:       "Audit slow service windows",
				Description: "Identify low-performing hours and adjust offer mix, staffing, and promotion timing.",
				DeepLink:    buildTabDeepLink(strconv.Itoa(int(businessID)), "analytics"),
				Priority:    "high",
			},
			{
				Title:       "Increase average order value",
				Description: "Improve upsell moments in menu design and assisted selling flows.",
				DeepLink:    buildTabDeepLink(strconv.Itoa(int(businessID)), "menu"),
				Priority:    "medium",
			},
			{
				Title:       "Tighten reporting automation",
				Description: "Review reporting plugins and schedules so decisions are based on fresh performance signals.",
				DeepLink:    buildTabDeepLink(strconv.Itoa(int(businessID)), "plugins"),
				Priority:    "medium",
			},
		},
		ExpectedImpact: "Executing these actions should improve conversion quality and reduce week-to-week performance volatility.",
		FollowUps:      defaultFollowUps(locale),
	}
}

// buildSetupResponse is the deterministic fallback for a business still in
// "setup" state (Task 1.1). It is honest about the lack of data — evidence
// uses only real counts from the payload — and returns launch-checklist
// actions instead of an invented growth plan.
func (s *DirectorConsoleService) buildSetupResponse(businessID uint, locale string, payload *directorContext) DirectorStructuredResponse {
	menuCount := int64(0)
	customers := int64(0)
	paymentsOn := false
	if payload != nil {
		menuCount = int64(payload.DataReadiness.MenuItemCount)
		customers = payload.DataReadiness.CustomerCount
		paymentsOn = payload.DataReadiness.PaymentsEnabled
	}
	link := func(tab string) string { return buildTabDeepLink(strconv.Itoa(int(businessID)), tab) }

	if directorUsesSpanishCopy(locale) {
		ar := resolvePromptLocale(locale).PromptFamily == "es_ar"
		pagos := "no"
		if paymentsOn {
			pagos = "sí"
		}
		summary := "Tu negocio recién se está configurando. Todavía no hay actividad suficiente para un análisis con datos, así que esta es tu lista de puesta en marcha."
		impact := "Cuando tomes tus primeros pedidos podré analizar ingresos, franjas horarias y clientes con datos reales."
		// Default copy is voseo (es_ar). Plain "es" uses tú forms for every
		// imperative — titles, descriptions, AND follow-ups — so the variants
		// stay internally consistent.
		titles := []string{"Armá tu menú", "Conectá pagos", "Creá mesas e imprimí los QR", "Definí horarios y perfil"}
		descs := []string{
			"Cargá categorías e ítems con precios para que los clientes puedan pedir.",
			"Activá un método de pago para empezar a cobrar.",
			"Generá mesas y sus códigos QR para recibir pedidos en el local.",
			"Completá horarios, datos y la página del negocio.",
		}
		followUps := []string{
			"¿Querés una guía paso a paso para la puesta en marcha?",
			"¿Por cuál paso querés empezar?",
		}
		if !ar {
			titles = []string{"Crea tu menú", "Conecta pagos", "Crea mesas e imprime los QR", "Define horarios y perfil"}
			descs = []string{
				"Carga categorías e ítems con precios para que los clientes puedan pedir.",
				"Activa un método de pago para empezar a cobrar.",
				"Genera mesas y sus códigos QR para recibir pedidos en el local.",
				"Completa horarios, datos y la página del negocio.",
			}
			followUps = []string{
				"¿Quieres una guía paso a paso para la puesta en marcha?",
				"¿Por cuál paso quieres empezar?",
			}
		}
		return DirectorStructuredResponse{
			Summary:   summary,
			Diagnosis: "Sin ingresos reconocidos y con pocos pedidos, cualquier métrica de crecimiento sería una suposición. Primero terminemos la configuración.",
			Evidence: []string{
				fmt.Sprintf("Ítems de menú configurados: %d.", menuCount),
				fmt.Sprintf("Pagos conectados: %s.", pagos),
				fmt.Sprintf("Clientes registrados: %d.", customers),
			},
			ActionPlan: []DirectorAction{
				{Title: titles[0], Description: descs[0], DeepLink: link("menu"), Priority: "high"},
				{Title: titles[1], Description: descs[1], DeepLink: link("plugins"), Priority: "high"},
				{Title: titles[2], Description: descs[2], DeepLink: link("tables"), Priority: "medium"},
				{Title: titles[3], Description: descs[3], DeepLink: link("settings"), Priority: "medium"},
			},
			ExpectedImpact: impact,
			FollowUps:      followUps,
		}
	}

	pagos := "no"
	if paymentsOn {
		pagos = "yes"
	}
	return DirectorStructuredResponse{
		Summary:   "Your business is just getting set up. There isn't enough activity yet for a data-driven analysis, so here's your launch checklist.",
		Diagnosis: "With no recognized revenue and only early orders, growth metrics would be guesses — let's finish setup so future analysis is grounded in real numbers.",
		Evidence: []string{
			fmt.Sprintf("Menu items configured: %d.", menuCount),
			fmt.Sprintf("Payments connected: %s.", pagos),
			fmt.Sprintf("Customers so far: %d.", customers),
		},
		ActionPlan: []DirectorAction{
			{Title: "Build your menu", Description: "Add categories and priced items so guests can order.", DeepLink: link("menu"), Priority: "high"},
			{Title: "Connect payments", Description: "Enable a payment method so you can start taking money.", DeepLink: link("plugins"), Priority: "high"},
			{Title: "Create tables & print QR codes", Description: "Generate tables and their QR codes to take in-venue orders.", DeepLink: link("tables"), Priority: "medium"},
			{Title: "Set hours & profile", Description: "Complete your hours, details, and business page.", DeepLink: link("settings"), Priority: "medium"},
		},
		ExpectedImpact: "Once you take your first orders, I can analyze revenue, dayparts, and customers with real data.",
		FollowUps: []string{
			"Want a step-by-step launch checklist?",
			"Which setup step should we start with?",
		},
	}
}

// assembleProposedActions loads the pending proposal rows created during this
// ask and maps them into the response DTO, decoding the stored preview JSON.
// Server-truth only: nothing here comes from model output.
func (s *DirectorConsoleService) assembleProposedActions(businessID uint, ids []uint) []director_actions.ProposedAction {
	rows, err := database.GetDirectorProposedActionsByIDs(businessID, ids)
	if err != nil || len(rows) == 0 {
		return nil
	}
	// Build a lookup by ID to preserve loop order.
	byID := make(map[uint]database.DirectorProposedAction, len(rows))
	for _, r := range rows {
		byID[r.ID] = r
	}
	out := make([]director_actions.ProposedAction, 0, len(ids))
	for _, id := range ids {
		r, ok := byID[id]
		if !ok || r.Status != database.DirectorProposalPending {
			continue
		}
		var preview director_actions.ActionPreview
		_ = json.Unmarshal([]byte(r.PreviewJSON), &preview)
		var meta struct {
			Title             string   `json:"title"`
			Description       string   `json:"description"`
			Warnings          []string `json:"warnings"`
			RequiresReconfirm bool     `json:"requires_reconfirm"`
		}
		// title/desc/warnings/requires_reconfirm are stored in the same preview_json blob (Task 4).
		_ = json.Unmarshal([]byte(r.PreviewJSON), &meta)
		out = append(out, director_actions.ProposedAction{
			ID:                r.PublicID,
			Kind:              director_actions.Kind(r.Kind),
			Title:             meta.Title,
			Description:       meta.Description,
			Preview:           preview,
			Warnings:          meta.Warnings,
			MenuVersion:       r.MenuVersion,
			RequiresReconfirm: meta.RequiresReconfirm,
			ExpiresAt:         r.ExpiresAt,
		})
	}
	return out
}

// buildHostileBlockResponse is served for abuse/injection (and unknown block
// categories). No context tools, provider, or proposals run for these turns.
func buildHostileBlockResponse(locale, category string) DirectorStructuredResponse {
	if directorUsesSpanishCopy(locale) {
		return DirectorStructuredResponse{
			Summary:   "No puedo ayudar con esa solicitud.",
			Diagnosis: "El mensaje fue bloqueado por las políticas de seguridad del copiloto.",
			Evidence:  []string{"No se revisó evidencia del negocio en este turno."},
			ActionPlan: []DirectorAction{
				{
					Title:       "Reformular la pregunta",
					Description: "Haz una pregunta operativa o de crecimiento sobre tu negocio.",
					Priority:    "high",
				},
			},
			ExpectedImpact: "Una pregunta de negocio clara permite un análisis útil y seguro.",
			FollowUps:      []string{"¿Qué métrica o área operativa quieres revisar?"},
		}
	}
	_ = category // kept for telemetry callers; response stays non-revealing
	return DirectorStructuredResponse{
		Summary:   "I can't help with that request.",
		Diagnosis: "This message was blocked by the copilot safety policy.",
		Evidence:  []string{"No business evidence was reviewed for this turn."},
		ActionPlan: []DirectorAction{
			{
				Title:       "Rephrase your question",
				Description: "Ask an operational or growth question about your business.",
				Priority:    "high",
			},
		},
		ExpectedImpact: "A clear business question unlocks useful, safe analysis.",
		FollowUps:      []string{"Which metric or operational area should we review?"},
	}
}

func buildScopeRedirectResponse(businessID uint, locale string) DirectorStructuredResponse {
	if directorUsesSpanishCopy(locale) {
		return DirectorStructuredResponse{
			Summary:   "Puedo ayudarte mejor con preguntas de crecimiento y operación del negocio.",
			Diagnosis: "Tu consulta parece fuera del contexto operativo/comercial del dashboard.",
			Evidence: []string{
				"Este copiloto está diseñado para ventas, operaciones, clientes y rentabilidad.",
			},
			ActionPlan: []DirectorAction{
				{
					Title:       "Revisar desempeño de ventas",
					Description: "Empecemos por detectar qué métricas están frenando crecimiento hoy.",
					DeepLink:    buildTabDeepLink(strconv.Itoa(int(businessID)), "analytics"),
					Priority:    "high",
				},
			},
			ExpectedImpact: "Si enfocamos la conversación en métricas de negocio, puedo darte acciones más útiles y accionables.",
			FollowUps: []string{
				"¿Qué objetivo quieres mejorar primero: ingresos, ticket promedio o retención?",
				"¿Quieres que analicemos por qué un día específico fue lento?",
			},
		}
	}

	return DirectorStructuredResponse{
		Summary:   "I can help most with business growth and operational questions.",
		Diagnosis: "Your last prompt looks outside the business context of this dashboard.",
		Evidence: []string{
			"This copilot is optimized for sales, operations, customer, and profitability decisions.",
		},
		ActionPlan: []DirectorAction{
			{
				Title:       "Review sales performance baseline",
				Description: "Start from current metrics and isolate where growth is leaking.",
				DeepLink:    buildTabDeepLink(strconv.Itoa(int(businessID)), "analytics"),
				Priority:    "high",
			},
		},
		ExpectedImpact: "Refocusing on business metrics will let me produce specific, high-impact actions.",
		FollowUps: []string{
			"Which metric do you want to improve first: revenue, average ticket, or retention?",
			"Do you want me to diagnose why a specific day was slow?",
		},
	}
}

// buildBudgetExceededResponse is the deterministic, server-built answer served
// when the business has hit its daily AI dollar ceiling. No model call is made,
// so this bounds Director spend to the ceiling that every other AI lane already
// enforces.
func buildBudgetExceededResponse(locale string) DirectorStructuredResponse {
	if directorUsesSpanishCopy(locale) {
		return DirectorStructuredResponse{
			Summary:        "Alcanzaste el límite diario de uso de IA para este negocio.",
			Diagnosis:      "Para proteger tus costos, el copiloto pausa las respuestas de IA hasta mañana.",
			Evidence:       []string{"El límite diario de gasto en IA de este negocio se agotó por hoy."},
			ExpectedImpact: "El acceso al copiloto se restablece automáticamente al inicio del próximo día.",
			FollowUps:      []string{"Vuelve mañana o contáctanos si necesitas un límite más alto."},
		}
	}
	return DirectorStructuredResponse{
		Summary:        "You've reached today's AI usage limit for this business.",
		Diagnosis:      "To protect your costs, the copilot pauses AI answers until tomorrow.",
		Evidence:       []string{"This business's daily AI spend ceiling has been reached for today."},
		ExpectedImpact: "Copilot access resets automatically at the start of the next day.",
		FollowUps:      []string{"Check back tomorrow, or contact us if you need a higher limit."},
	}
}

func parseDirectorJSON(raw string) (DirectorStructuredResponse, error) {
	text := strings.TrimSpace(raw)
	var parsed directorModelOutput

	if err := json.Unmarshal([]byte(text), &parsed); err != nil {
		start := strings.Index(text, "{")
		end := strings.LastIndex(text, "}")
		if start == -1 || end <= start {
			return DirectorStructuredResponse{}, err
		}
		if err2 := json.Unmarshal([]byte(text[start:end+1]), &parsed); err2 != nil {
			return DirectorStructuredResponse{}, err2
		}
	}

	return DirectorStructuredResponse(parsed), nil
}

func sanitizeStructuredResponse(response DirectorStructuredResponse) DirectorStructuredResponse {
	response.Summary = scrubDirectorInternalJargon(redactPII(response.Summary))
	response.Diagnosis = scrubDirectorInternalJargon(redactPII(response.Diagnosis))
	response.ExpectedImpact = scrubDirectorInternalJargon(redactPII(response.ExpectedImpact))
	cleaned := make([]string, 0, len(response.Evidence))
	for i := range response.Evidence {
		line := strings.TrimSpace(response.Evidence[i])
		if line == "" || directorEvidenceLooksLikeSchemaDump(line) {
			continue
		}
		cleaned = append(cleaned, scrubDirectorInternalJargon(redactPII(line)))
	}
	response.Evidence = cleaned
	for i := range response.ActionPlan {
		response.ActionPlan[i].Title = scrubDirectorInternalJargon(redactPII(response.ActionPlan[i].Title))
		response.ActionPlan[i].Description = scrubDirectorInternalJargon(redactPII(response.ActionPlan[i].Description))
	}
	for i := range response.FollowUps {
		response.FollowUps[i] = scrubDirectorInternalJargon(redactPII(response.FollowUps[i]))
	}
	return response
}

func redactPII(value string) string {
	return pii.Redact(value)
}

func extractTabFromDeepLink(link string) string {
	const marker = "tab="
	idx := strings.Index(link, marker)
	if idx == -1 {
		return ""
	}
	tab := link[idx+len(marker):]
	if amp := strings.Index(tab, "&"); amp != -1 {
		tab = tab[:amp]
	}
	return strings.TrimSpace(tab)
}

func buildTabDeepLink(businessID string, tab string) string {
	if _, ok := directorAllowedTabs[tab]; !ok {
		tab = "overview"
	}
	return fmt.Sprintf("/business/%s/dashboard?tab=%s", businessID, tab)
}

func toStringValue(value interface{}) string {
	if value == nil {
		return ""
	}

	switch v := value.(type) {
	case string:
		return v
	case []byte:
		return string(v)
	default:
		return fmt.Sprintf("%v", v)
	}
}

func fallbackString(value, fallback string) string {
	trimmed := strings.TrimSpace(value)
	if trimmed == "" {
		return fallback
	}
	return trimmed
}

func defaultFollowUps(locale string) []string {
	if directorUsesSpanishCopy(locale) {
		return []string{
			"¿Qué franja horaria quieres optimizar primero para crecer?",
			"¿Quieres una secuencia de 7 días para aumentar el ticket promedio?",
			"¿Prefieres enfocarte ahora en retención o en adquisición?",
		}
	}

	return []string{
		"Which daypart should we optimize first for growth?",
		"Do you want a 7-day action sequence for improving average ticket?",
		"Should we focus next on retention or new customer conversion?",
	}
}
