package server

import (
	"encoding/json"
	"fmt"
	"sort"
	"strconv"
	"strings"

	"github.com/stdevmac/payverge/backend/internal/database"
	"github.com/stdevmac/payverge/backend/internal/locales"
	"github.com/stdevmac/payverge/backend/internal/s3"
	"github.com/stdevmac/payverge/backend/internal/services"
)

const (
	waiterMenuEntityTypeMenuItem = "menu_item"
	waiterMenuEntityTypeBundle   = "bundle"
	waiterMenuEntityTypeOffer    = "offer"
)

// WaiterMenuEntityKey namespaces stable domain IDs. Menu item IDs are UUID
// strings while offer and bundle IDs are numeric; keeping the type in the key
// prevents collisions without coercing or rewriting canonical identity.
type WaiterMenuEntityKey struct {
	Type string `json:"type"`
	ID   string `json:"id"`
}

// WaiterMenuEntity is the canonical localized presentation and availability
// record shared by prompt grounding and response/tool validation.
type WaiterMenuEntity struct {
	ID          string `json:"id"`
	Type        string `json:"type"`
	DisplayName string `json:"display_name"`
	// SourceName is the untranslated menu name. It is empty when the menu is not
	// translated; prose renders it beside DisplayName so a guest holding a card
	// that still shows the stored name can find the dish (issue 869).
	SourceName  string   `json:"source_name,omitempty"`
	Description string   `json:"description,omitempty"`
	Price       float64  `json:"price"`
	Currency    string   `json:"currency,omitempty"`
	Available   bool     `json:"available"`
	Orderable   bool     `json:"orderable"`
	ImageURL    string   `json:"image_url,omitempty"`
	Allergens   []string `json:"allergens"`
	DietaryTags []string `json:"dietary_tags"`
	CategoryID  string   `json:"category_id,omitempty"`
}

type WaiterMenuCategory struct {
	ID          string                `json:"id"`
	DisplayName string                `json:"display_name"`
	EntityKeys  []WaiterMenuEntityKey `json:"entity_keys"`
}

type WaiterMenuSnapshotInput struct {
	Business      *database.Business
	Locale        string
	Mode          string
	BusinessOpen  bool
	Categories    []database.MenuCategory
	Offers        []database.Offer
	Bundles       []database.Bundle
	HiddenItemIDs map[string]bool
	// SoldOutItemIDs are 86'd / inventory-out dishes. They stay visible so the
	// waiter can say they are sold out, but they never enter recommend/order maps.
	// Distinct from HiddenItemIDs, which are omitted entirely (operator-secret).
	SoldOutItemIDs map[string]bool
	// SourceCategories is the untranslated menu as stored. It supplies alias
	// names (so a guest naming the English dish on a translated menu still
	// resolves) and the canonical dietary tags, which translation may localize
	// away from the canonical DIETARY_TAGS vocabulary. Optional.
	SourceCategories []database.MenuCategory
	// SourceBundles is the untranslated bundle catalog. Optional; when display
	// names are localized, aliases from these names let "hay date night?" still
	// resolve Date Night for Two.
	SourceBundles []database.Bundle
	// TrustedImageHost must come from the configured public asset host, never
	// from a hostname found in menu data.
	TrustedImageHost string
}

// WaiterMenuSnapshot owns one request's canonical menu identity. Exported
// collections are independent copies for deterministic validation; private
// projections back the legacy validator and prompt without re-reading or
// independently filtering menu data.
type WaiterMenuSnapshot struct {
	BusinessID         uint
	Locale             string
	Mode               string
	OrderingOpen       bool
	Categories         []WaiterMenuCategory
	Entities           []WaiterMenuEntity
	ByKey              map[WaiterMenuEntityKey]WaiterMenuEntity
	ByNormalizedName   map[string][]WaiterMenuEntityKey
	RecommendableByKey map[WaiterMenuEntityKey]WaiterMenuEntity
	OrderableByKey     map[WaiterMenuEntityKey]WaiterMenuEntity

	visibleCategories  []database.MenuCategory
	allergenCategories []database.MenuCategory
	visibleOffers      []database.Offer
	visibleBundles     []database.Bundle
	promptMenuJSON     string
	promptOffersJSON   string
	promptBundlesJSON  string
	// sourceNamesByKey holds normalized untranslated aliases per entity so
	// MatchEntitiesInText resolves an item the guest names in the source
	// language of the menu.
	sourceNamesByKey map[WaiterMenuEntityKey][]string
	// dietaryTagsByKey holds canonical DIETARY_TAGS ids per entity, merged from
	// the localized and source rows.
	dietaryTagsByKey map[WaiterMenuEntityKey]map[string]struct{}
}

type waiterPromptMenuCategory struct {
	ID          string                 `json:"id"`
	Name        string                 `json:"name"`
	Description string                 `json:"description,omitempty"`
	Items       []waiterPromptMenuItem `json:"items"`
	SortOrder   int                    `json:"sort_order"`
}

