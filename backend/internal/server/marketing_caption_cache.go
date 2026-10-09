package server

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"strings"
	"sync"
	"time"
)

const (
	marketingCaptionCacheTTL     = 24 * time.Hour
	marketingCaptionCacheMaxSize = 2048
)

type marketingCaptionCacheEntry struct {
	caption   string
	expiresAt time.Time
}

// In-process caption cache: keyed by suggestion fingerprint + tone + locale +
// profile fingerprint. Avoids re-calling the LLM on every library/feed reload
// for the same creative inputs (8–16 free captions per refresh). No schema.
type marketingCaptionCache struct {
	mu      sync.Mutex
	entries map[string]marketingCaptionCacheEntry
}

var globalMarketingCaptionCache = &marketingCaptionCache{
	entries: make(map[string]marketingCaptionCacheEntry),
}

func (c *marketingCaptionCache) get(key string) (string, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	entry, ok := c.entries[key]
	if !ok {
		return "", false
	}
	if time.Now().After(entry.expiresAt) {
		delete(c.entries, key)
		return "", false
	}
	return entry.caption, true
}

func (c *marketingCaptionCache) put(key, caption string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if len(c.entries) >= marketingCaptionCacheMaxSize {
		// Opportunistic prune of expired entries; if still full, drop oldest
		// by expiry (cheap approximate eviction).
		now := time.Now()
		var oldestKey string
		var oldestExpiry time.Time
		for k, e := range c.entries {
			if e.expiresAt.Before(now) {
				delete(c.entries, k)
				continue
			}
			if oldestKey == "" || e.expiresAt.Before(oldestExpiry) {
				oldestKey = k
				oldestExpiry = e.expiresAt
			}
		}
		if len(c.entries) >= marketingCaptionCacheMaxSize && oldestKey != "" {
			delete(c.entries, oldestKey)
		}
	}
	c.entries[key] = marketingCaptionCacheEntry{
		caption:   caption,
		expiresAt: time.Now().Add(marketingCaptionCacheTTL),
	}
}

func (c *marketingCaptionCache) clear() {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.entries = make(map[string]marketingCaptionCacheEntry)
}

// marketingCaptionCacheKey builds a stable key from the suggestion-facing
// inputs and the effective creative profile (tone/locale included).
func marketingCaptionCacheKey(
	businessID uint,
	itemName, play, tone, locale, angle, whyData string,
	profileFingerprint string,
) string {
	payload := struct {
		BusinessID uint   `json:"b"`
		ItemName   string `json:"i"`
		Play       string `json:"p"`
		Tone       string `json:"t"`
		Locale     string `json:"l"`
		Angle      string `json:"a"`
		WhyData    string `json:"w"`
		Profile    string `json:"f"`
	}{
		BusinessID: businessID,
		ItemName:   strings.TrimSpace(itemName),
		Play:       strings.ToLower(strings.TrimSpace(play)),
		Tone:       strings.ToLower(strings.TrimSpace(tone)),
		Locale:     strings.TrimSpace(locale),
		Angle:      strings.TrimSpace(angle),
		WhyData:    strings.TrimSpace(whyData),
		Profile:    profileFingerprint,
	}
	raw, err := json.Marshal(payload)
	if err != nil {
		// Extremely unlikely; fall back to a concatenated key.
		return strings.Join([]string{
			strings.TrimSpace(itemName),
			play, tone, locale, angle, whyData, profileFingerprint,
		}, "|")
	}
	sum := sha256.Sum256(raw)
	return hex.EncodeToString(sum[:])
}

func marketingProfileFingerprint(
	audience, voice, ctaStyle, hashtagBehavior, socialHandle string,
	avoidPhrases []string,
) string {
	payload := struct {
		Audience        string   `json:"a"`
		Voice           string   `json:"v"`
		CTAStyle        string   `json:"c"`
		HashtagBehavior string   `json:"h"`
		SocialHandle    string   `json:"s"`
		AvoidPhrases    []string `json:"x"`
	}{
		Audience:        strings.TrimSpace(audience),
		Voice:           strings.TrimSpace(voice),
		CTAStyle:        strings.TrimSpace(ctaStyle),
		HashtagBehavior: strings.TrimSpace(hashtagBehavior),
		SocialHandle:    strings.TrimSpace(socialHandle),
		AvoidPhrases:    avoidPhrases,
	}
	raw, err := json.Marshal(payload)
	if err != nil {
		return audience + "|" + voice + "|" + ctaStyle
	}
	sum := sha256.Sum256(raw)
	return hex.EncodeToString(sum[:8])
}

// marketingCaptionVariantFingerprint folds destination max_chars + must_include
// into the cache profile slot so regenerate with a shorter budget cannot hit a
// longer cached caption (story vs feed).
func marketingCaptionVariantFingerprint(profileFP string, maxChars int, mustInclude []string) string {
	payload := struct {
		Profile     string   `json:"p"`
		MaxChars    int      `json:"m"`
		MustInclude []string `json:"i"`
	}{
		Profile:     strings.TrimSpace(profileFP),
		MaxChars:    maxChars,
		MustInclude: mustInclude,
	}
	if payload.MustInclude == nil {
		payload.MustInclude = []string{}
	}
	raw, err := json.Marshal(payload)
	if err != nil {
		return profileFP
	}
	sum := sha256.Sum256(raw)
	return hex.EncodeToString(sum[:8])
}
