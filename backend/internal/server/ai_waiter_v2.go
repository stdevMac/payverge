package server

import (
	"fmt"
	"regexp"
	"strings"
	"time"
	"unicode"

	"github.com/stdevmac/payverge/backend/internal/assistantcontract"
	"github.com/stdevmac/payverge/backend/internal/llm"
	"github.com/stdevmac/payverge/backend/internal/services"
)

const (
	maxWaiterRecommendationEntities = 3
	maxWaiterMenuSources            = 12
	maxWaiterMenuSections           = 5
	maxWaiterPresentationRunes      = 240
	maxWaiterIdentityRunes          = 200
	maxWaiterAllergens              = 12
)

// WaiterFinalizeInput contains untrusted model presentation and the trusted
// request-scoped state used to rebuild every authoritative V2 field.
type WaiterFinalizeInput struct {
	ResponseID         string
	Locale             string
	Mode               string
	UserMessage        string
	ModelText          string
	ValidatedCalls     []llm.ToolCall
	Snapshot           WaiterMenuSnapshot
	OrderingBrowseOnly bool
	Visit              services.WaiterVisitFacts
	// Bill is the numeric shape of the same open bill Visit summarizes. The
	// handler fills it from the bill row (int64 cents), so check math is
	// computed server-side instead of asked of the model (issue 943).
	Bill waiterBillMoney
}

type waiterV2Intent int

const (
	waiterIntentOrdinary waiterV2Intent = iota
	waiterIntentRecommendation
	waiterIntentFullMenu
	waiterIntentSpecificItem
	waiterIntentUnsupportedMenu
	waiterIntentAllergen
	waiterIntentPairing
	waiterIntentAlternative
	waiterIntentSoldOut
	waiterIntentHours
	waiterIntentReservation
	waiterIntentBill
	waiterIntentParking
	waiterIntentDelivery
	waiterIntentTables
	waiterIntentServiceCall
	waiterIntentCategory
	waiterIntentWaitTime
)

type waiterV2Copy struct {
	recommendIntro string
	fullMenuIntro  string
	available      string
	unavailable    string
	helpAdd        string
	addLabel       string
	safeFallback   string
	socialReply    string
	// pairingIntro and alternativesIntro carry waiterItemPlaceholder, which is
	// replaced with a sanitized snapshot display name.
	pairingIntro      string
	alternativesIntro string
	// pickIntro and pickPairing carry waiterItemPlaceholder; avoidAllergen
	// carries waiterItemPlaceholder plus waiterListPlaceholder (the sanitized
	// allergen list). They power the dietary pick-with-why answer (issues
	// 790/816) instead of a catalog re-list.
	pickIntro     string
	pickPairing   string
	avoidAllergen string
	// billTip and billSplit carry waiterPercentPlaceholder / waiterAmountPlaceholder /
	// waiterTotalPlaceholder / waiterCountPlaceholder and are filled from the
	// server-owned bill total in cents, never from model prose (issue 943).
	billTip   string
	billSplit string
}

type waiterTrustedSelection struct {
	entity   WaiterMenuEntity
	quantity int
	notes    string
}

// FinalizeWaiterV2 rebuilds menu facts, sources, entities, and actions only
// from the canonical menu snapshot and server-validated cart calls. Raw model
// prose never becomes a menu fact, entity, source, action, or transaction claim.
func FinalizeWaiterV2(in WaiterFinalizeInput) (assistantcontract.Response, error) {
	if strings.TrimSpace(in.ResponseID) == "" {
		return assistantcontract.Response{}, fmt.Errorf("finalize waiter v2: response_id is required")
	}
	if in.Snapshot.BusinessID == 0 {
		return assistantcontract.Response{}, fmt.Errorf("finalize waiter v2: scoped menu snapshot is required")
	}
	mode, err := normalizeAIWaiterMode(in.Mode)
	if err != nil || mode != in.Snapshot.Mode {
		return assistantcontract.Response{}, fmt.Errorf("finalize waiter v2: mode does not match menu snapshot")
	}

	locale := normalizeWaiterMenuSnapshotLocale(in.Locale, in.Snapshot.Locale)
	copy := waiterFinalizerCopy(locale)
	nativeProse := waiterNativeSmallTalkLocale(locale)
	response := assistantcontract.NewResponse(strings.TrimSpace(in.ResponseID), "")
	retrievedAt := time.Now().UTC().Format(time.RFC3339)

	intent, matched := classifyWaiterV2Intent(locale, in.UserMessage, in.Snapshot)
	if intent == waiterIntentAllergen {
		finalizeWaiterAllergen(&response, locale, in.UserMessage, matched, in.Snapshot, copy, retrievedAt)
		return validateFinalizedWaiterResponse(response)
	}

	selections, requestedCartCall, selectionTruncated := trustedWaiterCallSelections(in)
	explicitCartIntent := explicitWaiterCartIntent(locale, in.UserMessage)
	if explicitCartIntent && len(selections) == 0 && !requestedCartCall {
		selections = waiterSelectionsFromMatched(matched, in.Snapshot)
	}
	if explicitCartIntent && len(selections) > 0 {
		requestedCartCall = true
	}
	// Task 3 removes invalid add_to_cart calls while intentionally preserving
	// unrelated non-cart calls. The user's affirmative cart request is therefore
	// the remaining trusted signal that a zero-selection turn must clarify,
	// rather than degrade to generic prose.
	knownUnavailableSpecificItem := len(matched) == 1 && !matched[0].Available
	zeroSelectionCartContext := waiterCartContext(locale, in.UserMessage) || intent == waiterIntentSpecificItem || len(matched) > 0
	if explicitCartIntent && zeroSelectionCartContext && len(selections) == 0 && !knownUnavailableSpecificItem {
		requestedCartCall = true
	}

	// Issue 867: a visit question ("what time do you close on Sunday? can I
	// order at 10:30pm?") trips the cart vocabulary through words like "order"
	// or "pedir", so the model can answer it with a stray add_to_cart — which
	// put a bottle of water on a live guest's check instead of the closing
	// time. When the guest named no menu entity at all, the visit answer wins
	// and every cart call on the turn is dropped.
	visitQuestionTurn := waiterInformationalIntent(intent) && len(matched) == 0

	if requestedCartCall && explicitCartIntent && !visitQuestionTurn {
		if mode != "ordering" || in.OrderingBrowseOnly || !in.Snapshot.OrderingOpen {
			response.Answer.Content = services.OrderingPausedMessage(locale)
			response.Status = assistantcontract.StatusBlocked
			return validateFinalizedWaiterResponse(response)
		}
		actionable := make([]waiterTrustedSelection, 0, len(selections))
		for _, selection := range selections {
			key := WaiterMenuEntityKey{Type: selection.entity.Type, ID: selection.entity.ID}
			if entity, ok := in.Snapshot.OrderableByKey[key]; ok {
				selection.entity = cloneWaiterMenuEntity(entity)
				actionable = append(actionable, selection)
			}
		}
		for _, selection := range actionable {
			appendWaiterEntityAndSource(&response, selection.entity, retrievedAt)
			response.Actions = append(response.Actions, waiterCartAction(selection, copy))
		}
		if len(actionable) == 0 {
			response.Answer.Content = services.ClarifyItemMessage(locale)
			response.Status = assistantcontract.StatusNeedsClarification
			return validateFinalizedWaiterResponse(response)
		}
		response.Answer.Content = waiterCartHelpAnswer(copy, actionable)
		if selectionTruncated {
			response.Status = assistantcontract.StatusDegraded
		}
		return validateFinalizedWaiterResponse(response)
	}

	switch intent {
	case waiterIntentFullMenu:
		finalizeWaiterFullMenu(&response, in.Snapshot, copy, retrievedAt)
	case waiterIntentSoldOut:
		finalizeWaiterSoldOut(&response, locale, in.Snapshot, copy, retrievedAt)
	case waiterIntentHours, waiterIntentReservation, waiterIntentParking, waiterIntentDelivery:
		finalizeWaiterVisitFacts(&response, locale, in.UserMessage, in.Snapshot, in.Visit, copy, retrievedAt, intent)
	case waiterIntentBill:
		response.Answer.Content = waiterBillAnswerWithMath(locale, in, copy)
	case waiterIntentTables:
		response.Answer.Content = services.WaiterTablesAnswer(locale)
	case waiterIntentServiceCall:
		response.Answer.Content = services.WaiterServiceCallAnswer(locale)
	case waiterIntentWaitTime:
		response.Answer.Content = services.WaiterWaitTimeAnswer(locale)
	case waiterIntentCategory:
		finalizeWaiterCategory(&response, locale, in.UserMessage, in.Snapshot, copy, retrievedAt)
	case waiterIntentRecommendation:
		finalizeWaiterRecommendation(&response, locale, in.UserMessage, selections, in.Snapshot, copy, retrievedAt, selectionTruncated)
	case waiterIntentPairing:
		if len(matched) != 1 {
			response.Answer.Content = services.ClarifyItemMessage(locale)
			response.Status = assistantcontract.StatusNeedsClarification
			break
		}
		finalizeWaiterPairing(&response, locale, in.UserMessage, matched[0], in.Snapshot, copy, retrievedAt)
	case waiterIntentAlternative:
		var anchor *WaiterMenuEntity
		if len(matched) == 1 {
			anchor = &matched[0]
		}
		finalizeWaiterAlternatives(&response, locale, in.UserMessage, anchor, in.Snapshot, copy, retrievedAt)
	case waiterIntentSpecificItem:
		// A guest may name several dishes in one breath ("how much are the
		// ribeye, the mixed grill and the malbec?"). Answering only the
		// single-match case threw resolved snapshot facts away (issue 868).
		named := waiterDistinctlyNamedEntities(in.Snapshot, in.UserMessage, matched)
		switch {
		// Same-named rows are an ambiguity the guest cannot resolve from the
		// answer, so clarification wins over pricing them all.
		case len(named) == 0, waiterEntitiesShareADisplayName(named):
			response.Answer.Content = services.ClarifyItemMessage(locale)
			response.Status = assistantcontract.StatusNeedsClarification
		case len(named) == 1:
			finalizeWaiterSpecificItem(&response, named[0], copy, retrievedAt)
		default:
			finalizeWaiterNamedItems(&response, locale, named, copy, retrievedAt)
		}
	case waiterIntentUnsupportedMenu:
		response.Answer.Content = services.ClarifyItemMessage(locale)
		response.Status = assistantcontract.StatusNeedsClarification
	default:
		if smallTalk, ok := validatedWaiterSmallTalk(locale, in.UserMessage, in.ModelText, in.Snapshot); nativeProse && ok {
			response.Answer.Format = assistantcontract.FormatPlainText
			response.Answer.Content = smallTalk
		} else if nativeProse && waiterSocialMessage(in.UserMessage) {
			response.Answer.Format = assistantcontract.FormatPlainText
			response.Answer.Content = copy.socialReply
		} else {
			response.Answer.Format = assistantcontract.FormatPlainText
			response.Answer.Content = copy.safeFallback
			response.Status = assistantcontract.StatusDegraded
		}
	}

	return validateFinalizedWaiterResponse(response)
}

