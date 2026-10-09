package demo

import (
	"strings"

	"github.com/stdevmac/payverge/backend/internal/database"
)

// demoLocale is a coarse language family for seeded demo copy. Demos enable
// English + Spanish menu translations; operator-facing seed (director chat,
// inventory categories) follows the business default language so an es UI
// does not show English badges and thread titles (MIN-4 / MIN-10).
func demoLocale(business *database.Business) string {
	if business == nil {
		return "en"
	}
	code := strings.ToLower(strings.TrimSpace(business.DefaultLanguage))
	if code == "" {
		code = strings.ToLower(strings.TrimSpace(business.SourceLanguage))
	}
	code = strings.ReplaceAll(code, "_", "-")
	if strings.HasPrefix(code, "es") {
		return "es"
	}
	return "en"
}

// demoOperatorLocale resolves the language family for OPERATOR-facing demo
// seed copy (the pinned Director briefing card). #820: the Director Console is
// an owner surface, so the demo owner's UI language (User.LanguageSelected)
// wins over business.DefaultLanguage — that field describes the GUEST menu
// tier, and an es guest menu must not pin a voseo Sage card onto an
// English-operating owner. Falls back to demoLocale (business language) only
// when the operator has no stored UI language.
func demoOperatorLocale(operatorLanguage string, business *database.Business) string {
	code := strings.ToLower(strings.TrimSpace(operatorLanguage))
	code = strings.ReplaceAll(code, "_", "-")
	if strings.HasPrefix(code, "es") {
		return "es"
	}
	if code != "" {
		return "en"
	}
	return demoLocale(business)
}

type directorDemoCopy struct {
	ThreadTitle string
	Locale      string
	Content     string
	Structured  string
}

func directorDemoSeed(locale string) directorDemoCopy {
	if locale == "es" {
		return directorDemoCopy{
			ThreadTitle: "Informe del Director de demostración",
			Locale:      "es",
			Content:     "Los ingresos de la cena superan la mediana de 30 días. Impulsá cócteles antes de las 20:00.",
			Structured:  `{"summary":"Los ingresos de la cena superan la mediana de 30 días.","diagnosis":"El attach de cócteles sube y la mano de obra está en objetivo, así que el alza es de servicio, no de descuentos.","expected_impact":"Destacar cócteles antes de las 20:00 debería extender la tendencia hasta el fin de semana."}`,
		}
	}
	return directorDemoCopy{
		ThreadTitle: "Demo Director Briefing",
		Locale:      "en",
		Content:     "Dinner revenue is tracking above the 30-day median. Push cocktails before 8pm.",
		Structured:  `{"summary":"Dinner revenue is tracking above the 30-day median.","diagnosis":"Cocktail attach is up and labor is on target, so the lift is service-driven rather than discount-driven.","expected_impact":"Featuring cocktails before 8pm should extend the trend through the weekend."}`,
	}
}

// inventoryCategoryLabel returns a localized free-text category badge for demo
// inventory rows (MIN-10). Categories are free-text on InventoryItem, so seed
// must write the operator-facing language directly.
func inventoryCategoryLabel(locale, en string) string {
	if locale != "es" {
		return en
	}
	switch en {
	case "Produce":
		return "Verduras"
	case "Protein":
		return "Proteína"
	case "Beverage":
		return "Bebidas"
	case "Dairy":
		return "Lácteos"
	default:
		return en
	}
}
