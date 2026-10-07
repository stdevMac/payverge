package server

import (
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"errors"
	"fmt"
	"log"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/stdevmac/payverge/backend/internal/database"
	"github.com/stdevmac/payverge/backend/internal/locales"
	"github.com/stdevmac/payverge/backend/internal/logger"
	"gorm.io/gorm"
)

const (
	offerTranslationEntityType  = "offer"
	bundleTranslationEntityType = "bundle"
)

// promotionTranslationBackfillInFlight dedups concurrent offer/bundle
// translation backfills per (business, language), mirroring
// menuTranslationBackfillInFlight. Without it, the guest read path spawned a
// raw goroutine on every request with missing promotion translations, so a
// burst of guests on the same untranslated language created a thundering herd
// of duplicate goroutines + duplicate translation-API calls.
var promotionTranslationBackfillInFlight sync.Map

// schedulePromotionTranslationBackfill launches backfillPromotionTranslationsForLanguage
// at most once per (business, language) at a time, under SafeGo so a translation
// panic can't crash the process (the old raw `go` had no recovery).
func schedulePromotionTranslationBackfill(businessID uint, sourceLanguage, languageCode string, offers []database.Offer, bundles []database.Bundle) {
	code := strings.TrimSpace(languageCode)
	if code == "" || (len(offers) == 0 && len(bundles) == 0) {
		return
	}

	key := fmt.Sprintf("%d:%s", businessID, code)
	if _, alreadyRunning := promotionTranslationBackfillInFlight.LoadOrStore(key, struct{}{}); alreadyRunning {
		return
	}

	logger.SafeGo(func() {
		defer promotionTranslationBackfillInFlight.Delete(key)
		backfillPromotionTranslationsForLanguage(sourceLanguage, code, offers, bundles)
	})
}

// waitForPromotionTranslationBackfills drains in-flight offer/bundle backfill
// goroutines so tests can swap the global DB without racing GetDBWrapper.
func waitForPromotionTranslationBackfills(timeout time.Duration) {
	deadline := time.Now().Add(timeout)
	for {
		empty := true
		promotionTranslationBackfillInFlight.Range(func(_, _ any) bool {
			empty = false
			return false
		})
		if empty {
			return
		}
		if time.Now().After(deadline) {
			return
		}
		time.Sleep(5 * time.Millisecond)
	}
}

type promotionTranslationService interface {
	TranslateText(text string, targetLanguages []string) (map[string]string, error)
	TranslateTextWithSource(text, sourceLanguage string, targetLanguages []string) (map[string]string, error)
}

func applyTranslationsToOffers(offers []database.Offer, languageCode string) ([]database.Offer, bool) {
	return applyTranslationsToOffersWithLookup(offers, languageCode, nil)
}

func applyTranslationsToOffersWithLookup(offers []database.Offer, languageCode string, sharedLookup *translationLookup) ([]database.Offer, bool) {
	translated := make([]database.Offer, len(offers))
	missingTranslations := false
	dbw := database.GetDBWrapper()
	if strings.TrimSpace(languageCode) == "" || (dbw == nil && sharedLookup == nil) {
		copy(translated, offers)
		return translated, false
	}

	// Batch the lookup per business (one query each; a single business in every
	// caller today) instead of two per-field queries per offer — same N+1
	// elimination as applyTranslationsToMenu, on the same hot guest path.
	var lookupFor func(uint) *translationLookup
	if sharedLookup == nil {
		lookupFor = newBusinessTranslationLookups(dbw, languageCode)
	}
	for i, offer := range offers {
		translatedOffer := offer
		lookup := sharedLookup
		if lookup == nil {
			lookup = lookupFor(offer.BusinessID)
		}

		if strings.TrimSpace(offer.Name) != "" {
			if value, ok := lookup.getFresh(offerTranslationEntityType, offer.ID, "name", offer.Name); ok {
				translatedOffer.Name = value
			} else {
				missingTranslations = true
			}
		}
		if strings.TrimSpace(offer.Description) != "" {
			if value, ok := lookup.getFresh(offerTranslationEntityType, offer.ID, "description", offer.Description); ok {
				translatedOffer.Description = value
			} else {
				missingTranslations = true
			}
		}

		translated[i] = translatedOffer
	}

	return translated, missingTranslations
}

