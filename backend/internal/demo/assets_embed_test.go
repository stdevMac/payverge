package demo

import (
	"fmt"
	"strings"
	"testing"

	"github.com/stdevmac/payverge/backend/internal/s3"
)

// TestSeedAssetsCoverEverySeededImageURL asserts the embedded asset tree is in
// lockstep with the URLs the seed writes into menus and business profiles. A
// menu item pointing at a demo-arg/assets key with no embedded file behind it would
// 404 on a fresh bucket — exactly the failure mode EnsureSeedAssets exists to
// prevent.
func TestSeedAssetsCoverEverySeededImageURL(t *testing.T) {
	keys, err := seedAssetKeys()
	if err != nil {
		t.Fatalf("seedAssetKeys: %v", err)
	}
	embedded := make(map[string]bool, len(keys))
	for _, k := range keys {
		embedded[k] = true
	}

	// Every seeded URL must resolve, through the configured public store, to
	// a key with an embedded file behind it.
	wanted := map[string]string{} // description -> seeded URL
	for id := range menuImages {
		wanted[fmt.Sprintf("menuImage(%s)", id)] = menuImage(id)
	}
	for id := range promoImages {
		wanted[fmt.Sprintf("promoImage(%s)", id)] = promoImage(id)
	}
	for _, p := range profiles() {
		wanted[fmt.Sprintf("profile %s hero", p.Key)] = p.Hero
	}

	referenced := make(map[string]bool)
	for desc, url := range wanted {
		key := strings.TrimPrefix(url, demoAssetURL(""))
		if key == url || url == "" {
			t.Errorf("%s -> %q: not under the demo asset base %q", desc, url, demoAssetURL(""))
			continue
		}
		key = seedAssetPrefix + "/" + key
		if url != s3.PublicURL(key) {
			t.Errorf("%s -> %q: want s3.PublicURL(%q) = %q", desc, url, key, s3.PublicURL(key))
		}
		referenced[key] = true
		if !embedded[key] {
			t.Errorf("%s -> %s: no embedded file at assets/%s", desc, url, strings.TrimPrefix(key, seedAssetPrefix+"/"))
		}
	}

	// And the inverse: every embedded file is actually referenced, so the
	// binary never carries dead image weight.
	for _, k := range keys {
		if !referenced[k] {
			t.Errorf("embedded asset %s is not referenced by any seeded URL", k)
		}
	}
}

// TestDemoAssetURLsFollowTheStorageDriver pins the URL contract: on local
// storage (no CDN) seeded images are same-origin /media URLs under
// PUBLIC_URL, behind a CDN they are "<cdn>/demo-arg/assets/...". Neither may
// point at a hard-coded third-party host.
func TestDemoAssetURLsFollowTheStorageDriver(t *testing.T) {
	restore := s3.SetPublicURL("https://pos.example.test")
	defer restore()

	got := menuImage("demo-provoleta")
	want := "https://pos.example.test/media/demo-arg/assets/carta/provoleta.jpg"
	if got != want {
		t.Fatalf("menuImage on local storage = %q, want %q", got, want)
	}
	if !s3.IsOwnMediaURL(got) {
		t.Fatalf("seeded URL %q must be recognised as our own /media URL", got)
	}
	if !isOwnHostedDemoAsset(got) || !isOwnHostedDemoAsset(profiles()[0].Hero) {
		t.Fatal("seeded demo assets must count as own-hosted (never re-hosted)")
	}
	if isOwnHostedDemoAsset("https://images.unsplash.com/photo-1.jpg") {
		t.Fatal("a third-party URL must not count as own-hosted")
	}
	if menuImage("no-such-item") != "" || promoImage("no-such-promo") != "" {
		t.Fatal("unknown ids must resolve to an empty image")
	}
}

func TestEnsureSeedAssetsUploadsOnlyMissing(t *testing.T) {
	keys, err := seedAssetKeys()
	if err != nil {
		t.Fatalf("seedAssetKeys: %v", err)
	}
	if len(keys) == 0 {
		t.Fatal("no embedded seed assets")
	}
	present := map[string]bool{keys[0]: true}

	var uploads []string
	uploaded, err := EnsureSeedAssets(
		func(key string) (bool, error) { return present[key], nil },
		func(data []byte, name, folderPath, contentType string) (string, error) {
			if len(data) == 0 {
				t.Fatalf("empty upload body for %s/%s", folderPath, name)
			}
			if contentType != "image/jpeg" {
				t.Fatalf("unexpected content type %q", contentType)
			}
			uploads = append(uploads, folderPath+"/"+name)
			return s3.PublicURL(folderPath + "/" + name), nil
		},
	)
	if err != nil {
		t.Fatalf("EnsureSeedAssets: %v", err)
	}
	if uploaded != len(keys)-1 || len(uploads) != len(keys)-1 {
		t.Fatalf("uploaded %d (%d calls), want %d", uploaded, len(uploads), len(keys)-1)
	}
	for _, u := range uploads {
		if u == keys[0] {
			t.Fatalf("re-uploaded already-present key %s", u)
		}
		if !strings.HasPrefix(u, seedAssetPrefix+"/") {
			t.Fatalf("upload key %s not under %s/", u, seedAssetPrefix)
		}
	}
}

func TestEnsureSeedAssetsAggregatesErrorsAndContinues(t *testing.T) {
	keys, err := seedAssetKeys()
	if err != nil {
		t.Fatalf("seedAssetKeys: %v", err)
	}
	if len(keys) < 2 {
		t.Skip("needs at least two embedded assets")
	}

	var uploads int
	uploaded, err := EnsureSeedAssets(
		func(key string) (bool, error) {
			if key == keys[0] {
				return false, fmt.Errorf("bucket unreachable")
			}
			return false, nil
		},
		func(data []byte, name, folderPath, contentType string) (string, error) {
			uploads++
			return s3.PublicURL(folderPath + "/" + name), nil
		},
	)
	if err == nil {
		t.Fatal("expected aggregated error")
	}
	if !strings.Contains(err.Error(), "bucket unreachable") {
		t.Fatalf("error should carry the head failure, got: %v", err)
	}
	if uploaded != len(keys)-1 || uploads != len(keys)-1 {
		t.Fatalf("healthy keys should still upload: uploaded=%d calls=%d want=%d", uploaded, uploads, len(keys)-1)
	}
}
