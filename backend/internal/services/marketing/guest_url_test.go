package marketing

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestBuildGuestURL_Table(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name      string
		in        GuestURLInput
		wantURL   string
		wantKind  GuestURLKind
		wantEmpty bool
	}{
		{
			name: "page off",
			in: GuestURLInput{
				CustomURL: "bistro", PageEnabled: false, Play: PlayFeaturedDish,
			},
			wantEmpty: true,
		},
		{
			name: "empty slug",
			in: GuestURLInput{
				CustomURL: "", PageEnabled: true, Play: PlayFeaturedDish,
			},
			wantEmpty: true,
		},
		{
			name: "rejects path injection",
			in: GuestURLInput{
				CustomURL: "foo/bar", PageEnabled: true, Play: PlayFeaturedDish,
			},
			wantEmpty: true,
		},
		{
			name: "rejects absolute url as slug",
			in: GuestURLInput{
				CustomURL: "https://evil.example/x", PageEnabled: true, Play: PlayFeaturedDish,
			},
			wantEmpty: true,
		},
		{
			name: "rejects dots traversal",
			in: GuestURLInput{
				CustomURL: "..", PageEnabled: true, Play: PlayFeaturedDish,
			},
			wantEmpty: true,
		},
		{
			name: "rejects non-http origin",
			in: GuestURLInput{
				Origin: "javascript:alert(1)", CustomURL: "bistro", PageEnabled: true, Play: PlayFeaturedDish,
			},
			wantEmpty: true,
		},
		{
			name: "featured dish → menu tab",
			in: GuestURLInput{
				CustomURL: "bistro", PageEnabled: true, Play: PlayFeaturedDish,
			},
			wantURL:  DefaultGuestOrigin() + "/b/bistro?tab=menu",
			wantKind: GuestURLKindMenu,
		},
		{
			name: "offer → menu tab",
			in: GuestURLInput{
				CustomURL: "bistro", PageEnabled: true, Play: PlayOffer,
			},
			wantURL:  DefaultGuestOrigin() + "/b/bistro?tab=menu",
			wantKind: GuestURLKindMenu,
		},
		{
			name: "win_back → reservations tab",
			in: GuestURLInput{
				CustomURL: "bistro", PageEnabled: true, Play: PlayWinBack,
			},
			wantURL:  DefaultGuestOrigin() + "/b/bistro?tab=reservations",
			wantKind: GuestURLKindReservations,
		},
		{
			name: "custom origin",
			in: GuestURLInput{
				Origin: "http://127.0.0.1:3000", CustomURL: "local-cafe", PageEnabled: true, Play: PlayHappyHour,
			},
			wantURL:  "http://127.0.0.1:3000/b/local-cafe?tab=menu",
			wantKind: GuestURLKindMenu,
		},
		{
			name: "unknown play → business landing",
			in: GuestURLInput{
				CustomURL: "bistro", PageEnabled: true, Play: Play("future_play"),
			},
			wantURL:  DefaultGuestOrigin() + "/b/bistro",
			wantKind: GuestURLKindBusiness,
		},
	}

	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			got := BuildGuestURL(tc.in)
			if tc.wantEmpty {
				require.Empty(t, got.URL)
				require.Empty(t, got.Kind)
				return
			}
			require.Equal(t, tc.wantURL, got.URL)
			require.Equal(t, tc.wantKind, got.Kind)
			// Security: never emit private asset hosts.
			require.False(t, strings.Contains(got.URL, "s3."))
			require.False(t, strings.Contains(got.URL, "X-Amz-"))
			require.True(t, strings.HasPrefix(got.URL, "http://") || strings.HasPrefix(got.URL, "https://"))
		})
	}
}

func TestAttachGuestURLs_OnlyWhenPageLive(t *testing.T) {
	t.Parallel()
	sugs := []CampaignSuggestion{
		{Play: PlayFeaturedDish, Title: "Star"},
		{Play: PlayWinBack, Title: "Come back"},
	}
	AttachGuestURLs(sugs, "my-place", false, "")
	require.Empty(t, sugs[0].GuestURL)
	require.Empty(t, sugs[1].GuestURL)

	AttachGuestURLs(sugs, "my-place", true, "")
	require.Equal(t, DefaultGuestOrigin()+"/b/my-place?tab=menu", sugs[0].GuestURL)
	require.Equal(t, string(GuestURLKindMenu), sugs[0].GuestURLKind)
	require.Equal(t, DefaultGuestOrigin()+"/b/my-place?tab=reservations", sugs[1].GuestURL)
	require.Equal(t, string(GuestURLKindReservations), sugs[1].GuestURLKind)
}

func TestBuildGuestURL_NoQRWhenEmpty(t *testing.T) {
	// Contract for FE: empty URL ⇒ no QR asset. Pure guard.
	t.Parallel()
	got := BuildGuestURL(GuestURLInput{CustomURL: "", PageEnabled: true, Play: PlayOffer})
	require.Empty(t, got.URL)
}

func TestNormalizeGuestSlug_Edges(t *testing.T) {
	t.Parallel()
	require.Empty(t, normalizeGuestSlug("a"))                     // too short
	require.Empty(t, normalizeGuestSlug(strings.Repeat("x", 65))) // too long
	require.Empty(t, normalizeGuestSlug("has space"))
	require.Empty(t, normalizeGuestSlug("café"))
	require.Equal(t, "ok-slug_1", normalizeGuestSlug("  ok-slug_1  "))
	require.Empty(t, normalizeGuestSlug("foo?bar"))
	require.Empty(t, normalizeGuestSlug("foo#bar"))
}

func TestAttachGuestURLs_EmptySlugNoop(t *testing.T) {
	t.Parallel()
	sugs := []CampaignSuggestion{{Play: PlayOffer, Title: "x"}}
	AttachGuestURLs(sugs, "", true, "")
	require.Empty(t, sugs[0].GuestURL)
	AttachGuestURLs(sugs, "bad/slug", true, "")
	require.Empty(t, sugs[0].GuestURL)
}

func TestBuildGuestURL_AllPlays(t *testing.T) {
	t.Parallel()
	for _, play := range []Play{
		PlayFeaturedDish, PlayMoveItem, PlayComboDeal, PlayOffer, PlayHappyHour,
	} {
		got := BuildGuestURL(GuestURLInput{
			CustomURL: "cafe", PageEnabled: true, Play: play,
		})
		require.Equal(t, DefaultGuestOrigin()+"/b/cafe?tab=menu", got.URL, string(play))
		require.Equal(t, GuestURLKindMenu, got.Kind, string(play))
	}
}

func TestDefaultGuestOriginFollowsPublicURL(t *testing.T) {
	for _, k := range []string{"PUBLIC_URL", "FRONTEND_URL", "BASE_URL", "NEXT_PUBLIC_BASE_URL", "APP_BASE_URL"} {
		t.Setenv(k, "")
	}
	t.Setenv("PUBLIC_URL", "https://pos.example.com/")
	require.Equal(t, "https://pos.example.com", DefaultGuestOrigin())
	got := BuildGuestURL(GuestURLInput{CustomURL: "bistro", PageEnabled: true, Play: PlayFeaturedDish})
	require.Equal(t, "https://pos.example.com/b/bistro?tab=menu", got.URL)
}