func validateFinalizedWaiterResponse(response assistantcontract.Response) (assistantcontract.Response, error) {
	if err := assistantcontract.Validate(response); err != nil {
		return assistantcontract.Response{}, fmt.Errorf("finalize waiter v2: %w", err)
	}
	return response, nil
}

// finalizeWaiterForHandler guarantees that hostile or unexpectedly oversized
// menu metadata cannot turn a guest request into a 500. The fallback contains
// no model-owned facts and is itself contract-valid.
func finalizeWaiterForHandler(in WaiterFinalizeInput) assistantcontract.Response {
	response, err := FinalizeWaiterV2(in)
	if err == nil {
		return response
	}
	locale := normalizeWaiterMenuSnapshotLocale(in.Locale, in.Snapshot.Locale)
	responseID := strings.TrimSpace(in.ResponseID)
	if responseID == "" || len([]rune(responseID)) > maxWaiterIdentityRunes {
		responseID = "waiter-degraded"
	}
	response = assistantcontract.NewResponse(responseID, services.ClarifyItemMessage(locale))
	response.Status = assistantcontract.StatusDegraded
	validated, validationErr := validateFinalizedWaiterResponse(response)
	if validationErr == nil {
		return validated
	}
	fallback := assistantcontract.NewResponse("waiter-degraded", services.ClarifyItemMessage(locale))
	fallback.Status = assistantcontract.StatusDegraded
	return fallback
}

func finalizeRejectedWaiterCart(responseID, locale string, browseOnly bool) assistantcontract.Response {
	content := services.ClarifyItemMessage(locale)
	status := assistantcontract.StatusNeedsClarification
	if browseOnly {
		content = services.OrderingPausedMessage(locale)
		status = assistantcontract.StatusBlocked
	}
	response := assistantcontract.NewResponse(responseID, content)
	response.Status = status
	validated, err := validateFinalizedWaiterResponse(response)
	if err == nil {
		return validated
	}
	fallback := assistantcontract.NewResponse("waiter-degraded", services.ClarifyItemMessage(locale))
	fallback.Status = assistantcontract.StatusDegraded
	return fallback
}

func classifyWaiterV2Intent(locale, userMessage string, snapshot WaiterMenuSnapshot) (waiterV2Intent, []WaiterMenuEntity) {
	message := normalizeName(userMessage)
	matched := waiterEntitiesMentionedByUser(message, snapshot)
	matched = preferWaiterOfferWhenDealAsk(userMessage, matched)
	dietaryRec := services.DietaryRecommendationIntent(locale, userMessage)
	if services.DetectAllergenIntent(locale, userMessage) && !dietaryRec {
		if len(matched) == 1 || !services.DetectRecommendationIntent(locale, userMessage) {
			return waiterIntentAllergen, matched
		}
	}
	if containsAnyWaiterPhrase(message, []string{
		"show me the full menu", "show the full menu", "show me your menu", "show your menu", "see the menu",
		"show me the menu", "show the menu", "whats on the menu", "what s on the menu", "what is on the menu",
		"menu please", "the menu please",
		"todo el menu", "menu completo", "mostrame el menu", "muestrame el menu", "ver el menu", "que tienen en el menu",
	}) || waiterBareFullMenuAsk(message) {
		return waiterIntentFullMenu, matched
	}
	if services.DetectHoursIntent(locale, userMessage) {
		return waiterIntentHours, matched
	}
	if services.DetectReservationIntent(locale, userMessage) {
		return waiterIntentReservation, matched
	}
	if services.DetectBillIntent(locale, userMessage) {
		return waiterIntentBill, matched
	}
	// Issue 943: "dividí entre 2" / "add a 10% tip" are check math, not menu
	// asks. Without this route they fell through to the cart branch and the
	// guest was told "I couldn't find that on the menu". Guarded on no menu
	// match so "split a Bife de chorizo" still resolves as an item turn.
	if len(matched) == 0 && waiterBillMathAsk(locale, userMessage).requested() {
		return waiterIntentBill, matched
	}
	if services.DetectParkingIntent(locale, userMessage) {
		return waiterIntentParking, matched
	}
	if services.DetectDeliveryIntent(locale, userMessage) {
		return waiterIntentDelivery, matched
	}
	if services.DetectServiceCallIntent(locale, userMessage) {
		return waiterIntentServiceCall, matched
	}
	if services.DetectTablesIntent(locale, userMessage) {
		return waiterIntentTables, matched
	}
	// "how long it takes?" is a kitchen-timing ask (issue 790b), checked before
	// the sold-out / recommendation ladder so it never degrades to the shrug.
	if services.DetectWaitTimeIntent(locale, userMessage) {
		return waiterIntentWaitTime, matched
	}
	// Listing asks ("what's 86'd?") win here. "nothing 86'd please" is a skip
	// constraint, not a listing, so DetectSoldOutIntent is false and the
	// recommendation branch below recommends orderable dishes instead (#767).
	if services.DetectSoldOutIntent(locale, userMessage) {
		return waiterIntentSoldOut, matched
	}
	if len(matched) == 1 && services.DetectPairingIntent(locale, userMessage) {
		return waiterIntentPairing, matched
	}
	unavailableMatched := waiterUnavailableMatched(matched)
	if services.DetectAlternativeIntent(locale, userMessage) && len(matched) == 0 && dietaryRec {
		return waiterIntentRecommendation, matched
	}
	if services.DetectAlternativeIntent(locale, userMessage) || (services.DetectRecommendationIntent(locale, userMessage) && unavailableMatched) {
		return waiterIntentAlternative, matched
	}
	if services.DetectRecommendationIntent(locale, userMessage) ||
		len(services.DetectDietaryTags(locale, userMessage)) > 0 ||
		services.DetectSoldOutSkipConstraint(locale, userMessage) {
		return waiterIntentRecommendation, matched
	}
	if len(matched) > 0 {
		return waiterIntentSpecificItem, matched
	}
	if len(snapshot.MatchCategoriesInText(userMessage)) > 0 {
		return waiterIntentCategory, nil
	}
	if containsAnyWaiterPhrase(message, []string{
		"menu", "dish", "item", "food", "drink", "order", "do you have", "serve", "price",
		"plato", "comida", "bebida", "pedir", "ordenar", "tienen", "hay", "precio", "cuesta",
	}) {
		return waiterIntentUnsupportedMenu, nil
	}
	return waiterIntentOrdinary, nil
}

