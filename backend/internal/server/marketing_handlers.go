package server

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"

	"github.com/gin-gonic/gin"
	"golang.org/x/text/unicode/norm"

	"github.com/stdevmac/payverge/backend/internal/database"
	"github.com/stdevmac/payverge/backend/internal/locales"
	"github.com/stdevmac/payverge/backend/internal/s3"
	"github.com/stdevmac/payverge/backend/internal/services"
	"github.com/stdevmac/payverge/backend/internal/services/marketing"
)

type marketingImageRequest struct {
	Name          string `json:"name"`
	Description   string `json:"description"`
	Ingredients   string `json:"ingredients"`
	Play          string `json:"play"` // happy_hour|featured_dish|move_item|win_back|combo_deal
	AspectRatio   string `json:"aspect_ratio"`
	TemplateStyle string `json:"template_style"`
	VisualMood    string `json:"visual_mood"`
	SafeRegion    string `json:"safe_region"`
	// Closed enum naming the band of the frame overlay copy will occupy. Derived
	// on the client from the chosen composition's geometry, not from operator
	// text, so it is safe to map to a prompt sentence (see ai.go).
	ReservedBand string `json:"reserved_band"`
}

const (
	maxMarketingImageNameRunes        = 160
	maxMarketingImageDescriptionRunes = 1000
	maxMarketingImageIngredientsRunes = 1000
	// SafeRegion is short overlay-copy guidance, not a caption or general prompt.
	// 240 Unicode runes accommodates a headline, CTA, price, and handle while
	// bounding the untrusted text forwarded to the image prompt.
	maxMarketingImageSafeRegionRunes = 240
	maxMarketingImageTextRunes       = 2400

	// Shared generation outlives the leader HTTP request so one disconnected
	// client cannot cancel work (and waste the single reserved credit) for every
	// waiter. It remains bounded independently to prevent orphaned provider work.
	marketingImageGenerationTimeout = 2 * time.Minute
)

func normalizeMarketingImageRequest(req marketingImageRequest) (marketingImageRequest, error) {
	req.Name = strings.TrimSpace(req.Name)
	req.Description = strings.TrimSpace(req.Description)
	req.Ingredients = strings.TrimSpace(req.Ingredients)
	req.Play = strings.ToLower(strings.TrimSpace(req.Play))
	req.AspectRatio = strings.ToLower(strings.TrimSpace(req.AspectRatio))
	req.TemplateStyle = strings.ToLower(strings.TrimSpace(req.TemplateStyle))
	req.VisualMood = strings.ToLower(strings.TrimSpace(req.VisualMood))
	req.SafeRegion = strings.TrimSpace(req.SafeRegion)
	req.ReservedBand = strings.ToLower(strings.TrimSpace(req.ReservedBand))

	if req.Name == "" {
		return marketingImageRequest{}, fmt.Errorf("name is required")
	}
	totalTextRunes := utf8.RuneCountInString(req.Name) +
		utf8.RuneCountInString(req.Description) +
		utf8.RuneCountInString(req.Ingredients) +
		utf8.RuneCountInString(req.SafeRegion)
	if totalTextRunes > maxMarketingImageTextRunes {
		return marketingImageRequest{}, fmt.Errorf("image text fields must contain at most %d characters in total", maxMarketingImageTextRunes)
	}
	if utf8.RuneCountInString(req.Name) > maxMarketingImageNameRunes {
		return marketingImageRequest{}, fmt.Errorf("name must contain at most %d characters", maxMarketingImageNameRunes)
	}
	if utf8.RuneCountInString(req.Description) > maxMarketingImageDescriptionRunes {
		return marketingImageRequest{}, fmt.Errorf("description must contain at most %d characters", maxMarketingImageDescriptionRunes)
	}
	if utf8.RuneCountInString(req.Ingredients) > maxMarketingImageIngredientsRunes {
		return marketingImageRequest{}, fmt.Errorf("ingredients must contain at most %d characters", maxMarketingImageIngredientsRunes)
	}
	if utf8.RuneCountInString(req.SafeRegion) > maxMarketingImageSafeRegionRunes {
		return marketingImageRequest{}, fmt.Errorf("safe_region must contain at most %d characters", maxMarketingImageSafeRegionRunes)
	}
	switch marketing.Play(req.Play) {
	case "",
		marketing.PlayHappyHour,
		marketing.PlayFeaturedDish,
		marketing.PlayMoveItem,
		marketing.PlayWinBack,
		marketing.PlayComboDeal,
		marketing.PlayOffer:
	default:
		return marketingImageRequest{}, fmt.Errorf("unknown play")
	}
	if req.AspectRatio == "" {
		req.AspectRatio = "1:1"
	}
	switch req.AspectRatio {
	case "1:1", "4:5", "9:16":
	default:
		return marketingImageRequest{}, fmt.Errorf("unknown aspect_ratio")
	}
	if req.TemplateStyle == "" {
		req.TemplateStyle = "editorial"
	}
	switch req.TemplateStyle {
	case "editorial", "bold", "minimal":
	default:
		return marketingImageRequest{}, fmt.Errorf("unknown template_style")
	}
	switch req.VisualMood {
	case "", "natural", "bright", "moody", "editorial", "rustic":
	default:
		return marketingImageRequest{}, fmt.Errorf("unknown visual_mood")
	}
	switch req.ReservedBand {
	case "", "top", "bottom", "center", "left", "right", "none":
	default:
		return marketingImageRequest{}, fmt.Errorf("unknown reserved_band")
	}
	return req, nil
}

