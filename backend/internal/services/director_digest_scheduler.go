package services

import (
	"fmt"
	"html"
	htmltemplate "html/template"
	"log"
	"sync"
	"time"

	"github.com/stdevmac/payverge/backend/internal/config"
	"github.com/stdevmac/payverge/backend/internal/database"
	"github.com/stdevmac/payverge/backend/internal/emails"
	"github.com/stdevmac/payverge/backend/internal/logger"
	"gorm.io/gorm"
)

// digestHourDefault is the local hour (0–23) at which the daily digest is sent.
const digestHourDefault = 8

// DigestInsight is a portable summary of a proactive insight, passed from the
// server package (which owns buildProactiveInsights) to the scheduler via a
// callback to avoid an import cycle.
type DigestInsight struct {
	Type   string
	Params map[string]interface{}
}

// InsightsFunc returns the current proactive insights for a business. The
// scheduler calls this when building the digest email body.
type InsightsFunc func(businessID uint) []DigestInsight

// DirectorDigestScheduler sends a daily AI briefing email to every active
// business whose configured timezone has reached the digest hour. Deduplication
// is DB-backed via an atomic claim on businesses.director_digest_last_sent_at,
// so restarts and multi-replica deployments cannot re-send
// the same day's digest.
type DirectorDigestScheduler struct {
	db           *gorm.DB
	emailServer  *emails.EmailServer
	insightsFunc InsightsFunc
	stopChan     chan struct{}
	wg           sync.WaitGroup
	mu           sync.Mutex
	isRunning    bool
}

// NewDirectorDigestScheduler creates a new DirectorDigestScheduler.
// insightsFn may be nil — the digest will still be sent, but without a
// summary of proactive insights.
func NewDirectorDigestScheduler(db *gorm.DB, emailServer *emails.EmailServer, insightsFn InsightsFunc) *DirectorDigestScheduler {
	return &DirectorDigestScheduler{
		db:           db,
		emailServer:  emailServer,
		insightsFunc: insightsFn,
		stopChan:     make(chan struct{}),
	}
}

// Start begins the scheduler loop in a background goroutine.
func (s *DirectorDigestScheduler) Start() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.isRunning {
		return fmt.Errorf("director digest scheduler already running")
	}
	s.isRunning = true
	s.stopChan = make(chan struct{}) // re-create so a Stop→Start cycle is safe
	s.wg.Add(1)
	go s.loop()
	log.Println("Director digest scheduler started")
	return nil
}

// Stop gracefully shuts down the scheduler and waits for in-flight work.
func (s *DirectorDigestScheduler) Stop() {
	s.mu.Lock()
	defer s.mu.Unlock()
	if !s.isRunning {
		return
	}
	close(s.stopChan)
	s.wg.Wait()
	s.isRunning = false
	log.Println("Director digest scheduler stopped")
}

func (s *DirectorDigestScheduler) loop() {
	defer s.wg.Done()
	ticker := time.NewTicker(15 * time.Minute)
	defer ticker.Stop()

	// Run immediately on startup so the first check is not delayed 15 minutes.
	logger.SafeTick("director-digest-scheduler", s.checkDigests)

	for {
		select {
		case <-ticker.C:
			logger.SafeTick("director-digest-scheduler", s.checkDigests)
		case <-s.stopChan:
			return
		}
	}
}

