package services

import (
	"regexp"
	"strings"
	"unicode"
	"unicode/utf8"
)

var (
	demoMarketingHashtagPattern = regexp.MustCompile(`(?i)#payverge[\p{L}\p{N}_]*`)
	demoMarketingVenuePattern   = regexp.MustCompile(`(?i)payverge(?:\s+ai(?:\s+pro)?)?(?:\s+demo(?:\s+lounge)?)?`)
	demoMarketingPunctGap       = regexp.MustCompile(`\s+([.,!?])`)
)

// LooksLikeDemoMarketingVenue reports names that must not appear in public
// marketing copy (internal demo showrooms and leftover Payverge product brands).
func LooksLikeDemoMarketingVenue(name string, isDemo bool) bool {
	if isDemo {
		return true
	}
	folded := strings.ToLower(strings.TrimSpace(name))
	if folded == "" {
		return false
	}
	return strings.Contains(folded, "payverge") || strings.Contains(folded, "demo lounge")
}

// PublicMarketingVenueName is the venue name allowed in a public caption.
// Demo showrooms omit the name so the model cannot advertise the product.
func PublicMarketingVenueName(name string, isDemo bool) string {
	name = strings.TrimSpace(name)
	if LooksLikeDemoMarketingVenue(name, isDemo) {
		return ""
	}
	return name
}

// StripDemoMarketingBranding removes demo venue names and #Payverge* hashtags
// from generated public copy. Defense in depth after the prompt.
func StripDemoMarketingBranding(caption string) string {
	caption = strings.TrimSpace(caption)
	if caption == "" {
		return ""
	}
	caption = demoMarketingHashtagPattern.ReplaceAllString(caption, " ")
	caption = demoMarketingVenuePattern.ReplaceAllString(caption, " ")
	caption = demoMarketingPunctGap.ReplaceAllString(caption, "$1")
	caption = strings.Join(strings.Fields(caption), " ")
	caption = strings.TrimSpace(caption)
	caption = strings.TrimLeftFunc(caption, func(r rune) bool {
		return unicode.IsPunct(r) && r != '#' && r != '@'
	})
	caption = strings.TrimSpace(caption)
	if utf8.RuneCountInString(caption) == 0 {
		return ""
	}
	return caption
}
