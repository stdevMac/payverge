package database

import (
	"context"
	"fmt"
	"strings"
	"unicode"

	"gorm.io/gorm"
)

// reservedPublicSlugPrefixes must never appear as the first segment of a
// public /b/<slug> path. Mirrored in frontend CustomURLInput.
var reservedPublicSlugPrefixes = []string{
	"admin",
	"api",
	"demo",
	"b",
	"internal",
}

// IsReservedPublicSlug reports whether slug equals a reserved token or uses
// one as its first hyphen-separated segment (e.g. demo-lounge, admin-1).
func IsReservedPublicSlug(slug string) bool {
	s := strings.ToLower(strings.TrimSpace(slug))
	if s == "" {
		return false
	}
	for _, p := range reservedPublicSlugPrefixes {
		if s == p || strings.HasPrefix(s, p+"-") {
			return true
		}
	}
	return false
}

// SlugifyDisplayName converts a business display name into a URL-safe slug:
// lowercase ASCII alphanumerics with single hyphens. Non-ASCII letters are
// dropped (accents stripped only when they decompose; simple pass keeps a-z0-9).
func SlugifyDisplayName(name string) string {
	var b strings.Builder
	b.Grow(len(name))
	prevHyphen := false
	for _, r := range strings.ToLower(strings.TrimSpace(name)) {
		switch {
		case r >= 'a' && r <= 'z', r >= '0' && r <= '9':
			b.WriteRune(r)
			prevHyphen = false
		case unicode.IsLetter(r) || unicode.IsNumber(r):
			// Fold common Latin accents into ASCII when possible via simple map;
			// otherwise skip so the slug stays [a-z0-9-].
			if ascii, ok := latinFold[r]; ok {
				b.WriteByte(ascii)
				prevHyphen = false
			}
		default:
			if !prevHyphen && b.Len() > 0 {
				b.WriteByte('-')
				prevHyphen = true
			}
		}
	}
	return strings.Trim(b.String(), "-")
}

// latinFold maps common accented letters to ASCII for public slugs.
var latinFold = map[rune]byte{
	'á': 'a', 'à': 'a', 'ä': 'a', 'â': 'a', 'ã': 'a', 'å': 'a',
	'é': 'e', 'è': 'e', 'ë': 'e', 'ê': 'e',
	'í': 'i', 'ì': 'i', 'ï': 'i', 'î': 'i',
	'ó': 'o', 'ò': 'o', 'ö': 'o', 'ô': 'o', 'õ': 'o',
	'ú': 'u', 'ù': 'u', 'ü': 'u', 'û': 'u',
	'ñ': 'n', 'ç': 'c',
	'ý': 'y', 'ÿ': 'y',
}

// AllocatePublicCustomURL builds a public slug from the display name only and
// appends a numeric disambiguator (-2, -3, …) when the base is taken.
// excludeBusinessID is the row being updated (0 for create). Business
// creation calls it so every venue has a routable /b/<slug> from the start.
func AllocatePublicCustomURL(ctx context.Context, tx *gorm.DB, displayName string, excludeBusinessID uint) (string, error) {
	base := SlugifyDisplayName(displayName)
	if base == "" {
		base = "venue"
	}
	if IsReservedPublicSlug(base) {
		base = "venue-" + base
	}

	candidate := base
	for n := 2; n <= 10000; n++ {
		taken, err := customURLTaken(ctx, tx, candidate, excludeBusinessID)
		if err != nil {
			return "", err
		}
		if !taken {
			return candidate, nil
		}
		candidate = fmt.Sprintf("%s-%d", base, n)
	}
	return "", fmt.Errorf("unable to allocate unique custom URL for %q", displayName)
}

func customURLTaken(ctx context.Context, tx *gorm.DB, slug string, excludeBusinessID uint) (bool, error) {
	q := tx.WithContext(ctx).Model(&Business{}).
		Where("custom_url <> '' AND LOWER(custom_url) = LOWER(?)", slug)
	if excludeBusinessID != 0 {
		q = q.Where("id != ?", excludeBusinessID)
	}
	var count int64
	if err := q.Count(&count).Error; err != nil {
		return false, err
	}
	return count > 0, nil
}
