package server

import (
	"fmt"
	"log"
	"strings"
	"sync"

	"github.com/stdevmac/payverge/backend/internal/logger"
)

// menuTranslationBackfillInFlight de-duplicates concurrent storefront backfills.
// Without it, every guest viewing an as-yet-untranslated menu in the same
// language would spawn its own translation job, hammering the Google Translate
// API and writing duplicate rows. Key is "businessID:languageCode".
var menuTranslationBackfillInFlight sync.Map

// enqueueMenuTranslationBackfill is the operator/guest missing-translation
// hook. Tests replace it to assert the operator read path no longer discards
// the missing flag.
var enqueueMenuTranslationBackfill = scheduleMenuTranslationBackfill

func menuTranslationSourceLanguage(defaultLanguage string) string {
	source := strings.TrimSpace(defaultLanguage)
	if source == "" {
		return "en"
	}
	return source
}

func persistableTranslation(original, translated string) bool {
	translated = strings.TrimSpace(translated)
	original = strings.TrimSpace(original)
	return translated != "" && translated != original
}

// scheduleMenuTranslationBackfill kicks off a one-shot, de-duplicated background
// translation of the whole menu into languageCode when the guest read path found
// missing menu translations. It mirrors backfillPromotionTranslationsForLanguage
// but reuses the batch translation path, so storefront menus self-heal for
// businesses that added a guest language before automatic translation existed.
func scheduleMenuTranslationBackfill(businessID uint, languageCode string) {
	if !IsBatchTranslationEnabled() {
		return
	}

	code := strings.TrimSpace(languageCode)
	if code == "" {
		return
	}

	key := fmt.Sprintf("%d:%s", businessID, code)
	if _, alreadyRunning := menuTranslationBackfillInFlight.LoadOrStore(key, struct{}{}); alreadyRunning {
		return
	}

	logger.SafeGo(func() {
		defer menuTranslationBackfillInFlight.Delete(key)
		if err := translateMenuIntoLanguages(businessID, []string{code}, nil); err != nil {
			log.Printf("Menu translation backfill failed for business %d language %s: %v", businessID, code, err)
		}
	})
}
