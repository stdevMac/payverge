package services

import (
	"context"
	"fmt"
	"regexp"
	"sort"
	"strconv"
	"strings"

	"github.com/stdevmac/payverge/backend/internal/database"
	"github.com/stdevmac/payverge/backend/internal/services/director_tools"
)

// menuPerfDish is an operator-facing row assembled from menu cards, tool
// results, and sibling Director threads. It is never serialized as wire JSON.
type menuPerfDish struct {
	Name          string
	Units         int
	Revenue       float64
	Price         float64
	PlateCost     float64
	MarginDollars float64
	FoodCostPct   float64
	HasSales      bool
	HasCost       bool
	FromSibling   bool
}

type menuPerformanceSnapshot struct {
	Dishes       []menuPerfDish
	SiblingNotes []string
}

func (s menuPerformanceSnapshot) usable() bool {
	if len(s.SiblingNotes) > 0 {
		return true
	}
	for _, d := range s.Dishes {
		if strings.TrimSpace(d.Name) != "" && (d.HasCost || d.HasSales || d.FromSibling) {
			return true
		}
	}
	return len(s.Dishes) > 0
}

func directorWantsMenuPerformance(question string) bool {
	q := strings.ToLower(question)
	for _, kw := range []string{
		// Stems, not noun forms: operators paraphrase the advertised copy with
		// the adjective ("best-selling dishes"), and the noun-only list used to
		// drop those asks before grounding could run.
		"best sell", "best-sell", "bestsell",
		"más vendid", "mas vendid", "más vendido", "mas vendido",
		"thinnest margin", "thin margin", "thin nest",
		"margin analysis", "food cost", "food-cost",
		"margen", "márgen",
		"slow mover", "slow-mover", "slow moving", "underperform",
		"moviendo poco", "se estan moviendo", "se están moviendo",
		"poco esta", "poco esta semana", "poco esta noche",
		"top sell", "top-sell", "top items",
	} {
		if strings.Contains(q, kw) {
			return true
		}
	}
	return false
}

func directorMenuPerformanceIntent(question string) string {
	if directorWantsMarginPreview(question) {
		return "margin_preview"
	}
	q := strings.ToLower(question)
	for _, kw := range []string{
		"margin", "margen", "márgen", "food cost", "food-cost", "thinnest", "thin margin",
	} {
		if strings.Contains(q, kw) {
			return "margin"
		}
	}
	for _, kw := range []string{
		"slow", "poco", "underperform", "moviendo",
	} {
		if strings.Contains(q, kw) {
			return "slow"
		}
	}
	return "best"
}

func directorThreadLooksLikeMenuPerformance(title string) bool {
	t := strings.ToLower(title)
	for _, kw := range []string{
		"best-sell", "best sell", "bestseller",
		"thin", "margin", "margen",
		"más vendid", "mas vendid",
		"moviendo poco", "underperform",
		"food-cost", "food cost", "foodcost",
		"plowhorse", "thinnes",
	} {
		if strings.Contains(t, kw) {
			return true
		}
	}
	return false
}

func directorLooksLikeInsufficientData(resp DirectorStructuredResponse) bool {
	blob := strings.ToLower(directorProseBlob(resp))
	for _, p := range []string{
		"no hay datos suficientes",
		"not enough data",
		"no tengo cifras",
		"don't have figures",
		"do not have figures",
		"todavía no hay actividad suficiente",
		"isn't enough activity yet",
		"isnt enough activity yet",
		"0 units",
		"0 unidades",
		"qty_sold is 0",
		"quantity sold is 0",
		"units sold is 0",
		"units sold of 0",
		"cannot identify",
		"can't identify",
		"need the top-sellers",
		"need the food-cost",
		"food cost percentage is not available",
		"food cost % is not available",
		"food-cost percentage is not available",
		"cannot show current margin",
		"cannot show a new-margin",
		"cannot show new margin",
		"plate cost information is not available",
		"plate cost is not available",
		"what price to consider",
		"what price should",
		"what price would you",
		"update plate cost",
	} {
		if strings.Contains(blob, p) {
			return true
		}
	}
	return false
}

