package server

import (
	"strings"

	"github.com/gin-gonic/gin"

	"github.com/stdevmac/payverge/backend/internal/config"
	"github.com/stdevmac/payverge/backend/internal/database"
	"github.com/stdevmac/payverge/backend/internal/demomode"
)

// demoOutsideImageAction names the DEMO_MODE refusal for a menu, offer or
// bundle image that is not on this instance's own media storage.
const demoOutsideImageAction = "menu, offer and bundle images from outside the demo's own media storage"

// demoImageAllowed reports whether a menu, offer or bundle image URL may be
// written in DEMO_MODE. Those images show on the public storefront, and the
// demo hands the same owner session to everyone, so a visitor may clear an
// image, point at our own public media (validateStorefrontImageURL: the
// instance's /media path or the configured public bucket host), or keep a URL
// that is already stored. Uploads are refused in the demo, so in practice that
// means reusing the seeded photos.
func demoImageAllowed(raw string, stored map[string]struct{}) bool {
	raw = strings.TrimSpace(raw)
	if raw == "" || validateStorefrontImageURL(raw) == nil {
		return true
	}
	_, ok := stored[raw]
	return ok
}

// demoMenuImagesAllowed applies demoImageAllowed to every image of every item.
func demoMenuImagesAllowed(categories []database.MenuCategory, stored map[string]struct{}) bool {
	for _, category := range categories {
		for _, item := range category.Items {
			if !demoImageAllowed(item.Image, stored) || !demoImageAllowed(item.CompositionImage, stored) {
				return false
			}
			for _, image := range item.Images {
				if !demoImageAllowed(image, stored) {
					return false
				}
			}
		}
	}
	return true
}

// storedMenuImages returns every image URL in the business's current menu.
func storedMenuImages(businessID uint) map[string]struct{} {
	stored := map[string]struct{}{}
	_, categories, err := database.GetMenuByBusinessID(businessID)
	if err != nil {
		return stored
	}
	add := func(raw string) {
		if raw = strings.TrimSpace(raw); raw != "" {
			stored[raw] = struct{}{}
		}
	}
	for _, category := range categories {
		for _, item := range category.Items {
			add(item.Image)
			add(item.CompositionImage)
			for _, image := range item.Images {
				add(image)
			}
		}
	}
	return stored
}

// refuseDemoMenuImages writes the demo refusal and returns true when a menu
// write in DEMO_MODE carries an image URL from outside our own media that is
// not already in the business's menu. The stored menu is read only when an
// outside URL shows up.
func refuseDemoMenuImages(c *gin.Context, businessID uint, categories []database.MenuCategory) bool {
	if !config.DemoModeEnabled() || demoMenuImagesAllowed(categories, nil) {
		return false
	}
	if demoMenuImagesAllowed(categories, storedMenuImages(businessID)) {
		return false
	}
	demomode.Refuse(c, demomode.KindStorefront, demoOutsideImageAction)
	return true
}

// refuseDemoImage is refuseDemoMenuImages for one offer or bundle image:
// current is the stored value ("" on create), which may be kept.
func refuseDemoImage(c *gin.Context, next, current string) bool {
	if !config.DemoModeEnabled() {
		return false
	}
	stored := map[string]struct{}{}
	if current = strings.TrimSpace(current); current != "" {
		stored[current] = struct{}{}
	}
	if demoImageAllowed(next, stored) {
		return false
	}
	demomode.Refuse(c, demomode.KindStorefront, demoOutsideImageAction)
	return true
}