// loadActiveBusinesses returns a projected slice of active
// businesses containing only the columns that the digest path consumes.
// Sensitive columns (onboarding_state, etc.) are never hydrated.
//
// Audited column set:
//   - id                            — primary key, used everywhere
//   - business_id                   — buildDirectorConsoleURL (URL slug)
//   - name                          — sendDigestEmail businessName
//   - owner_name                    — sendDigestEmail ownerName
//   - email                         — recipient + DetermineBusinessOwnerLanguage
//   - timezone                      — maybeDispatch local-hour gate
//   - default_language              — DetermineBusinessOwnerLanguage fallback
//   - user_id                       — DetermineBusinessOwnerLanguage → GetUserByID
//   - owner_address                 — DetermineBusinessOwnerLanguage → GetUserByAddress
//   - director_digest_last_sent_at  — DB-backed dedup claim
func (s *DirectorDigestScheduler) loadActiveBusinesses() ([]database.Business, error) {
	var businesses []database.Business
	q := s.db.
		Select("id", "business_id", "name", "owner_name", "email", "timezone", "default_language", "user_id", "owner_address", "director_digest_last_sent_at")
	err := q.Where("is_active = ?", true).Find(&businesses).Error
	return businesses, err
}

// checkDigests queries all active businesses, determines whether the
// digest should be sent for each one, and dispatches emails.
func (s *DirectorDigestScheduler) checkDigests() {
	businesses, err := s.loadActiveBusinesses()
	if err != nil {
		log.Printf("[DirectorDigestScheduler] failed to query active businesses: %v", err)
		return
	}
	if len(businesses) == 0 {
		return
	}

	now := time.Now().UTC()
	for i := range businesses {
		s.maybeDispatch(&businesses[i], now)
	}
}

// maybeDispatch checks whether a digest is due for a single business and, if
// so, sends the email.
func (s *DirectorDigestScheduler) maybeDispatch(business *database.Business, now time.Time) {
	// Resolve the business timezone.
	tz := business.Timezone
	if tz == "" {
		tz = "UTC"
	}
	loc, err := time.LoadLocation(tz)
	if err != nil {
		loc = time.UTC
	}

	localNow := now.In(loc)

	// Only send when the local hour matches the configured digest hour.
	if localNow.Hour() != digestHourDefault {
		return
	}

	// Claim today's digest atomically before sending. The DB-backed claim
	// survives restarts and multi-replica deployments.
	if !s.claimDigest(business.ID, now) {
		return
	}
	s.sendDigestEmail(business, localNow)
}

func (s *DirectorDigestScheduler) sendDigestEmail(business *database.Business, localNow time.Time) {
	if business.Email == "" {
		log.Printf("[DirectorDigestScheduler] business %d has no email address, skipping digest", business.ID)
		return
	}

	ownerName := business.OwnerName
	if ownerName == "" {
		ownerName = business.Name
	}
	if ownerName == "" {
		ownerName = "Business Owner"
	}

	businessName := business.Name
	if businessName == "" {
		businessName = ownerName
	}

	language := DetermineBusinessOwnerLanguage(business)
	digestDate := formatLocalizedDate(localNow, language)
	directorConsoleURL := buildDirectorConsoleURL(business)

	insightsSummary := s.buildInsightsSummaryHTML(business.ID, language)

	if err := s.emailServer.SendDirectorDigestEmail(
		[]string{business.Email},
		ownerName,
		businessName,
		digestDate,
		directorConsoleURL,
		insightsSummary,
		language,
	); err != nil {
		log.Printf("[DirectorDigestScheduler] failed to send digest email to business %d: %v", business.ID, err)
		// Release the DB claim so the scheduler retries on the next tick.
		s.db.Model(&database.Business{}).
			Where("id = ?", business.ID).
			Update("director_digest_last_sent_at", nil)
		return
	}

	log.Printf("[DirectorDigestScheduler] sent daily digest email for business %d (%s)", business.ID, business.Name)
}

// claimDigest atomically reserves today's digest for a business: it stamps
// director_digest_last_sent_at=now WHERE the existing stamp is NULL or from a
// previous UTC day. RowsAffected==1 means this caller won. Replaces the
// per-process lastSent map so a restart cannot re-send and replicas cannot
// double-send.
func (s *DirectorDigestScheduler) claimDigest(businessID uint, now time.Time) bool {
	res := s.db.Model(&database.Business{}).
		Where("id = ? AND (director_digest_last_sent_at IS NULL OR director_digest_last_sent_at < ?)",
			businessID, now.Truncate(24*time.Hour)).
		Update("director_digest_last_sent_at", now)
	if res.Error != nil {
		log.Printf("[DirectorDigestScheduler] claim failed for business %d: %v", businessID, res.Error)
		return false
	}
	return res.RowsAffected == 1
}