// directorSmallPriceBumpDollars is the default "small price adjustment" for
// advertised new-margin walkthroughs that stay preview-only.
const directorSmallPriceBumpDollars = 1.50

func directorWantsMarginPreview(question string) bool {
	q := strings.ToLower(question)
	for _, kw := range []string{
		"new margin", "new-margin", "nuevo margen", "margen nuevo",
		"new food cost", "new food-cost", "nuevo costo de insumos",
		"price adjustment", "ajuste de precio", "ajuste de precios",
		"before anything goes live", "before it goes live",
		"before anything is live", "before it is live",
		"antes de que esté en vivo", "antes de que este en vivo",
		"antes de que vaya en vivo", "antes de que salga en vivo",
		"antes de que entre en vivo", "antes de que esté live", "antes de que este live",
		"show me the new margin",
	} {
		if strings.Contains(q, kw) {
			return true
		}
	}
	return false
}

func directorHasNewMarginDollars(resp DirectorStructuredResponse, question string) bool {
	if directorLooksLikeInsufficientData(resp) {
		return false
	}
	blob := strings.ToLower(directorProseBlob(resp))
	hasCurrent := strings.Contains(blob, "current margin $") || strings.Contains(blob, "margen actual $")
	hasNew := strings.Contains(blob, "new margin $") || strings.Contains(blob, "margen nuevo $")
	hasFood := (strings.Contains(blob, "food cost") || strings.Contains(blob, "costo de insumos")) && strings.Contains(blob, "%")
	if hasCurrent && hasNew && hasFood {
		return true
	}
	// An owner-named +$2 preview may say "New margin $12.00" without the
	// "current margin $" phrase. Do not replace that with the default +$1.50.
	if _, named := directorNamedPriceBump(question); named && hasNew {
		return true
	}
	return false
}

var (
	directorPlusBumpRe      = regexp.MustCompile(`\+\s*\$?\s*(\d+(?:\.\d+)?)`)
	directorByNumDollarsRe  = regexp.MustCompile(`\bby\s+(\d+(?:\.\d+)?)\s*dollars?\b`)
	directorByWordDollarsRe = regexp.MustCompile(`\bby\s+(one|two|three|four|five)\s+dollars?\b`)
	directorByAmountRe      = regexp.MustCompile(`\bby\s+\$?\s*(\d+(?:\.\d+)?)\b`)
	directorWordDollarsRe   = regexp.MustCompile(`\b(one|two|three|four|five)\s+dollars?\b`)
	directorEsNumDollarsRe  = regexp.MustCompile(`\b(\d+(?:\.\d+)?)\s*d[oó]lares?\b`)
	directorEsWordDollarsRe = regexp.MustCompile(`\b(un|uno|dos|tres|cuatro|cinco)\s+d[oó]lares?\b`)
	directorBumpWordValues  = map[string]float64{
		"one": 1, "two": 2, "three": 3, "four": 4, "five": 5,
		"un": 1, "uno": 1, "dos": 2, "tres": 3, "cuatro": 4, "cinco": 5,
	}
)

func directorLooksLikeNamedRaise(q string) bool {
	for _, kw := range []string{
		"raise", "increase", "bump", "lift",
		"ajuste", "aumento", "aumenta", "aumentá", "aumentalo",
		"subi", "subí", "subilo",
		"by ",
	} {
		if strings.Contains(q, kw) {
			return true
		}
	}
	return false
}

