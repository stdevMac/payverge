package services

import (
	"context"
	"image"
	"image/color"
	"strings"
	"testing"

	"github.com/stdevmac/payverge/backend/internal/llm"
	"github.com/stdevmac/payverge/backend/internal/s3"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// useLocalImageStore points the s3 package wrappers at a temp-dir LocalStore
// with relative /media URLs (the zero-config self-hosted default).
func useLocalImageStore(t *testing.T) s3.Store {
	t.Helper()
	public, err := s3.NewLocalStore(t.TempDir())
	require.NoError(t, err)
	protected, err := s3.NewLocalStore(t.TempDir())
	require.NoError(t, err)
	t.Cleanup(s3.SetStores(public, protected))
	t.Cleanup(s3.SetPublicURL(""))
	return public
}

// Item names carry accents, parentheses and even slashes. They used to be
// spliced into the object key verbatim (lowercased, spaces -> "_"), which the
// local driver rejects and which let "../" reach an S3 key. The folder segment
// is now s3.SafeKeySegment(itemName).
func TestAIItemImagesUseSafeKeySegmentsOnLocalStorage(t *testing.T) {
	store := useLocalImageStore(t)
	canvas := image.NewRGBA(image.Rect(0, 0, 32, 32))
	for y := 0; y < 32; y++ {
		for x := 0; x < 32; x++ {
			canvas.SetRGBA(x, y, color.RGBA{R: 200, G: uint8(x * 8), B: uint8(y * 8), A: 255})
		}
	}
	pngBytes := encodePNG(t, canvas)

	cap := &capturingMenuProvider{resp: &llm.Response{Images: []llm.ImageOutput{{MIMEType: "image/png", Data: pngBytes}}}}
	svc := NewMenuAIService(cap, llm.ModelConfig{Menu: "m", Image: "i"})

	cases := []struct {
		run    func(name string) (*GeneratedImage, error)
		folder string
	}{
		{
			run: func(name string) (*GeneratedImage, error) {
				return svc.RegenerateItemImage(context.Background(), 91, name, "d", "", nil)
			},
			folder: "/media/menu_items/ai_generated/noquis_de_papa_400_g_etc/",
		},
		{
			run: func(name string) (*GeneratedImage, error) {
				return svc.EnhanceItemImage(context.Background(), 91, pngBytes, "image/png", name, "d", "1:1", nil)
			},
			folder: "/media/menu_items/ai_enhanced/noquis_de_papa_400_g_etc/",
		},
	}
	for _, tc := range cases {
		img, err := tc.run("Ñoquis de papá (400 g) ../../etc")
		require.NoError(t, err)
		require.NotNil(t, img)
		assert.True(t, strings.HasPrefix(img.URL, tc.folder), img.URL)

		key, err := s3.KeyFromURL(img.URL)
		require.NoError(t, err)
		require.NoError(t, s3.ValidateKey(key))
		ok, err := store.Exists(context.Background(), key)
		require.NoError(t, err)
		assert.True(t, ok, "generated image must be stored at %s", key)
	}
}
