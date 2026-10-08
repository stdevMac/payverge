package server

import (
	"errors"
	"fmt"
	"log"
	"math"
	"net/http"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"

	"github.com/stdevmac/payverge/backend/internal/database"
	"github.com/stdevmac/payverge/backend/internal/services/marketing"
)

// marketingActivitySuggestion is the snapshot the frontend sends when recording
// an activity (posted/dismissed). Fields mirror CampaignSuggestion JSON.
type marketingActivitySuggestion struct {
	ID               string                            `json:"id"`
	Play             string                            `json:"play"`
	Title            string                            `json:"title"`
	TargetName       string                            `json:"target_name"`
	ImageURL         string                            `json:"image_url"`
	Caption          string                            `json:"caption"`
	CreativeSnapshot *marketingCreativeSnapshotRequest `json:"creative_snapshot"`
}

type marketingCropRequest struct {
	X    *float64 `json:"x"`
	Y    *float64 `json:"y"`
	Zoom *float64 `json:"zoom"`
}

type marketingCreativeSnapshotRequest struct {
	Caption     string                `json:"caption"`
	ImageURL    string                `json:"image_url"`
	ImageSource string                `json:"image_source"`
	Template    string                `json:"template"`
	Aspect      string                `json:"aspect"`
	Slots       map[string]string     `json:"slots"`
	Crop        *marketingCropRequest `json:"crop"`
	FontFamily  string                `json:"font_family"`
	// Wave 1 art direction. Absent on legacy rows, which render through the
	// legacy composition mapped from Template.
	Kit         string `json:"kit"`
	Composition string `json:"composition"`
	Treatment   string `json:"treatment"`
	// Wave 3 motion. Absent means a legacy still image.
	MediaKind    string `json:"media_kind"`
	MotionPreset string `json:"motion_preset"`
	// Wave 4 campaign kit; absent on every pre-Wave-4 client.
	KitFormats []string `json:"kit_formats"`
	// Season 1 destination pack. Absent = legacy / no destination chosen.
	// Charset-bounded like composition — not a closed whitelist — so FE can
	// grow destinations without a backend deploy each time.
	DestinationID string `json:"destination_id"`
	// S3-Loop: optional freeform channel tag when the operator confirms
	// mark_posted. Not a schedule field. Prefer request-root PostedChannel when
	// both are set (handler merges before validation).
	PostedChannel string `json:"posted_channel"`
	// S3-Reach: optional promo / correlation code on packs (e.g. PV-HH-A3F2).
	// Charset-bounded; not a schedule field and not a coupon ledger.
	PromoCode string `json:"promo_code"`
	// S3-Reach: optional print-shop preset id. Charset-bounded like destination_id.
	PrintPreset string `json:"print_preset"`
}

type recordMarketingActivityRequest struct {
	// Action: post | dismiss | restore | mark_ready | approve
	Action           string                            `json:"action"`
	SuggestionID     string                            `json:"suggestion_id"`
	Suggestion       *marketingActivitySuggestion      `json:"suggestion"`
	CreativeSnapshot *marketingCreativeSnapshotRequest `json:"creative_snapshot"`
	// S3-Loop: optional freeform channel (e.g. "Instagram", "WhatsApp group").
	// Applied only on post; never a schedule timestamp.
	PostedChannel string `json:"posted_channel"`
}

// marketingActivityIsOwner is true when the request principal is the business
// owner (wallet/email), not a staff member. Staff tokens set staff_id.
func marketingActivityIsOwner(c *gin.Context) bool {
	if _, ok := c.Get("staff_id"); ok {
		return false
	}
	return true
}

// loadCurrentMarketingActivityStatus returns the status for (business, suggestion)
// or "" when no row exists.
func loadCurrentMarketingActivityStatus(businessID uint, suggestionID string) (string, error) {
	var row database.MarketingActivity
	err := database.GetDB().
		Select("status").
		Where("business_id = ? AND suggestion_id = ?", businessID, suggestionID).
		Limit(1).
		Take(&row).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return "", nil
		}
		return "", err
	}
	return row.Status, nil
}

const (
	marketingSnapshotMaxCaptionRunes  = 280
	marketingSnapshotMaxHeadlineRunes = 120
	marketingSnapshotMaxCTARunes      = 80
	marketingSnapshotMaxFontRunes     = 32
)