func marketingImageDedupKey(businessID uint, req marketingImageRequest) string {
	canonical, _ := json.Marshal(struct {
		Name          string `json:"name"`
		Description   string `json:"description"`
		Ingredients   string `json:"ingredients"`
		Play          string `json:"play"`
		AspectRatio   string `json:"aspect_ratio"`
		TemplateStyle string `json:"template_style"`
		VisualMood    string `json:"visual_mood"`
		SafeRegion    string `json:"safe_region"`
		ReservedBand  string `json:"reserved_band"`
	}{
		Name: req.Name, Description: req.Description, Ingredients: req.Ingredients,
		Play: req.Play, AspectRatio: req.AspectRatio, TemplateStyle: req.TemplateStyle,
		VisualMood: req.VisualMood, SafeRegion: req.SafeRegion, ReservedBand: req.ReservedBand,
	})
	digest := sha256.Sum256(canonical)
	return fmt.Sprintf("marketing-image|%d|%x", businessID, digest)
}

// GenerateMarketingImage creates a text-free hero photo for a marketing post.
// Reuses the image-credit reserve→generate→refund flow (1 credit per photo).
// The frontend composites editable vector text (name/price/CTA/logo) on top.
func GenerateMarketingImage(c *gin.Context) {
	business, ok := getAIMenuRouteBusiness(c)
	if !ok {
		return
	}
	var req marketingImageRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid request"})
		return
	}
	normalized, err := normalizeMarketingImageRequest(req)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error(), "code": "invalid_marketing_image"})
		return
	}
	req = normalized
	if req.VisualMood == "" {
		settings, settingsErr := database.GetDBWrapper().GetMarketingSettings(business.ID)
		if settingsErr != nil {
			log.Printf("GenerateMarketingImage: load settings for business %s: %v", business.BusinessId, settingsErr)
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to load marketing settings"})
			return
		}
		req.VisualMood = effectiveMarketingCreativeProfile(
			business,
			settings.Sanitized().CreativeProfile,
		).VisualMood
	}
	service := GetAIService()
	if service == nil {
		respondAINotConfigured(c)
		return
	}

	// Screen operator-supplied text before reserving a credit, for parity with the
	// menu image-gen handlers. The classifier fails open on its own errors.
	guardText := strings.TrimSpace(req.Name + " " + req.Description + " " + req.Ingredients + " " + req.SafeRegion)
	if allowed, category := evaluateImagePromptGuardrail(imagePromptClassifier, c.Request.Context(), business.ID, determineBusinessOwnerLanguage(business), guardText); !allowed {
		log.Printf("GenerateMarketingImage: prompt blocked (%s) for business %s", category, business.BusinessId)
		c.JSON(http.StatusUnprocessableEntity, gin.H{"error": "That request can't be used for an image. Try describing the dish itself.", "code": "prompt_rejected"})
		return
	}

	// Every normalized prompt input participates in the key so visually distinct
	// composition requests never collapse into one paid generation.
	key := marketingImageDedupKey(business.ID, req)
	v, genErr := doSharedImageJobWithTimeout(c.Request.Context(), key, marketingImageGenerationTimeout, func(jobCtx context.Context) (interface{}, error) {
		result, gErr := reserveGenerateRefundImage(jobCtx, business, func() (*services.GeneratedImage, error) {
			return service.GenerateMenuImage(jobCtx, services.MenuImagePrompt{
				BusinessID:              business.ID,
				EntityType:              "marketing_hero",
				Name:                    strings.TrimSpace(req.Name),
				Description:             strings.TrimSpace(req.Description),
				Ingredients:             strings.TrimSpace(req.Ingredients),
				MarketingPlay:           strings.TrimSpace(req.Play),
				MarketingAspectRatio:    req.AspectRatio,
				MarketingTemplateStyle:  req.TemplateStyle,
				MarketingVisualMood:     req.VisualMood,
				MarketingSafeRegionText: req.SafeRegion,
				MarketingReservedBand:   req.ReservedBand,
			})
		})
		if gErr != nil {
			return nil, gErr
		}
		if recErr := database.RecordAIGeneratedImage(business.ID, result.URL, "marketing", result.Model); recErr != nil {
			log.Printf("GenerateMarketingImage: provenance record failed for business %s: %v", business.BusinessId, recErr)
		}
		return result, nil
	})
	if respondImageDailyLimit(c, genErr) {
		return
	}
	if genErr != nil {
		log.Printf("GenerateMarketingImage: failed for business %s: %v", business.BusinessId, genErr)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to generate image"})
		return
	}
	image := v.(*services.GeneratedImage)
	c.JSON(http.StatusOK, gin.H{
		"url":       image.URL,
		"credit":    "Generated by AI",
		"mime_type": image.MIMEType,
		"model":     image.Model,
	})
}

// marketingCleanupRequest asks for an image-to-image cleanup of a photo the
// operator already has. Deliberately NOT a generation request: there is no
// prompt field, because the whole value of this path is that the result is
// still the operator's own food.
type marketingCleanupRequest struct {
	ImageURL    string `json:"image_url"`
	Name        string `json:"name"`
	Description string `json:"description"`
	AspectRatio string `json:"aspect_ratio"`
}