type waiterPromptMenuItem struct {
	ID               string                    `json:"id"`
	Name             string                    `json:"name"`
	Description      string                    `json:"description,omitempty"`
	Price            float64                   `json:"price"`
	Currency         string                    `json:"currency,omitempty"`
	Image            string                    `json:"image,omitempty"`
	Images           []string                  `json:"images"`
	CompositionImage string                    `json:"composition_image,omitempty"`
	Options          []database.MenuItemOption `json:"options"`
	Allergens        []string                  `json:"allergens"`
	DietaryTags      []string                  `json:"dietary_tags"`
	IsAvailable      bool                      `json:"is_available"`
	Orderable        bool                      `json:"orderable"`
	SortOrder        int                       `json:"sort_order"`
}

type waiterPromptOffer struct {
	ID            uint    `json:"id"`
	Name          string  `json:"name"`
	Description   string  `json:"description,omitempty"`
	Image         string  `json:"image,omitempty"`
	DiscountType  string  `json:"discount_type"`
	DiscountValue float64 `json:"discount_value"`
	ApplicableTo  string  `json:"applicable_to"`
	TargetID      *string `json:"target_id,omitempty"`
	Code          *string `json:"code,omitempty"`
	Available     bool    `json:"available"`
}

type waiterPromptBundle struct {
	ID          uint                     `json:"id"`
	Name        string                   `json:"name"`
	Description string                   `json:"description,omitempty"`
	Price       float64                  `json:"price"`
	Currency    string                   `json:"currency,omitempty"`
	Image       string                   `json:"image,omitempty"`
	Items       []database.BundleItemRef `json:"items"`
	Available   bool                     `json:"available"`
	Orderable   bool                     `json:"orderable"`
}