// newBusinessTranslationLookups returns a memoised factory that builds (and
// caches) one batched translationLookup per businessID for the given language.
// Callers that operate on a single business's entities therefore issue exactly
// one translation query.
func newBusinessTranslationLookups(dbw *database.DB, languageCode string) func(uint) *translationLookup {
	cache := make(map[uint]*translationLookup)
	return func(businessID uint) *translationLookup {
		if l, ok := cache[businessID]; ok {
			return l
		}
		l := newTranslationLookup(dbw, businessID, languageCode)
		cache[businessID] = l
		return l
	}
}

func applyTranslationsToBundles(bundles []database.Bundle, languageCode string) ([]database.Bundle, bool) {
	return applyTranslationsToBundlesWithLookup(bundles, languageCode, nil)
}

func applyTranslationsToBundlesWithLookup(bundles []database.Bundle, languageCode string, sharedLookup *translationLookup) ([]database.Bundle, bool) {
	translated := make([]database.Bundle, len(bundles))
	missingTranslations := false
	dbw := database.GetDBWrapper()
	if strings.TrimSpace(languageCode) == "" || (dbw == nil && sharedLookup == nil) {
		copy(translated, bundles)
		return translated, false
	}

	var lookupFor func(uint) *translationLookup
	if sharedLookup == nil {
		lookupFor = newBusinessTranslationLookups(dbw, languageCode)
	}
	for i, bundle := range bundles {
		translatedBundle := bundle
		lookup := sharedLookup
		if lookup == nil {
			lookup = lookupFor(bundle.BusinessID)
		}

		if strings.TrimSpace(bundle.Name) != "" {
			if value, ok := lookup.getFresh(bundleTranslationEntityType, bundle.ID, "name", bundle.Name); ok {
				translatedBundle.Name = value
			} else {
				missingTranslations = true
			}
		}
		if strings.TrimSpace(bundle.Description) != "" {
			if value, ok := lookup.getFresh(bundleTranslationEntityType, bundle.ID, "description", bundle.Description); ok {
				translatedBundle.Description = value
			} else {
				missingTranslations = true
			}
		}

		translated[i] = translatedBundle
	}

	return translated, missingTranslations
}

func getLatestTranslation(dbw *database.DB, businessID uint, entityType string, entityID uint, fieldName, languageCode string) (*database.Translation, bool, error) {
	if dbw == nil || dbw.GetGorm() == nil {
		return nil, false, errors.New("translation database unavailable")
	}
	var translation database.Translation
	err := dbw.GetGorm().
		Where("business_id = ? AND entity_type = ? AND entity_id = ? AND field_name = ? AND language_code = ?",
			businessID, entityType, entityID, fieldName, languageCode).
		Order("id DESC").
		First(&translation).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, false, nil
	}
	if err != nil {
		return nil, false, err
	}
	return &translation, true, nil
}

// translationLookup is an in-memory snapshot of one business's translations for a
// single language. It replaces the per-field getLatestTranslationText DB query —
// which, on the hot guest menu path (applyTranslationsToMenu), fired once per
// category/item name+description and once per option/allergen/tag, i.e. an N+1 of
// ~2·(categories+items) + options + allergens + tags queries per menu render in a
// non-default language (the common case for the 21-locale guest tier). The whole
// set is now loaded in one batched read and resolved from a map.
type translationLookup struct {
	byKey         map[string]string
	ambiguousKeys map[string]struct{}
	// revision is the full SHA-256 content revision for the effective
	// business+language snapshot. It is empty when no relevant rows exist.
	revision string
	err      error
	// origByKey holds the OriginalText the stored translation was generated from,
	// so the read path can detect that the source text at a given position has
	// since changed (e.g. after a reorder or an in-place edit) and treat the
	// stale translation as missing instead of serving it. Keyed identically to
	// byKey.
	origByKey map[string]string
}

type translationLookupEntity struct {
	entityType string
	entityID   uint
}

// translationLookupScope bounds a guest lookup to the entities present in the
// response. The query still resolves only the latest row per field in SQL, so
// historical duplicates never enter the process or the digest.
type translationLookupScope struct {
	entitiesByKey         map[string]translationLookupEntity
	targetKeysByEntityKey map[string]map[string]struct{}
}