func directorNamedPriceBump(question string) (float64, bool) {
	q := strings.ToLower(strings.TrimSpace(question))
	if q == "" {
		return 0, false
	}
	if m := directorPlusBumpRe.FindStringSubmatch(q); len(m) == 2 {
		if n, err := strconv.ParseFloat(m[1], 64); err == nil && n > 0 {
			return n, true
		}
	}
	if m := directorByNumDollarsRe.FindStringSubmatch(q); len(m) == 2 {
		if n, err := strconv.ParseFloat(m[1], 64); err == nil && n > 0 {
			return n, true
		}
	}
	if m := directorByWordDollarsRe.FindStringSubmatch(q); len(m) == 2 {
		if n, ok := directorBumpWordValues[m[1]]; ok {
			return n, true
		}
	}
	if m := directorByAmountRe.FindStringSubmatch(q); len(m) == 2 {
		if n, err := strconv.ParseFloat(m[1], 64); err == nil && n > 0 {
			return n, true
		}
	}
	if directorLooksLikeNamedRaise(q) {
		if m := directorWordDollarsRe.FindStringSubmatch(q); len(m) == 2 {
			if n, ok := directorBumpWordValues[m[1]]; ok {
				return n, true
			}
		}
		if m := directorEsNumDollarsRe.FindStringSubmatch(q); len(m) == 2 {
			if n, err := strconv.ParseFloat(m[1], 64); err == nil && n > 0 {
				return n, true
			}
		}
		if m := directorEsWordDollarsRe.FindStringSubmatch(q); len(m) == 2 {
			if n, ok := directorBumpWordValues[m[1]]; ok {
				return n, true
			}
		}
	}
	return 0, false
}

func directorPreviewBump(question string) float64 {
	if n, ok := directorNamedPriceBump(question); ok {
		return n
	}
	return directorSmallPriceBumpDollars
}

func directorNamesKnownDish(resp DirectorStructuredResponse, snap menuPerformanceSnapshot) bool {
	blob := strings.ToLower(directorProseBlob(resp))
	for _, d := range snap.Dishes {
		name := strings.ToLower(strings.TrimSpace(d.Name))
		if name != "" && strings.Contains(blob, name) {
			return true
		}
	}
	return false
}