// BuildWaiterMenuSnapshot builds one deterministic, locale-normalized menu
// view after translation and active-promotion resolution. Hidden item IDs
// (operator-secret) are omitted entirely. 86'd / sold-out and manually
// unavailable items remain visible so the waiter can explain them honestly,
// but never enter recommendation/order maps.
func BuildWaiterMenuSnapshot(input WaiterMenuSnapshotInput) (WaiterMenuSnapshot, error) {
	if input.Business == nil || input.Business.ID == 0 {
		return WaiterMenuSnapshot{}, fmt.Errorf("scoped business is required")
	}
	mode, err := normalizeAIWaiterMode(input.Mode)
	if err != nil {
		return WaiterMenuSnapshot{}, err
	}

	snapshot := WaiterMenuSnapshot{
		BusinessID:         input.Business.ID,
		Locale:             normalizeWaiterMenuSnapshotLocale(input.Locale, input.Business.DefaultLanguage),
		Mode:               mode,
		OrderingOpen:       mode == "ordering" && input.BusinessOpen && input.Business.KitchenEnabled && input.Business.OrdersEnabled && database.IsBusinessOperational(input.Business),
		Categories:         []WaiterMenuCategory{},
		Entities:           []WaiterMenuEntity{},
		ByKey:              make(map[WaiterMenuEntityKey]WaiterMenuEntity),
		ByNormalizedName:   make(map[string][]WaiterMenuEntityKey),
		RecommendableByKey: make(map[WaiterMenuEntityKey]WaiterMenuEntity),
		OrderableByKey:     make(map[WaiterMenuEntityKey]WaiterMenuEntity),
		visibleCategories:  []database.MenuCategory{},
		allergenCategories: []database.MenuCategory{},
		visibleOffers:      []database.Offer{},
		visibleBundles:     []database.Bundle{},
		sourceNamesByKey:   make(map[WaiterMenuEntityKey][]string),
		dietaryTagsByKey:   make(map[WaiterMenuEntityKey]map[string]struct{}),
	}

	sourceNamesByItemID, sourceTagsByItemID := indexWaiterSourceMenu(input.SourceCategories)
	sourceBundleNameByID := make(map[uint]string, len(input.SourceBundles))
	for _, source := range input.SourceBundles {
		if name := strings.TrimSpace(source.Name); name != "" {
			sourceBundleNameByID[source.ID] = name
		}
	}

	categories := cloneWaiterMenuCategories(input.Categories)
	sort.SliceStable(categories, func(i, j int) bool {
		if categories[i].SortOrder == categories[j].SortOrder {
			return categories[i].ID < categories[j].ID
		}
		return categories[i].SortOrder < categories[j].SortOrder
	})
	seenCategoryIDs := make(map[string]struct{}, len(categories))
	for _, category := range categories {
		category.ID = strings.TrimSpace(category.ID)
		if category.ID != "" {
			if _, exists := seenCategoryIDs[category.ID]; exists {
				return WaiterMenuSnapshot{}, fmt.Errorf("duplicate waiter menu category %s", category.ID)
			}
			seenCategoryIDs[category.ID] = struct{}{}
		}
		sort.SliceStable(category.Items, func(i, j int) bool {
			if category.Items[i].SortOrder == category.Items[j].SortOrder {
				return category.Items[i].ID < category.Items[j].ID
			}
			return category.Items[i].SortOrder < category.Items[j].SortOrder
		})
		for index := range category.Items {
			category.Items[index] = sanitizeWaiterMenuItemImages(category.Items[index], input.TrustedImageHost)
		}
		allergenItems := make([]database.MenuItem, 0, len(category.Items))
		for _, item := range category.Items {
			item.ID = strings.TrimSpace(item.ID)
			if strings.TrimSpace(item.Name) == "" || (item.ID != "" && input.HiddenItemIDs[item.ID]) {
				continue
			}
			cloned := cloneWaiterMenuItem(item)
			if cloned.ID != "" && input.SoldOutItemIDs[cloned.ID] {
				cloned.IsAvailable = false
			}
			allergenItems = append(allergenItems, cloned)
		}
		if len(allergenItems) > 0 {
			allergenCategory := category
			allergenCategory.Items = allergenItems
			snapshot.allergenCategories = append(snapshot.allergenCategories, allergenCategory)
		}
		if category.ID == "" {
			continue
		}
		visibleItems := make([]database.MenuItem, 0, len(category.Items))
		entityKeys := make([]WaiterMenuEntityKey, 0, len(category.Items))
		for _, item := range category.Items {
			item.ID = strings.TrimSpace(item.ID)
			if item.ID == "" || input.HiddenItemIDs[item.ID] {
				continue
			}
			if input.SoldOutItemIDs[item.ID] {
				item.IsAvailable = false
			}
			key := WaiterMenuEntityKey{Type: waiterMenuEntityTypeMenuItem, ID: item.ID}
			entity := WaiterMenuEntity{
				ID: item.ID, Type: key.Type, DisplayName: strings.TrimSpace(item.Name),
				SourceName:  sourceNamesByItemID[item.ID],
				Description: strings.TrimSpace(item.Description), Price: item.Price, Currency: strings.TrimSpace(item.Currency),
				Available: item.IsAvailable, Orderable: item.IsAvailable && snapshot.OrderingOpen,
				ImageURL: primaryWaiterMenuImage(item), Allergens: cloneStrings(item.Allergens),
				DietaryTags: cloneStrings(item.DietaryTags), CategoryID: category.ID,
			}
			if entity.DisplayName == "" {
				continue
			}
			if err := snapshot.addEntity(key, entity, entity.Available, entity.Orderable); err != nil {
				return WaiterMenuSnapshot{}, err
			}
			snapshot.indexSourceIdentity(key, entity.DisplayName, sourceNamesByItemID[item.ID], entity.DietaryTags, sourceTagsByItemID[item.ID])
			visibleItems = append(visibleItems, cloneWaiterMenuItem(item))
			entityKeys = append(entityKeys, key)
		}
		if len(visibleItems) == 0 {
			continue
		}
		category.Items = visibleItems
		snapshot.visibleCategories = append(snapshot.visibleCategories, category)
		snapshot.Categories = append(snapshot.Categories, WaiterMenuCategory{
			ID: category.ID, DisplayName: strings.TrimSpace(category.Name), EntityKeys: append([]WaiterMenuEntityKey(nil), entityKeys...),
		})
	}

	bundles := cloneWaiterBundles(input.Bundles)
	sort.SliceStable(bundles, func(i, j int) bool { return bundles[i].ID < bundles[j].ID })
	bundleRefs := make(map[uint][]database.BundleItemRef)
	for _, bundle := range bundles {
		if bundle.ID == 0 || bundle.BusinessID != input.Business.ID || !bundle.IsActive || strings.TrimSpace(bundle.Name) == "" {
			continue
		}
		var refs []database.BundleItemRef
		if err := json.Unmarshal([]byte(bundle.Items), &refs); err != nil || len(refs) == 0 {
			continue
		}
		available := true
		orderable := snapshot.OrderingOpen
		valid := true
		canonicalRefs := make([]database.BundleItemRef, 0, len(refs))
		allergens := make([]string, 0)
		seenAllergens := make(map[string]struct{})
		for _, ref := range refs {
			itemID := strings.TrimSpace(ref.MenuItemID)
			item, ok := snapshot.ByKey[WaiterMenuEntityKey{Type: waiterMenuEntityTypeMenuItem, ID: itemID}]
			if !ok || itemID == "" || ref.Quantity <= 0 {
				valid = false
				break
			}
			available = available && item.Available
			orderable = orderable && item.Orderable
			canonicalRefs = append(canonicalRefs, database.BundleItemRef{
				MenuItemID: itemID,
				Name:       item.DisplayName,
				Quantity:   ref.Quantity,
			})
			for _, allergen := range item.Allergens {
				normalized := normalizeName(allergen)
				if normalized == "" {
					continue
				}
				if _, exists := seenAllergens[normalized]; exists {
					continue
				}
				seenAllergens[normalized] = struct{}{}
				allergens = append(allergens, allergen)
			}
		}
		if !valid {
			continue
		}
		bundle.Image = sanitizeWaiterMenuImageURL(bundle.Image, input.TrustedImageHost)
		id := strconv.FormatUint(uint64(bundle.ID), 10)
		key := WaiterMenuEntityKey{Type: waiterMenuEntityTypeBundle, ID: id}
		entity := WaiterMenuEntity{
			ID: id, Type: key.Type, DisplayName: strings.TrimSpace(bundle.Name), SourceName: sourceBundleNameByID[bundle.ID],
			Description: strings.TrimSpace(bundle.Description),
			Price:       bundle.Price, Currency: strings.TrimSpace(bundle.Currency), Available: available, Orderable: orderable,
			ImageURL: bundle.Image, Allergens: allergens, DietaryTags: []string{},
		}
		if err := snapshot.addEntity(key, entity, available, orderable); err != nil {
			return WaiterMenuSnapshot{}, err
		}
		snapshot.indexSourceIdentity(key, entity.DisplayName, sourceBundleNameByID[bundle.ID], entity.DietaryTags)
		snapshot.visibleBundles = append(snapshot.visibleBundles, cloneWaiterBundle(bundle))
		bundleRefs[bundle.ID] = canonicalRefs
	}

	offers := cloneWaiterOffers(input.Offers)
	sort.SliceStable(offers, func(i, j int) bool { return offers[i].ID < offers[j].ID })
	for _, offer := range offers {
		if offer.ID == 0 || offer.BusinessID != input.Business.ID || !offer.IsActive || strings.TrimSpace(offer.Name) == "" {
			continue
		}
		available, valid := snapshot.resolveOfferAvailability(offer)
		if !valid {
			continue
		}
		offer.Image = sanitizeWaiterMenuImageURL(offer.Image, input.TrustedImageHost)
		id := strconv.FormatUint(uint64(offer.ID), 10)
		key := WaiterMenuEntityKey{Type: waiterMenuEntityTypeOffer, ID: id}
		entity := WaiterMenuEntity{
			ID: id, Type: key.Type, DisplayName: strings.TrimSpace(offer.Name), Description: strings.TrimSpace(offer.Description),
			Price: offer.DiscountValue, Available: available, Orderable: false, ImageURL: offer.Image,
			Allergens: []string{}, DietaryTags: []string{},
		}
		if err := snapshot.addEntity(key, entity, available, false); err != nil {
			return WaiterMenuSnapshot{}, err
		}
		snapshot.visibleOffers = append(snapshot.visibleOffers, cloneWaiterOffer(offer))
	}

	menuJSON, err := json.Marshal(snapshot.promptMenuProjection())
	if err != nil {
		return WaiterMenuSnapshot{}, fmt.Errorf("marshal waiter menu snapshot: %w", err)
	}
	offersJSON, err := json.Marshal(snapshot.promptOffersProjection())
	if err != nil {
		return WaiterMenuSnapshot{}, fmt.Errorf("marshal waiter offers snapshot: %w", err)
	}
	bundlesJSON, err := json.Marshal(snapshot.promptBundlesProjection(bundleRefs))
	if err != nil {
		return WaiterMenuSnapshot{}, fmt.Errorf("marshal waiter bundles snapshot: %w", err)
	}
	snapshot.promptMenuJSON = string(menuJSON)
	snapshot.promptOffersJSON = string(offersJSON)
	snapshot.promptBundlesJSON = string(bundlesJSON)
	return snapshot, nil
}