func (s *translationLookupScope) add(entityType string, entityID uint) {
	s.addForTarget(entityType, entityID, fmt.Sprintf("%s|%d", entityType, entityID))
}

func (s *translationLookupScope) addForTarget(entityType string, entityID uint, targetKey string) {
	if s.entitiesByKey == nil {
		s.entitiesByKey = make(map[string]translationLookupEntity)
	}
	if s.targetKeysByEntityKey == nil {
		s.targetKeysByEntityKey = make(map[string]map[string]struct{})
	}
	key := translationLookupEntityKey(entityType, entityID)
	s.entitiesByKey[key] = translationLookupEntity{entityType: entityType, entityID: entityID}
	if s.targetKeysByEntityKey[key] == nil {
		s.targetKeysByEntityKey[key] = make(map[string]struct{})
	}
	s.targetKeysByEntityKey[key][targetKey] = struct{}{}
}

func (s translationLookupScope) ambiguousKeys() map[string]struct{} {
	ambiguous := make(map[string]struct{})
	for key, targetKeys := range s.targetKeysByEntityKey {
		if len(targetKeys) > 1 {
			ambiguous[key] = struct{}{}
		}
	}
	return ambiguous
}

func (s translationLookupScope) entities() []translationLookupEntity {
	entities := make([]translationLookupEntity, 0, len(s.entitiesByKey))
	for _, entity := range s.entitiesByKey {
		entities = append(entities, entity)
	}
	sort.Slice(entities, func(i, j int) bool {
		if entities[i].entityType == entities[j].entityType {
			return entities[i].entityID < entities[j].entityID
		}
		return entities[i].entityType < entities[j].entityType
	})
	return entities
}

func menuTranslationLookupScope(categories []database.MenuCategory) translationLookupScope {
	var scope translationLookupScope
	for categoryIndex, category := range categories {
		categoryPositionID := uint(categoryIndex)
		categoryTargetKey := fmt.Sprintf("category:%d", categoryIndex)
		scope.addForTarget("category", categoryPositionID, categoryTargetKey)
		addNumericMenuEntityIDForTarget(&scope, "category", category.ID, categoryTargetKey)
		for itemIndex, item := range category.Items {
			entityID := uint(categoryIndex*1000 + itemIndex)
			itemTargetKey := fmt.Sprintf("menu_item:%d:%d", categoryIndex, itemIndex)
			scope.addForTarget("menu_item", entityID, itemTargetKey)
			addNumericMenuEntityIDForTarget(&scope, "menu_item", item.ID, itemTargetKey)
			for optionIndex := range item.Options {
				optionEntityID := entityID*1000 + uint(optionIndex)
				optionTargetKey := fmt.Sprintf("menu_item_option:%d:%d:%d", categoryIndex, itemIndex, optionIndex)
				scope.addForTarget("menu_item_option", optionEntityID, optionTargetKey)
				addNumericMenuEntityIDForTarget(&scope, "menu_item_option", item.Options[optionIndex].ID, optionTargetKey)
			}
			for allergenIndex := range item.Allergens {
				scope.addForTarget("allergen", entityID*10000+uint(allergenIndex), fmt.Sprintf("allergen:%d:%d:%d", categoryIndex, itemIndex, allergenIndex))
			}
			for tagIndex := range item.DietaryTags {
				scope.addForTarget("dietary_tag", entityID*100000+uint(tagIndex), fmt.Sprintf("dietary_tag:%d:%d:%d", categoryIndex, itemIndex, tagIndex))
			}
		}
	}
	return scope
}

// addNumericMenuEntityIDForTarget keeps the legacy positional translation keys working
// while also admitting rows keyed by the persisted numeric entity ID. Menu
// category/item/option IDs are strings on the wire for historical reasons, so
// only numeric IDs can be represented by translations.EntityID (uint). A
// non-numeric ID remains addressable through the positional key used by the
// existing writers.
func addNumericMenuEntityIDForTarget(scope *translationLookupScope, entityType, rawID, targetKey string) {
	parsed, err := strconv.ParseUint(strings.TrimSpace(rawID), 10, strconv.IntSize)
	if err != nil {
		return
	}
	scope.addForTarget(entityType, uint(parsed), targetKey)
}