func normalizeMarketingCleanupRequest(req marketingCleanupRequest) (marketingCleanupRequest, error) {
	req.ImageURL = strings.TrimSpace(req.ImageURL)
	req.Name = strings.TrimSpace(req.Name)
	req.Description = strings.TrimSpace(req.Description)
	req.AspectRatio = strings.ToLower(strings.TrimSpace(req.AspectRatio))

	if req.ImageURL == "" {
		return marketingCleanupRequest{}, fmt.Errorf("image_url is required")
	}
	if utf8.RuneCountInString(req.Name) > maxMarketingImageNameRunes {
		return marketingCleanupRequest{}, fmt.Errorf("name must contain at most %d characters", maxMarketingImageNameRunes)
	}
	if utf8.RuneCountInString(req.Description) > maxMarketingImageDescriptionRunes {
		return marketingCleanupRequest{}, fmt.Errorf("description must contain at most %d characters", maxMarketingImageDescriptionRunes)
	}
	if req.AspectRatio == "" {
		req.AspectRatio = "1:1"
	}
	switch req.AspectRatio {
	case "1:1", "4:5", "9:16":
	default:
		return marketingCleanupRequest{}, fmt.Errorf("unknown aspect_ratio")
	}
	return req, nil
}

// CleanUpMarketingImage relights and declutters a photo the operator already
// owns, at the aspect ratio of the post it is destined for.
//
// It reuses the Menu tab's `EnhanceItemImage`, and therefore its prompt, which
// forbids substituting the dish. That is the point of the feature and not an
// implementation detail: "that is actually my food" is what separates this from
// generic AI imagery, and it is pinned by
// TestBuildEnhanceImagePromptForbidsSubstitution.
//
// Credits come from the same pool as generation, via reserveGenerateRefundImage
// — the same flow the Menu tab's enhance endpoint already uses, so there is no
// second balance, no second exhausted-state and no second purchase path.
//
// POST /api/v1/business/:id/ai/marketing/image/cleanup
func CleanUpMarketingImage(c *gin.Context) {
	business, ok := getAIMenuRouteBusiness(c)
	if !ok {
		return
	}

	var raw marketingCleanupRequest
	if err := c.ShouldBindJSON(&raw); err != nil {
		RespondBindError(c, err)
		return
	}
	req, err := normalizeMarketingCleanupRequest(raw)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error(), "code": "invalid_marketing_cleanup"})
		return
	}

	// SSRF guard: only ever fetch from our own public asset host. An operator
	// can put any string in a photo URL, including an internal address.
	imageBytes, mimeType, fetchErr := s3.DownloadPublicAsset(c.Request.Context(), req.ImageURL)
	if fetchErr != nil {
		log.Printf("CleanUpMarketingImage: asset fetch rejected for business %s: %v", business.BusinessId, fetchErr)
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid image url"})
		return
	}

	service := GetMenuAIService()
	if service == nil {
		respondAINotConfigured(c)
		return
	}

	// The source bytes participate in the key, so two operators cleaning up two
	// different photos of the same dish never collapse into one paid job.
	key := imageJobBriefKey(imageJobBrief{
		BusinessID:   business.ID,
		Tool:         "marketing-cleanup",
		Name:         req.Name,
		Description:  req.Description + "|" + req.AspectRatio,
		SourceSHA256: sourceImageSHA256(imageBytes),
	})
	v, genErr := doSharedImageJobWithTimeout(c.Request.Context(), key, marketingImageGenerationTimeout, func(jobCtx context.Context) (interface{}, error) {
		result, gErr := reserveGenerateRefundImage(jobCtx, business, func() (*services.GeneratedImage, error) {
			// Marketing cleanup has no menu-item context, so no dietary tags.
			return service.EnhanceItemImage(jobCtx, business.ID, imageBytes, mimeType, req.Name, req.Description, req.AspectRatio, nil)
		})
		if gErr != nil {
			return nil, gErr
		}
		if recErr := database.RecordAIGeneratedImage(business.ID, result.URL, "marketing_cleanup", result.Model); recErr != nil {
			log.Printf("CleanUpMarketingImage: provenance record failed for business %s: %v", business.BusinessId, recErr)
		}
		return result, nil
	})
	if respondImageDailyLimit(c, genErr) {
		return
	}
	if genErr != nil {
		log.Printf("CleanUpMarketingImage: cleanup failed for business %s: %v", business.BusinessId, genErr)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to clean up image"})
		return
	}
	image := v.(*services.GeneratedImage)
	c.JSON(http.StatusOK, gin.H{
		"url":       image.URL,
		"credit":    "Enhanced by AI",
		"mime_type": image.MIMEType,
		"model":     image.Model,
	})
}

// marketingEngine is wired once at startup from main.go.
var marketingEngine *marketing.Engine

// SetMarketingEngine wires the suggestion engine (called from main.go).
func SetMarketingEngine(e *marketing.Engine) { marketingEngine = e }

func businessLocation(b *database.Business) *time.Location {
	if b == nil || strings.TrimSpace(b.Timezone) == "" {
		return time.UTC
	}
	loc, err := time.LoadLocation(b.Timezone)
	if err != nil {
		return time.UTC
	}
	return loc
}

// GetMarketingSuggestions returns the ranked, data-driven campaign feed.
func GetMarketingSuggestions(c *gin.Context) {
	business, ok := getAIMenuRouteBusiness(c)
	if !ok {
		return
	}
	if marketingEngine == nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "Marketing engine not initialized"})
		return
	}
	batch, err := marketingEngine.SuggestionBatch(c.Request.Context(), business.ID, businessLocation(business), services.DetermineBusinessOwnerLanguage(business))
	if err != nil {
		log.Printf("GetMarketingSuggestions: business %s: %v", business.BusinessId, err)
		c.JSON(http.StatusInternalServerError, gin.H{
			"error": "Failed to build suggestions",
			"code":  "suggestions_unavailable",
		})
		return
	}
	// S2-D: attach public guest deep links when the storefront is live.
	// Uses the instance PUBLIC_URL origin only — never private/signed asset URLs.
	marketing.AttachGuestURLs(
		batch.Suggestions,
		business.CustomURL,
		business.BusinessPageEnabled,
		marketing.DefaultGuestOrigin(),
	)
	c.JSON(http.StatusOK, batch)
}