func (snapshot *WaiterMenuSnapshot) addEntity(key WaiterMenuEntityKey, entity WaiterMenuEntity, recommendable, orderable bool) error {
	if _, exists := snapshot.ByKey[key]; exists {
		return fmt.Errorf("duplicate waiter menu entity %s:%s", key.Type, key.ID)
	}
	entity = cloneWaiterMenuEntity(entity)
	snapshot.Entities = append(snapshot.Entities, cloneWaiterMenuEntity(entity))
	snapshot.ByKey[key] = cloneWaiterMenuEntity(entity)
	if normalized := normalizeName(entity.DisplayName); normalized != "" {
		snapshot.ByNormalizedName[normalized] = append(snapshot.ByNormalizedName[normalized], key)
	}
	if recommendable {
		snapshot.RecommendableByKey[key] = cloneWaiterMenuEntity(entity)
	}
	if orderable {
		snapshot.OrderableByKey[key] = cloneWaiterMenuEntity(entity)
	}
	return nil
}

// indexWaiterSourceMenu collects untranslated names and canonical dietary tags
// keyed by menu item ID from the as-stored menu.
func indexWaiterSourceMenu(categories []database.MenuCategory) (map[string]string, map[string][]string) {
	names := make(map[string]string)
	tags := make(map[string][]string)
	for _, category := range categories {
		for _, item := range category.Items {
			id := strings.TrimSpace(item.ID)
			if id == "" {
				continue
			}
			if name := strings.TrimSpace(item.Name); name != "" {
				names[id] = name
			}
			if len(item.DietaryTags) > 0 {
				tags[id] = item.DietaryTags
			}
		}
	}
	return names, tags
}

