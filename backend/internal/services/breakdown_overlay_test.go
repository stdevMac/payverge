package services

import (
	"bytes"
	"context"
	"errors"
	"image"
	"image/color"
	"image/jpeg"
	"image/png"
	"strings"
	"testing"

	"github.com/fogleman/gg"
	"github.com/stretchr/testify/require"

	"github.com/stdevmac/payverge/backend/internal/llm"
)

type staticBreakdownImageProvider struct {
	data []byte
	mime string
}

func (p *staticBreakdownImageProvider) Generate(_ context.Context, _ llm.GenerateRequest) (*llm.Response, error) {
	return &llm.Response{Images: []llm.ImageOutput{{MIMEType: p.mime, Data: p.data}}}, nil
}

func TestSplitIngredientNames_DoesNotInventAndKeepsSpelling(t *testing.T) {
	tests := []struct {
		name string
		raw  string
		want []string
	}{
		{
			name: "harvest bowl description stays vegetarian tokens only",
			raw:  "Roasted vegetables, grains, herbs",
			want: []string{"Roasted vegetables", "grains", "herbs"},
		},
		{
			name: "exact vinaigrette spelling is preserved character for character",
			raw:  "Maple-Dijon Vinaigrette, Toasted Seeds & Herbs",
			want: []string{"Maple-Dijon Vinaigrette", "Toasted Seeds & Herbs"},
		},
		{
			name: "cocktail description is one caption, not a garbled title",
			raw:  "Bright aperitif cocktail",
			want: []string{"Bright aperitif cocktail"},
		},
		{
			name: "newlines and bullets become list items",
			raw:  "Aperitivo\n• Prosecco\nSoda water",
			want: []string{"Aperitivo", "Prosecco", "Soda water"},
		},
		{
			name: "empty input yields nothing",
			raw:  "   ",
			want: nil,
		},
		{
			name: "duplicates collapse without inventing replacements",
			raw:  "kale, Kale, quinoa",
			want: []string{"kale", "quinoa"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := splitIngredientNames(tt.raw)
			require.Equal(t, tt.want, got)
			joined := strings.ToLower(strings.Join(got, " "))
			for _, banned := range []string{"chicken", "beef", "pork", "fish", "shrimp", "bacon"} {
				require.NotContains(t, joined, banned, "splitter must not invent animal protein")
			}
		})
	}
}

func TestBreakdownLayerLabels_PrefersIngredientsAndNeverAddsDishTitle(t *testing.T) {
	labels := breakdownLayerLabels(MenuImagePrompt{
		Name:        "Demo Spritz",
		Description: "Bright aperitif cocktail",
		Ingredients: "Aperitivo, Prosecco, Soda water, Orange slice",
	})
	require.Equal(t, []string{"Aperitivo", "Prosecco", "Soda water", "Orange slice"}, labels)
	for _, label := range labels {
		require.NotEqual(t, "Demo Spritz", label)
		require.NotContains(t, strings.ToUpper(label), "APERITIFI")
	}
}

func paintLayerBlob(img *image.RGBA, cx, cy, rx, ry int, c color.RGBA) {
	b := img.Bounds()
	for y := cy - ry; y <= cy+ry; y++ {
		for x := cx - rx; x <= cx+rx; x++ {
			if x < b.Min.X || y < b.Min.Y || x >= b.Max.X || y >= b.Max.Y {
				continue
			}
			dx := float64(x-cx) / float64(rx)
			dy := float64(y-cy) / float64(ry)
			if dx*dx+dy*dy <= 1 {
				img.SetRGBA(x, y, c)
			}
		}
	}
}

func encodePNG(t *testing.T, img image.Image) []byte {
	t.Helper()
	var buf bytes.Buffer
	require.NoError(t, png.Encode(&buf, img))
	return buf.Bytes()
}

func studioCanvas(w, h int) *image.RGBA {
	img := image.NewRGBA(image.Rect(0, 0, w, h))
	bg := color.RGBA{R: 250, G: 249, B: 246, A: 255}
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			img.SetRGBA(x, y, bg)
		}
	}
	return img
}