func normalizeMarketingSnapshotFont(raw string) (string, error) {
	font := strings.TrimSpace(raw)
	if font == "" {
		return "", nil
	}
	if utf8.RuneCountInString(font) > marketingSnapshotMaxFontRunes {
		return "", fmt.Errorf("font_family must be at most %d characters", marketingSnapshotMaxFontRunes)
	}
	switch strings.ToLower(font) {
	case "inter":
		return "Inter", nil
	case "sans":
		return "Sans", nil
	case "serif":
		return "Serif", nil
	default:
		return "", fmt.Errorf("font_family must be one of Inter, Sans, or Serif")
	}
}

const marketingSnapshotMaxIdentRunes = 64

// marketingKits is the closed set of Wave 1 art-direction kits. Kept in sync
// with frontend artDirection/kits.ts KIT_ORDER.
var marketingKits = []string{"editorial", "bold", "minimal", "chalkboard", "linen", "ticket"}

// normalizeMarketingSnapshotKit accepts "" (legacy) or a known kit id.
func normalizeMarketingSnapshotKit(raw string) (string, error) {
	kit := strings.TrimSpace(strings.ToLower(raw))
	if kit == "" {
		return "", nil
	}
	if !oneOf(kit, marketingKits...) {
		return "", fmt.Errorf("kit must be one of %s", strings.Join(marketingKits, ", "))
	}
	return kit, nil
}

// normalizeMarketingSnapshotIdent accepts "" or a bounded identifier. Composition
// and treatment ids are frontend-owned and expected to grow each wave, so they
// are length- and charset-bounded rather than whitelisted.
func normalizeMarketingSnapshotIdent(raw string, field string) (string, error) {
	v := strings.TrimSpace(raw)
	if v == "" {
		return "", nil
	}
	if utf8.RuneCountInString(v) > marketingSnapshotMaxIdentRunes {
		return "", fmt.Errorf("%s must be at most %d characters", field, marketingSnapshotMaxIdentRunes)
	}
	for _, r := range v {
		if !(r >= 'a' && r <= 'z') && !(r >= 'A' && r <= 'Z') && !(r >= '0' && r <= '9') && r != '_' && r != '-' {
			return "", fmt.Errorf("%s must contain only letters, digits, hyphens, and underscores", field)
		}
	}
	return v, nil
}

// marketingMediaKinds is the closed set of creative media kinds. Unlike
// composition and treatment ids — which are frontend-owned and expected to grow
// each wave — this is a backend concept the server reasons about, so it is
// whitelisted rather than charset-bounded.
var marketingMediaKinds = []string{"image", "video"}

// normalizeMarketingSnapshotMediaKind accepts "" (legacy still) or a known kind.
func normalizeMarketingSnapshotMediaKind(raw string) (string, error) {
	kind := strings.TrimSpace(strings.ToLower(raw))
	if kind == "" {
		return "", nil
	}
	if !oneOf(kind, marketingMediaKinds...) {
		return "", fmt.Errorf("media_kind must be one of %s", strings.Join(marketingMediaKinds, ", "))
	}
	return kind, nil
}

// marketingFormats is the closed set of campaign-kit formats. Kept in sync with
// the frontend registry, formats/formats.ts FORMAT_ORDER.
//
// The three original strings ("1:1", "4:5", "9:16") ARE the ids of the three
// original formats, not aliases for them. Keeping the spelling identical is what
// makes Wave 4 a zero-migration change: every stored creative_snapshot row
// already carries a valid format id.
//
// Unlike `composition` and `treatment` (which are charset-bounded because the
// frontend grows them every wave), a format id is whitelisted: each one implies
// concrete pixel dimensions, a platform-safe area, and — for "5:7" — a physical
// trim size. An unknown id here is a render this build cannot perform.
var marketingFormats = []string{"1:1", "4:5", "9:16", "wide", "strip", "5:7"}

