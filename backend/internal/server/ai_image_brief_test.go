package server

import (
	"context"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/stdevmac/payverge/backend/internal/services"
)

func TestImageJobBriefKey_DistinguishesMaterialFields(t *testing.T) {
	base := imageJobBrief{
		BusinessID:  7,
		Tool:        "generate",
		Name:        "House Burger",
		Description: "Angus beef",
		EntityType:  "menu_item",
		Currency:    "USD",
	}
	baseKey := imageJobBriefKey(base)

	cases := []struct {
		name string
		mut  func(imageJobBrief) imageJobBrief
	}{
		{"description", func(b imageJobBrief) imageJobBrief { b.Description = "Wagyu beef"; return b }},
		{"prompt", func(b imageJobBrief) imageJobBrief { b.Prompt = "side angle"; return b }},
		{"source", func(b imageJobBrief) imageJobBrief { b.SourceSHA256 = "abc"; return b }},
		{"entity", func(b imageJobBrief) imageJobBrief { b.EntityType = "offer"; return b }},
		{"scope", func(b imageJobBrief) imageJobBrief { b.OfferScope = "menu"; return b }},
		{"related", func(b imageJobBrief) imageJobBrief { b.Related = []string{"Fries"}; return b }},
		{"bundle", func(b imageJobBrief) imageJobBrief {
			b.BundleItems = []canonicalBundleItem{{Name: "Burger", Quantity: 2}}
			return b
		}},
		{"price", func(b imageJobBrief) imageJobBrief { b.BundlePrice = "19.9900"; return b }},
		{"currency", func(b imageJobBrief) imageJobBrief { b.Currency = "EUR"; return b }},
		{"aspect", func(b imageJobBrief) imageJobBrief { b.AspectRatio = "16:9"; return b }},
		{"tool", func(b imageJobBrief) imageJobBrief { b.Tool = "regenerate"; return b }},
		{"ingredients", func(b imageJobBrief) imageJobBrief { b.Ingredients = "beef, bun"; return b }},
		{"stamp", func(b imageJobBrief) imageJobBrief { b.OfferStamp = "2x1"; return b }},
		{"dietary", func(b imageJobBrief) imageJobBrief { b.DietaryTags = []string{"vegetarian"}; return b }},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			assert.NotEqual(t, baseKey, imageJobBriefKey(tc.mut(base)), "field %s must change the key", tc.name)
		})
	}
}

func TestImageJobBriefKey_NormalizesWhitespaceAndCase(t *testing.T) {
	a := imageJobBrief{BusinessID: 1, Tool: "Generate", Name: "  House   Burger ", Description: "tasty"}
	b := imageJobBrief{BusinessID: 1, Tool: "generate", Name: "house burger", Description: "tasty"}
	assert.Equal(t, imageJobBriefKey(a), imageJobBriefKey(b))

	// Related order is insignificant.
	r1 := imageJobBrief{BusinessID: 1, Tool: "generate", Name: "x", Related: []string{"B", "A"}}
	r2 := imageJobBrief{BusinessID: 1, Tool: "generate", Name: "x", Related: []string{"A", "B"}}
	assert.Equal(t, imageJobBriefKey(r1), imageJobBriefKey(r2))

	// Dietary tag order and case are insignificant; nil and empty collapse to
	// the same key (backward compatibility with pre-#601 briefs).
	d1 := imageJobBrief{BusinessID: 1, Tool: "regenerate", Name: "x", DietaryTags: []string{"Vegan", "halal"}}
	d2 := imageJobBrief{BusinessID: 1, Tool: "regenerate", Name: "x", DietaryTags: []string{"halal", "vegan"}}
	assert.Equal(t, imageJobBriefKey(d1), imageJobBriefKey(d2))
	d3 := imageJobBrief{BusinessID: 1, Tool: "regenerate", Name: "x"}
	d4 := imageJobBrief{BusinessID: 1, Tool: "regenerate", Name: "x", DietaryTags: []string{}}
	assert.Equal(t, imageJobBriefKey(d3), imageJobBriefKey(d4))
}

func TestImageJobBriefKey_FromMenuImagePrompt(t *testing.T) {
	p1 := services.MenuImagePrompt{
		Name: "Pizza", Description: "cheese", EntityType: "menu_item",
		RelatedItemNames: []string{"Salad"}, BundlePrice: 12.5, Currency: "usd",
	}
	p2 := p1
	p2.Description = "pepperoni"
	k1 := imageJobBriefKey(briefFromMenuImagePrompt(3, "generate", p1))
	k2 := imageJobBriefKey(briefFromMenuImagePrompt(3, "generate", p2))
	assert.NotEqual(t, k1, k2)
	// Same name alone must not collide across descriptions (the bug we fix).
	assert.NotEqual(t, imageJobBriefKey(imageJobBrief{BusinessID: 3, Tool: "generate", Name: "Pizza"}), k1)
}

func TestDoSharedImageJob_CallerCancelDoesNotCancelSharedWork(t *testing.T) {
	var started atomic.Int32
	var finished atomic.Int32
	release := make(chan struct{})

	// Shared work blocks until release; caller A cancels before release.
	key := imageJobBriefKey(imageJobBrief{BusinessID: 9, Tool: "generate", Name: "shared-cancel"})

	callerA, cancelA := context.WithCancel(context.Background())
	callerB := context.Background()

	var wg sync.WaitGroup
	var errA, errB error
	var valB interface{}

	wg.Add(2)
	go func() {
		defer wg.Done()
		_, errA = doSharedImageJob(callerA, key, func(ctx context.Context) (interface{}, error) {
			started.Add(1)
			select {
			case <-release:
				finished.Add(1)
				return "ok", nil
			case <-ctx.Done():
				// Shared work must NOT observe caller A's cancel.
				return nil, ctx.Err()
			}
		})
	}()
	go func() {
		defer wg.Done()
		// Ensure A has entered singleflight first.
		time.Sleep(20 * time.Millisecond)
		valB, errB = doSharedImageJob(callerB, key, func(ctx context.Context) (interface{}, error) {
			// Should not run a second body.
			started.Add(1)
			<-release
			finished.Add(1)
			return "ok", nil
		})
	}()

	// Wait for shared body to start, then cancel A only.
	require.Eventually(t, func() bool { return started.Load() >= 1 }, time.Second, 5*time.Millisecond)
	cancelA()
	time.Sleep(30 * time.Millisecond)
	close(release)
	wg.Wait()

	require.ErrorIs(t, errA, context.Canceled)
	require.NoError(t, errB)
	assert.Equal(t, "ok", valB)
	assert.Equal(t, int32(1), started.Load(), "shared body must run once")
	assert.Equal(t, int32(1), finished.Load(), "shared body must complete despite caller cancel")
}