// waiterInformationalIntent reports whether the classified intent answers a
// question about the visit or the catalog as a whole, rather than a request to
// move the cart. These turns are fully answerable from server-owned snapshot
// and visit facts, so they never need — and must never be overridden by — a
// model-emitted cart call (issue 867).
func waiterInformationalIntent(intent waiterV2Intent) bool {
	switch intent {
	case waiterIntentHours, waiterIntentReservation, waiterIntentBill, waiterIntentParking,
		waiterIntentDelivery, waiterIntentTables, waiterIntentServiceCall, waiterIntentWaitTime,
		waiterIntentSoldOut, waiterIntentFullMenu:
		return true
	default:
		return false
	}
}

// waiterVisitQuestionTurn reports whether the guest asked one of those
// questions without naming a single menu entity. Naming a dish keeps the turn
// orderable ("add a ribeye, and when do you close?").
func waiterVisitQuestionTurn(locale, userMessage string, snapshot WaiterMenuSnapshot) bool {
	intent, matched := classifyWaiterV2Intent(locale, userMessage, snapshot)
	return waiterInformationalIntent(intent) && len(matched) == 0
}

func preferWaiterOfferWhenDealAsk(userMessage string, matched []WaiterMenuEntity) []WaiterMenuEntity {
	if len(matched) < 2 {
		return matched
	}
	message := normalizeWaiterSearchText(userMessage)
	if !containsAnyWaiterPhrase(message, []string{"deal", "off", "discount", "promo", "oferta", "descuento"}) {
		return matched
	}
	offers := make([]WaiterMenuEntity, 0, 1)
	for _, entity := range matched {
		if entity.Type == waiterMenuEntityTypeOffer {
			offers = append(offers, entity)
		}
	}
	if len(offers) == 1 {
		return offers
	}
	return matched
}

func waiterUnavailableMatched(matched []WaiterMenuEntity) bool {
	for _, entity := range matched {
		if !entity.Available {
			return true
		}
	}
	return false
}

func waiterSelectionsFromMatched(matched []WaiterMenuEntity, snapshot WaiterMenuSnapshot) []waiterTrustedSelection {
	if len(matched) == 0 {
		return nil
	}
	out := make([]waiterTrustedSelection, 0, len(matched))
	for _, entity := range matched {
		key := WaiterMenuEntityKey{Type: entity.Type, ID: entity.ID}
		orderable, ok := snapshot.OrderableByKey[key]
		if !ok || !representableWaiterEntityIdentity(orderable) {
			continue
		}
		out = append(out, waiterTrustedSelection{entity: cloneWaiterMenuEntity(orderable), quantity: 1})
	}
	return out
}

func waiterEntitiesMentionedByUser(message string, snapshot WaiterMenuSnapshot) []WaiterMenuEntity {
	return snapshot.MatchEntitiesInText(message)
}

func trustedWaiterCallSelections(in WaiterFinalizeInput) ([]waiterTrustedSelection, bool, bool) {
	byKey := make(map[WaiterMenuEntityKey]waiterTrustedSelection)
	ambiguous := make(map[WaiterMenuEntityKey]struct{})
	requested := false
	for _, call := range in.ValidatedCalls {
		if call.Name != "add_to_cart" {
			continue
		}
		requested = true
		key, quantity, notes, ok := waiterCartCallSelection(call.Args)
		if !ok {
			continue
		}
		entity, ok := in.Snapshot.RecommendableByKey[key]
		if !ok || !representableWaiterEntityIdentity(entity) {
			continue
		}
		if _, invalid := ambiguous[key]; invalid {
			continue
		}
		selection := waiterTrustedSelection{entity: cloneWaiterMenuEntity(entity), quantity: quantity, notes: notes}
		if existing, duplicate := byKey[key]; duplicate {
			if existing.quantity != selection.quantity || existing.notes != selection.notes {
				delete(byKey, key)
				ambiguous[key] = struct{}{}
			}
			continue
		}
		byKey[key] = selection
	}
	selections := make([]waiterTrustedSelection, 0, len(byKey))
	truncated := false
	for _, entity := range in.Snapshot.Entities {
		key := WaiterMenuEntityKey{Type: entity.Type, ID: entity.ID}
		selection, ok := byKey[key]
		if !ok {
			continue
		}
		if len(selections) == 8 {
			truncated = true
			continue
		}
		selections = append(selections, selection)
	}
	return selections, requested, truncated
}

func waiterCartCallSelection(args map[string]any) (WaiterMenuEntityKey, int, string, bool) {
	key, ok := waiterCartCallKey(args)
	if !ok {
		return WaiterMenuEntityKey{}, 0, "", false
	}
	quantity, ok := toInt(args["quantity"])
	if !ok || quantity < 1 || quantity > 20 {
		return WaiterMenuEntityKey{}, 0, "", false
	}
	notes := ""
	if rawNotes, present := args["notes"]; present {
		value, ok := rawNotes.(string)
		if !ok {
			return WaiterMenuEntityKey{}, 0, "", false
		}
		notes = strings.TrimSpace(value)
	}
	if len([]rune(notes)) > 200 || strings.IndexFunc(notes, unsafeWaiterUnicodeRune) >= 0 {
		return WaiterMenuEntityKey{}, 0, "", false
	}
	return key, quantity, notes, true
}

func waiterCartCallKey(args map[string]any) (WaiterMenuEntityKey, bool) {
	itemValue, itemPresent := args["menu_item_id"]
	bundleValue, bundlePresent := args["bundle_id"]
	itemSet := itemPresent && itemValue != nil
	bundleSet := bundlePresent && bundleValue != nil
	if itemSet == bundleSet {
		return WaiterMenuEntityKey{}, false
	}
	if itemSet {
		id, ok := stableStringID(itemValue)
		return WaiterMenuEntityKey{Type: waiterMenuEntityTypeMenuItem, ID: id}, ok
	}
	id, ok := stableBundleID(bundleValue)
	return WaiterMenuEntityKey{Type: waiterMenuEntityTypeBundle, ID: id}, ok
}