// normalizeMarketingSnapshotKitFormats accepts nil/empty (legacy) or a deduped
// list drawn from marketingFormats, preserving first-seen order.
//
// Order is preserved rather than sorted because it is the operator's export
// order and the frontend renders the kit in it; sorting here would silently
// reorder the archive. Duplicates are dropped rather than rejected because a
// double-click on a format chip is a UI event, not a client bug.
func normalizeMarketingSnapshotKitFormats(raw []string) ([]string, error) {
	if len(raw) == 0 {
		return nil, nil
	}
	if len(raw) > len(marketingFormats) {
		return nil, fmt.Errorf("kit_formats must contain at most %d entries", len(marketingFormats))
	}
	seen := make(map[string]struct{}, len(raw))
	out := make([]string, 0, len(raw))
	for _, entry := range raw {
		format := strings.TrimSpace(entry)
		if !oneOf(format, marketingFormats...) {
			return nil, fmt.Errorf("kit_formats must contain only %s", strings.Join(marketingFormats, ", "))
		}
		if _, dup := seen[format]; dup {
			continue
		}
		seen[format] = struct{}{}
		out = append(out, format)
	}
	return out, nil
}

func normalizeAndValidateMarketingCreativeSnapshot(in *marketingCreativeSnapshotRequest) (*database.MarketingCreativeSnapshot, error) {
	if in == nil {
		return nil, fmt.Errorf("creative_snapshot is required for posted activity")
	}
	if !oneOf(in.Aspect, marketingFormats...) {
		return nil, fmt.Errorf("aspect must be one of %s", strings.Join(marketingFormats, ", "))
	}
	if !oneOf(in.Template, "editorial", "bold", "minimal") {
		return nil, fmt.Errorf("template must be one of editorial, bold, or minimal")
	}
	if !oneOf(in.ImageSource, "menu", "offer", "bundle", "gallery", "upload", "generated") {
		return nil, fmt.Errorf("image_source is invalid")
	}
	if utf8.RuneCountInString(in.Caption) > marketingSnapshotMaxCaptionRunes {
		return nil, fmt.Errorf("caption must be at most %d characters", marketingSnapshotMaxCaptionRunes)
	}
	for _, key := range []string{"headline", "dishName"} {
		if utf8.RuneCountInString(in.Slots[key]) > marketingSnapshotMaxHeadlineRunes {
			return nil, fmt.Errorf("%s must be at most %d characters", key, marketingSnapshotMaxHeadlineRunes)
		}
	}
	if utf8.RuneCountInString(in.Slots["cta"]) > marketingSnapshotMaxCTARunes {
		return nil, fmt.Errorf("cta must be at most %d characters", marketingSnapshotMaxCTARunes)
	}
	fontFamily, err := normalizeMarketingSnapshotFont(in.FontFamily)
	if err != nil {
		return nil, err
	}
	kit, err := normalizeMarketingSnapshotKit(in.Kit)
	if err != nil {
		return nil, err
	}
	composition, err := normalizeMarketingSnapshotIdent(in.Composition, "composition")
	if err != nil {
		return nil, err
	}
	treatment, err := normalizeMarketingSnapshotIdent(in.Treatment, "treatment")
	if err != nil {
		return nil, err
	}
	mediaKind, err := normalizeMarketingSnapshotMediaKind(in.MediaKind)
	if err != nil {
		return nil, err
	}
	motionPreset, err := normalizeMarketingSnapshotIdent(in.MotionPreset, "motion_preset")
	if err != nil {
		return nil, err
	}
	// A video with nothing to play is not a creative. This is the only
	// cross-field rule here, and it is worth the coupling: the alternative is a
	// Library row that badges itself as motion and then renders a still.
	if mediaKind == "video" && motionPreset == "" {
		return nil, fmt.Errorf("motion_preset is required when media_kind is video")
	}
	kitFormats, err := normalizeMarketingSnapshotKitFormats(in.KitFormats)
	if err != nil {
		return nil, err
	}
	destinationID, err := normalizeMarketingSnapshotIdent(in.DestinationID, "destination_id")
	if err != nil {
		return nil, err
	}
	postedChannel, err := normalizeMarketingPostedChannel(in.PostedChannel)
	if err != nil {
		return nil, err
	}
	promoCode, err := normalizeMarketingSnapshotIdent(in.PromoCode, "promo_code")
	if err != nil {
		return nil, err
	}
	printPreset, err := normalizeMarketingSnapshotIdent(in.PrintPreset, "print_preset")
	if err != nil {
		return nil, err
	}

	crop := database.MarketingCrop{X: 0.5, Y: 0.5, Zoom: 1}
	if in.Crop != nil {
		if in.Crop.X != nil {
			crop.X = *in.Crop.X
		}
		if in.Crop.Y != nil {
			crop.Y = *in.Crop.Y
		}
		if in.Crop.Zoom != nil {
			crop.Zoom = *in.Crop.Zoom
		}
	}
	if math.IsNaN(crop.X) || math.IsInf(crop.X, 0) {
		return nil, fmt.Errorf("crop.x must be finite")
	}
	if math.IsNaN(crop.Y) || math.IsInf(crop.Y, 0) {
		return nil, fmt.Errorf("crop.y must be finite")
	}
	if math.IsNaN(crop.Zoom) || math.IsInf(crop.Zoom, 0) {
		return nil, fmt.Errorf("crop.zoom must be finite")
	}
	if crop.X < 0 || crop.X > 1 {
		return nil, fmt.Errorf("crop.x must be between 0 and 1")
	}
	if crop.Y < 0 || crop.Y > 1 {
		return nil, fmt.Errorf("crop.y must be between 0 and 1")
	}
	if crop.Zoom < 1 || crop.Zoom > 3 {
		return nil, fmt.Errorf("crop.zoom must be between 1 and 3")
	}

	return &database.MarketingCreativeSnapshot{
		Caption:       in.Caption,
		ImageURL:      in.ImageURL,
		ImageSource:   in.ImageSource,
		Template:      in.Template,
		Aspect:        in.Aspect,
		Slots:         in.Slots,
		Crop:          crop,
		FontFamily:    fontFamily,
		Kit:           kit,
		Composition:   composition,
		Treatment:     treatment,
		MediaKind:     mediaKind,
		MotionPreset:  motionPreset,
		KitFormats:    kitFormats,
		DestinationID: destinationID,
		PostedChannel: postedChannel,
		PromoCode:     promoCode,
		PrintPreset:   printPreset,
	}, nil
}