// indexSourceIdentity records the alias names and canonical dietary tags used by
// MatchEntitiesInText and RecommendableEntities. Aliases that normalize to the
// localized display name are skipped so a match is never counted twice.
func (snapshot *WaiterMenuSnapshot) indexSourceIdentity(key WaiterMenuEntityKey, displayName, sourceName string, tagSets ...[]string) {
	snapshot.addWaiterNameAliases(key, displayName, sourceName)
	canonical := make(map[string]struct{})
	for _, set := range tagSets {
		for _, tag := range set {
			if id, ok := services.CanonicalDietaryTagID(tag); ok {
				canonical[id] = struct{}{}
			}
		}
	}
	if len(canonical) > 0 {
		snapshot.dietaryTagsByKey[key] = canonical
	}
}

// addWaiterNameAliases indexes the full source name, tokens of length >= 5, and
// adjacent token pairs from both the localized display name and the untranslated
// source name. That lets "el steak esta?" resolve Steak Plate on a translated
// menu and "hay date night?" resolve Date Night for Two.
func (snapshot *WaiterMenuSnapshot) addWaiterNameAliases(key WaiterMenuEntityKey, names ...string) {
	seen := map[string]struct{}{}
	for _, alias := range snapshot.sourceNamesByKey[key] {
		seen[alias] = struct{}{}
	}
	display := ""
	if entity, ok := snapshot.ByKey[key]; ok {
		display = normalizeName(entity.DisplayName)
		seen[display] = struct{}{}
	}
	add := func(alias string) {
		normalized := normalizeName(alias)
		if normalized == "" {
			return
		}
		if _, dup := seen[normalized]; dup {
			return
		}
		seen[normalized] = struct{}{}
		snapshot.sourceNamesByKey[key] = append(snapshot.sourceNamesByKey[key], normalized)
	}
	for _, name := range names {
		normalized := normalizeName(name)
		if normalized == "" {
			continue
		}
		if normalized != display {
			add(normalized)
		}
		tokens := strings.Fields(normalized)
		for _, token := range tokens {
			if len([]rune(token)) >= 5 && !waiterAliasStopword(token) && !waiterGenericAliasToken(token) {
				add(token)
			}
		}
		for i := 0; i+1 < len(tokens); i++ {
			if waiterAliasStopword(tokens[i]) || waiterAliasStopword(tokens[i+1]) {
				continue
			}
			add(tokens[i] + " " + tokens[i+1])
		}
	}
}

func waiterAliasStopword(token string) bool {
	switch token {
	case "de", "la", "el", "los", "las", "the", "of", "for", "a", "an", "y", "and",
		"con", "en", "un", "una", "del", "al", "to", "with", "para", "por", "le":
		return true
	default:
		return false
	}
}

func waiterGenericAliasToken(token string) bool {
	switch token {
	case "house", "plate", "salad", "special", "classic", "grilled", "fresh", "green",
		"garden", "home", "style", "sauce", "soup", "juice", "water", "wine", "beer",
		"tea", "coffee", "tart", "cider", "cake", "pie", "bread", "rice", "fries",
		"night", "dish", "item", "menu", "extra", "side", "bowl", "spicy", "sweet":
		return true
	default:
		return false
	}
}

// MatchEntitiesInText returns the snapshot entities whose localized display name
// or untranslated alias appears in message, in snapshot order. It is the single
// place a guest sentence is resolved to canonical menu identity — model prose
// never participates.
func (snapshot WaiterMenuSnapshot) MatchCategoriesInText(message string) []WaiterMenuCategory {
	if strings.TrimSpace(message) == "" {
		return nil
	}
	padded := waiterPaddedSearchText(message)
	matched := make([]WaiterMenuCategory, 0, 1)
	for _, category := range snapshot.Categories {
		if paddedTextContainsWaiterPhrase(padded, category.DisplayName) {
			matched = append(matched, category)
			continue
		}
		// "what pizzas do you have" names the "Pizza" category in the plural.
		if name := normalizeWaiterSearchText(category.DisplayName); name != "" && !strings.HasSuffix(name, "s") &&
			(paddedTextContainsWaiterPhrase(padded, name+"s") || paddedTextContainsWaiterPhrase(padded, name+"es")) {
			matched = append(matched, category)
		}
	}
	return matched
}