type marketingCaptionRequest struct {
	ItemName string `json:"item_name"`
	Play     string `json:"play"`
	Tone     string `json:"tone"`
	Language string `json:"language"`
	Angle    string `json:"angle"`
	WhyData  string `json:"why_data"`
	// MaxChars is the destination soft budget for AI generation (optional).
	// Absent/zero → server default (280). Clamped to [40, 280].
	MaxChars int `json:"max_chars"`
	// HashtagBehavior optional override from destination (e.g. Google forces none).
	// Empty keeps creative_profile.hashtag_behavior.
	HashtagBehavior string `json:"hashtag_behavior"`
	// MustInclude optional short phrases (CTA/handle) — max 3 × 40 runes.
	MustInclude []string `json:"must_include"`
}

const (
	maxMarketingCaptionRequestItemRunes      = 160
	maxMarketingCaptionRequestAngleRunes     = 500
	maxMarketingCaptionRequestReasonRunes    = 500
	minMarketingCaptionRequestMaxChars       = 40
	maxMarketingCaptionRequestMaxChars       = 280
	maxMarketingCaptionRequestMustInclude    = 3
	maxMarketingCaptionRequestMustIncludeRun = 40
)

func validateMarketingCaptionRequest(req *marketingCaptionRequest) error {
	req.ItemName = strings.TrimSpace(req.ItemName)
	req.Play = strings.TrimSpace(req.Play)
	req.Tone = strings.ToLower(strings.TrimSpace(req.Tone))
	req.Language = strings.TrimSpace(req.Language)
	req.Angle = strings.TrimSpace(req.Angle)
	req.WhyData = strings.TrimSpace(req.WhyData)
	req.HashtagBehavior = strings.ToLower(strings.TrimSpace(req.HashtagBehavior))

	if req.ItemName == "" {
		return fmt.Errorf("item_name is required")
	}
	if utf8.RuneCountInString(req.ItemName) > maxMarketingCaptionRequestItemRunes {
		return fmt.Errorf("item_name must contain at most %d characters", maxMarketingCaptionRequestItemRunes)
	}
	if utf8.RuneCountInString(req.Angle) > maxMarketingCaptionRequestAngleRunes {
		return fmt.Errorf("angle must contain at most %d characters", maxMarketingCaptionRequestAngleRunes)
	}
	if utf8.RuneCountInString(req.WhyData) > maxMarketingCaptionRequestReasonRunes {
		return fmt.Errorf("why_data must contain at most %d characters", maxMarketingCaptionRequestReasonRunes)
	}
	switch req.Tone {
	case "", "warm", "playful", "elegant", "punchy":
	default:
		return fmt.Errorf("unknown marketing tone")
	}
	if req.Language != "" && !locales.IsGuestLocale(req.Language) {
		return fmt.Errorf("unknown marketing language")
	}
	switch req.HashtagBehavior {
	case "", "none", "light", "standard":
	default:
		return fmt.Errorf("unknown hashtag_behavior")
	}
	if req.MaxChars < 0 {
		return fmt.Errorf("max_chars must be positive when set")
	}
	if req.MaxChars > 0 {
		if req.MaxChars < minMarketingCaptionRequestMaxChars {
			return fmt.Errorf("max_chars must be at least %d", minMarketingCaptionRequestMaxChars)
		}
		if req.MaxChars > maxMarketingCaptionRequestMaxChars {
			req.MaxChars = maxMarketingCaptionRequestMaxChars
		}
	}
	mustInclude := make([]string, 0, len(req.MustInclude))
	for _, phrase := range req.MustInclude {
		phrase = strings.TrimSpace(phrase)
		if phrase == "" {
			continue
		}
		if utf8.RuneCountInString(phrase) > maxMarketingCaptionRequestMustIncludeRun {
			return fmt.Errorf("must_include phrases must contain at most %d characters each", maxMarketingCaptionRequestMustIncludeRun)
		}
		mustInclude = append(mustInclude, phrase)
	}
	if len(mustInclude) > maxMarketingCaptionRequestMustInclude {
		return fmt.Errorf("must_include must contain at most %d phrases", maxMarketingCaptionRequestMustInclude)
	}
	req.MustInclude = mustInclude

	switch marketing.Play(req.Play) {
	case marketing.PlayHappyHour,
		marketing.PlayFeaturedDish,
		marketing.PlayMoveItem,
		marketing.PlayWinBack,
		marketing.PlayComboDeal,
		marketing.PlayOffer:
		return nil
	default:
		return fmt.Errorf("unknown marketing play")
	}
}