func (s *DirectorConsoleService) collectMenuPerformance(ctx context.Context, businessID uint, locale string, currentThreadID uint) menuPerformanceSnapshot {
	snap := menuPerformanceSnapshot{}
	byName := map[string]int{}

	upsert := func(d menuPerfDish) {
		name := strings.TrimSpace(d.Name)
		if name == "" {
			return
		}
		key := strings.ToLower(name)
		if idx, ok := byName[key]; ok {
			cur := snap.Dishes[idx]
			if d.HasSales {
				cur.HasSales = true
				cur.Units = d.Units
				cur.Revenue = d.Revenue
			}
			if d.HasCost {
				cur.HasCost = true
				cur.PlateCost = d.PlateCost
				cur.MarginDollars = d.MarginDollars
				cur.FoodCostPct = d.FoodCostPct
				if d.Price > 0 {
					cur.Price = d.Price
				}
			}
			if d.FromSibling {
				cur.FromSibling = true
			}
			if d.Price > cur.Price {
				cur.Price = d.Price
			}
			snap.Dishes[idx] = cur
			return
		}
		d.Name = name
		byName[key] = len(snap.Dishes)
		snap.Dishes = append(snap.Dishes, d)
	}

	for _, d := range collectPlateCards(businessID) {
		upsert(d)
	}

	if s != nil {
		env := director_tools.ToolEnv{
			BusinessID: businessID,
			Locale:     locale,
			DB:         s.db,
			Analytics:  s.analytics,
			Location:   nil,
		}
		if biz, err := s.db.GetBusinessByID(businessID); err == nil {
			env.Location = database.ResolveBusinessLocation(biz)
		}
		mergeToolItems := func(items []map[string]any, sold bool, costed bool) {
			for _, row := range items {
				d := menuPerfDish{}
				if name, _ := row["name"].(string); name != "" {
					d.Name = name
				}
				if d.Name == "" {
					continue
				}
				if sold {
					d.HasSales = true
					d.Units = toolInt(row["quantity"])
					if d.Units == 0 {
						d.Units = toolInt(row["qty_sold"])
					}
					d.Revenue = toolFloat(row["revenue"])
				}
				if costed {
					price := toolFloat(row["avg_price"])
					unitCost := toolFloat(row["unit_cost"])
					if price > 0 && unitCost > 0 {
						d.HasCost = true
						d.Price = price
						d.PlateCost = unitCost
						d.MarginDollars = toolFloat(row["margin_per_unit"])
						if d.MarginDollars == 0 {
							d.MarginDollars = price - unitCost
						}
						d.FoodCostPct = toolFloat(row["food_cost_pct"])
						if d.FoodCostPct == 0 && price > 0 {
							d.FoodCostPct = unitCost / price
						}
					}
					if q := toolInt(row["qty_sold"]); q > 0 {
						d.HasSales = true
						d.Units = q
					}
				}
				upsert(d)
			}
		}

		top := &director_tools.MenuTopItemsTool{}
		if res, err := top.Run(ctx, map[string]any{"period": "week", "limit": float64(10)}, env); err == nil {
			mergeToolItems(toolItemMaps(res.Data["items"]), true, false)
		}
		if !snap.hasSales() {
			if res, err := top.Run(ctx, map[string]any{"period": "month", "limit": float64(10)}, env); err == nil {
				mergeToolItems(toolItemMaps(res.Data["items"]), true, false)
			}
		}
		food := &director_tools.FoodCostAnalysisTool{}
		if res, err := food.Run(ctx, map[string]any{"period": "week", "limit": float64(10)}, env); err == nil {
			mergeToolItems(toolItemMaps(res.Data["items"]), false, true)
		}

		threads, err := database.ListDirectorConsoleThreads(businessID, 50)
		if err == nil {
			for _, th := range threads {
				if th.ID == currentThreadID {
					continue
				}
				title := strings.TrimSpace(th.Title)
				if title == "" {
					continue
				}
				if directorThreadLooksLikeMenuPerformance(title) {
					if len(snap.SiblingNotes) < 3 {
						snap.SiblingNotes = append(snap.SiblingNotes, title)
					}
				}
				lower := strings.ToLower(title)
				for i := range snap.Dishes {
					if strings.Contains(lower, strings.ToLower(snap.Dishes[i].Name)) {
						snap.Dishes[i].FromSibling = true
					}
				}
			}
		}
	}

	return snap
}

func (s menuPerformanceSnapshot) hasSales() bool {
	for _, d := range s.Dishes {
		if d.HasSales && d.Units > 0 {
			return true
		}
	}
	return false
}

func (s menuPerformanceSnapshot) operatorProse(locale string) string {
	var b strings.Builder
	if directorUsesSpanishCopy(locale) {
		b.WriteString("Carta (precio vs costo de plato): ")
	} else {
		b.WriteString("Menu card (price vs plate cost): ")
	}
	parts := make([]string, 0, len(s.Dishes))
	for _, d := range s.Dishes {
		if !d.HasCost {
			if d.Price > 0 {
				parts = append(parts, fmt.Sprintf("%s $%.2f", d.Name, d.Price))
			} else {
				parts = append(parts, d.Name)
			}
			continue
		}
		parts = append(parts, fmt.Sprintf("%s $%.2f price, $%.2f plate cost, $%.2f margin",
			d.Name, d.Price, d.PlateCost, d.MarginDollars))
	}
	b.WriteString(strings.Join(parts, "; "))
	if s.hasSales() {
		b.WriteString(" Sold mix: ")
		sold := make([]string, 0)
		for _, d := range s.Dishes {
			if d.HasSales && d.Units > 0 {
				sold = append(sold, fmt.Sprintf("%s %d sold, $%.2f", d.Name, d.Units, d.Revenue))
			}
		}
		b.WriteString(strings.Join(sold, "; "))
	}
	if len(s.SiblingNotes) > 0 {
		b.WriteString(" Earlier conversations: ")
		b.WriteString(strings.Join(s.SiblingNotes, " | "))
	}
	return b.String()
}