func (snapshot WaiterMenuSnapshot) UnavailableEntities(limit int) []WaiterMenuEntity {
	selected := make([]WaiterMenuEntity, 0, 4)
	for _, entity := range snapshot.Entities {
		if limit > 0 && len(selected) >= limit {
			break
		}
		if entity.Type == waiterMenuEntityTypeOffer || entity.Available {
			continue
		}
		if !representableWaiterEntityIdentity(entity) {
			continue
		}
		selected = append(selected, entity)
	}
	return selected
}

func (snapshot WaiterMenuSnapshot) MatchEntitiesInText(message string) []WaiterMenuEntity {
	if strings.TrimSpace(message) == "" {
		return nil
	}
	// Normalize the guest turn once: a full menu can carry hundreds of entities
	// and re-normalizing the message per entity dominated this scan.
	padded := waiterPaddedSearchText(message)
	matched := make([]WaiterMenuEntity, 0, 2)
	for _, entity := range snapshot.Entities {
		if waiterPaddedTextMatchesEntityName(padded, entity.DisplayName) {
			matched = append(matched, entity)
			continue
		}
		// Aliases are rare (only translated menus carry them), so the extra map
		// read stays off the common path.
		for _, alias := range snapshot.sourceNamesByKey[WaiterMenuEntityKey{Type: entity.Type, ID: entity.ID}] {
			if waiterPaddedTextMatchesEntityName(padded, alias) {
				matched = append(matched, entity)
				break
			}
		}
	}
	return matched
}

// RecommendableEntities returns available, non-offer snapshot entities suitable
// for a recommendation, in snapshot order, filtered to those carrying every
// requested canonical dietary tag. When the guest asked to avoid an allergen
// (gluten-free / dairy-free / nut-free) and no dish carries that tag, items
// that do not list the avoided allergen are returned instead. Dishes that list
// the allergen are never recommended. limit <= 0 means unbounded. Offers are
// excluded because a discount is not a dish.
func (snapshot WaiterMenuSnapshot) RecommendableEntities(dietaryTags []string, limit int) []WaiterMenuEntity {
	tagged := snapshot.collectRecommendable(dietaryTags, true, limit)
	if len(dietaryTags) == 0 || len(tagged) > 0 || !services.DietaryAskAvoidsAllergens(dietaryTags) {
		return tagged
	}
	return snapshot.collectRecommendable(dietaryTags, false, limit)
}

func (snapshot WaiterMenuSnapshot) collectRecommendable(dietaryTags []string, requireTags bool, limit int) []WaiterMenuEntity {
	selected := make([]WaiterMenuEntity, 0, len(snapshot.Entities))
	for _, entity := range snapshot.Entities {
		if limit > 0 && len(selected) >= limit {
			break
		}
		if entity.Type == waiterMenuEntityTypeOffer {
			continue
		}
		key := WaiterMenuEntityKey{Type: entity.Type, ID: entity.ID}
		if _, ok := snapshot.RecommendableByKey[key]; !ok {
			continue
		}
		if services.EntityContainsAvoidedAllergen(entity.Allergens, dietaryTags) {
			continue
		}
		if requireTags && !snapshot.entityHasDietaryTags(key, dietaryTags) {
			continue
		}
		selected = append(selected, entity)
	}
	return selected
}

func (snapshot WaiterMenuSnapshot) entityHasDietaryTags(key WaiterMenuEntityKey, required []string) bool {
	if len(required) == 0 {
		return true
	}
	present := snapshot.dietaryTagsByKey[key]
	for _, tag := range required {
		if _, ok := present[tag]; !ok {
			return false
		}
	}
	return true
}

func (snapshot WaiterMenuSnapshot) resolveOfferAvailability(offer database.Offer) (bool, bool) {
	switch strings.ToLower(strings.TrimSpace(offer.ApplicableTo)) {
	case "", "all":
		for key, entity := range snapshot.ByKey {
			if (key.Type == waiterMenuEntityTypeMenuItem || key.Type == waiterMenuEntityTypeBundle) && entity.Available {
				return true, true
			}
		}
		return false, true
	case "item":
		if offer.TargetID == nil {
			return false, false
		}
		entity, ok := snapshot.ByKey[WaiterMenuEntityKey{Type: waiterMenuEntityTypeMenuItem, ID: strings.TrimSpace(*offer.TargetID)}]
		return entity.Available, ok
	case "bundle":
		if offer.TargetID == nil {
			return false, false
		}
		entity, ok := snapshot.ByKey[WaiterMenuEntityKey{Type: waiterMenuEntityTypeBundle, ID: strings.TrimSpace(*offer.TargetID)}]
		return entity.Available, ok
	case "category":
		if offer.TargetID == nil {
			return false, false
		}
		target := strings.TrimSpace(*offer.TargetID)
		for _, category := range snapshot.Categories {
			if category.ID == target {
				for _, key := range category.EntityKeys {
					if snapshot.ByKey[key].Available {
						return true, true
					}
				}
				return false, true
			}
		}
		return false, false
	default:
		return false, false
	}
}