// GenerateMarketingCaption returns a single caption (free; no credit).
// Results are cached in-process for ~24h keyed by suggestion fingerprint +
// tone + locale + profile fingerprint so feed reloads do not re-call the LLM.
func GenerateMarketingCaption(c *gin.Context) {
	business, ok := getAIMenuRouteBusiness(c)
	if !ok {
		return
	}
	var req marketingCaptionRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid request"})
		return
	}
	if err := validateMarketingCaptionRequest(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error(), "code": "invalid_marketing_caption"})
		return
	}
	if !marketingCaptionSubjectOnMenu(business, req.ItemName) {
		log.Printf("GenerateMarketingCaption: subject %q is not on business %s menu", req.ItemName, business.BusinessId)
		refuseUnsellableMarketingCaption(c)
		return
	}
	if marketingCaptionTargetUnsellable(business, req) {
		refuseUnsellableMarketingCaption(c)
		return
	}
	service := GetAIService()
	if service == nil {
		// Local/dev and partially-configured deploys often boot without an AI
		// provider. Return an empty caption so the marketing feed falls back to
		// the client template instead of 503-toasting "Something went wrong".
		c.JSON(http.StatusOK, gin.H{
			"caption":      "",
			"ai_available": false,
			"code":         "ai_unavailable",
		})
		return
	}
	settings, err := database.GetDBWrapper().GetMarketingSettings(business.ID)
	if err != nil {
		log.Printf("GenerateMarketingCaption: load settings for business %s: %v", business.BusinessId, err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to load marketing settings"})
		return
	}
	profile := effectiveMarketingCreativeProfile(business, settings.Sanitized().CreativeProfile)
	if tone := strings.ToLower(strings.TrimSpace(req.Tone)); tone != "" {
		profile.DefaultTone = tone
	}
	if language := strings.TrimSpace(req.Language); language != "" {
		profile.DefaultLanguage = language
	}
	// Destination / operator override for hashtags (Google, WhatsApp force none).
	if req.HashtagBehavior != "" {
		profile.HashtagBehavior = req.HashtagBehavior
	}
	socialHandle := marketingSocialHandle(business.SocialMedia)
	profileFP := marketingProfileFingerprint(
		profile.Audience,
		profile.Voice,
		profile.CTAStyle,
		profile.HashtagBehavior,
		socialHandle,
		profile.AvoidPhrases,
	)
	// Include destination budget + must_include so story vs feed regenerates
	// do not share a 280-char feed caption from cache.
	cacheKey := marketingCaptionCacheKey(
		business.ID,
		req.ItemName,
		req.Play,
		profile.DefaultTone,
		profile.DefaultLanguage,
		req.Angle,
		req.WhyData,
		marketingCaptionVariantFingerprint(profileFP, req.MaxChars, req.MustInclude),
	)
	// Claim facts are loaded once per request so both cache hits and fresh
	// generations are validated against the current business record. A false
	// opening-hours / price / offer claim must never leave the platform.
	claimFacts := loadMarketingClaimFacts(business)

	if cached, ok := globalMarketingCaptionCache.get(cacheKey); ok {
		cached = services.StripDemoMarketingBranding(cached)
		if !marketingCaptionTextUnsellable(business, cached) {
			if cleaned, safe := services.SanitizeMarketingCaptionClaims(cached, claimFacts); safe {
				cleaned = services.StripDemoMarketingBranding(cleaned)
				if cleaned != "" && !marketingCaptionTextUnsellable(business, cleaned) {
					c.JSON(http.StatusOK, gin.H{"caption": cleaned, "cached": true})
					return
				}
			}
		}
		// Stale cached caption failed the claim guard or now names an 86'd dish.
		log.Printf("GenerateMarketingCaption: claim guard rejected cached caption for business %s", business.BusinessId)
	}
	publicVenueName := services.PublicMarketingVenueName(business.Name, business.IsDemo)
	caption, err := service.GenerateMarketingCaption(c.Request.Context(), services.MarketingCreativeBrief{
		BusinessID:          business.ID,
		Locale:              profile.DefaultLanguage,
		BusinessName:        publicVenueName,
		ItemName:            strings.TrimSpace(req.ItemName),
		Play:                strings.TrimSpace(req.Play),
		Tone:                profile.DefaultTone,
		Angle:               strings.TrimSpace(req.Angle),
		Reason:              strings.TrimSpace(req.WhyData),
		City:                strings.TrimSpace(business.Address.City),
		BusinessType:        strings.TrimSpace(business.BusinessType),
		BusinessDescription: strings.TrimSpace(business.Description),
		Audience:            profile.Audience,
		Voice:               profile.Voice,
		CTAStyle:            profile.CTAStyle,
		HashtagBehavior:     profile.HashtagBehavior,
		SocialHandle:        socialHandle,
		AvoidPhrases:        profile.AvoidPhrases,
		MaxChars:            req.MaxChars,
		MustInclude:         req.MustInclude,
	})
	if err != nil {
		// Provider/transient failures must not red-banner the marketing tab.
		// Empty caption → client uses composeLocalizedFallbackCaption.
		log.Printf("GenerateMarketingCaption: business %s: %v", business.BusinessId, err)
		c.JSON(http.StatusOK, gin.H{
			"caption":      "",
			"ai_available": true,
			"code":         "ai_generation_failed",
		})
		return
	}

	caption = services.StripDemoMarketingBranding(caption)
	if caption == "" {
		c.JSON(http.StatusOK, gin.H{
			"caption":      "",
			"ai_available": true,
			"code":         "claim_guard_rejected",
		})
		return
	}
	if marketingCaptionTextUnsellable(business, caption) {
		refuseUnsellableMarketingCaption(c)
		return
	}

	// Apply after copy generation (insight → copy hop). Never publish ungrounded
	// factual claims; strip the offending sentence(s) when the rest is usable.
	cleaned, safe := services.SanitizeMarketingCaptionClaims(caption, claimFacts)
	if !safe {
		if ok, violations := services.GuardMarketingClaims(caption, claimFacts); !ok {
			log.Printf("GenerateMarketingCaption: claim guard rejected for business %s: %s",
				business.BusinessId, services.FormatMarketingClaimViolations(violations))
		}
		c.JSON(http.StatusOK, gin.H{
			"caption":      "",
			"ai_available": true,
			"code":         "claim_guard_rejected",
		})
		return
	}
	if cleaned != caption {
		log.Printf("GenerateMarketingCaption: claim guard stripped ungrounded claims for business %s", business.BusinessId)
	}
	cleaned = services.StripDemoMarketingBranding(cleaned)
	if cleaned == "" {
		c.JSON(http.StatusOK, gin.H{
			"caption":      "",
			"ai_available": true,
			"code":         "claim_guard_rejected",
		})
		return
	}
	if marketingCaptionTextUnsellable(business, cleaned) {
		refuseUnsellableMarketingCaption(c)
		return
	}
	globalMarketingCaptionCache.put(cacheKey, cleaned)
	c.JSON(http.StatusOK, gin.H{"caption": cleaned, "ai_available": true})
}