func collectPlateCards(businessID uint) []menuPerfDish {
	_, cats, err := database.GetMenuByBusinessID(businessID)
	if err != nil {
		return nil
	}
	out := make([]menuPerfDish, 0)
	for _, cat := range cats {
		for _, item := range cat.Items {
			name := strings.TrimSpace(item.Name)
			if name == "" || item.Price <= 0 {
				continue
			}
			d := menuPerfDish{Name: name, Price: item.Price}
			if item.Cogs > 0 {
				d.HasCost = true
				d.PlateCost = item.Cogs
				d.MarginDollars = item.Price - item.Cogs
				d.FoodCostPct = item.Cogs / item.Price
			}
			out = append(out, d)
		}
	}
	return out
}

func toolItemMaps(raw any) []map[string]any {
	switch v := raw.(type) {
	case []map[string]any:
		return v
	case []any:
		out := make([]map[string]any, 0, len(v))
		for _, row := range v {
			if m, ok := row.(map[string]any); ok {
				out = append(out, m)
			}
		}
		return out
	default:
		return nil
	}
}

func toolInt(v any) int {
	switch n := v.(type) {
	case int:
		return n
	case int64:
		return int(n)
	case float64:
		return int(n)
	case float32:
		return int(n)
	default:
		return 0
	}
}

func toolFloat(v any) float64 {
	switch n := v.(type) {
	case float64:
		return n
	case float32:
		return float64(n)
	case int:
		return float64(n)
	case int64:
		return float64(n)
	default:
		return 0
	}
}

func (s *DirectorConsoleService) groundMenuPerformanceIfNeeded(question, locale string, businessID uint, resp DirectorStructuredResponse, snap menuPerformanceSnapshot) DirectorStructuredResponse {
	if (!directorWantsMenuPerformance(question) && !directorWantsMarginPreview(question)) || !snap.usable() {
		return resp
	}
	namesDish := directorNamesKnownDish(resp, snap)
	noData := directorLooksLikeInsufficientData(resp)
	leaks := directorContainsBannedWire(directorProseBlob(resp))
	missingPreview := directorWantsMarginPreview(question) && !directorHasNewMarginDollars(resp, question)
	if namesDish && !noData && !leaks && !missingPreview {
		return resp
	}
	return s.buildMenuPerformanceResponse(businessID, locale, question, snap)
}