func normalizeWaiterMenuSnapshotLocale(raw, fallback string) string {
	for _, candidate := range []string{raw, fallback} {
		candidate = strings.TrimSpace(candidate)
		for _, locale := range locales.GuestLocales() {
			if strings.EqualFold(locale.Canonical, candidate) {
				return locale.Canonical
			}
		}
	}
	return locales.Default().Canonical
}

func primaryWaiterMenuImage(item database.MenuItem) string {
	for _, image := range item.Images {
		if trimmed := strings.TrimSpace(image); trimmed != "" {
			return trimmed
		}
	}
	if image := strings.TrimSpace(item.Image); image != "" {
		return image
	}
	return strings.TrimSpace(item.CompositionImage)
}

func sanitizeWaiterMenuItemImages(item database.MenuItem, trustedHost string) database.MenuItem {
	item.Image = sanitizeWaiterMenuImageURL(item.Image, trustedHost)
	item.CompositionImage = sanitizeWaiterMenuImageURL(item.CompositionImage, trustedHost)
	images := make([]string, 0, len(item.Images))
	for _, raw := range item.Images {
		if image := sanitizeWaiterMenuImageURL(raw, trustedHost); image != "" {
			images = append(images, image)
		}
	}
	item.Images = images
	return item
}

// sanitizeWaiterMenuImageURL keeps an image URL only when it is one of ours:
// a same-origin media URL ("/media/<key>" or "${PUBLIC_URL}/media/<key>", the
// local-driver shape, where trustedHost is empty) for a public key, or an
// https URL on the configured public asset host (CDN / legacy S3).
func sanitizeWaiterMenuImageURL(raw, trustedHost string) string {
	image := strings.TrimSpace(raw)
	if s3.IsOwnMediaURL(image) {
		if key, err := s3.KeyFromURL(image); err == nil && !s3.IsProtectedMediaKey(key) {
			return image
		}
		return ""
	}
	if err := s3.ValidatePublicAssetURL(image, trustedHost); err != nil {
		return ""
	}
	return image
}

func (snapshot WaiterMenuSnapshot) PromptMenuJSON() string {
	if snapshot.promptMenuJSON == "" {
		return "[]"
	}
	return snapshot.promptMenuJSON
}

func (snapshot WaiterMenuSnapshot) PromptOffersJSON() string {
	if snapshot.promptOffersJSON == "" {
		return "[]"
	}
	return snapshot.promptOffersJSON
}

func (snapshot WaiterMenuSnapshot) PromptBundlesJSON() string {
	if snapshot.promptBundlesJSON == "" {
		return "[]"
	}
	return snapshot.promptBundlesJSON
}

func (snapshot WaiterMenuSnapshot) VisibleMenuCategories() []database.MenuCategory {
	return cloneWaiterMenuCategories(snapshot.visibleCategories)
}

// AllergenMenuCategories returns read-only menu facts, including legacy rows
// that predate stable menu IDs. Anonymous rows never enter prompt,
// recommendation, or orderable projections, but their declared allergen facts
// remain available to the deterministic safety response.
func (snapshot WaiterMenuSnapshot) AllergenMenuCategories() []database.MenuCategory {
	return cloneWaiterMenuCategories(snapshot.allergenCategories)
}

func (snapshot WaiterMenuSnapshot) OrderableMenuCategories() []database.MenuCategory {
	categories := cloneWaiterMenuCategories(snapshot.visibleCategories)
	out := make([]database.MenuCategory, 0, len(categories))
	for _, category := range categories {
		items := make([]database.MenuItem, 0, len(category.Items))
		for _, item := range category.Items {
			if _, ok := snapshot.OrderableByKey[WaiterMenuEntityKey{Type: waiterMenuEntityTypeMenuItem, ID: item.ID}]; ok {
				items = append(items, cloneWaiterMenuItem(item))
			}
		}
		if len(items) == 0 {
			continue
		}
		category.Items = items
		out = append(out, category)
	}
	return out
}

func (snapshot WaiterMenuSnapshot) OrderableBundles() []database.Bundle {
	out := make([]database.Bundle, 0, len(snapshot.visibleBundles))
	for _, bundle := range snapshot.visibleBundles {
		key := WaiterMenuEntityKey{Type: waiterMenuEntityTypeBundle, ID: strconv.FormatUint(uint64(bundle.ID), 10)}
		if _, ok := snapshot.OrderableByKey[key]; ok {
			out = append(out, cloneWaiterBundle(bundle))
		}
	}
	return out
}

