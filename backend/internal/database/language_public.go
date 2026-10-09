package database

import "sync"

// PublicSupportedLanguage is the storefront language-picker projection:
// code / name / native_name only. id, is_active, created_at, and updated_at
// are unused on the guest landing page (#566).
type PublicSupportedLanguage struct {
	Code       string `json:"code"`
	Name       string `json:"name"`
	NativeName string `json:"native_name"`
}

var (
	publicSupportedLangMu     sync.RWMutex
	publicSupportedLangCache  []PublicSupportedLanguage
	publicSupportedLangLoaded bool
)

func init() {
	RegisterOnDBChange(InvalidatePublicSupportedLanguages)
}

// InvalidatePublicSupportedLanguages drops the in-process catalogue cache.
// Call after InitializeDefaultLanguages or any supported_languages write.
func InvalidatePublicSupportedLanguages() {
	publicSupportedLangMu.Lock()
	publicSupportedLangCache = nil
	publicSupportedLangLoaded = false
	publicSupportedLangMu.Unlock()
}

// GetPublicBusinessLanguages is the bounded, projected business-language
// read for GET /business/:customUrl. It never selects the embedded Business
// row (json:"-" on the model; this Select is the query-shape half).
func GetPublicBusinessLanguages(businessID uint) ([]BusinessLanguage, error) {
	var languages []BusinessLanguage
	err := GetDBWrapper().GetGorm().
		Select(publicStorefrontBusinessLanguageColumns).
		Where("business_id = ?", businessID).
		Order("is_default DESC, display_order ASC").
		Limit(PublicStorefrontLanguageLimit).
		Find(&languages).Error
	if ignoreMissingRelation(err) != nil {
		return nil, err
	}
	if languages == nil {
		languages = []BusinessLanguage{}
	}
	return languages, nil
}

// GetCachedPublicSupportedLanguages returns the platform language catalogue
// projected to picker fields. The first call hits the DB; later calls reuse
// the process cache until InvalidatePublicSupportedLanguages.
func GetCachedPublicSupportedLanguages() ([]PublicSupportedLanguage, error) {
	publicSupportedLangMu.RLock()
	if publicSupportedLangLoaded {
		out := append([]PublicSupportedLanguage(nil), publicSupportedLangCache...)
		publicSupportedLangMu.RUnlock()
		return out, nil
	}
	publicSupportedLangMu.RUnlock()

	publicSupportedLangMu.Lock()
	defer publicSupportedLangMu.Unlock()
	if publicSupportedLangLoaded {
		return append([]PublicSupportedLanguage(nil), publicSupportedLangCache...), nil
	}

	var rows []SupportedLanguage
	err := GetDBWrapper().GetGorm().
		Select(publicStorefrontSupportedLanguageColumns).
		Where("is_active = ?", true).
		Order("name").
		Limit(PublicSupportedLanguageLimit).
		Find(&rows).Error
	if ignoreMissingRelation(err) != nil {
		return nil, err
	}

	out := make([]PublicSupportedLanguage, 0, len(rows))
	for _, row := range rows {
		out = append(out, PublicSupportedLanguage{
			Code:       row.Code,
			Name:       row.Name,
			NativeName: row.NativeName,
		})
	}
	publicSupportedLangCache = out
	publicSupportedLangLoaded = true
	return append([]PublicSupportedLanguage(nil), out...), nil
}