// finalizeWaiterRecommendation answers "what's good here?" from the canonical
// snapshot. Server-validated tool selections still win when the model supplied
// them, but a recommendation no longer *requires* one: without a selection the
// server picks available snapshot entities itself, optionally filtered by the
// dietary tags the guest asked for. Model prose is never consulted, so this
// cannot surface an off-menu item (#577).
func finalizeWaiterRecommendation(
	response *assistantcontract.Response, locale, userMessage string,
	selections []waiterTrustedSelection, snapshot WaiterMenuSnapshot,
	copy waiterV2Copy, retrievedAt string, truncated bool,
) {
	dietaryTags := waiterDietaryTags(locale, userMessage)
	// A pregnancy, a table with children, or an explicit "sin alcohol" rules out
	// every alcoholic candidate — including the cross-category companion the
	// dietary pick reaches for, which is how a "Copa de Malbec" landed beside a
	// vegetarian salad for a pregnant guest (issue 941). Wine carries the
	// vegetarian and vegan tags, so no existing filter caught it. Filtering here
	// covers the dietary pick, the grounded list, and model-supplied selections
	// alike.
	alcoholFree := waiterAlcoholFreeAsk(userMessage)
	candidates := make([]WaiterMenuEntity, 0, maxWaiterRecommendationEntities)
	if len(selections) > 0 {
		for _, selection := range selections {
			if len(candidates) == maxWaiterRecommendationEntities {
				truncated = true
				break
			}
			candidates = append(candidates, selection.entity)
		}
		if alcoholFree {
			candidates = filterWaiterAlcoholicEntities(snapshot, candidates)
		}
	} else {
		// Fetch unbounded, filter, then truncate — see
		// waiterRecommendableEntitiesForAsk for why the order is load-bearing.
		candidates = waiterRecommendableEntitiesForAsk(snapshot, dietaryTags, maxWaiterRecommendationEntities, alcoholFree)
	}
	excludedName := services.ExtractExcludedDishName(locale, userMessage)
	if len(selections) == 0 && len(dietaryTags) > 0 && excludedName == "" {
		// A dietary "what can I eat?" gets one pick with a why (and an optional
		// companion) instead of a catalog re-list — issues 790/816. Exclusion
		// asks ("que no sea milanesa") and model-selected turns keep the list.
		finalizeWaiterDietaryPick(response, locale, dietaryTags, candidates, snapshot, copy, retrievedAt)
	} else {
		intro := copy.recommendIntro
		if excludedName != "" && len(snapshot.MatchEntitiesInText(excludedName)) == 0 {
			offMenu := services.WaiterOffMenuNamed(locale, excludedName)
			if len(candidates) == 0 {
				response.Answer.Content = offMenu + "\n\n" + services.ClarifyItemMessage(locale)
				response.Status = assistantcontract.StatusNeedsClarification
				return
			}
			intro = offMenu + "\n" + intro
		}
		finalizeWaiterGroundedList(response, locale, intro, candidates, copy, retrievedAt, truncated)
	}
	if services.DietaryAskAvoidsAllergens(dietaryTags) && len(response.Entities) > 0 {
		notice := services.AllergenDisclaimer(locale)
		if !strings.Contains(response.Answer.Content, notice) {
			response.Answer.Content = services.AppendAllergenDisclaimer(locale, response.Answer.Content)
		}
		response.Notices = append(response.Notices, assistantcontract.Notice{
			ID: "allergen-staff-confirmation", Kind: "warning", Message: notice,
		})
	}
}

// finalizeWaiterDietaryPick answers a dietary "what can I eat?" with one
// grounded pick plus its price and description, an optional cross-category
// companion from the same filtered candidate set, and — when no dish carries
// the requested tag and the candidates came from the allergen-exclusion
// fallback — which flagged dishes to skip, read from their own allergen chips
// (issue 816). Every named dish still comes from the canonical snapshot; the
// staff-confirmation disclaimer is appended by the caller.
func finalizeWaiterDietaryPick(
	response *assistantcontract.Response, locale string, dietaryTags []string,
	candidates []WaiterMenuEntity, snapshot WaiterMenuSnapshot,
	copy waiterV2Copy, retrievedAt string,
) {
	if len(candidates) == 0 {
		if services.DietaryAskAvoidsAllergens(dietaryTags) {
			// Nothing on the menu is safe to recommend for an allergen-avoidance
			// ask — keep the staff-confirmation refusal (safety posture from
			// issues #767/#577 stays intact).
			response.Answer.Content = services.AllergenRefusal(locale)
			response.Status = assistantcontract.StatusNeedsClarification
			notice := services.AllergenDisclaimer(locale)
			if !strings.Contains(response.Answer.Content, notice) {
				response.Answer.Content = services.AppendAllergenDisclaimer(locale, response.Answer.Content)
			}
			response.Notices = append(response.Notices, assistantcontract.Notice{
				ID: "allergen-staff-confirmation", Kind: "warning", Message: notice,
			})
			return
		}
		response.Answer.Content = services.ClarifyItemMessage(locale)
		response.Status = assistantcontract.StatusNeedsClarification
		return
	}

	primary := safeWaiterPresentationEntity(candidates[0])
	if entityID, sourceID := appendWaiterEntityAndSource(response, candidates[0], retrievedAt); entityID == "" || sourceID == "" {
		response.Answer.Content = services.ClarifyItemMessage(locale)
		response.Status = assistantcontract.StatusNeedsClarification
		return
	}
	line := waiterCopyWithItem(copy.pickIntro, waiterEntityBoldName(primary))
	if price := waiterEntityPriceLabel(primary); price != "" {
		line += " (" + price + ")"
	}
	if description := strings.TrimSpace(primary.Description); description != "" {
		line += " — " + description
	}
	lines := []string{line}
	usedKeys := map[WaiterMenuEntityKey]struct{}{
		{Type: candidates[0].Type, ID: candidates[0].ID}: {},
	}

	// Companion suggestion stays inside the same dietary-filtered candidate
	// set, preferring a different category (a drink beside a main).
	for _, candidate := range candidates[1:] {
		if candidate.CategoryID == candidates[0].CategoryID {
			continue
		}
		if entityID, sourceID := appendWaiterEntityAndSource(response, candidate, retrievedAt); entityID != "" && sourceID != "" {
			companion := safeWaiterPresentationEntity(candidate)
			lines = append(lines, waiterCopyWithItem(copy.pickPairing, waiterEntityBoldName(companion)))
			usedKeys[WaiterMenuEntityKey{Type: candidate.Type, ID: candidate.ID}] = struct{}{}
		}
		break
	}

	// When no dish carries the requested tag(s), the candidates came from the
	// allergen-exclusion fallback — so name the flagged dishes to skip. When a
	// tagged dish exists the tag is the guest-facing signal and no avoid list
	// is rendered (keeps #767 celiac answers naming only the safe dish).
	//
	// Avoid dishes are deliberately TEXT-ONLY — never appended as entities or
	// sources. The guest UI renders every menu-item entity as a priced
	// tap-to-add card, and an avoid dish is usually still orderable, so
	// emitting it as an entity would ship a one-tap ADD button for the very
	// dish the allergen warning tells the guest to skip (adversarial B1).
	if services.DietaryAskAvoidsAllergens(dietaryTags) && len(snapshot.collectRecommendable(dietaryTags, true, 1)) == 0 {
		avoidCount := 0
		for _, entity := range snapshot.RecommendableEntities(nil, 0) {
			if avoidCount == 2 {
				break
			}
			key := WaiterMenuEntityKey{Type: entity.Type, ID: entity.ID}
			if _, used := usedKeys[key]; used {
				continue
			}
			if !services.EntityContainsAvoidedAllergen(entity.Allergens, dietaryTags) {
				continue
			}
			if !representableWaiterEntityIdentity(entity) {
				continue
			}
			presented := safeWaiterPresentationEntity(entity)
			if strings.TrimSpace(presented.DisplayName) == "" {
				continue
			}
			avoidLine := waiterCopyWithItem(copy.avoidAllergen, waiterEntityBoldName(presented))
			avoidLine = strings.ReplaceAll(avoidLine, waiterListPlaceholder, strings.Join(waiterLocalizedAllergens(locale, presented.Allergens), ", "))
			lines = append(lines, avoidLine)
			usedKeys[key] = struct{}{}
			avoidCount++
		}
	}

	response.Answer.Content = strings.Join(lines, "\n")
}

// finalizeWaiterPairing answers "what goes with X?" by naming available items
// from a different snapshot category than the anchor the guest named.
//
// The companion set is the classic wine-pairing surface, so it needs the same
// alcohol awareness as the recommendation path: "estoy embarazada, ¿qué va bien
// con el ojo de bife?" used to answer with a "Copa de Malbec" because this
// finalizer never saw the guest's message at all (issue 941).
func finalizeWaiterPairing(
	response *assistantcontract.Response, locale, userMessage string, anchor WaiterMenuEntity,
	snapshot WaiterMenuSnapshot, copy waiterV2Copy, retrievedAt string,
) {
	anchor = safeWaiterPresentationEntity(anchor)
	if !representableWaiterEntityIdentity(anchor) {
		response.Answer.Content = services.ClarifyItemMessage(locale)
		response.Status = assistantcontract.StatusNeedsClarification
		return
	}
	pairings := waiterCompanionEntities(snapshot, anchor, false, waiterAlcoholFreeAsk(userMessage))
	if len(pairings) == 0 {
		response.Answer.Content = services.ClarifyItemMessage(locale)
		response.Status = assistantcontract.StatusNeedsClarification
		return
	}
	appendWaiterEntityAndSource(response, anchor, retrievedAt)
	finalizeWaiterGroundedList(response, locale, waiterCopyWithItem(copy.pairingIntro, waiterEntityProseName(anchor)), pairings, copy, retrievedAt, false)
}