func (s *DirectorConsoleService) buildMenuPerformanceResponse(businessID uint, locale string, question string, snap menuPerformanceSnapshot) DirectorStructuredResponse {
	intent := directorMenuPerformanceIntent(question)
	link := buildTabDeepLink(fmt.Sprintf("%d", businessID), "menu")

	costed := make([]menuPerfDish, 0, len(snap.Dishes))
	sold := make([]menuPerfDish, 0, len(snap.Dishes))
	for _, d := range snap.Dishes {
		if d.HasCost {
			costed = append(costed, d)
		}
		if d.HasSales && d.Units > 0 {
			sold = append(sold, d)
		}
	}
	sort.SliceStable(costed, func(i, j int) bool {
		if costed[i].FoodCostPct != costed[j].FoodCostPct {
			return costed[i].FoodCostPct > costed[j].FoodCostPct
		}
		return costed[i].MarginDollars < costed[j].MarginDollars
	})
	sort.SliceStable(sold, func(i, j int) bool {
		if sold[i].Units != sold[j].Units {
			return sold[i].Units > sold[j].Units
		}
		return sold[i].Revenue > sold[j].Revenue
	})

	evidence := make([]string, 0, 6)
	for _, note := range snap.SiblingNotes {
		if directorUsesSpanishCopy(locale) {
			evidence = append(evidence, "Conversación anterior: "+note)
		} else {
			evidence = append(evidence, "Earlier conversation: "+note)
		}
	}
	previewDishes := selectMenuPerfPreviewDishes(snap)
	previewBump := directorPreviewBump(question)
	previewSummary, previewEvidence := marginPreviewLines(previewDishes, previewBump, directorUsesSpanishCopy(locale))
	if intent == "margin_preview" && len(previewEvidence) > 0 {
		evidence = append(evidence, previewEvidence...)
	} else {
		for _, d := range costed {
			if directorUsesSpanishCopy(locale) {
				evidence = append(evidence, fmt.Sprintf("%s: precio $%.2f, costo de plato $%.2f, margen $%.2f.",
					d.Name, d.Price, d.PlateCost, d.MarginDollars))
			} else {
				evidence = append(evidence, fmt.Sprintf("%s: $%.2f price, $%.2f plate cost, $%.2f margin.",
					d.Name, d.Price, d.PlateCost, d.MarginDollars))
			}
		}
	}
	for _, d := range sold {
		if directorUsesSpanishCopy(locale) {
			evidence = append(evidence, fmt.Sprintf("%s: %d vendidos, $%.2f.", d.Name, d.Units, d.Revenue))
		} else {
			evidence = append(evidence, fmt.Sprintf("%s: %d sold, $%.2f.", d.Name, d.Units, d.Revenue))
		}
	}
	if len(evidence) == 0 {
		for _, d := range snap.Dishes {
			evidence = append(evidence, d.Name)
		}
	}

	spanish := directorUsesSpanishCopy(locale)
	ar := resolvePromptLocale(locale).PromptFamily == "es_ar"

	var summary, diagnosis, impact, actionTitle, actionDesc string
	follow := defaultFollowUps(locale)

	thinName := ""
	if len(costed) > 0 {
		thinName = costed[0].Name
	}
	bestName := ""
	if len(sold) > 0 {
		bestName = sold[0].Name
	} else {
		for _, d := range snap.Dishes {
			if d.FromSibling {
				bestName = d.Name
				break
			}
		}
		if bestName == "" && len(snap.Dishes) > 0 {
			bestName = snap.Dishes[0].Name
		}
	}

	if spanish {
		switch intent {
		case "margin_preview":
			if previewSummary != "" {
				summary = previewSummary
			} else {
				summary = "Puedo comparar márgenes con el precio y el costo de plato de la carta, sin inventar unidades vendidas."
			}
			diagnosis = "Solo vista previa — no se encoló ni se guardó nada. El costo de plato ya está en la carta, así que el porcentaje de insumos y el margen nuevo se pueden mostrar sin esperar la mezcla de esta semana."
			impact = "Esto es solo vista previa: no hay un cambio de precio encolado, no se guardó nada y el menú no cambió."
		case "margin":
			if thinName != "" && len(costed) > 0 {
				d := costed[0]
				summary = fmt.Sprintf("El margen más delgado en la carta es %s: $%.2f de margen sobre un precio de $%.2f (costo de plato $%.2f).",
					d.Name, d.MarginDollars, d.Price, d.PlateCost)
			} else {
				summary = "Puedo comparar márgenes con el precio y el costo de plato de la carta, sin inventar unidades vendidas."
			}
			diagnosis = "No uso una mezcla semanal vacía para decir que no hay márgenes: el costo de plato ya está en el menú."
		case "slow":
			summary = "No voy a tratar una semana sin mezcla de ventas como que la carta está parada."
			if len(costed) > 0 {
				summary += " " + formatLiveCardClause(costed, true)
			}
			diagnosis = "Si la mezcla de esta semana aún no está, eso no significa que esos platos no se vendan. La carta sigue en vivo."
		default:
			if bestName != "" {
				summary = fmt.Sprintf("%s es el plato a mirar: ya apareció en un análisis anterior de más vendidos y está en la carta.", bestName)
			} else {
				summary = "Hay platos en la carta para analizar; no voy a decir que no hay más vendidos."
			}
			diagnosis = "Una conversación previa o la carta ya nombran platos. No trato una semana en $0 como 'no hay datos'."
		}
		if impact == "" {
			impact = "Con precio y costo de plato se puede decidir qué proteger, subir de precio o empujar hoy."
		}
		if ar {
			actionTitle = "Revisá el menú"
			actionDesc = "Confirmá precios y costos de plato en el tab de menú."
		} else {
			actionTitle = "Revisa el menú"
			actionDesc = "Confirma precios y costos de plato en el tab de menú."
		}
	} else {
		switch intent {
		case "margin_preview":
			if previewSummary != "" {
				summary = previewSummary
			} else {
				summary = "I can compare margins from menu price versus plate cost without inventing units sold."
			}
			diagnosis = "Preview only — nothing is queued or staged. Plate cost is already on the menu card, so food cost share and a new-margin preview are available without waiting on this week's mix."
			impact = "This is preview-only: no price change is staged, nothing is queued, and the menu is unchanged."
		case "margin":
			if thinName != "" && len(costed) > 0 {
				d := costed[0]
				summary = fmt.Sprintf("The thinnest menu-card margin is %s: $%.2f margin on a $%.2f price ($%.2f plate cost).",
					d.Name, d.MarginDollars, d.Price, d.PlateCost)
			} else {
				summary = "I can compare margins from menu price versus plate cost without inventing units sold."
			}
			diagnosis = "An empty weekly mix is not 'no margins': plate cost is already on the menu card."
		case "slow":
			summary = "I will not treat a missing weekly mix as proof live dishes are not selling."
			if len(costed) > 0 {
				summary += " " + formatLiveCardClause(costed, false)
			}
			diagnosis = "A missing weekly mix is not proof those dishes are not selling. The menu is live."
		default:
			if bestName != "" {
				summary = fmt.Sprintf("%s is the dish to watch: earlier analysis already treated it as a best seller, and it is live on the menu.", bestName)
			} else {
				summary = "The menu has dishes I can analyze; I will not say there are no best sellers."
			}
			diagnosis = "A prior conversation or the menu card already names dishes. I am not treating a $0 week as 'no data'."
		}
		if impact == "" {
			impact = "Price and plate cost are enough to decide what to protect, reprice, or push tonight."
		}
		actionTitle = "Review the menu"
		actionDesc = "Confirm prices and plate costs on the menu tab."
	}

	return DirectorStructuredResponse{
		Summary:   summary,
		Diagnosis: diagnosis,
		Evidence:  evidence,
		ActionPlan: []DirectorAction{{
			Title:       actionTitle,
			Description: actionDesc,
			DeepLink:    link,
			Priority:    "high",
		}},
		ExpectedImpact: impact,
		FollowUps:      follow,
	}
}

