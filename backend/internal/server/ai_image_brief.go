package server

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"strings"
	"time"
	"unicode"

	"github.com/stdevmac/payverge/backend/internal/llm"
	"github.com/stdevmac/payverge/backend/internal/services"
)

// imageJobBrief is the canonical, order-stable description of an image
// generation request used for single-flight coalescing. Every field that can
// change the rendered image must be represented; only order-insensitive
// collections are sorted before hashing.
type imageJobBrief struct {
	BusinessID   uint                  `json:"business_id"`
	Tool         string                `json:"tool"`
	Name         string                `json:"name"`
	Description  string                `json:"description"`
	Prompt       string                `json:"prompt"`
	SourceSHA256 string                `json:"source_sha256"`
	EntityType   string                `json:"entity_type"`
	OfferScope   string                `json:"offer_scope"`
	Related      []string              `json:"related"`
	BundleItems  []canonicalBundleItem `json:"bundle_items"`
	BundlePrice  string                `json:"bundle_price"`
	Currency     string                `json:"currency"`
	AspectRatio  string                `json:"aspect_ratio"`
	OfferStamp   string                `json:"offer_stamp"`
	Ingredients  string                `json:"ingredients"`
	// DietaryTags change the rendered prompt (#588/#601) so they must be part
	// of the coalescing key. omitempty keeps tagless keys byte-identical to the
	// pre-#601 shape.
	DietaryTags []string `json:"dietary_tags,omitempty"`
}

type canonicalBundleItem struct {
	Name     string `json:"name"`
	Quantity int    `json:"quantity"`
}

// normalizeBriefText trims and collapses internal whitespace for stable keys.
func normalizeBriefText(s string) string {
	return strings.Join(strings.FieldsFunc(strings.TrimSpace(s), unicode.IsSpace), " ")
}

// imageJobBriefKey returns a stable SHA-256 hex key for the brief.
func imageJobBriefKey(b imageJobBrief) string {
	b.Tool = normalizeBriefText(strings.ToLower(b.Tool))
	b.Name = normalizeBriefText(strings.ToLower(b.Name))
	b.Description = normalizeBriefText(b.Description)
	b.Prompt = normalizeBriefText(b.Prompt)
	b.EntityType = normalizeBriefText(strings.ToLower(b.EntityType))
	b.OfferScope = normalizeBriefText(strings.ToLower(b.OfferScope))
	b.Currency = normalizeBriefText(strings.ToUpper(b.Currency))
	b.AspectRatio = normalizeBriefText(b.AspectRatio)
	b.OfferStamp = normalizeBriefText(b.OfferStamp)
	b.Ingredients = normalizeBriefText(b.Ingredients)
	b.SourceSHA256 = strings.ToLower(strings.TrimSpace(b.SourceSHA256))

	// Dietary tags are order-insensitive for coalescing.
	if len(b.DietaryTags) > 0 {
		norm := make([]string, 0, len(b.DietaryTags))
		for _, t := range b.DietaryTags {
			if n := normalizeBriefText(strings.ToLower(t)); n != "" {
				norm = append(norm, n)
			}
		}
		sortStrings(norm)
		b.DietaryTags = norm
	}

	// Related names are order-insensitive for coalescing.
	if len(b.Related) > 0 {
		norm := make([]string, 0, len(b.Related))
		for _, r := range b.Related {
			if n := normalizeBriefText(strings.ToLower(r)); n != "" {
				norm = append(norm, n)
			}
		}
		sortStrings(norm)
		b.Related = norm
	}

	// Bundle items preserve declared order (quantity/name matter for layout)
	// but names are normalized.
	for i := range b.BundleItems {
		b.BundleItems[i].Name = normalizeBriefText(strings.ToLower(b.BundleItems[i].Name))
	}

	raw, err := json.Marshal(b)
	if err != nil {
		// Extremely unlikely; fall back to a non-colliding key.
		return fmt.Sprintf("marshal-err|%d|%s|%s", b.BusinessID, b.Tool, b.Name)
	}
	sum := sha256.Sum256(raw)
	return hex.EncodeToString(sum[:])
}

func sortStrings(ss []string) {
	// Simple insertion sort — related lists are small (≤30).
	for i := 1; i < len(ss); i++ {
		j := i
		for j > 0 && ss[j-1] > ss[j] {
			ss[j-1], ss[j] = ss[j], ss[j-1]
			j--
		}
	}
}

// sourceImageSHA256 returns the hex SHA-256 of raw image bytes, or "" if empty.
func sourceImageSHA256(data []byte) string {
	if len(data) == 0 {
		return ""
	}
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:])
}

// briefFromMenuImagePrompt builds a brief for the generate tool.
func briefFromMenuImagePrompt(businessID uint, tool string, p services.MenuImagePrompt) imageJobBrief {
	items := make([]canonicalBundleItem, 0, len(p.BundleItems))
	for _, it := range p.BundleItems {
		items = append(items, canonicalBundleItem{Name: it.Name, Quantity: it.Quantity})
	}
	price := ""
	if p.BundlePrice != 0 {
		price = fmt.Sprintf("%.4f", p.BundlePrice)
	}
	return imageJobBrief{
		BusinessID:  businessID,
		Tool:        tool,
		Name:        p.Name,
		Description: p.Description,
		EntityType:  p.EntityType,
		OfferScope:  p.OfferScope,
		Related:     append([]string(nil), p.RelatedItemNames...),
		BundleItems: items,
		BundlePrice: price,
		Currency:    p.Currency,
		OfferStamp:  p.OfferStampText,
		Ingredients: p.Ingredients,
		DietaryTags: append([]string(nil), p.DietaryTags...),
	}
}

// doSharedImageJob runs fn under singleflight with an independent timeout so
// one caller's cancellation never cancels shared generation work. The waiting
// caller still observes its own context cancellation. timeout <= 0 uses the
// standard image feature budget.
func doSharedImageJob(callerCtx context.Context, key string, fn func(context.Context) (interface{}, error)) (interface{}, error) {
	return doSharedImageJobWithTimeout(callerCtx, key, llm.FeatureTimeout("image"), fn)
}

// doSharedImageJobWithTimeout is the same as doSharedImageJob but allows a
// longer feature-specific budget (e.g. marketing composition).
func doSharedImageJobWithTimeout(callerCtx context.Context, key string, timeout time.Duration, fn func(context.Context) (interface{}, error)) (interface{}, error) {
	if timeout <= 0 {
		timeout = llm.FeatureTimeout("image")
	}
	ch := imageJobGroup.DoChan(key, func() (interface{}, error) {
		jobCtx, cancel := context.WithTimeout(context.WithoutCancel(callerCtx), timeout)
		defer cancel()
		return fn(jobCtx)
	})
	select {
	case <-callerCtx.Done():
		return nil, callerCtx.Err()
	case res := <-ch:
		return res.Val, res.Err
	}
}