// finalizeWaiterAlternatives answers "X is unavailable, what else?" with
// available items from the same category, or with a plain grounded
// recommendation when the guest never named an on-menu anchor.
func finalizeWaiterAlternatives(
	response *assistantcontract.Response, locale, userMessage string, anchor *WaiterMenuEntity,
	snapshot WaiterMenuSnapshot, copy waiterV2Copy, retrievedAt string,
) {
	// Same gap as the pairing finalizer (issue 941): a substitute list is drawn
	// from the whole carta, so an alcohol-free ask must reach it too.
	alcoholFree := waiterAlcoholFreeAsk(userMessage)
	if anchor == nil {
		finalizeWaiterGroundedList(response, locale, copy.recommendIntro,
			waiterRecommendableEntitiesForAsk(snapshot, services.DetectDietaryTags(locale, ""), maxWaiterRecommendationEntities, alcoholFree),
			copy, retrievedAt, false)
		return
	}
	presented := safeWaiterPresentationEntity(*anchor)
	if !representableWaiterEntityIdentity(presented) {
		response.Answer.Content = services.ClarifyItemMessage(locale)
		response.Status = assistantcontract.StatusNeedsClarification
		return
	}
	alternatives := waiterCompanionEntities(snapshot, presented, true, alcoholFree)
	if len(alternatives) == 0 {
		response.Answer.Content = services.ClarifyItemMessage(locale)
		response.Status = assistantcontract.StatusNeedsClarification
		return
	}
	intro := copy.recommendIntro
	if !presented.Available {
		intro = waiterCopyWithItem(copy.alternativesIntro, waiterEntityProseName(presented))
	}
	appendWaiterEntityAndSource(response, presented, retrievedAt)
	finalizeWaiterGroundedList(response, locale, intro, alternatives, copy, retrievedAt, false)
}

// waiterCompanionEntities picks up to maxWaiterRecommendationEntities available
// snapshot entities related to anchor. sameCategory selects substitutes (same
// category, anchor excluded); otherwise it selects pairings (any other
// category). Both fall back to "any other available entity" so a single-category
// menu still answers.
//
// excludeAlcohol drops alcoholic entities inside the scan rather than after it,
// so a wine-forward carta still fills the companion slots with what the guest
// can actually drink instead of returning an empty list (issue 941).
func waiterCompanionEntities(snapshot WaiterMenuSnapshot, anchor WaiterMenuEntity, sameCategory, excludeAlcohol bool) []WaiterMenuEntity {
	anchorKey := WaiterMenuEntityKey{Type: anchor.Type, ID: anchor.ID}
	categoryNames := waiterCategoryNamesByID(snapshot)
	preferred := make([]WaiterMenuEntity, 0, maxWaiterRecommendationEntities)
	fallback := make([]WaiterMenuEntity, 0, maxWaiterRecommendationEntities)
	for _, entity := range snapshot.RecommendableEntities(nil, 0) {
		if (WaiterMenuEntityKey{Type: entity.Type, ID: entity.ID}) == anchorKey {
			continue
		}
		if excludeAlcohol && waiterEntityIsAlcoholic(entity, categoryNames) {
			continue
		}
		matchesCategory := entity.CategoryID == anchor.CategoryID
		if matchesCategory == sameCategory && len(preferred) < maxWaiterRecommendationEntities {
			preferred = append(preferred, entity)
			continue
		}
		if len(fallback) < maxWaiterRecommendationEntities {
			fallback = append(fallback, entity)
		}
	}
	if len(preferred) > 0 {
		return preferred
	}
	return fallback
}

// finalizeWaiterGroundedList renders a bulleted answer from snapshot entities,
// registering each as an entity + source so every named dish is evidence-backed.
func finalizeWaiterGroundedList(
	response *assistantcontract.Response, locale, intro string,
	entities []WaiterMenuEntity, copy waiterV2Copy, retrievedAt string, truncated bool,
) {
	presented := make([]WaiterMenuEntity, 0, len(entities))
	for _, entity := range entities {
		entityID, sourceID := appendWaiterEntityAndSource(response, entity, retrievedAt)
		if entityID == "" || sourceID == "" {
			truncated = true
			continue
		}
		presented = append(presented, entity)
	}
	if len(presented) == 0 {
		response.Answer.Content = services.ClarifyItemMessage(locale)
		response.Status = assistantcontract.StatusNeedsClarification
		return
	}
	response.Answer.Content = intro + "\n" + waiterEntityBulletList(presented, copy)
	if truncated {
		response.Status = assistantcontract.StatusDegraded
	}
}

func finalizeWaiterFullMenu(response *assistantcontract.Response, snapshot WaiterMenuSnapshot, copy waiterV2Copy, retrievedAt string) {
	response.Answer.Content = copy.fullMenuIntro
	truncated := false
	for _, category := range snapshot.Categories {
		if len(response.Sections) == maxWaiterMenuSections {
			truncated = true
			break
		}
		if !boundedWaiterIdentity(category.ID) {
			truncated = true
			continue
		}
		section := assistantcontract.Section{
			ID: category.ID, Title: safeWaiterPresentation(category.DisplayName), Answer: "",
			Steps: []string{}, ActionIDs: []string{}, SourceIDs: []string{}, EntityIDs: []string{},
		}
		lines := make([]string, 0, len(category.EntityKeys))
		for _, key := range category.EntityKeys {
			if len(response.Sources) == maxWaiterMenuSources {
				truncated = true
				break
			}
			entity, ok := snapshot.ByKey[key]
			if !ok || !representableWaiterEntityIdentity(entity) {
				truncated = true
				continue
			}
			entityID, sourceID := appendWaiterEntityAndSource(response, entity, retrievedAt)
			if entityID == "" || sourceID == "" {
				truncated = true
				continue
			}
			section.EntityIDs = append(section.EntityIDs, entityID)
			section.SourceIDs = append(section.SourceIDs, sourceID)
			lines = append(lines, waiterEntityLine(entity, copy))
		}
		if len(lines) == 0 {
			continue
		}
		section.Answer = strings.Join(lines, "\n")
		response.Sections = append(response.Sections, section)
	}
	if truncated {
		response.Status = assistantcontract.StatusDegraded
	}
}

func finalizeWaiterSoldOut(response *assistantcontract.Response, locale string, snapshot WaiterMenuSnapshot, copy waiterV2Copy, retrievedAt string) {
	unavailable := snapshot.UnavailableEntities(maxWaiterRecommendationEntities)
	if len(unavailable) == 0 {
		response.Answer.Content = services.WaiterSoldOutNone(locale)
		return
	}
	finalizeWaiterGroundedList(response, locale, services.WaiterSoldOutIntro(locale), unavailable, copy, retrievedAt, false)
}

func finalizeWaiterVisitFacts(response *assistantcontract.Response, locale, userMessage string, snapshot WaiterMenuSnapshot, visit services.WaiterVisitFacts, copy waiterV2Copy, retrievedAt string, intent waiterV2Intent) {
	hours := intent == waiterIntentHours || services.DetectHoursIntent(locale, userMessage)
	reservations := intent == waiterIntentReservation || services.DetectReservationIntent(locale, userMessage)
	parking := intent == waiterIntentParking || services.DetectParkingIntent(locale, userMessage)
	delivery := intent == waiterIntentDelivery || services.DetectDeliveryIntent(locale, userMessage)

	parts := make([]string, 0, 4)
	if hours {
		parts = append(parts, services.WaiterHoursAnswer(locale, visit))
	}
	if reservations {
		parts = append(parts, services.WaiterReservationAnswer(locale, visit))
	}
	if parking {
		parts = append(parts, services.WaiterParkingAnswer(locale))
	}
	if delivery {
		parts = append(parts, services.WaiterDeliveryAnswer(locale, visit))
	}

	entities := waiterVisitMenuMentions(userMessage, snapshot)
	if len(entities) == 0 {
		if len(parts) == 0 {
			response.Answer.Content = copy.safeFallback
			response.Status = assistantcontract.StatusDegraded
			return
		}
		response.Answer.Content = strings.Join(parts, "\n\n")
		return
	}
	intro := strings.Join(parts, "\n\n")
	if intro != "" {
		intro += "\n" + copy.recommendIntro
	} else {
		intro = copy.recommendIntro
	}
	finalizeWaiterGroundedList(response, locale, intro, entities, copy, retrievedAt, false)
}