func numericMenuEntityID(rawID string) (uint, bool) {
	parsed, err := strconv.ParseUint(strings.TrimSpace(rawID), 10, strconv.IntSize)
	if err != nil {
		return 0, false
	}
	return uint(parsed), true
}

func guestMenuTranslationLookupScope(categories []database.MenuCategory, offers []database.Offer, bundles []database.Bundle) translationLookupScope {
	scope := menuTranslationLookupScope(categories)
	for _, offer := range offers {
		scope.add(offerTranslationEntityType, offer.ID)
	}
	for _, bundle := range bundles {
		scope.add(bundleTranslationEntityType, bundle.ID)
	}
	return scope
}

func translationLookupEntityKey(entityType string, entityID uint) string {
	return fmt.Sprintf("%s|%d", entityType, entityID)
}

func translationLookupKey(entityType string, entityID uint, fieldName string) string {
	return translationLookupEntityKey(entityType, entityID) + "|" + fieldName
}

// newTranslationLookup loads only the effective row for each translation
// field: the highest-id non-identity row when one exists, otherwise the
// highest-id row (so a proper-noun echo can still stand). When a scope is
// supplied, only entities represented by the current response are read;
// this keeps the public guest path bounded by menu size rather than
// historical translation-row count.
//
// languageCode is expanded through locales.TranslationFallbackChain so a
// guest request for es-AR reuses existing es rows when es-AR rows are
// missing. More-specific rows still win when both exist.
func newTranslationLookup(dbw *database.DB, businessID uint, languageCode string, scopes ...translationLookupScope) *translationLookup {
	lookup := &translationLookup{
		byKey:         make(map[string]string),
		ambiguousKeys: make(map[string]struct{}),
		origByKey:     make(map[string]string),
	}
	var scope *translationLookupScope
	if len(scopes) > 0 {
		scope = &scopes[0]
		if len(scope.entitiesByKey) == 0 {
			return lookup
		}
		for key := range scope.ambiguousKeys() {
			lookup.ambiguousKeys[key] = struct{}{}
		}
	}
	if dbw == nil || dbw.GetGorm() == nil {
		lookup.err = errors.New("translation database unavailable")
		return lookup
	}
	codes := locales.TranslationFallbackChain(languageCode)
	if len(codes) == 0 {
		return lookup
	}
	placeholders := strings.Repeat("?,", len(codes))
	placeholders = placeholders[:len(placeholders)-1]
	query := strings.Builder{}
	query.WriteString(`
		SELECT t.id, t.entity_type, t.entity_id, t.field_name,
		       t.language_code, t.translated_text, t.original_text, t.updated_at
		FROM translations AS t
		INNER JOIN (
			SELECT entity_type, entity_id, field_name, language_code, MAX(id) AS id
			FROM translations AS cand
			WHERE cand.business_id = ? AND cand.language_code IN (`)
	query.WriteString(placeholders)
	query.WriteString(")")
	args := []any{businessID}
	for _, code := range codes {
		args = append(args, code)
	}
	if scope != nil {
		query.WriteString(" AND (")
		for i, entity := range scope.entities() {
			if i > 0 {
				query.WriteString(" OR ")
			}
			query.WriteString("(cand.entity_type = ? AND cand.entity_id = ?)")
			args = append(args, entity.entityType, entity.entityID)
		}
		query.WriteString(")")
	}
	// Prefer a real translation over a newer identity echo of the source
	// (#514). Keep the identity row only when no non-identity sibling exists.
	query.WriteString(`
			AND (
				cand.original_text IS NULL
				OR TRIM(cand.original_text) = ''
				OR cand.translated_text <> cand.original_text
				OR NOT EXISTS (
					SELECT 1 FROM translations AS nx
					WHERE nx.business_id = cand.business_id
					  AND nx.entity_type = cand.entity_type
					  AND nx.entity_id = cand.entity_id
					  AND nx.field_name = cand.field_name
					  AND nx.language_code = cand.language_code
					  AND (
						nx.original_text IS NULL
						OR TRIM(nx.original_text) = ''
						OR nx.translated_text <> nx.original_text
					  )
				)
			)
			GROUP BY cand.entity_type, cand.entity_id, cand.field_name, cand.language_code
		) AS effective ON effective.id = t.id
		WHERE t.business_id = ? AND t.language_code IN (`)
	query.WriteString(placeholders)
	query.WriteString(`)
		ORDER BY t.id ASC`)
	args = append(args, businessID)
	for _, code := range codes {
		args = append(args, code)
	}

	var rows []translationSnapshotRow
	if err := dbw.GetGorm().Raw(query.String(), args...).Scan(&rows).Error; err != nil {
		lookup.err = fmt.Errorf("load translations for business %d language %s: %w", businessID, languageCode, err)
		return lookup
	}
	rank := make(map[string]int, len(codes))
	for i, code := range codes {
		rank[code] = i
	}
	type translationCandidate struct {
		rank int
		text string
		orig string
	}
	candidates := make(map[string][]translationCandidate, len(rows))
	for _, r := range rows {
		key := translationLookupKey(r.EntityType, r.EntityID, r.FieldName)
		rnk, known := rank[r.LanguageCode]
		if !known {
			continue
		}
		translated := strings.TrimSpace(r.TranslatedText)
		if translated == "" {
			continue
		}
		candidates[key] = append(candidates[key], translationCandidate{
			rank: rnk,
			text: translated,
			orig: strings.TrimSpace(r.OriginalText.String),
		})
	}
	isIdentity := func(c translationCandidate) bool {
		return c.orig != "" && c.text == c.orig
	}
	for key, list := range candidates {
		chosen := list[0]
		for _, c := range list[1:] {
			chosenIdent := isIdentity(chosen)
			candIdent := isIdentity(c)
			switch {
			case !candIdent && (chosenIdent || c.rank < chosen.rank):
				chosen = c
			case candIdent && chosenIdent && c.rank < chosen.rank:
				chosen = c
			}
		}
		lookup.byKey[key] = chosen.text
		lookup.origByKey[key] = chosen.orig
	}
	lookup.revision = translationRevisionDigest(rows)
	return lookup
}