func refuseUnsellableMarketingCaption(c *gin.Context) {
	c.JSON(http.StatusOK, gin.H{
		"caption":      "",
		"ai_available": true,
		"code":         "item_unavailable",
	})
}

func marketingCaptionTargetUnsellable(business *database.Business, req marketingCaptionRequest) bool {
	hidden, categories, bundles, offers := loadMarketingUnsellableCatalog(business)
	if len(hidden) == 0 {
		return false
	}
	haystack := strings.Join([]string{req.ItemName, req.Angle, req.WhyData}, "\n")
	return marketingHaystackHitsUnsellable(haystack, hidden, categories, bundles, offers)
}

// marketingCaptionSubjectOnMenu reports whether the caption subject names
// something this venue actually sells. item_name arrives straight from the
// client, so without this check any string — a dish off another venue's demo
// carta, an operator typo, a hallucinated special — became a promo subject and
// the model wrote copy for a plate this kitchen does not cook (#873).
//
// Legitimate subjects are a menu item, a menu category, a bundle, an offer, or
// the venue's own name: resolveCaptionSubject falls back to business.name when
// a play has no dish target (win_back with no hero item, happy_hour with no
// live offer).
func marketingCaptionSubjectOnMenu(business *database.Business, subject string) bool {
	if business == nil {
		return false
	}
	key := marketingFoldedAlnum(subject)
	if key == "" {
		return false
	}
	if marketingCaptionSubjectMatches(key, business.Name) ||
		marketingCaptionSubjectMatches(key, services.PublicMarketingVenueName(business.Name, business.IsDemo)) {
		return true
	}
	// Same cached pricing snapshot the 86 guard below reads — no new query
	// shape, and it degrades the same way: a transient snapshot failure must
	// not silence the whole marketing tab.
	_, categories, offers, bundles, err := menuDataForBusiness(business.ID)
	if err != nil {
		log.Printf("marketingCaptionSubjectOnMenu: menu for business %d: %v", business.ID, err)
		return true
	}
	named := 0
	match := func(candidate string) bool {
		if strings.TrimSpace(candidate) == "" {
			return false
		}
		named++
		return marketingCaptionSubjectMatches(key, candidate)
	}
	for _, category := range categories {
		if match(category.Name) {
			return true
		}
		for _, item := range category.Items {
			if match(item.Name) {
				return true
			}
		}
	}
	for _, bundle := range bundles {
		if match(bundle.Name) {
			return true
		}
	}
	for _, offer := range offers {
		if match(offer.Name) {
			return true
		}
	}
	// No carta in Payverge yet (menu not imported). There is nothing to
	// contradict the subject with, so stay out of the operator's way — the
	// point of this guard is a venue whose real menu says otherwise.
	return named == 0
}

// marketingCaptionSubjectMinTruncatedPrefix is the shortest folded subject that
// may match a catalog name by prefix. resolveCaptionSubject clips a subject
// longer than 160 runes and appends an ellipsis, so only very long names ever
// arrive truncated; a short prefix would let "Bife" pass as "Bife de Chorizo"
// and reopen the hole this guard closes.
const marketingCaptionSubjectMinTruncatedPrefix = 40

func marketingCaptionSubjectMatches(subjectKey, candidate string) bool {
	key := marketingFoldedAlnum(candidate)
	if key == "" || subjectKey == "" {
		return false
	}
	if key == subjectKey {
		return true
	}
	return utf8.RuneCountInString(subjectKey) >= marketingCaptionSubjectMinTruncatedPrefix &&
		strings.HasPrefix(key, subjectKey)
}

func marketingCaptionTextUnsellable(business *database.Business, caption string) bool {
	hidden, categories, bundles, offers := loadMarketingUnsellableCatalog(business)
	if len(hidden) == 0 {
		return false
	}
	return marketingHaystackHitsUnsellable(caption, hidden, categories, bundles, offers)
}

// loadMarketingUnsellableCatalog is the item-level 86 set for public captions.
// Operator context is required: guest hours / kitchen / business-lock gates
// would mask inventory_out as venue-wide closed and let Date Night still sell
// an 86'd steak.
func loadMarketingUnsellableCatalog(business *database.Business) (map[string]bool, []database.MenuCategory, []database.Bundle, []database.Offer) {
	if business == nil {
		return nil, nil, nil, nil
	}
	_, categories, offers, bundles, err := menuDataForBusiness(business.ID)
	if err != nil {
		return nil, nil, nil, nil
	}
	projection := services.ProjectOrderability(business, categories, services.OrderabilityContextOperator, true)
	hidden := make(map[string]bool)
	for _, category := range categories {
		for _, item := range category.Items {
			id := strings.TrimSpace(item.ID)
			if id == "" {
				continue
			}
			if !item.IsAvailable {
				hidden[id] = true
				continue
			}
			decision, ok := projection[id]
			if !ok {
				continue
			}
			if decision.State == services.OrderabilityInventoryOut || !decision.Orderable {
				hidden[id] = true
			}
		}
	}
	return hidden, categories, bundles, offers
}