func waiterVisitMenuMentions(userMessage string, snapshot WaiterMenuSnapshot) []WaiterMenuEntity {
	seen := make(map[WaiterMenuEntityKey]struct{}, maxWaiterRecommendationEntities)
	out := make([]WaiterMenuEntity, 0, maxWaiterRecommendationEntities)
	add := func(entity WaiterMenuEntity) {
		if len(out) >= maxWaiterRecommendationEntities || !representableWaiterEntityIdentity(entity) {
			return
		}
		key := WaiterMenuEntityKey{Type: entity.Type, ID: entity.ID}
		if _, exists := seen[key]; exists {
			return
		}
		seen[key] = struct{}{}
		out = append(out, entity)
	}
	for _, entity := range snapshot.MatchEntitiesInText(userMessage) {
		add(entity)
	}
	for _, category := range snapshot.MatchCategoriesInText(userMessage) {
		for _, key := range category.EntityKeys {
			entity, ok := snapshot.ByKey[key]
			if ok {
				add(entity)
			}
		}
	}
	return out
}

func finalizeWaiterCategory(response *assistantcontract.Response, locale, userMessage string, snapshot WaiterMenuSnapshot, copy waiterV2Copy, retrievedAt string) {
	categories := snapshot.MatchCategoriesInText(userMessage)
	entities := make([]WaiterMenuEntity, 0, maxWaiterRecommendationEntities)
	for _, category := range categories {
		for _, key := range category.EntityKeys {
			if len(entities) == maxWaiterRecommendationEntities {
				break
			}
			entity, ok := snapshot.ByKey[key]
			if !ok || !representableWaiterEntityIdentity(entity) {
				continue
			}
			entities = append(entities, entity)
		}
	}
	finalizeWaiterGroundedList(response, locale, copy.recommendIntro, entities, copy, retrievedAt, false)
}

func finalizeWaiterSpecificItem(response *assistantcontract.Response, entity WaiterMenuEntity, copy waiterV2Copy, retrievedAt string) {
	if !representableWaiterEntityIdentity(entity) {
		response.Answer.Content = copy.safeFallback
		response.Status = assistantcontract.StatusNeedsClarification
		return
	}
	appendWaiterEntityAndSource(response, entity, retrievedAt)
	response.Answer.Content = waiterSpecificItemAnswer(entity, copy)
}

func finalizeWaiterAllergen(
	response *assistantcontract.Response, locale, userMessage string, matched []WaiterMenuEntity,
	snapshot WaiterMenuSnapshot, copy waiterV2Copy, retrievedAt string,
) {
	if len(matched) == 1 {
		entity := safeWaiterPresentationEntity(matched[0])
		if representableWaiterEntityIdentity(entity) {
			appendWaiterEntityAndSource(response, entity, retrievedAt)
			response.Answer.Content = services.AllergenAnswerFromItem(locale, waiterEntityProseName(entity), waiterLocalizedAllergens(locale, entity.Allergens))
		} else {
			response.Answer.Content = services.AllergenRefusal(locale)
			response.Status = assistantcontract.StatusNeedsClarification
		}
	} else if len(matched) == 0 {
		if displayName, allergens, ok := legacyWaiterAllergenFacts(userMessage, snapshot); ok {
			response.Answer.Content = services.AllergenAnswerFromItem(locale, displayName, waiterLocalizedAllergens(locale, allergens))
		} else if tags := waiterAllergenAvoidanceTags(userMessage); len(tags) > 0 {
			// Issue 942: the guest named the allergen instead of asking for
			// "sin X", so no dish resolved and the answer was a refuse-all
			// hedge. It is a dietary ask — answer it with what IS safe, plus
			// the walk-through of which flagged dishes to skip.
			finalizeWaiterDietaryPick(response, locale, tags,
				waiterRecommendableEntitiesForAsk(snapshot, tags, maxWaiterRecommendationEntities, waiterAlcoholFreeAsk(userMessage)),
				snapshot, copy, retrievedAt)
			appendWaiterAllergenSafetyNotice(response, locale)
			return
		} else {
			response.Answer.Content = services.AllergenRefusal(locale)
			response.Status = assistantcontract.StatusNeedsClarification
			if name := services.ExtractExcludedDishName(locale, userMessage); name != "" {
				response.Answer.Content = services.WaiterOffMenuNamed(locale, name) + "\n\n" + response.Answer.Content
			}
		}
	} else {
		response.Answer.Content = services.AllergenRefusal(locale)
		response.Status = assistantcontract.StatusNeedsClarification
	}
	notice := services.AllergenDisclaimer(locale)
	if !strings.Contains(response.Answer.Content, notice) {
		response.Answer.Content = services.AppendAllergenDisclaimer(locale, response.Answer.Content)
	}
	response.Notices = append(response.Notices, assistantcontract.Notice{
		ID: "allergen-staff-confirmation", Kind: "warning", Message: notice,
	})
}

func legacyWaiterAllergenFacts(userMessage string, snapshot WaiterMenuSnapshot) (string, []string, bool) {
	message := normalizeName(userMessage)
	var matched *WaiterMenuEntity
	for _, category := range snapshot.AllergenMenuCategories() {
		for _, item := range category.Items {
			name := normalizeName(item.Name)
			if name == "" || !containsWaiterPhrase(message, name) {
				continue
			}
			presentation := safeWaiterPresentationEntity(WaiterMenuEntity{
				DisplayName: item.Name,
				Allergens:   append([]string(nil), item.Allergens...),
			})
			if matched != nil {
				return "", nil, false
			}
			matched = &presentation
		}
	}
	if matched == nil {
		return "", nil, false
	}
	return matched.DisplayName, matched.Allergens, true
}

func appendWaiterEntityAndSource(response *assistantcontract.Response, entity WaiterMenuEntity, retrievedAt string) (string, string) {
	if !representableWaiterEntityIdentity(entity) {
		return "", ""
	}
	entity = safeWaiterPresentationEntity(entity)
	entityID := entity.Type + ":" + entity.ID
	sourceID := "menu:" + entity.Type + ":" + entity.ID
	for _, existing := range response.Entities {
		if existing.ID == entityID {
			return entityID, existing.SourceID
		}
	}
	response.Sources = append(response.Sources, assistantcontract.Source{
		ID: sourceID, Type: entity.Type, Title: entity.DisplayName,
		Origin: "business_menu", RetrievedAt: retrievedAt,
	})
	availability := "unavailable"
	if entity.Available {
		availability = "available"
	}
	response.Entities = append(response.Entities, assistantcontract.Entity{
		ID: entityID, Type: entity.Type, DisplayName: entity.DisplayName,
		Availability: availability, SourceID: sourceID,
	})
	return entityID, sourceID
}

func boundedWaiterIdentity(value string) bool {
	value = strings.TrimSpace(value)
	return value != "" && len([]rune(value)) <= maxWaiterIdentityRunes
}

func representableWaiterEntityIdentity(entity WaiterMenuEntity) bool {
	if !boundedWaiterIdentity(entity.ID) || !boundedWaiterIdentity(entity.Type) {
		return false
	}
	return boundedWaiterIdentity(entity.Type+":"+entity.ID) &&
		boundedWaiterIdentity("menu:"+entity.Type+":"+entity.ID) &&
		boundedWaiterIdentity("add-cart:"+entity.Type+":"+entity.ID)
}