func (s *DirectorDigestScheduler) buildInsightsSummaryHTML(businessID uint, language string) htmltemplate.HTML {
	if s.insightsFunc == nil {
		return htmltemplate.HTML("")
	}
	insights := s.insightsFunc(businessID)
	if len(insights) == 0 {
		allGood := "&#10003; Everything looks good today — no issues found."
		if isSpanishLanguage(language) {
			allGood = "&#10003; Todo se ve bien hoy — no se encontraron problemas."
		}
		// allGood contains only static, trusted text with a safe HTML entity.
		return htmltemplate.HTML(fmt.Sprintf(`<p style="margin:0 0 18px 0;color:#059669;font-weight:600;">%s</p>`, allGood))
	}

	buf := `<ul style="margin:0 0 18px 0;padding-left:20px;">`
	for _, ins := range insights {
		buf += `<li style="margin:0 0 8px 0;">` + formatInsightText(ins, language) + `</li>`
	}
	buf += `</ul>`
	// buf is constructed from trusted static markup + HTML-escaped user data
	// (item names are escaped in formatInsightText). Marking as HTML.HTML
	// prevents html/template from double-escaping the static tags.
	return htmltemplate.HTML(buf)
}

func formatInsightText(ins DigestInsight, language string) string {
	// waste_high carries amount (dollars) + ingredient rather than count/item_names,
	// so it gets its own localized line instead of the count-based formatters.
	if ins.Type == "waste_high" {
		amount, _ := ins.Params["amount"].(float64)
		ingredient, _ := ins.Params["ingredient"].(string)
		ingredient = html.EscapeString(ingredient)
		if isSpanishLanguage(language) {
			return fmt.Sprintf("<strong>$%.0f de pérdida de inventario</strong> esta semana — %s es tu mayor pérdida; revisa merma, porciones y conteos.", amount, ingredient)
		}
		return fmt.Sprintf("<strong>$%.0f of inventory loss</strong> this week — %s is your biggest loss; review spoilage, portioning, and counts.", amount, ingredient)
	}

	// labor_high carries pct (fraction) + amount (dollars), no count/item_names.
	if ins.Type == "labor_high" {
		pct, _ := ins.Params["pct"].(float64)
		amount, _ := ins.Params["amount"].(float64)
		if isSpanishLanguage(language) {
			return fmt.Sprintf("<strong>El costo laboral es el %.0f%% de las ventas</strong> esta semana ($%.0f) — por encima del objetivo; revisa turnos y horas.", pct*100, amount)
		}
		return fmt.Sprintf("<strong>Labor is %.0f%% of sales</strong> this week ($%.0f) — above target; review shifts and hours.", pct*100, amount)
	}

	count, _ := ins.Params["count"].(int)
	if count == 0 {
		if countF, ok := ins.Params["count"].(float64); ok {
			count = int(countF)
		}
	}
	names, _ := ins.Params["item_names"].([]string)
	nameStr := ""
	if len(names) > 0 {
		// Escape each user-controlled item name individually before joining so
		// that a crafted name (e.g. "<script>…</script>") cannot be injected as
		// markup into the surrounding HTML. The enclosing <strong>/<li> tags are
		// trusted static code written by this function, not user data.
		escaped := make([]string, len(names))
		for i, n := range names {
			escaped[i] = html.EscapeString(n)
		}
		nameStr = ": " + joinStringSlice(escaped, ", ")
	}

	stock := readInventoryStockSplit(ins.Params, count)

	if isSpanishLanguage(language) {
		return formatInsightTextES(ins.Type, count, nameStr, stock)
	}
	return formatInsightTextEN(ins.Type, count, nameStr, stock)
}