func (snapshot WaiterMenuSnapshot) promptMenuProjection() []waiterPromptMenuCategory {
	out := make([]waiterPromptMenuCategory, 0, len(snapshot.visibleCategories))
	for _, category := range snapshot.visibleCategories {
		items := make([]waiterPromptMenuItem, 0, len(category.Items))
		for _, item := range category.Items {
			entity := snapshot.ByKey[WaiterMenuEntityKey{Type: waiterMenuEntityTypeMenuItem, ID: item.ID}]
			items = append(items, waiterPromptMenuItem{
				ID: item.ID, Name: item.Name, Description: item.Description, Price: item.Price, Currency: item.Currency,
				Image: item.Image, Images: cloneStrings(item.Images), CompositionImage: item.CompositionImage,
				Options: append([]database.MenuItemOption(nil), item.Options...), Allergens: cloneStrings(item.Allergens),
				DietaryTags: cloneStrings(item.DietaryTags), IsAvailable: item.IsAvailable, Orderable: entity.Orderable,
				SortOrder: item.SortOrder,
			})
		}
		out = append(out, waiterPromptMenuCategory{
			ID: category.ID, Name: category.Name, Description: category.Description, Items: items, SortOrder: category.SortOrder,
		})
	}
	return out
}

func (snapshot WaiterMenuSnapshot) promptOffersProjection() []waiterPromptOffer {
	out := make([]waiterPromptOffer, 0, len(snapshot.visibleOffers))
	for _, offer := range snapshot.visibleOffers {
		entity := snapshot.ByKey[WaiterMenuEntityKey{Type: waiterMenuEntityTypeOffer, ID: strconv.FormatUint(uint64(offer.ID), 10)}]
		out = append(out, waiterPromptOffer{
			ID: offer.ID, Name: offer.Name, Description: offer.Description, Image: offer.Image,
			DiscountType: offer.DiscountType, DiscountValue: offer.DiscountValue, ApplicableTo: offer.ApplicableTo,
			TargetID: cloneStringPointer(offer.TargetID), Code: cloneStringPointer(offer.Code), Available: entity.Available,
		})
	}
	return out
}

func (snapshot WaiterMenuSnapshot) promptBundlesProjection(refsByID map[uint][]database.BundleItemRef) []waiterPromptBundle {
	out := make([]waiterPromptBundle, 0, len(snapshot.visibleBundles))
	for _, bundle := range snapshot.visibleBundles {
		entity := snapshot.ByKey[WaiterMenuEntityKey{Type: waiterMenuEntityTypeBundle, ID: strconv.FormatUint(uint64(bundle.ID), 10)}]
		out = append(out, waiterPromptBundle{
			ID: bundle.ID, Name: bundle.Name, Description: bundle.Description, Price: bundle.Price,
			Currency: bundle.Currency, Image: bundle.Image, Items: append([]database.BundleItemRef(nil), refsByID[bundle.ID]...),
			Available: entity.Available, Orderable: entity.Orderable,
		})
	}
	return out
}

func cloneWaiterMenuCategories(categories []database.MenuCategory) []database.MenuCategory {
	out := make([]database.MenuCategory, len(categories))
	for index, category := range categories {
		out[index] = category
		out[index].Items = make([]database.MenuItem, len(category.Items))
		for itemIndex, item := range category.Items {
			out[index].Items[itemIndex] = cloneWaiterMenuItem(item)
		}
	}
	return out
}

func cloneWaiterMenuItem(item database.MenuItem) database.MenuItem {
	item.Images = cloneStrings(item.Images)
	item.Options = append([]database.MenuItemOption(nil), item.Options...)
	item.Allergens = cloneStrings(item.Allergens)
	item.DietaryTags = cloneStrings(item.DietaryTags)
	return item
}

func cloneWaiterOffers(offers []database.Offer) []database.Offer {
	out := make([]database.Offer, len(offers))
	for index, offer := range offers {
		out[index] = cloneWaiterOffer(offer)
	}
	return out
}

func cloneWaiterOffer(offer database.Offer) database.Offer {
	offer.TargetID = cloneStringPointer(offer.TargetID)
	offer.Code = cloneStringPointer(offer.Code)
	return offer
}

func cloneWaiterBundles(bundles []database.Bundle) []database.Bundle {
	out := make([]database.Bundle, len(bundles))
	for index, bundle := range bundles {
		out[index] = cloneWaiterBundle(bundle)
	}
	return out
}

func cloneWaiterBundle(bundle database.Bundle) database.Bundle {
	return bundle
}

func cloneWaiterMenuEntity(entity WaiterMenuEntity) WaiterMenuEntity {
	entity.Allergens = cloneStrings(entity.Allergens)
	entity.DietaryTags = cloneStrings(entity.DietaryTags)
	return entity
}

func cloneStrings(values []string) []string {
	return append([]string(nil), values...)
}

func cloneStringPointer(value *string) *string {
	if value == nil {
		return nil
	}
	copy := *value
	return &copy
}