const marketingPostedChannelMaxRunes = 40

// normalizeMarketingPostedChannel accepts "" or a short freeform channel tag
// the operator typed when confirming mark_posted. Spaces and common punctuation
// are allowed (e.g. "Instagram Stories"); control characters are rejected.
// This is never a schedule field.
func normalizeMarketingPostedChannel(raw string) (string, error) {
	v := strings.TrimSpace(raw)
	if v == "" {
		return "", nil
	}
	if utf8.RuneCountInString(v) > marketingPostedChannelMaxRunes {
		return "", fmt.Errorf("posted_channel must be at most %d characters", marketingPostedChannelMaxRunes)
	}
	for _, r := range v {
		if r < 32 || r == 127 {
			return "", fmt.Errorf("posted_channel must not contain control characters")
		}
	}
	return v, nil
}

func oneOf(value string, allowed ...string) bool {
	for _, candidate := range allowed {
		if value == candidate {
			return true
		}
	}
	return false
}

// actorFromContext resolves a short actor label for the created_by column from
// the gin context values the auth/RBAC middleware sets (address / staff_id).
func actorFromContext(c *gin.Context) string {
	if addr, ok := c.Get("address"); ok {
		if s, _ := addr.(string); s != "" {
			return s
		}
	}
	if sid, ok := c.Get("staff_id"); ok {
		switch v := sid.(type) {
		case string:
			if v != "" {
				return "staff:" + v
			}
		case uint:
			return "staff:" + strconv.FormatUint(uint64(v), 10)
		}
	}
	return ""
}

// GetMarketingActivity returns a paginated, filterable, newest-first page of
// marketing activity for the business. RBAC marketing:read.
func GetMarketingActivity(c *gin.Context) {
	business, ok := getAIMenuRouteBusiness(c)
	if !ok {
		return
	}
	page, _ := strconv.Atoi(c.Query("page"))
	perPage, _ := strconv.Atoi(c.Query("per_page"))
	q := database.MarketingActivityQuery{
		BusinessID: business.ID,
		Status:     strings.TrimSpace(c.Query("status")),
		Play:       strings.TrimSpace(c.Query("play")),
		Page:       page,
		PerPage:    perPage,
	}
	if from := parseActivityTime(c.Query("from")); from != nil {
		q.From = from
	}
	if to := parseActivityTime(c.Query("to")); to != nil {
		q.To = to
	}
	if handledFrom := parseActivityTime(c.Query("handled_from")); handledFrom != nil {
		q.HandledFrom = handledFrom
	}
	rows, total, err := database.GetDBWrapper().ListMarketingActivities(q)
	if err != nil {
		log.Printf("GetMarketingActivity: business %s: %v", business.BusinessId, err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to load activity"})
		return
	}
	q = q.Normalize()
	c.JSON(http.StatusOK, gin.H{
		"activity": rows,
		"total":    total,
		"page":     q.Page,
		"per_page": q.PerPage,
	})
}

func parseActivityTime(raw string) *time.Time {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return nil
	}
	if t, err := time.Parse(time.RFC3339, raw); err == nil {
		return &t
	}
	if t, err := time.Parse("2006-01-02", raw); err == nil {
		return &t
	}
	return nil
}