func selectMenuPerfPreviewDishes(snap menuPerformanceSnapshot) []menuPerfDish {
	costedSold := make([]menuPerfDish, 0)
	costed := make([]menuPerfDish, 0)
	for _, d := range snap.Dishes {
		if !d.HasCost || d.PlateCost <= 0 || d.Price <= 0 {
			continue
		}
		costed = append(costed, d)
		if d.HasSales && d.Units > 0 {
			costedSold = append(costedSold, d)
		}
	}
	sort.SliceStable(costedSold, func(i, j int) bool {
		if costedSold[i].Units != costedSold[j].Units {
			return costedSold[i].Units > costedSold[j].Units
		}
		return costedSold[i].FoodCostPct > costedSold[j].FoodCostPct
	})
	sort.SliceStable(costed, func(i, j int) bool {
		if costed[i].FoodCostPct != costed[j].FoodCostPct {
			return costed[i].FoodCostPct > costed[j].FoodCostPct
		}
		return costed[i].MarginDollars < costed[j].MarginDollars
	})
	seen := map[string]bool{}
	out := make([]menuPerfDish, 0, 2)
	add := func(list []menuPerfDish) {
		for _, d := range list {
			if len(out) >= 2 {
				return
			}
			key := strings.ToLower(d.Name)
			if seen[key] {
				continue
			}
			seen[key] = true
			out = append(out, d)
		}
	}
	add(costedSold)
	add(costed)
	return out
}