func waiterCartAction(selection waiterTrustedSelection, copy waiterV2Copy) assistantcontract.Action {
	entity := safeWaiterPresentationEntity(selection.entity)
	quantity := selection.quantity
	var notes *string
	if selection.notes != "" {
		value := selection.notes
		notes = &value
	}
	return assistantcontract.Action{
		ID:   "add-cart:" + entity.Type + ":" + entity.ID,
		Type: "add_cart_item", Label: copy.addLabel + " " + waiterEntityProseName(entity),
		Target: assistantcontract.ActionTarget{Kind: entity.Type, ID: entity.ID, Quantity: &quantity, Notes: notes},
		State:  "ready", Confirmation: "none",
	}
}

func waiterCartHelpAnswer(copy waiterV2Copy, selections []waiterTrustedSelection) string {
	names := make([]string, 0, len(selections))
	for _, selection := range selections {
		names = append(names, waiterEntityProseName(selection.entity))
	}
	return copy.helpAdd + " " + strings.Join(names, ", ") + "."
}

func waiterEntityBulletList(entities []WaiterMenuEntity, copy waiterV2Copy) string {
	lines := make([]string, 0, len(entities))
	for _, entity := range entities {
		lines = append(lines, waiterEntityLine(entity, copy))
	}
	return strings.Join(lines, "\n")
}

// waiterEntityPriceLabel renders the snapshot price ("$14.00"). Empty when the
// snapshot has no positive price, so "how much cost the harvest bowl" states
// the price instead of describing around it (issue 790d) without ever
// inventing one. Offer entities never get a label: their Price field carries
// DiscountValue, so a "10% off" offer would render as "$10.00" (adversarial B5).
func waiterEntityPriceLabel(entity WaiterMenuEntity) string {
	if entity.Type == waiterMenuEntityTypeOffer {
		return ""
	}
	if entity.Price <= 0 {
		return ""
	}
	return aiWaiterCurrencySymbol(entity.Currency) + fmt.Sprintf("%.2f", entity.Price)
}

func waiterEntityLine(entity WaiterMenuEntity, copy waiterV2Copy) string {
	entity = safeWaiterPresentationEntity(entity)
	state := copy.unavailable
	if entity.Available {
		state = copy.available
	}
	line := "- " + waiterEntityBoldName(entity)
	if price := waiterEntityPriceLabel(entity); price != "" {
		line += " — " + price
	}
	return line + " — " + state
}

func waiterSpecificItemAnswer(entity WaiterMenuEntity, copy waiterV2Copy) string {
	entity = safeWaiterPresentationEntity(entity)
	state := copy.unavailable
	if entity.Available {
		state = copy.available
	}
	answer := waiterEntityBoldName(entity)
	if price := waiterEntityPriceLabel(entity); price != "" {
		answer += " — " + price
	}
	answer += " — " + state
	if description := strings.TrimSpace(entity.Description); description != "" {
		answer += "\n\n" + description
	}
	return answer
}

func waiterSocialMessage(userMessage string) bool {
	message := normalizeWaiterSearchText(userMessage)
	return containsAnyWaiterPhrase(message, []string{
		"hi", "hello", "hey", "thanks", "thank you", "goodbye", "bye",
		"hola", "gracias", "muchas gracias", "chau", "buen dia", "buenas tardes", "buenas noches",
	})
}

func validatedWaiterSmallTalk(locale, userMessage, modelText string, snapshot WaiterMenuSnapshot) (string, bool) {
	if locale != "en" && locale != "es" && locale != "es-AR" {
		return "", false
	}
	message := normalizeWaiterSearchText(userMessage)
	if !containsAnyWaiterPhrase(message, []string{
		"tell me a joke", "make me laugh", "tell me something funny",
		"contame un chiste", "cuentame un chiste", "decime algo gracioso", "dime algo gracioso",
	}) {
		return "", false
	}
	text := strings.TrimSpace(modelText)
	if text == "" || len([]rune(text)) > 1000 || strings.IndexFunc(text, unsafeWaiterUnicodeRune) >= 0 ||
		waiterDangerousHTMLBlock.MatchString(text) || waiterHTMLTag.MatchString(text) ||
		waiterMarkdownLink.MatchString(text) || waiterPresentationURL.MatchString(text) ||
		waiterAnyURIScheme.MatchString(text) || waiterCredentialSyntax.MatchString(text) ||
		strings.ContainsAny(text, "*_`#><[]") {
		return "", false
	}
	normalized := normalizeWaiterSearchText(text)
	compact := compactWaiterSearchText(text)
	if containsAnyWaiterPhrase(normalized, waiterForbiddenSmallTalkPhrases) || containsCompactWaiterPhrase(compact, waiterForbiddenCompactSmallTalkPhrases) {
		return "", false
	}
	for _, entity := range snapshot.Entities {
		if waiterPresentationNameAppears(normalized, compact, entity.DisplayName) {
			return "", false
		}
	}
	for _, category := range snapshot.Categories {
		if waiterPresentationNameAppears(normalized, compact, category.DisplayName) {
			return "", false
		}
	}
	return strings.Join(strings.Fields(text), " "), true
}

var waiterForbiddenSmallTalkPhrases = []string{
	"menu", "special", "specials", "price", "cost", "ingredient", "allergen", "nut free", "celiac", "gluten free",
	"open at", "close at", "hours", "address", "located", "street",
	"reservation", "reserved", "booking", "table", "payment", "paid", "charged", "refund",
	"order", "ordered", "cart", "added", "confirmed", "complete", "completed", "succeeded", "success", "ready",
	"available", "unavailable", "menú", "especial", "especiales", "precio", "ingrediente", "alérgeno", "alergeno",
	"abre a", "cierra a", "horario", "dirección", "direccion", "ubicado", "ubicada", "calle",
	"reserva", "mesa", "pago", "pagado", "cobrado", "reembolso", "pedido", "carrito", "agregado", "confirmado", "completado", "listo",
}

var waiterForbiddenCompactSmallTalkPhrases = []string{
	"menu", "price", "ingredient", "allergen", "nut free", "celiac", "gluten free",
	"open at", "close at", "hours", "reservation", "reserved", "booking", "payment", "paid", "charged", "refund",
	"order", "ordered", "cart", "added", "confirmed", "complete", "completed", "succeeded", "success",
	"available", "unavailable", "precio", "ingrediente", "alergeno", "horario", "reserva", "pago", "pagado",
	"cobrado", "reembolso", "pedido", "carrito", "agregado", "confirmado", "completado",
}

func containsCompactWaiterPhrase(compact string, phrases []string) bool {
	for _, phrase := range phrases {
		candidate := compactWaiterSearchText(phrase)
		if candidate != "" && strings.Contains(compact, candidate) {
			return true
		}
	}
	return false
}

func unsafeWaiterUnicodeRune(r rune) bool {
	return unicode.IsControl(r) || unicode.In(r, unicode.Cf)
}

func waiterPresentationNameAppears(normalized, compact, name string) bool {
	if normalizedName := normalizeWaiterSearchText(name); normalizedName != "" && containsWaiterPhrase(normalized, normalizedName) {
		return true
	}
	compactName := compactWaiterSearchText(name)
	return len([]rune(compactName)) >= 4 && strings.Contains(compact, compactName)
}

func compactWaiterSearchText(value string) string {
	return strings.Map(func(r rune) rune {
		if unicode.IsLetter(r) || unicode.IsNumber(r) {
			return unicode.ToLower(r)
		}
		return -1
	}, value)
}