type translationSnapshotRow struct {
	ID             uint
	EntityType     string
	EntityID       uint
	FieldName      string
	LanguageCode   string
	TranslatedText string
	OriginalText   sql.NullString
	UpdatedAt      sql.NullTime
}

func translationRevisionDigest(rows []translationSnapshotRow) string {
	if len(rows) == 0 {
		return ""
	}
	hash := sha256.New()
	for _, row := range rows {
		// PostgreSQL text cannot contain NUL, so these separators make the
		// serialized row tuple unambiguous while keeping hashing allocation-light.
		originalText := "null"
		if row.OriginalText.Valid {
			originalText = "text:" + row.OriginalText.String
		}
		updatedAt := "null"
		if row.UpdatedAt.Valid {
			updatedAt = "time:" + strconv.FormatInt(row.UpdatedAt.Time.UTC().UnixNano(), 10)
		}
		_, _ = fmt.Fprintf(hash, "%d\x00%s\x00%d\x00%s\x00%s\x00%s\x00%s\x00%s\x00",
			row.ID, row.EntityType, row.EntityID, row.FieldName, row.LanguageCode,
			originalText, row.TranslatedText, updatedAt)
	}
	return hex.EncodeToString(hash.Sum(nil))
}

// get mirrors getLatestTranslationText's contract: a present, non-empty latest
// translation returns (text, true); a missing key or empty latest returns ("", false).
func (l *translationLookup) get(entityType string, entityID uint, fieldName string) (string, bool) {
	if _, ambiguous := l.ambiguousKeys[translationLookupEntityKey(entityType, entityID)]; ambiguous {
		return "", false
	}
	text, ok := l.byKey[translationLookupKey(entityType, entityID, fieldName)]
	if !ok || text == "" {
		return "", false
	}
	return text, true
}