func TestPlanBreakdownAnnotations_UsesDetectedLayerCentroidsNotGuessedY(t *testing.T) {
	const w, h = 800, 800
	// Three layers bunched in the lower half — guessed even spacing
	// would put captions at ~720 / 400 / 80.
	blobYs := []int{780, 640, 500}
	layers := make([]breakdownLayer, len(blobYs))
	for i, y := range blobYs {
		layers[i] = breakdownLayer{CentroidX: 400, CentroidY: y, MinY: y - 20, MaxY: y + 20, Area: 100}
	}
	labels := []string{"black beans", "quinoa", "Maple-Dijon Vinaigrette"}
	plan, err := planBreakdownAnnotations(w, h, labels, layers)
	require.NoError(t, err)
	require.Len(t, plan, 3)

	guessed := guessedEvenLayerYs(h, 3)
	require.InDelta(t, 400, float64(guessed[1]), 20, "sanity: leftover even-Y mid slot is the frame center")
	require.InDelta(t, 80, float64(guessed[2]), 20, "sanity: leftover even-Y top slot is near the top")

	for i, ann := range plan {
		require.Equal(t, labels[i], ann.Label)
		require.Equal(t, blobYs[i], ann.TargetY, "leader must hit the detected layer, not i/(n-1)")
		require.NotEqual(t, guessed[i], ann.TargetY, "target collapsed onto the leftover guessed-Y slot")
		require.Greater(t, absInt(ann.TargetY-guessed[i]), 50, "target too close to leftover even-Y slot %d", guessed[i])
	}
}

func TestFinalizeBreakdownImage_LeadersFollowUnevenBitmapLayers(t *testing.T) {
	const w, h = 800, 800
	img := studioCanvas(w, h)
	blobYs := []int{780, 640, 500}
	colors := []color.RGBA{
		{R: 70, G: 90, B: 40, A: 255},
		{R: 210, G: 190, B: 120, A: 255},
		{R: 230, G: 150, B: 40, A: 255},
	}
	for i, y := range blobYs {
		paintLayerBlob(img, w/2, y, 90, 28, colors[i])
	}

	prompt := MenuImagePrompt{
		EntityType:  "menu_item",
		Name:        "Harvest Bowl",
		Ingredients: "black beans, quinoa, roasted sweet potato",
		DietaryTags: []string{"vegetarian"},
	}
	out, err := FinalizeBreakdownImage(encodePNG(t, img), prompt)
	require.NoError(t, err)
	decoded, err := jpeg.Decode(bytes.NewReader(out))
	require.NoError(t, err)

	guessed := guessedEvenLayerYs(h, 3)
	// The leftover even-Y mid/top slots sit on empty cream, not on a blob.
	midGuess := color.RGBAModel.Convert(decoded.At(w/2, guessed[1])).(color.RGBA)
	require.Greater(t, int(midGuess.R), 180, "guessed mid-Y must not receive a teal target dot")

	// Each actual blob centroid must have the teal leader endpoint.
	for _, y := range blobYs {
		c := color.RGBAModel.Convert(decoded.At(w/2, y)).(color.RGBA)
		require.Greater(t, int(c.G), int(c.R), "leader must land on the bitmap layer at y=%d, got %v", y, c)
	}
}

func TestFinalizeBreakdownImage_StripsLeftoverAperitifiTitle(t *testing.T) {
	const w, h = 800, 800
	img := studioCanvas(w, h)
	paintLayerBlob(img, w/2, 520, 80, 30, color.RGBA{R: 40, G: 120, B: 80, A: 255})
	paintLayerBlob(img, w/2, 640, 80, 30, color.RGBA{R: 210, G: 190, B: 120, A: 255})

	dc := gg.NewContextForRGBA(img)
	require.NoError(t, setBreakdownFont(dc, 32))
	dc.SetRGB(0.08, 0.08, 0.08)
	title := "BRIGHT APERITIFI COCKTAIL"
	tw, _ := dc.MeasureString(title)
	titleX := float64(w)/2 - tw/2
	titleY := 72.0
	dc.DrawString(title, titleX, titleY)

	titleBox := image.Rect(int(titleX)-4, int(titleY)-28, int(titleX+tw)+4, int(titleY)+8)
	darkBefore := countDark(img, titleBox)
	require.Greater(t, darkBefore, 200, "precondition: garbled title must be painted")

	// Overlay-only (guessed Y, no strip) would leave APERITIFI in the pixels.
	// Finalize must paint it out.
	out, err := FinalizeBreakdownImage(encodePNG(t, img), MenuImagePrompt{
		EntityType:  "menu_item",
		Name:        "Demo Spritz",
		Ingredients: "Aperitivo, Prosecco",
	})
	require.NoError(t, err)
	decoded, err := jpeg.Decode(bytes.NewReader(out))
	require.NoError(t, err)
	rgba := breakdownOriginRGBA(decoded)

	darkAfter := countDark(rgba, titleBox)
	require.Less(t, darkAfter, darkBefore/8, "leftover APERITIFI title must be stripped, before=%d after=%d", darkBefore, darkAfter)
	require.False(t, breakdownHasBakedInText(rgba), "text-line detector must be clean after strip")
}