func explicitWaiterCartIntent(locale, userMessage string) bool {
	message := normalizeWaiterSearchText(userMessage)
	if message == "" || (locale != "en" && locale != "es" && locale != "es-AR") {
		return false
	}
	if locale == "en" {
		if containsAnyWaiterPhrase(message, []string{
			"do not add", "do not put", "do not get", "do not order",
			"don t add", "don t put", "don t get", "don t order",
			"dont add", "dont put", "dont get", "dont order",
			"do not want", "don t want", "dont want",
			"not add", "not put", "not get", "not order",
			"never add", "never put", "never get", "never order",
			"without adding", "without putting", "without getting", "without ordering",
			"no add", "no put", "no get", "no order",
			"cancel", "ready", "status", "did you add", "was it added", "already added",
		}) {
			return false
		}
		return containsAnyWaiterPhrase(message, []string{"add", "put", "get me", "order", "i d like", "i would like"})
	}
	if containsAnyWaiterSubstring(message, []string{
		"no quiero",
		"no agreg", "no añad", "no anad", "no pong", "no poner", "no sum", "no pid", "no ped", "no orden", "no traig", "no traer",
		"nunca agreg", "nunca añad", "nunca anad", "nunca pong", "nunca poner", "nunca sum", "nunca pid", "nunca ped", "nunca orden", "nunca traig", "nunca traer",
		"sin agreg", "sin añad", "sin anad", "sin poner", "sin sum", "sin pedir", "sin ordenar", "sin traer", "no quiero pedir", "no quiero ordenar",
	}) ||
		strings.Contains(message, "cancel") || strings.Contains(message, "listo mi pedido") || strings.Contains(message, "estado de mi pedido") ||
		strings.Contains(message, "ya agreg") || strings.Contains(message, "agregaste") {
		return false
	}
	return strings.Contains(message, "agreg") || strings.Contains(message, "añad") || strings.Contains(message, "anad") || strings.Contains(message, "sum") ||
		containsAnyWaiterPhrase(message, []string{
			"poner", "pon", "pone", "poné", "ponelo", "ponlo", "quiero pedir", "pedime", "pídeme",
			"quiero ordenar", "ordena", "ordená", "traeme", "tráeme", "mandame", "mandá", "manda",
		})
}

func waiterCartContext(locale, userMessage string) bool {
	message := normalizeWaiterSearchText(userMessage)
	if locale == "en" {
		return containsAnyWaiterPhrase(message, []string{"add", "put", "get me", "order", "cart", "menu"})
	}
	return containsAnyWaiterSubstring(message, []string{"agreg", "añad", "anad", "sum", "carrito", "menu", "menú", "pedido", "mand"}) ||
		containsAnyWaiterPhrase(message, []string{
			"poner", "pon", "pone", "poné", "ponelo", "ponlo", "pedime", "pídeme", "ordena", "ordená", "traeme", "tráeme", "mandame",
		})
}

func containsAnyWaiterSubstring(text string, values []string) bool {
	for _, value := range values {
		if strings.Contains(text, value) {
			return true
		}
	}
	return false
}

var (
	waiterDangerousHTMLBlock = regexp.MustCompile(`(?is)<\s*(?:script|style|iframe|object|form)\b[^>]*>.*?<\s*/\s*(?:script|style|iframe|object|form)\s*>`)
	waiterHTMLTag            = regexp.MustCompile(`(?s)<[^>]*>`)
	waiterMarkdownLink       = regexp.MustCompile(`!?\[([^\]]*)\]\([^\r\n]*?\)`)
	waiterPresentationURL    = regexp.MustCompile(`(?i)(?:https?://|www\.)[^\s]+`)
	waiterAnyURIScheme       = regexp.MustCompile(`(?i)(?:\b(?:javascript|data|file|blob|ftp|mailto|tel):|\b[a-z][a-z0-9+.-]{1,31}://)`)
	waiterCredentialSyntax   = regexp.MustCompile(`[^\s:@]+:[^\s@]+@[^\s]+`)
)

func safeWaiterPresentation(value string) string {
	value = waiterDangerousHTMLBlock.ReplaceAllString(value, " ")
	value = waiterMarkdownLink.ReplaceAllString(value, "$1")
	value = waiterPresentationURL.ReplaceAllString(value, " ")
	value = waiterHTMLTag.ReplaceAllString(value, " ")
	value = strings.Map(func(r rune) rune {
		if unsafeWaiterUnicodeRune(r) {
			return ' '
		}
		switch r {
		case '*', '_', '`', '#', '>', '[', ']', '(', ')', '!', '\\':
			return ' '
		default:
			return r
		}
	}, value)
	value = strings.Join(strings.Fields(value), " ")
	runes := []rune(value)
	if len(runes) > maxWaiterPresentationRunes {
		value = strings.TrimSpace(string(runes[:maxWaiterPresentationRunes]))
	}
	return value
}

func safeWaiterPresentationEntity(entity WaiterMenuEntity) WaiterMenuEntity {
	entity = cloneWaiterMenuEntity(entity)
	entity.DisplayName = safeWaiterPresentation(entity.DisplayName)
	if entity.DisplayName == "" {
		entity.DisplayName = entity.ID
	}
	entity.SourceName = safeWaiterPresentation(entity.SourceName)
	entity.Description = safeWaiterPresentation(entity.Description)
	if len(entity.Allergens) > maxWaiterAllergens {
		entity.Allergens = entity.Allergens[:maxWaiterAllergens]
	}
	for index := range entity.Allergens {
		entity.Allergens[index] = safeWaiterPresentation(entity.Allergens[index])
	}
	return entity
}

// waiterBareFullMenuAsks are broad openers that mean "list the menu" only when
// they are the whole message. As substrings they would hijack recommendation,
// dietary and category asks ("what do you have that's vegan", "what do you
// have for dessert"), and this classifier also runs in LLM mode.
var waiterBareFullMenuAsks = map[string]struct{}{
	"what do you have": {}, "what do you serve": {}, "full menu": {}, "the full menu": {},
	"what do you have today": {}, "what do you serve today": {},
}

func waiterBareFullMenuAsk(message string) bool {
	_, ok := waiterBareFullMenuAsks[strings.TrimSpace(normalizeWaiterSearchText(message))]
	return ok
}

func containsAnyWaiterPhrase(text string, phrases []string) bool {
	if len(phrases) == 0 {
		return false
	}
	padded := waiterPaddedSearchText(text)
	for _, phrase := range phrases {
		if paddedTextContainsWaiterPhrase(padded, phrase) {
			return true
		}
	}
	return false
}

func containsWaiterPhrase(text, phrase string) bool {
	return paddedTextContainsWaiterPhrase(waiterPaddedSearchText(text), phrase)
}

// waiterPaddedSearchText normalizes a haystack once so callers scanning many
// phrases against the same guest message (menu entity resolution walks every
// entity on the menu) do not re-normalize it per phrase.
func waiterPaddedSearchText(text string) string {
	return " " + normalizeWaiterSearchText(text) + " "
}

func paddedTextContainsWaiterPhrase(padded, phrase string) bool {
	phrase = normalizeWaiterSearchText(phrase)
	return phrase != "" && strings.Contains(padded, " "+phrase+" ")
}

// waiterPaddedTextMatchesEntityName resolves "Date Night" to "Date Night for Two"
// without requiring the guest to quote the full bundle title.
func waiterPaddedTextMatchesEntityName(padded, name string) bool {
	for _, phrase := range waiterEntityMatchPhrases(name) {
		if paddedTextContainsWaiterPhrase(padded, phrase) {
			return true
		}
	}
	return false
}

func waiterEntityMatchPhrases(name string) []string {
	normalized := normalizeWaiterSearchText(name)
	if normalized == "" {
		return nil
	}
	phrases := []string{normalized}
	words := strings.Fields(normalized)
	if len(words) >= 3 {
		phrases = append(phrases, strings.Join(words[:2], " "))
	}
	return phrases
}

func normalizeWaiterSearchText(value string) string {
	return strings.Join(strings.Fields(strings.Map(func(r rune) rune {
		if unicode.IsLetter(r) || unicode.IsNumber(r) {
			return unicode.ToLower(r)
		}
		return ' '
	}, value)), " ")
}

// appendWaiterAllergenSafetyNotice guarantees the staff-confirmation posture on
// an allergen answer exactly once, whichever finalizer produced the prose.
func appendWaiterAllergenSafetyNotice(response *assistantcontract.Response, locale string) {
	notice := services.AllergenDisclaimer(locale)
	if !strings.Contains(response.Answer.Content, notice) {
		response.Answer.Content = services.AppendAllergenDisclaimer(locale, response.Answer.Content)
	}
	for _, existing := range response.Notices {
		if existing.ID == "allergen-staff-confirmation" {
			return
		}
	}
	response.Notices = append(response.Notices, assistantcontract.Notice{
		ID: "allergen-staff-confirmation", Kind: "warning", Message: notice,
	})
}