// getFresh is get() plus a staleness guard: it only reports a hit when the
// translation row's stored OriginalText still matches the current source text.
// Menu translations are keyed by POSITION (categoryIndex*1000+itemIndex), so a
// reorder or an in-place text edit leaves a translation attached to a position
// whose source has changed. Comparing OriginalText lets the read path treat
// that mismatch as MISSING — which flags the menu for backfill and self-heals —
// instead of serving item A's translated name on item B forever.
func (l *translationLookup) getFresh(entityType string, entityID uint, fieldName, currentSource string) (string, bool) {
	key := translationLookupKey(entityType, entityID, fieldName)
	if _, ambiguous := l.ambiguousKeys[translationLookupEntityKey(entityType, entityID)]; ambiguous {
		return "", false
	}
	text, ok := l.byKey[key]
	if !ok || text == "" {
		return "", false
	}
	original := strings.TrimSpace(l.origByKey[key])
	if original != strings.TrimSpace(currentSource) {
		return "", false
	}
	if original != "" && strings.TrimSpace(text) == original {
		return "", false
	}
	return text, true
}

func autoTranslateOffer(businessID, offerID uint, name, description string) error {
	return autoTranslatePromotionEntity(businessID, offerTranslationEntityType, offerID, name, description)
}

func autoTranslateBundle(businessID, bundleID uint, name, description string) error {
	return autoTranslatePromotionEntity(businessID, bundleTranslationEntityType, bundleID, name, description)
}

func autoTranslatePromotionEntity(businessID uint, entityType string, entityID uint, name, description string) error {
	dbw := database.GetDBWrapper()
	if dbw == nil {
		return errors.New("database wrapper not available")
	}

	translationService := GetTranslationService()
	if translationService == nil {
		return errors.New("translation service not available")
	}

	business, err := database.GetBusinessByID(businessID)
	if err != nil {
		return fmt.Errorf("failed to load business default language: %w", err)
	}
	sourceLanguage := strings.TrimSpace(business.DefaultLanguage)
	if sourceLanguage == "" {
		sourceLanguage = "en"
	}

	businessLanguages, err := dbw.LanguageService.GetBusinessLanguages(businessID)
	if err != nil {
		return fmt.Errorf("failed to get business languages: %w", err)
	}
	if len(businessLanguages) == 0 {
		return nil
	}

	for _, language := range businessLanguages {
		if language.IsDefault {
			continue
		}

		targetLanguage := strings.TrimSpace(language.LanguageCode)
		if targetLanguage == "" {
			continue
		}

		if err := translatePromotionField(dbw, translationService, businessID, entityType, entityID, "name", name, sourceLanguage, targetLanguage); err != nil {
			log.Printf("Failed translating %s name entity %d to %s: %v", entityType, entityID, targetLanguage, err)
		}
		if err := translatePromotionField(dbw, translationService, businessID, entityType, entityID, "description", description, sourceLanguage, targetLanguage); err != nil {
			log.Printf("Failed translating %s description entity %d to %s: %v", entityType, entityID, targetLanguage, err)
		}
	}

	return nil
}

func backfillPromotionTranslationsForLanguage(sourceLanguage, languageCode string, offers []database.Offer, bundles []database.Bundle) {
	targetLanguage := strings.TrimSpace(languageCode)
	if targetLanguage == "" {
		return
	}

	dbw := database.GetDBWrapper()
	if dbw == nil {
		return
	}
	translationService := GetTranslationService()
	if translationService == nil {
		return
	}

	source := strings.TrimSpace(sourceLanguage)
	if source == "" {
		source = "en"
	}

	for _, offer := range offers {
		if err := translatePromotionField(dbw, translationService, offer.BusinessID, offerTranslationEntityType, offer.ID, "name", offer.Name, source, targetLanguage); err != nil {
			log.Printf("Offer translation backfill failed for offer %d name: %v", offer.ID, err)
		}
		if err := translatePromotionField(dbw, translationService, offer.BusinessID, offerTranslationEntityType, offer.ID, "description", offer.Description, source, targetLanguage); err != nil {
			log.Printf("Offer translation backfill failed for offer %d description: %v", offer.ID, err)
		}
	}

	for _, bundle := range bundles {
		if err := translatePromotionField(dbw, translationService, bundle.BusinessID, bundleTranslationEntityType, bundle.ID, "name", bundle.Name, source, targetLanguage); err != nil {
			log.Printf("Bundle translation backfill failed for bundle %d name: %v", bundle.ID, err)
		}
		if err := translatePromotionField(dbw, translationService, bundle.BusinessID, bundleTranslationEntityType, bundle.ID, "description", bundle.Description, source, targetLanguage); err != nil {
			log.Printf("Bundle translation backfill failed for bundle %d description: %v", bundle.ID, err)
		}
	}
}