// inventoryStockSplit carries the oversold/at-zero breakdown of an
// inventory_out_of_stock insight (R2-7). has_oversold alone is a boolean OR
// across the set, so a mixed set read as if every item were oversold.
type inventoryStockSplit struct {
	OversoldCount int
	ZeroCount     int
	// Pre-escaped, ", "-joined name lists. Empty when the producer did not
	// carry the split (older payload) — the copy then stays single-group.
	OversoldNames string
	ZeroNames     string
}

// Mixed reports a set that is part oversold and part exactly zero, with names
// for both groups. Without both name lists there is nothing honest to render,
// so the single-group copy stands.
func (s inventoryStockSplit) Mixed() bool {
	return s.OversoldCount > 0 && s.ZeroCount > 0 &&
		s.OversoldNames != "" && s.ZeroNames != ""
}

func (s inventoryStockSplit) HasOversold() bool { return s.OversoldCount > 0 }

func readInventoryStockSplit(params map[string]interface{}, count int) inventoryStockSplit {
	split := inventoryStockSplit{}

	if v, ok := params["oversold_count"].(int); ok {
		split.OversoldCount = v
	} else if f, ok := params["oversold_count"].(float64); ok {
		split.OversoldCount = int(f)
	} else if b, ok := params["has_oversold"].(bool); ok && b {
		// Legacy payload: the whole set is treated as oversold, exactly as
		// before this split existed.
		split.OversoldCount = count
	}
	if split.OversoldCount > count {
		split.OversoldCount = count
	}
	split.ZeroCount = count - split.OversoldCount
	if split.ZeroCount < 0 {
		split.ZeroCount = 0
	}

	split.OversoldNames = escapeAndJoinNames(params["oversold_names"])
	split.ZeroNames = escapeAndJoinNames(params["zero_names"])
	return split
}

// escapeAndJoinNames HTML-escapes each user-controlled name before joining, so
// a crafted item name cannot inject markup into the digest body.
func escapeAndJoinNames(value interface{}) string {
	names, _ := value.([]string)
	if len(names) == 0 {
		return ""
	}
	escaped := make([]string, len(names))
	for i, n := range names {
		escaped[i] = html.EscapeString(n)
	}
	return joinStringSlice(escaped, ", ")
}

func formatInsightTextEN(insType string, count int, nameStr string, stock inventoryStockSplit) string {
	hasOversold := stock.HasOversold()
	switch insType {
	case "inventory_out_of_stock":
		// R2-7: a set that is part oversold and part exactly zero gets both
		// groups named with their own counts — "N items are oversold: <every
		// name>" was false for the at-zero half of the list.
		if stock.Mixed() {
			return fmt.Sprintf(
				"<strong>%d inventory items need attention</strong> — oversold (%d): %s; at zero (%d): %s. Count and correct stock before the next service.",
				count, stock.OversoldCount, stock.OversoldNames, stock.ZeroCount, stock.ZeroNames,
			)
		}
		// L1-22: negative stock is oversold, not "at zero".
		if hasOversold {
			label := "inventory items are oversold"
			if count == 1 {
				label = "inventory item is oversold"
			}
			return fmt.Sprintf("<strong>%d %s</strong>%s — count and correct stock before the next service.", count, label, nameStr)
		}
		label := "inventory items"
		if count == 1 {
			label = "inventory item"
		}
		return fmt.Sprintf("<strong>%d %s at zero</strong>%s — review usage and replenish stock.", count, label, nameStr)
	case "inventory_low_stock":
		return fmt.Sprintf("<strong>%d items running low</strong>%s — reorder soon.", count, nameStr)
	case "stale_open_bills":
		return fmt.Sprintf("<strong>%d open bills</strong> have been idle for over 2 hours — check if guests left.", count)
	case "ai_conversations_pending":
		return fmt.Sprintf("<strong>%d AI conversations</strong> are paused and waiting for staff follow-up.", count)
	case "food_cost_high":
		return fmt.Sprintf("<strong>%d menu items over 40%% food cost</strong>%s — review pricing or portioning.", count, nameStr)
	default:
		return fmt.Sprintf("Action needed: %s (%d items)", html.EscapeString(insType), count)
	}
}