func marginPreviewLines(costed []menuPerfDish, bump float64, spanish bool) (summary string, evidence []string) {
	n := len(costed)
	if n > 2 {
		n = 2
	}
	if n == 0 {
		return "", nil
	}
	parts := make([]string, 0, n)
	for i := 0; i < n; i++ {
		d := costed[i]
		newPrice := d.Price + bump
		newM := d.MarginDollars + bump
		curFood := 0.0
		newFood := 0.0
		if d.Price > 0 {
			curFood = (d.PlateCost / d.Price) * 100
		}
		if newPrice > 0 {
			newFood = (d.PlateCost / newPrice) * 100
		}
		if spanish {
			parts = append(parts, fmt.Sprintf("%s margen actual $%.2f (costo de insumos %.1f%%), margen nuevo $%.2f (costo de insumos %.1f%%) en una vista previa de $%.2f a $%.2f — solo vista previa, no se encoló nada",
				d.Name, d.MarginDollars, curFood, newM, newFood, bump, newPrice))
			evidence = append(evidence, fmt.Sprintf("%s: precio $%.2f, costo de plato $%.2f, margen actual $%.2f, costo de insumos %.1f%%, precio de vista previa $%.2f, margen nuevo $%.2f, costo de insumos %.1f%%. Solo vista previa, no se encoló nada.",
				d.Name, d.Price, d.PlateCost, d.MarginDollars, curFood, newPrice, newM, newFood))
		} else {
			parts = append(parts, fmt.Sprintf("%s current margin $%.2f (food cost %.1f%%), new margin $%.2f (food cost %.1f%%) in a $%.2f preview to $%.2f — preview only, nothing is queued",
				d.Name, d.MarginDollars, curFood, newM, newFood, bump, newPrice))
			evidence = append(evidence, fmt.Sprintf("%s: $%.2f price, $%.2f plate cost, current margin $%.2f, food cost %.1f%%, preview price $%.2f, new margin $%.2f, food cost %.1f%%. Preview only, nothing is queued.",
				d.Name, d.Price, d.PlateCost, d.MarginDollars, curFood, newPrice, newM, newFood))
		}
	}
	if spanish {
		if n == 1 {
			return parts[0] + ".", evidence
		}
		return fmt.Sprintf("%s y %s son los platos con el margen más delgado en la carta. %s.",
			costed[0].Name, costed[1].Name, strings.Join(parts, ". ")), evidence
	}
	if n == 1 {
		return parts[0] + ".", evidence
	}
	return fmt.Sprintf("%s and %s are the thinnest-margin dishes on the card. %s.",
		costed[0].Name, costed[1].Name, strings.Join(parts, ". ")), evidence
}

func formatLiveCardClause(costed []menuPerfDish, spanish bool) string {
	parts := make([]string, 0, len(costed))
	for i, d := range costed {
		if i >= 3 {
			break
		}
		if spanish {
			parts = append(parts, fmt.Sprintf("%s ($%.2f, costo $%.2f)", d.Name, d.Price, d.PlateCost))
		} else {
			parts = append(parts, fmt.Sprintf("%s ($%.2f, plate cost $%.2f)", d.Name, d.Price, d.PlateCost))
		}
	}
	if spanish {
		return "En la carta: " + strings.Join(parts, "; ") + "."
	}
	return "On the card: " + strings.Join(parts, "; ") + "."
}