func marketingHaystackHitsUnsellable(
	haystack string,
	hidden map[string]bool,
	categories []database.MenuCategory,
	bundles []database.Bundle,
	offers []database.Offer,
) bool {
	haystack = strings.TrimSpace(haystack)
	if haystack == "" || len(hidden) == 0 {
		return false
	}
	for _, category := range categories {
		for _, item := range category.Items {
			id := strings.TrimSpace(item.ID)
			if !hidden[id] {
				continue
			}
			if marketingNameMentioned(haystack, item.Name) || marketingNameMentioned(haystack, id) {
				return true
			}
		}
	}
	for _, bundle := range bundles {
		if len(filterBundlesByHiddenItems([]database.Bundle{bundle}, hidden)) != 0 {
			continue
		}
		if marketingNameMentioned(haystack, bundle.Name) ||
			marketingNameMentioned(haystack, fmt.Sprintf("%d", bundle.ID)) {
			return true
		}
	}
	for _, offer := range offers {
		name := strings.TrimSpace(offer.Name)
		if name == "" || !marketingNameMentioned(haystack, name) {
			continue
		}
		if offerTargetsHiddenMenuItem(offer, hidden) {
			return true
		}
	}
	return false
}

func offerTargetsHiddenMenuItem(offer database.Offer, hidden map[string]bool) bool {
	if len(hidden) == 0 || offer.TargetID == nil {
		return false
	}
	target := strings.TrimSpace(*offer.TargetID)
	if target == "" {
		return false
	}
	if hidden[target] {
		return true
	}
	if !strings.EqualFold(strings.TrimSpace(offer.ApplicableTo), "bundle") {
		return false
	}
	return hidden[target]
}

// marketingFoldedAlnum reduces a name to its letters and digits so casing,
// spacing and punctuation cannot decide whether two names are the same.
//
// The NFC pass is load-bearing (#873). Only letters and numbers survive the
// fold, and a combining mark is neither: a precomposed "é" (U+00E9) is kept
// while a decomposed "e" + U+0301 silently loses its accent, so the same
// visible name folded to two different keys. macOS/iOS paste paths and OCR
// carta imports emit decomposed text while the web form emits precomposed, so
// both forms reach this helper. That was harmless while this only powered the
// 86 blocklist, where a missed match failed open; as the caption subject
// whitelist it refused strings that render identically, in both directions.
func marketingFoldedAlnum(s string) string {
	var b strings.Builder
	for _, r := range norm.NFC.String(strings.ToLower(strings.TrimSpace(s))) {
		if unicode.IsLetter(r) || unicode.IsNumber(r) {
			b.WriteRune(r)
		}
	}
	return b.String()
}

func marketingNameMentioned(haystack, name string) bool {
	name = strings.TrimSpace(name)
	haystack = strings.TrimSpace(haystack)
	if name == "" || haystack == "" {
		return false
	}
	if marketingNameMentionedOne(haystack, name) {
		return true
	}
	for _, part := range strings.FieldsFunc(haystack, func(r rune) bool {
		return r == '\n' || r == ',' || r == ';'
	}) {
		if marketingNameMentionedOne(part, name) {
			return true
		}
	}
	return false
}

func marketingNameMentionedOne(haystack, name string) bool {
	haystack = strings.TrimSpace(haystack)
	name = strings.TrimSpace(name)
	if name == "" || haystack == "" {
		return false
	}
	if strings.EqualFold(haystack, name) {
		return true
	}
	foldedHay := marketingFoldedAlnum(haystack)
	foldedName := marketingFoldedAlnum(name)
	if foldedHay == "" || foldedName == "" {
		return false
	}
	if foldedHay == foldedName || strings.Contains(foldedHay, foldedName) {
		return true
	}
	// "Date Night" refers to "Date Night for Two".
	if utf8.RuneCountInString(name) >= 8 &&
		utf8.RuneCountInString(haystack) >= 8 &&
		(strings.HasPrefix(foldedName, foldedHay) || strings.Contains(foldedName, foldedHay)) {
		return true
	}
	fields := strings.Fields(name)
	if len(fields) == 0 {
		return false
	}
	token := strings.Trim(fields[0], ".,!?")
	if utf8.RuneCountInString(token) < 5 {
		return false
	}
	pattern := regexp.MustCompile(`(?i)(?:^|[^\p{L}\p{N}_#])` + regexp.QuoteMeta(token) + `(?:$|[^\p{L}\p{N}_])`)
	return pattern.MatchString(haystack)
}

// loadMarketingClaimFacts builds the ground-truth snapshot used by the
// marketing claim guard. Failures loading hours/offers/menu degrade to empty
// slices so the guard fail-closes on any claim of that kind.
func loadMarketingClaimFacts(business *database.Business) services.MarketingClaimFacts {
	if business == nil {
		return services.MarketingClaimFacts{}
	}
	hours, err := database.GetBusinessOperatingHours(business.ID)
	if err != nil {
		log.Printf("loadMarketingClaimFacts: hours for business %d: %v", business.ID, err)
		hours = nil
	}
	offers, err := database.GetOffersByBusinessID(business.ID)
	if err != nil {
		log.Printf("loadMarketingClaimFacts: offers for business %d: %v", business.ID, err)
		offers = nil
	}
	prices := marketingMenuPrices(business.ID)
	return services.BuildMarketingClaimFacts(business.Phone, business.Address, hours, offers, prices)
}

