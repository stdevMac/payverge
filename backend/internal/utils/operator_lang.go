package utils

import "strings"

// NormalizeOperatorLang maps Accept-Language / ?lang= values to the operator
// CSV locale family: "es" for any Spanish variant, otherwise "en".
func NormalizeOperatorLang(lang string) string {
	l := strings.ToLower(strings.TrimSpace(lang))
	l = strings.ReplaceAll(l, "_", "-")
	if l == "es" || strings.HasPrefix(l, "es-") {
		return "es"
	}
	return "en"
}
