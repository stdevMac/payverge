package services

import (
	"fmt"
	"strings"

	"github.com/stdevmac/payverge/backend/internal/config"
	"github.com/stdevmac/payverge/backend/internal/database"
	"github.com/stdevmac/payverge/backend/internal/emails"
	"github.com/stdevmac/payverge/backend/internal/locales"
)

func DetermineBusinessOwnerLanguage(business *database.Business) string {
	if business == nil {
		return emails.LanguageEnglish
	}

	if business.UserID != nil {
		if user, err := database.GetUserByID(*business.UserID); err == nil && strings.TrimSpace(user.LanguageSelected) != "" {
			return user.LanguageSelected
		}
	}

	if strings.TrimSpace(business.OwnerAddress) != "" {
		if user, err := database.GetUserByAddress(strings.ToLower(business.OwnerAddress)); err == nil && strings.TrimSpace(user.LanguageSelected) != "" {
			return user.LanguageSelected
		}
	}

	if strings.TrimSpace(business.Email) != "" {
		if user, err := database.GetUserByEmail(business.Email); err == nil && strings.TrimSpace(user.LanguageSelected) != "" {
			return user.LanguageSelected
		}
	}

	if strings.TrimSpace(business.DefaultLanguage) != "" {
		return business.DefaultLanguage
	}

	return emails.LanguageEnglish
}

// resolveOwnerEmailLanguage maps any business/owner language code onto the
// transactional-email template family that actually ships on disk. Only the
// three families eng/es/es_ar have templates (see emails.templateLanguageFamilies);
// es-AR resolves to the Rioplatense "es_ar" family, neutral Spanish to "es",
// and everything else (unknown/empty/locale-without-templates) to the English
// "eng" family. Mirrors handlers.normalizeGiftLanguage so all email paths agree.
func resolveOwnerEmailLanguage(language string) string {
	lang := strings.TrimSpace(language)
	if loc, ok := locales.Lookup(lang); ok && (loc.EmailFamily == "es" || loc.EmailFamily == "es_ar") {
		return loc.EmailFamily
	}
	// Defensive: tolerate casing/underscore variants the exact registry lookup
	// misses (operator UI stores canonical codes, but be safe).
	switch strings.ToLower(strings.ReplaceAll(lang, "_", "-")) {
	case "es-ar":
		return "es_ar"
	case "es":
		return "es"
	}
	return "eng"
}

// BusinessOwnerEmailLanguage returns the transactional-email template family for
// an owner-facing email to the given business. It sources the language from the
// owner's UI locale (DetermineBusinessOwnerLanguage — User.LanguageSelected first,
// then business.DefaultLanguage, then English) and resolves it to a shipping
// family via resolveOwnerEmailLanguage.
//
// EMAIL-2: owner-facing lifecycle/milestone senders previously keyed on
// business.DefaultLanguage (the *customer* display language), so an operator
// whose account language is es-AR got the wrong-language email. This unifies
// them with onboarding/report/digest, which already use the owner's UI locale.
func BusinessOwnerEmailLanguage(business *database.Business) string {
	return resolveOwnerEmailLanguage(DetermineBusinessOwnerLanguage(business))
}

func BuildBusinessDashboardURL(business *database.Business) string {
	baseURL := config.FrontendBaseURL()
	if business == nil {
		return fmt.Sprintf("%s/business", baseURL)
	}
	if strings.TrimSpace(business.BusinessId) != "" {
		return fmt.Sprintf("%s/business/%s/dashboard", baseURL, business.BusinessId)
	}
	return fmt.Sprintf("%s/business/%d/dashboard", baseURL, business.ID)
}

func SendBusinessOnboardingEmail(business *database.Business) error {
	if business == nil || strings.TrimSpace(business.Email) == "" || emails.EmailServerInstance == nil {
		return nil
	}

	ownerName := strings.TrimSpace(business.OwnerName)
	if ownerName == "" {
		ownerName = strings.TrimSpace(business.Name)
	}

	return emails.EmailServerInstance.SendBusinessOnboardingEmail(
		[]string{business.Email},
		ownerName,
		BuildBusinessDashboardURL(business),
		DetermineBusinessOwnerLanguage(business),
	)
}

// SendAdminNewSignupEmailForBusiness alerts the instance admin inbox
// (emails.AdminsEmails) whenever a new business is created. Non-fatal by construction — it is always called via `log.Printf` on
// error by the two creation-path callers, never propagated as a creation
// failure.
func SendAdminNewSignupEmailForBusiness(business *database.Business) error {
	if business == nil || emails.EmailServerInstance == nil {
		return nil
	}

	return emails.EmailServerInstance.SendAdminNewSignupEmail(emails.AdminNewSignupInfo{
		BusinessName: business.Name,
		OwnerName:    business.OwnerName,
		OwnerEmail:   business.Email,
		Country:      business.Address.Country,
		BusinessType: business.BusinessType,
		DashboardURL: BuildBusinessDashboardURL(business),
	})
}