func TestFinalizeBreakdownImage_RejectsVegetarianMeatHallucination(t *testing.T) {
	const w, h = 800, 800
	img := studioCanvas(w, h)
	paintLayerBlob(img, w/2, 640, 100, 36, color.RGBA{R: 50, G: 110, B: 45, A: 255})   // kale
	paintLayerBlob(img, w/2, 520, 100, 36, color.RGBA{R: 210, G: 190, B: 130, A: 255}) // quinoa
	// Cooked-chicken pink — the 8/19 Harvest Bowl hallucination.
	paintLayerBlob(img, w/2, 400, 110, 40, color.RGBA{R: 205, G: 145, B: 130, A: 255})

	_, err := FinalizeBreakdownImage(encodePNG(t, img), MenuImagePrompt{
		EntityType:  "menu_item",
		Name:        "Harvest Bowl",
		Ingredients: "kale, quinoa, roasted vegetables",
		DietaryTags: []string{"vegetarian"},
	})
	require.Error(t, err)
	require.True(t, errors.Is(err, errBreakdownAnimalProtein), "got %v", err)
}

func TestFinalizeBreakdownImage_AllowsVegetarianProduce(t *testing.T) {
	const w, h = 800, 800
	img := studioCanvas(w, h)
	paintLayerBlob(img, w/2, 680, 90, 30, color.RGBA{R: 50, G: 110, B: 45, A: 255})
	paintLayerBlob(img, w/2, 580, 90, 30, color.RGBA{R: 210, G: 190, B: 130, A: 255})
	paintLayerBlob(img, w/2, 480, 90, 30, color.RGBA{R: 230, G: 140, B: 50, A: 255}) // sweet potato

	out, err := FinalizeBreakdownImage(encodePNG(t, img), MenuImagePrompt{
		EntityType:  "menu_item",
		Name:        "Harvest Bowl",
		Ingredients: "kale, quinoa, roasted sweet potato",
		DietaryTags: []string{"vegetarian"},
	})
	require.NoError(t, err)
	require.NotEmpty(t, out)
}

func TestFinalizeBreakdownImage_FailsClosedOnMissingLayers(t *testing.T) {
	img := studioCanvas(400, 400)
	_, err := FinalizeBreakdownImage(encodePNG(t, img), MenuImagePrompt{
		EntityType:  "menu_item",
		Ingredients: "kale, quinoa, herbs",
	})
	require.Error(t, err)
	require.True(t, errors.Is(err, errBreakdownNoLayers) || errors.Is(err, errBreakdownLayerCount), "got %v", err)
}

func TestFinalizeBreakdownImage_InvalidSourceErrors(t *testing.T) {
	_, err := FinalizeBreakdownImage([]byte("not-an-image"), MenuImagePrompt{Ingredients: "kale"})
	require.Error(t, err)
}

func TestGenerateMenuImage_FailsClosedOnVegetarianMeat(t *testing.T) {
	const w, h = 800, 800
	img := studioCanvas(w, h)
	paintLayerBlob(img, w/2, 400, 120, 50, color.RGBA{R: 205, G: 145, B: 130, A: 255})
	pngBytes := encodePNG(t, img)

	svc, err := NewAIService(&staticBreakdownImageProvider{data: pngBytes, mime: "image/png"}, llm.ModelConfig{Image: "test-image"})
	require.NoError(t, err)

	_, err = svc.GenerateMenuImage(context.Background(), MenuImagePrompt{
		EntityType:  "menu_item",
		Name:        "Harvest Bowl",
		Ingredients: "kale, quinoa, roasted vegetables",
		DietaryTags: []string{"vegetarian"},
	})
	require.Error(t, err)
	require.True(t, errors.Is(err, errBreakdownAnimalProtein), "must not skip the dietary guard and ship the bitmap, got %v", err)
}

func countDark(img *image.RGBA, r image.Rectangle) int {
	n := 0
	b := img.Bounds()
	for y := r.Min.Y; y < r.Max.Y; y++ {
		for x := r.Min.X; x < r.Max.X; x++ {
			if x < b.Min.X || y < b.Min.Y || x >= b.Max.X || y >= b.Max.Y {
				continue
			}
			c := img.RGBAAt(x, y)
			if luminance(c) < 80 {
				n++
			}
		}
	}
	return n
}

func absInt(v int) int {
	if v < 0 {
		return -v
	}
	return v
}
