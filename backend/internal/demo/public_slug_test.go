package demo

import (
	"context"
	"fmt"
	"strconv"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
	"gorm.io/gorm"

	"github.com/stdevmac/payverge/backend/internal/database"
)

// Finding 35: public CustomURL must never embed admin user id or plan-tier
// keys (e.g. demo-admin-1-ai-pro-demo-lounge). Default slug = display name
// only, with a numeric disambiguator on collision.
func TestPublicCustomURL_NoInternalIdentifiers(t *testing.T) {
	db := newDemoServiceTestDB(t)
	admin := seedAdmin(t, db, "slug-admin@example.com")

	svc := NewService(db, Options{Now: fixedNow, SeedVersion: "test-seed", BaselineDays: 7})
	_, err := svc.EnsureForAdmin(context.Background(), admin.ID)
	require.NoError(t, err)

	var businesses []database.Business
	require.NoError(t, db.Where("demo_owner_user_id = ?", admin.ID).Find(&businesses).Error)
	require.Len(t, businesses, 2)

	adminIDStr := strconv.FormatUint(uint64(admin.ID), 10)
	for _, b := range businesses {
		slug := strings.ToLower(b.CustomURL)
		require.NotEmpty(t, slug, "business %q must have a public custom_url", b.Name)

		// No legacy admin-key pattern.
		require.NotContains(t, slug, "demo-admin", "slug must not leak demo-admin prefix: %q", slug)
		require.NotContains(t, slug, "admin-"+adminIDStr, "slug must not embed admin user id: %q", slug)
		// Plan-tier internal keys must not be structural prefixes/suffixes of
		// the old generator (CustomURLSuffix was "ai-pro-demo-lounge" / "core-demo-kitchen"
		// behind demo-admin-{id}-).
		require.False(t, strings.HasPrefix(slug, "demo-"), "reserved demo- prefix: %q", slug)
		require.False(t, strings.Contains(slug, "admin-"), "slug must not contain admin- key: %q", slug)

		// Derived from display name only (hyphenated lowercase of Name).
		base := database.SlugifyDisplayName(b.Name)
		require.True(t,
			slug == base || strings.HasPrefix(slug, base+"-"),
			"slug %q must be derived from display name base %q", slug, base,
		)
	}
}

func TestPublicCustomURL_CollisionGetsNumericDisambiguator(t *testing.T) {
	db := newDemoServiceTestDB(t)

	// Seed a venue that already owns the display-name base slug.
	name := "Parrilla Quebracho Azul"
	base := database.SlugifyDisplayName(name)
	require.Equal(t, "parrilla-quebracho-azul", base)
	require.NoError(t, db.Create(&database.Business{
		BusinessId: "preexisting-slug-holder",
		Name:       "Someone Else",
		CustomURL:  base,
		IsActive:   true,
	}).Error)

	admin := seedAdmin(t, db, "slug-collision@example.com")
	svc := NewService(db, Options{Now: fixedNow, SeedVersion: "test-seed", BaselineDays: 7})
	_, err := svc.EnsureForAdmin(context.Background(), admin.ID)
	require.NoError(t, err)

	var aiPro database.Business
	require.NoError(t, db.Where("demo_owner_user_id = ? AND name = ?", admin.ID, name).First(&aiPro).Error)
	require.Equal(t, base+"-2", aiPro.CustomURL,
		"collision must append numeric disambiguator, got %q", aiPro.CustomURL)
}

func TestSlugifyDisplayName(t *testing.T) {
	require.Equal(t, "bodegon-mesa-larga", database.SlugifyDisplayName("Bodegón Mesa Larga"))
	require.Equal(t, "parrilla-quebracho-azul", database.SlugifyDisplayName("Parrilla Quebracho Azul"))
	require.Equal(t, "cafe-aurora", database.SlugifyDisplayName("  Café Aurora!! "))
	require.Equal(t, "", database.SlugifyDisplayName("!!!"))
}