// RecordMarketingActivityHandler records a posted/dismissed/ready/approved
// activity or restores a dismissed one. Route RBAC is marketing:write; within
// the handler, approve is owner-only and staff may mark_posted only after
// approved (S2-E handoff). No due dates.
func RecordMarketingActivityHandler(c *gin.Context) {
	business, ok := getAIMenuRouteBusiness(c)
	if !ok {
		return
	}
	var req recordMarketingActivityRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid request"})
		return
	}
	isOwner := marketingActivityIsOwner(c)

	switch req.Action {
	case marketing.ActivityActionRestore:
		id := req.SuggestionID
		if id == "" && req.Suggestion != nil {
			id = req.Suggestion.ID
		}
		if id == "" {
			c.JSON(http.StatusBadRequest, gin.H{"error": "suggestion_id required"})
			return
		}
		restored, err := database.GetDBWrapper().RestoreMarketingActivity(business.ID, id)
		if err != nil {
			log.Printf("RecordMarketingActivity(restore): business %s: %v", business.BusinessId, err)
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to restore"})
			return
		}
		c.JSON(http.StatusOK, gin.H{"restored": restored})
		return

	case marketing.ActivityActionPost,
		marketing.ActivityActionDismiss,
		marketing.ActivityActionMarkReady,
		marketing.ActivityActionApprove:
		if req.Suggestion == nil || req.Suggestion.ID == "" {
			c.JSON(http.StatusBadRequest, gin.H{"error": "suggestion snapshot required"})
			return
		}

		current, err := loadCurrentMarketingActivityStatus(business.ID, req.Suggestion.ID)
		if err != nil {
			log.Printf("RecordMarketingActivity(load status): business %s: %v", business.BusinessId, err)
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to load activity"})
			return
		}

		if terr := marketing.ValidateHandoffTransition(req.Action, current, isOwner); terr != nil {
			code := "invalid_handoff_transition"
			statusCode := http.StatusConflict
			if ht, ok := terr.(*marketing.HandoffTransitionError); ok {
				code = ht.Code
				if ht.Code == "owner_required" || ht.Code == "approval_required" {
					statusCode = http.StatusForbidden
				}
			}
			c.JSON(statusCode, gin.H{"error": terr.Error(), "code": code})
			return
		}

		status := marketing.StatusForAction(req.Action)
		var creativeSnapshot *database.MarketingCreativeSnapshot

		// Creative snapshot required for post, mark_ready, and approve (approve
		// may reuse existing COALESCE snapshot if client omits it).
		needsSnapshot := req.Action == marketing.ActivityActionPost ||
			req.Action == marketing.ActivityActionMarkReady
		if req.Action == marketing.ActivityActionApprove {
			// Prefer a fresh snapshot if sent; otherwise allow empty so COALESCE
			// keeps the ready-state creative on the row.
			requestedSnapshot := req.CreativeSnapshot
			if requestedSnapshot == nil {
				requestedSnapshot = req.Suggestion.CreativeSnapshot
			}
			if requestedSnapshot != nil {
				// Root-level posted_channel wins when the snapshot omits it.
				if strings.TrimSpace(requestedSnapshot.PostedChannel) == "" && strings.TrimSpace(req.PostedChannel) != "" {
					requestedSnapshot.PostedChannel = req.PostedChannel
				}
				cs, nerr := normalizeAndValidateMarketingCreativeSnapshot(requestedSnapshot)
				if nerr != nil {
					c.JSON(http.StatusBadRequest, gin.H{
						"error": nerr.Error(),
						"code":  "invalid_creative_snapshot",
					})
					return
				}
				creativeSnapshot = cs
			}
		} else if needsSnapshot {
			requestedSnapshot := req.CreativeSnapshot
			if requestedSnapshot == nil {
				requestedSnapshot = req.Suggestion.CreativeSnapshot
			}
			if requestedSnapshot != nil {
				// S3-Loop: optional freeform channel on mark_posted (request root
				// or snapshot). Never a schedule field.
				if strings.TrimSpace(requestedSnapshot.PostedChannel) == "" && strings.TrimSpace(req.PostedChannel) != "" {
					requestedSnapshot.PostedChannel = req.PostedChannel
				}
			}
			cs, nerr := normalizeAndValidateMarketingCreativeSnapshot(requestedSnapshot)
			if nerr != nil {
				c.JSON(http.StatusBadRequest, gin.H{
					"error": nerr.Error(),
					"code":  "invalid_creative_snapshot",
				})
				return
			}
			creativeSnapshot = cs
		}

		row, rerr := database.GetDBWrapper().RecordMarketingActivity(database.MarketingActivity{
			BusinessID:       business.ID,
			SuggestionID:     req.Suggestion.ID,
			Play:             req.Suggestion.Play,
			Title:            req.Suggestion.Title,
			TargetName:       req.Suggestion.TargetName,
			Status:           status,
			ImageURL:         req.Suggestion.ImageURL,
			Caption:          req.Suggestion.Caption,
			CreativeSnapshot: creativeSnapshot,
			CreatedBy:        actorFromContext(c),
		})
		if rerr != nil {
			log.Printf("RecordMarketingActivity(%s): business %s: %v", req.Action, business.BusinessId, rerr)
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to record activity"})
			return
		}
		// S3-Loop: mark_posted feeds Director briefing/proactive insights — drop
		// the 60s owner-home caches so the next open shows the new post count.
		if status == "posted" {
			invalidateOwnerHomeCaches(business.ID)
		}
		c.JSON(http.StatusOK, gin.H{"activity": row})
		return

	default:
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid action", "code": "invalid_action"})
		return
	}
}