func marketingMenuPrices(businessID uint) []float64 {
	_, categories, err := database.GetMenuByBusinessID(businessID)
	if err != nil || len(categories) == 0 {
		return nil
	}
	var prices []float64
	seen := map[float64]struct{}{}
	for _, cat := range categories {
		for _, item := range cat.Items {
			if item.Price <= 0 {
				continue
			}
			if _, ok := seen[item.Price]; ok {
				continue
			}
			seen[item.Price] = struct{}{}
			prices = append(prices, item.Price)
		}
	}
	return prices
}

func effectiveMarketingCreativeProfile(business *database.Business, profile database.MarketingCreativeProfile) database.MarketingCreativeProfile {
	if business == nil {
		business = &database.Business{}
	}
	if strings.TrimSpace(profile.DefaultLanguage) == "" {
		profile.DefaultLanguage = services.DetermineBusinessOwnerLanguage(business)
	}
	if strings.TrimSpace(profile.DefaultLanguage) == "" {
		profile.DefaultLanguage = "en"
	}
	if strings.TrimSpace(profile.DefaultTone) == "" {
		profile.DefaultTone = "warm"
	}
	if strings.TrimSpace(profile.VisualMood) == "" {
		profile.VisualMood = derivedMarketingVisualMood(business)
	}
	if strings.TrimSpace(profile.CTAStyle) == "" {
		profile.CTAStyle = "soft"
	}
	if strings.TrimSpace(profile.HashtagBehavior) == "" {
		profile.HashtagBehavior = "standard"
	}
	if strings.TrimSpace(profile.Audience) == "" {
		audience := "local diners"
		if businessType := strings.TrimSpace(business.BusinessType); businessType != "" {
			audience = "local " + strings.ReplaceAll(businessType, "_", " ") + " guests"
		}
		if city := strings.TrimSpace(business.Address.City); city != "" {
			audience += " in " + city
		}
		profile.Audience = audience
	}
	if strings.TrimSpace(profile.Voice) == "" {
		if strings.TrimSpace(business.Description) != "" {
			profile.Voice = "welcoming, specific, and faithful to the business description"
		} else if businessType := strings.TrimSpace(business.BusinessType); businessType != "" {
			profile.Voice = "clear, welcoming, and appropriate for a " + strings.ReplaceAll(businessType, "_", " ")
		} else {
			profile.Voice = "clear, welcoming, and factual"
		}
	}
	if profile.AvoidPhrases == nil {
		profile.AvoidPhrases = []string{}
	}
	return profile
}

func derivedMarketingVisualMood(business *database.Business) string {
	if business == nil {
		return "natural"
	}
	switch strings.ToLower(strings.TrimSpace(business.BusinessType)) {
	case "bar":
		return "moody"
	case "cafe", "bakery", "quick_service", "food_truck":
		return "bright"
	case "fine_dining":
		return "editorial"
	}
	description := strings.ToLower(business.Description)
	switch {
	case containsAny(description, "rustic", "wood-fired", "wood fired", "farm", "heritage"):
		return "rustic"
	case containsAny(description, "cocktail", "nightlife", "late-night", "late night", "lounge"):
		return "moody"
	case containsAny(description, "tasting menu", "fine dining", "elegant"):
		return "editorial"
	case strings.EqualFold(strings.TrimSpace(business.DesignSettings.FontFamily), "serif"):
		return "editorial"
	default:
		return "natural"
	}
}

func containsAny(value string, candidates ...string) bool {
	for _, candidate := range candidates {
		if strings.Contains(value, candidate) {
			return true
		}
	}
	return false
}

func marketingSocialHandle(raw string) string {
	value := strings.TrimSpace(raw)
	if value == "" {
		return ""
	}
	if strings.HasPrefix(value, "{") {
		var links map[string]any
		if err := json.Unmarshal([]byte(value), &links); err != nil {
			return ""
		}
		value = ""
		for _, platform := range []string{"instagram", "tiktok", "twitter", "x", "facebook"} {
			if candidate, ok := links[platform].(string); ok && strings.TrimSpace(candidate) != "" {
				value = candidate
				break
			}
		}
		if value == "" {
			return ""
		}
	}
	value = strings.TrimSpace(value)
	if strings.Contains(value, "://") {
		parsed, err := url.Parse(value)
		if err != nil || (parsed.Scheme != "http" && parsed.Scheme != "https") || parsed.Host == "" || !isRecognizedMarketingSocialHost(parsed.Hostname()) {
			return ""
		}
		path := strings.Trim(parsed.EscapedPath(), "/")
		if path == "" {
			return ""
		}
		segment := strings.Split(path, "/")[0]
		decoded, err := url.PathUnescape(segment)
		if err != nil {
			return ""
		}
		value = decoded
	} else if strings.Contains(value, "/") {
		return ""
	}
	value = strings.Trim(value, "@/ ")
	if value == "" || !marketingSocialHandlePattern.MatchString(value) {
		return ""
	}
	return "@" + value
}

var marketingSocialHandlePattern = regexp.MustCompile(`^[A-Za-z0-9._-]{1,30}$`)

func isRecognizedMarketingSocialHost(host string) bool {
	switch strings.ToLower(strings.TrimSpace(host)) {
	case "instagram.com", "www.instagram.com",
		"tiktok.com", "www.tiktok.com",
		"twitter.com", "www.twitter.com",
		"x.com", "www.x.com",
		"facebook.com", "www.facebook.com":
		return true
	default:
		return false
	}
}