func formatInsightTextES(insType string, count int, nameStr string, stock inventoryStockSplit) string {
	hasOversold := stock.HasOversold()
	switch insType {
	case "inventory_out_of_stock":
		// R2-7: conjunto mixto (parte sobrevendido, parte en cero) — cada grupo
		// va con su propio conteo y sus propios nombres.
		if stock.Mixed() {
			return fmt.Sprintf(
				"<strong>%d insumos de inventario necesitan atención</strong> — sobrevendidos (%d): %s; en cero (%d): %s. Cuenta y corrige el stock antes del próximo servicio.",
				count, stock.OversoldCount, stock.OversoldNames, stock.ZeroCount, stock.ZeroNames,
			)
		}
		// L1-22: stock negativo es "Sobrevendido", no "en cero".
		if hasOversold {
			label := "insumos de inventario están sobrevendidos"
			if count == 1 {
				label = "insumo de inventario está sobrevendido"
			}
			// R2-8: neutral tuteo — this body is shared by es and es-AR.
			return fmt.Sprintf("<strong>%d %s</strong>%s — cuenta y corrige el stock antes del próximo servicio.", count, label, nameStr)
		}
		label := "insumos de inventario"
		if count == 1 {
			label = "insumo de inventario"
		}
		return fmt.Sprintf("<strong>%d %s en cero</strong>%s — revisa su uso y repón existencias.", count, label, nameStr)
	case "inventory_low_stock":
		return fmt.Sprintf("<strong>%d productos con stock bajo</strong>%s — haz un pedido pronto.", count, nameStr)
	case "stale_open_bills":
		return fmt.Sprintf("<strong>%d cuentas abiertas</strong> llevan más de 2 horas inactivas — verifica si los clientes se fueron.", count)
	case "ai_conversations_pending":
		return fmt.Sprintf("<strong>%d conversaciones IA</strong> están pausadas esperando seguimiento del staff.", count)
	case "food_cost_high":
		return fmt.Sprintf("<strong>%d platos con costo de insumos sobre 40%%</strong>%s — revisa precios o porciones.", count, nameStr)
	default:
		return fmt.Sprintf("Acción necesaria: %s (%d elementos)", html.EscapeString(insType), count)
	}
}

func isSpanishLanguage(lang string) bool {
	return lang == "es" || lang == "es_ar" || lang == "es-AR"
}

func formatLocalizedDate(t time.Time, language string) string {
	if isSpanishLanguage(language) {
		months := []string{"", "enero", "febrero", "marzo", "abril", "mayo", "junio",
			"julio", "agosto", "septiembre", "octubre", "noviembre", "diciembre"}
		return fmt.Sprintf("%d de %s de %d", t.Day(), months[t.Month()], t.Year())
	}
	return t.Format("January 2, 2006")
}

func joinStringSlice(s []string, sep string) string {
	result := ""
	for i, v := range s {
		if i > 0 {
			result += sep
		}
		result += v
	}
	return result
}

// buildDirectorConsoleURL constructs the deep-link URL to the Director Console
// tab of the business dashboard.
func buildDirectorConsoleURL(business *database.Business) string {
	baseURL := config.FrontendBaseURL()
	if business == nil {
		return fmt.Sprintf("%s/business", baseURL)
	}
	businessSlug := business.BusinessId
	if businessSlug == "" {
		businessSlug = fmt.Sprintf("%d", business.ID)
	}
	return fmt.Sprintf("%s/business/%s/dashboard?tab=director-console", baseURL, businessSlug)
}