// GetMarketingSettingsHandler returns the business's automation settings.
// RBAC marketing:read.
func GetMarketingSettingsHandler(c *gin.Context) {
	business, ok := getAIMenuRouteBusiness(c)
	if !ok {
		return
	}
	s, err := database.GetDBWrapper().GetMarketingSettings(business.ID)
	if err != nil {
		log.Printf("GetMarketingSettings: business %s: %v", business.BusinessId, err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to load settings"})
		return
	}
	c.JSON(http.StatusOK, gin.H{
		"enabled":          s.Enabled,
		"disabled_plays":   s.DisabledPlays,
		"creative_profile": s.CreativeProfile,
	})
}

type putMarketingSettingsRequest struct {
	Enabled         bool                               `json:"enabled"`
	DisabledPlays   []string                           `json:"disabled_plays"`
	CreativeProfile *database.MarketingCreativeProfile `json:"creative_profile"`
}

// PutMarketingSettingsHandler validates + persists automation settings.
// RBAC marketing:write.
func PutMarketingSettingsHandler(c *gin.Context) {
	business, ok := getAIMenuRouteBusiness(c)
	if !ok {
		return
	}
	var req putMarketingSettingsRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid request"})
		return
	}
	var creativeProfile database.MarketingCreativeProfile
	if req.CreativeProfile != nil {
		creativeProfile = *req.CreativeProfile
	} else {
		stored, err := database.GetDBWrapper().GetMarketingSettings(business.ID)
		if err != nil {
			log.Printf("PutMarketingSettings: load existing settings for business %s: %v", business.BusinessId, err)
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to save settings"})
			return
		}
		creativeProfile = stored.CreativeProfile
	}
	settings := database.MarketingSettings{
		Enabled:         req.Enabled,
		DisabledPlays:   req.DisabledPlays,
		CreativeProfile: creativeProfile,
	}.Sanitized()
	if err := database.ValidateMarketingCreativeProfile(settings.CreativeProfile); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error(), "code": "invalid_marketing_settings"})
		return
	}
	if err := database.ValidateMarketingSettings(settings); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error(), "code": "invalid_play"})
		return
	}
	stored, err := database.GetDBWrapper().UpdateMarketingSettings(business.ID, settings)
	if err != nil {
		log.Printf("PutMarketingSettings: business %s: %v", business.BusinessId, err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to save settings"})
		return
	}
	c.JSON(http.StatusOK, gin.H{
		"enabled":          stored.Enabled,
		"disabled_plays":   stored.DisabledPlays,
		"creative_profile": stored.CreativeProfile,
	})
}