func translatePromotionField(
	dbw *database.DB,
	translationService promotionTranslationService,
	businessID uint,
	entityType string,
	entityID uint,
	fieldName string,
	originalValue string,
	sourceLanguage string,
	targetLanguage string,
) error {
	original := strings.TrimSpace(originalValue)
	if original == "" {
		return nil
	}

	existing, found, err := getLatestTranslation(dbw, businessID, entityType, entityID, fieldName, targetLanguage)
	if err != nil {
		return fmt.Errorf("load existing %s translation: %w", entityType, err)
	}
	if found && strings.TrimSpace(existing.TranslatedText) != "" && strings.TrimSpace(existing.OriginalText) == original {
		return nil
	}

	translatedText, err := translateTextWithSourceFallback(translationService, original, sourceLanguage, targetLanguage)
	if err != nil {
		return err
	}
	if !persistableTranslation(original, translatedText) {
		return nil
	}

	translation := &database.Translation{
		BusinessID:        businessID,
		EntityType:        entityType,
		EntityID:          entityID,
		FieldName:         fieldName,
		LanguageCode:      targetLanguage,
		OriginalText:      original,
		TranslatedText:    translatedText,
		IsAutoTranslated:  true,
		TranslationSource: "google_translate",
	}
	return dbw.TranslationService.SaveTranslation(translation)
}

func translateTextWithSourceFallback(
	translationService promotionTranslationService,
	text string,
	sourceLanguage string,
	targetLanguage string,
) (string, error) {
	if strings.TrimSpace(text) == "" {
		return "", nil
	}

	if strings.TrimSpace(sourceLanguage) != "" {
		if translated, err := translationService.TranslateTextWithSource(text, sourceLanguage, []string{targetLanguage}); err == nil {
			if value := strings.TrimSpace(translated[targetLanguage]); value != "" {
				return value, nil
			}
		}
	}

	translated, err := translationService.TranslateText(text, []string{targetLanguage})
	if err != nil {
		return "", err
	}
	return translated[targetLanguage], nil
}

func deleteTranslationsForOffer(businessID, offerID uint) error {
	dbw := database.GetDBWrapper()
	if dbw == nil {
		return errors.New("database wrapper not available")
	}
	return dbw.GetGorm().
		Where("business_id = ? AND entity_type = ? AND entity_id = ?", businessID, offerTranslationEntityType, offerID).
		Delete(&database.Translation{}).Error
}

func deleteTranslationsForBundle(businessID, bundleID uint) error {
	dbw := database.GetDBWrapper()
	if dbw == nil {
		return errors.New("database wrapper not available")
	}
	return dbw.GetGorm().
		Where("business_id = ? AND entity_type = ? AND entity_id = ?", businessID, bundleTranslationEntityType, bundleID).
		Delete(&database.Translation{}).Error
}

// TranslateExistingPromotionContent translates every offer and bundle of the
// business into all configured non-default business languages. Idempotent at
// the field level (existing translation rows are skipped). Exported so the
// languages-update handler can fire it alongside the menu + business-content
// translation when an operator adds a guest language. (D4b)
func TranslateExistingPromotionContent(businessID uint) error {
	return translateExistingPromotionContent(businessID)
}

func translateExistingPromotionContent(businessID uint) error {
	offers, err := database.GetOffersByBusinessID(businessID)
	if err != nil {
		return fmt.Errorf("failed to load offers for translation: %w", err)
	}
	for _, offer := range offers {
		if err := autoTranslateOffer(businessID, offer.ID, offer.Name, offer.Description); err != nil {
			log.Printf("Failed translating offer %d: %v", offer.ID, err)
		}
	}

	bundles, err := database.GetBundlesByBusinessID(businessID)
	if err != nil {
		return fmt.Errorf("failed to load bundles for translation: %w", err)
	}
	for _, bundle := range bundles {
		if err := autoTranslateBundle(businessID, bundle.ID, bundle.Name, bundle.Description); err != nil {
			log.Printf("Failed translating bundle %d: %v", bundle.ID, err)
		}
	}

	return nil
}