func TestIsReservedPublicSlug(t *testing.T) {
	for _, s := range []string{"admin", "api", "demo", "b", "internal", "demo-foo", "admin-1", "b-x", "API", "Demo-Lounge"} {
		require.True(t, database.IsReservedPublicSlug(s), s)
	}
	for _, s := range []string{"payverge-core-demo-kitchen", "my-restaurant", "aurora", "demoed", "cab"} {
		require.False(t, database.IsReservedPublicSlug(s), s)
	}
}

func TestAllocatePublicCustomURL_SkipsReservedBase(t *testing.T) {
	db := newDemoServiceTestDB(t)
	err := db.Transaction(func(tx *gorm.DB) error {
		slug, err := database.AllocatePublicCustomURL(context.Background(), tx, "Demo", 0)
		require.NoError(t, err)
		require.False(t, database.IsReservedPublicSlug(slug), slug)
		require.True(t, strings.HasPrefix(slug, "venue-"), "reserved base must be namespaced: %q", slug)
		return nil
	})
	require.NoError(t, err)
}

// Ensure two admins with identical display names get distinct public slugs.
func TestPublicCustomURL_IsolatedAcrossAdmins(t *testing.T) {
	db := newDemoServiceTestDB(t)
	adminA := seedAdmin(t, db, "slug-a@example.com")
	adminB := seedAdmin(t, db, "slug-b@example.com")
	svc := NewService(db, Options{Now: fixedNow, SeedVersion: "test-seed", BaselineDays: 7})

	_, err := svc.EnsureForAdmin(context.Background(), adminA.ID)
	require.NoError(t, err)
	_, err = svc.EnsureForAdmin(context.Background(), adminB.ID)
	require.NoError(t, err)

	var a, b []database.Business
	require.NoError(t, db.Where("demo_owner_user_id = ?", adminA.ID).Find(&a).Error)
	require.NoError(t, db.Where("demo_owner_user_id = ?", adminB.ID).Find(&b).Error)
	require.Len(t, a, 2)
	require.Len(t, b, 2)

	seen := map[string]struct{}{}
	for _, biz := range append(a, b...) {
		require.NotEmpty(t, biz.CustomURL)
		require.NotContains(t, strings.ToLower(biz.CustomURL), "demo-admin")
		key := strings.ToLower(biz.CustomURL)
		_, dup := seen[key]
		require.False(t, dup, "duplicate public slug %q", key)
		seen[key] = struct{}{}
	}
	// Both admins share the same two display names — second admin must get -2 variants.
	require.Contains(t, seen, "bodegon-mesa-larga")
	require.Contains(t, seen, "parrilla-quebracho-azul")
	require.Contains(t, seen, "bodegon-mesa-larga-2")
	require.Contains(t, seen, "parrilla-quebracho-azul-2")
}

func TestLegacyAdminKeySlugIsRewrittenOnEnsure(t *testing.T) {
	db := newDemoServiceTestDB(t)
	admin := seedAdmin(t, db, "slug-rewrite@example.com")

	// Pre-seed the AI Pro business with the leaky legacy custom_url.
	legacy := fmt.Sprintf("demo-admin-%d-ai-pro-demo-lounge", admin.ID)
	bid := fmt.Sprintf("demo-admin-%d-secondary", admin.ID)
	require.NoError(t, db.Create(&database.Business{
		BusinessId:      bid,
		Name:            "Payverge AI Pro Demo Lounge",
		CustomURL:       legacy,
		IsActive:        true,
		IsDemo:          true,
		DemoOwnerUserID: &admin.ID,
		UserID:          &admin.ID,
		Kind:            database.BusinessKindDemo,
	}).Error)

	svc := NewService(db, Options{Now: fixedNow, SeedVersion: "test-seed", BaselineDays: 7})
	_, err := svc.EnsureForAdmin(context.Background(), admin.ID)
	require.NoError(t, err)

	var biz database.Business
	require.NoError(t, db.Where("business_id = ?", bid).First(&biz).Error)
	require.NotEqual(t, legacy, biz.CustomURL)
	require.NotContains(t, strings.ToLower(biz.CustomURL), "demo-admin")
	require.True(t, strings.HasPrefix(biz.CustomURL, database.SlugifyDisplayName(biz.Name)))
}
