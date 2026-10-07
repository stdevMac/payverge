package services

import (
	"bytes"
	"fmt"
	"image"
	"image/color"
	"image/draw"
	"image/jpeg"
	_ "image/png"
	"strings"
	"sync"

	"github.com/fogleman/gg"
	"github.com/golang/freetype/truetype"
	"golang.org/x/image/font/gofont/goregular"
)

// maxBreakdownLayers caps exploded-view annotations so pills stay readable
// on a 1:1 marketing tile. Extra tokens are dropped, never invented.
const maxBreakdownLayers = 8

const (
	breakdownSideBandFrac   = 0.24
	breakdownTopPadFrac     = 0.10
	breakdownBottomPadFrac  = 0.10
	breakdownEdgeMarginFrac = 0.03
)

type breakdownSide string

const (
	breakdownSideLeft  breakdownSide = "left"
	breakdownSideRight breakdownSide = "right"
)

// breakdownAnnotation is one programmatically placed caption + leader line.
// Label is the exact operator-supplied ingredient string (no model spelling).
// LayerIndex 0 is the bottom layer; Target is the intended stack point.
type breakdownAnnotation struct {
	Label                string
	LayerIndex           int
	Side                 breakdownSide
	TargetX, TargetY     int
	LineFromX, LineFromY int
	PillMinX, PillMinY   int
	PillMaxX, PillMaxY   int
}

var (
	breakdownFontOnce sync.Once
	breakdownFont     *truetype.Font
	breakdownFontErr  error
)

func loadBreakdownFont() (*truetype.Font, error) {
	breakdownFontOnce.Do(func() {
		breakdownFont, breakdownFontErr = truetype.Parse(goregular.TTF)
	})
	return breakdownFont, breakdownFontErr
}

func setBreakdownFont(dc *gg.Context, size float64) error {
	font, err := loadBreakdownFont()
	if err != nil {
		return err
	}
	dc.SetFontFace(truetype.NewFace(font, &truetype.Options{Size: size}))
	return nil
}

// splitIngredientNames turns a free-text ingredients/description field into
// caption strings. It only splits on list separators — it never invents a
// protein, sauce, or garnish that was not already in the operator text.
func splitIngredientNames(raw string) []string {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return nil
	}
	normalized := strings.NewReplacer(
		"\n", ",",
		"\r", ",",
		";", ",",
		"•", ",",
		"·", ",",
		"|", ",",
	).Replace(raw)

	parts := strings.Split(normalized, ",")
	out := make([]string, 0, len(parts))
	seen := make(map[string]struct{}, len(parts))
	for _, part := range parts {
		label := strings.Join(strings.Fields(strings.TrimSpace(part)), " ")
		label = strings.Trim(label, "-–—•*")
		label = strings.TrimSpace(label)
		if label == "" {
			continue
		}
		key := strings.ToLower(label)
		if _, dup := seen[key]; dup {
			continue
		}
		seen[key] = struct{}{}
		out = append(out, label)
		if len(out) >= maxBreakdownLayers {
			break
		}
	}
	return out
}

// breakdownLayerLabels is the single source of caption strings used by both
// the generation prompt (layer count / stack order) and the post-generation
// overlay, so the two cannot drift.
func breakdownLayerLabels(promptData MenuImagePrompt) []string {
	labels := splitIngredientNames(promptData.Ingredients)
	if len(labels) == 0 {
		labels = splitIngredientNames(promptData.Description)
	}
	return labels
}

// guessedEvenLayerYs is the leftover algorithm: evenly spaced Y slots.
// Tests assert production targeting does NOT use these values when the
// bitmap's layers sit elsewhere.
func guessedEvenLayerYs(h, n int) []int {
	if n < 1 {
		return nil
	}
	topPad := int(float64(h) * breakdownTopPadFrac)
	bottomPad := int(float64(h) * breakdownBottomPadFrac)
	usable := h - topPad - bottomPad
	out := make([]int, n)
	for i := 0; i < n; i++ {
		t := 0.5
		if n > 1 {
			t = float64(i) / float64(n-1)
		}
		out[i] = (h - bottomPad) - int(t*float64(usable))
	}
	return out
}

func planBreakdownAnnotations(w, h int, labels []string, layers []breakdownLayer) ([]breakdownAnnotation, error) {
	if w < 64 || h < 64 || len(labels) == 0 {
		return nil, errBreakdownNoLayers
	}
	if len(labels) > maxBreakdownLayers {
		labels = labels[:maxBreakdownLayers]
	}
	if len(layers) != len(labels) {
		return nil, fmt.Errorf("%w: %d layers for %d labels", errBreakdownLayerCount, len(layers), len(labels))
	}

	sideW := int(float64(w) * breakdownSideBandFrac)
	margin := int(float64(w) * breakdownEdgeMarginFrac)
	if margin < 8 {
		margin = 8
	}
	pillH := int(float64(h) * 0.058)
	if pillH < 22 {
		pillH = 22
	}
	if pillH > 56 {
		pillH = 56
	}

	out := make([]breakdownAnnotation, 0, len(labels))
	for i, label := range labels {
		layer := layers[i]
		y := layer.CentroidY
		x := layer.CentroidX
		if x < sideW || x > w-sideW {
			x = w / 2
		}
		side := breakdownSideLeft
		if i%2 == 1 {
			side = breakdownSideRight
		}

		var pillMinX, pillMaxX, lineFromX int
		if side == breakdownSideLeft {
			pillMinX = margin
			pillMaxX = sideW - margin/2
			if pillMaxX < pillMinX+24 {
				pillMaxX = pillMinX + sideW/2
			}
			lineFromX = pillMaxX
		} else {
			pillMaxX = w - margin
			pillMinX = w - sideW + margin/2
			if pillMaxX < pillMinX+24 {
				pillMinX = pillMaxX - sideW/2
			}
			lineFromX = pillMinX
		}
		pillMinY := y - pillH/2
		pillMaxY := y + pillH/2
		if pillMinY < 4 {
			shift := 4 - pillMinY
			pillMinY += shift
			pillMaxY += shift
		}
		if pillMaxY > h-4 {
			shift := pillMaxY - (h - 4)
			pillMinY -= shift
			pillMaxY -= shift
		}

		out = append(out, breakdownAnnotation{
			Label:      label,
			LayerIndex: i,
			Side:       side,
			TargetX:    x,
			TargetY:    y,
			LineFromX:  lineFromX,
			LineFromY:  y,
			PillMinX:   pillMinX,
			PillMinY:   pillMinY,
			PillMaxX:   pillMaxX,
			PillMaxY:   pillMaxY,
		})
	}
	return out, nil
}

func breakdownOriginRGBA(src image.Image) *image.RGBA {
	b := src.Bounds()
	dst := image.NewRGBA(image.Rect(0, 0, b.Dx(), b.Dy()))
	draw.Draw(dst, dst.Bounds(), &image.Uniform{C: color.White}, image.Point{}, draw.Src)
	draw.Draw(dst, dst.Bounds(), src, b.Min, draw.Over)
	return dst
}

func renderBreakdownOverlay(base *image.RGBA, plan []breakdownAnnotation) *image.RGBA {
	dc := gg.NewContextForRGBA(base)
	h := float64(base.Bounds().Dy())
	fontSize := clampFloat(h/36, 12, 26)
	if err := setBreakdownFont(dc, fontSize); err != nil {
		return nil
	}

	lineW := clampFloat(h/360, 2, 4)
	dotR := clampFloat(h/150, 3.5, 7)
	cream := color.RGBA{R: 0xfa, G: 0xf9, B: 0xf6, A: 255}
	ink := color.RGBA{R: 0x1c, G: 0x19, B: 0x17, A: 255}
	teal := color.RGBA{R: 0x1a, G: 0x6b, B: 0x6a, A: 255}

	for _, ann := range plan {
		dc.SetColor(teal)
		dc.SetLineWidth(lineW)
		dc.DrawLine(float64(ann.LineFromX), float64(ann.LineFromY), float64(ann.TargetX), float64(ann.TargetY))
		dc.Stroke()
		dc.DrawCircle(float64(ann.TargetX), float64(ann.TargetY), dotR)
		dc.Fill()

		px := float64(ann.PillMinX)
		py := float64(ann.PillMinY)
		pw := float64(ann.PillMaxX - ann.PillMinX)
		ph := float64(ann.PillMaxY - ann.PillMinY)
		radius := ph / 2
		if radius > 12 {
			radius = 12
		}
		dc.SetColor(cream)
		dc.DrawRoundedRectangle(px, py, pw, ph, radius)
		dc.Fill()
		dc.SetColor(teal)
		dc.SetLineWidth(1.25)
		dc.DrawRoundedRectangle(px, py, pw, ph, radius)
		dc.Stroke()

		lines := dc.WordWrap(ann.Label, pw-14)
		if len(lines) == 0 {
			lines = []string{ann.Label}
		}
		if len(lines) > 2 {
			lines = []string{lines[0], truncateRunes(strings.Join(lines[1:], " "), 22)}
		}
		_, lineH := dc.MeasureString("Hg")
		gap := lineH * 0.2
		blockH := float64(len(lines))*lineH + float64(len(lines)-1)*gap
		ty := py + (ph-blockH)/2 + lineH*0.82
		dc.SetColor(ink)
		for _, line := range lines {
			tw, _ := dc.MeasureString(line)
			dc.DrawString(line, px+(pw-tw)/2, ty)
			ty += lineH + gap
		}
	}
	return base
}

func clampFloat(v, lo, hi float64) float64 {
	if v < lo {
		return lo
	}
	if v > hi {
		return hi
	}
	return v
}

func encodeBreakdownJPEG(img image.Image) ([]byte, error) {
	var buf bytes.Buffer
	if err := jpeg.Encode(&buf, img, &jpeg.Options{Quality: aiGeneratedJPEGQuality}); err != nil {
		return nil, fmt.Errorf("encode annotated breakdown: %w", err)
	}
	return buf.Bytes(), nil
}

func isMenuItemBreakdown(promptData MenuImagePrompt) bool {
	entity := strings.ToLower(strings.TrimSpace(promptData.EntityType))
	return entity == "" || entity == "menu_item"
}

// FinalizeBreakdownImage is the fail-closed post-process for a menu-item
// exploded view: strip leftover model type, reject animal protein on
// vegetarian/vegan items, then stamp captions on the *detected* layers.
func FinalizeBreakdownImage(src []byte, promptData MenuImagePrompt) ([]byte, error) {
	if len(src) == 0 {
		return nil, fmt.Errorf("empty breakdown image")
	}
	img, _, err := image.Decode(bytes.NewReader(src))
	if err != nil {
		return nil, fmt.Errorf("decode breakdown image: %w", err)
	}
	base := breakdownOriginRGBA(img)

	stripBakedInBreakdownText(base)
	if breakdownHasBakedInText(base) {
		return nil, errBreakdownBakedInText
	}
	if dietaryForbidsAnimalProtein(promptData.DietaryTags) && breakdownHasAnimalProtein(base) {
		return nil, errBreakdownAnimalProtein
	}

	labels := breakdownLayerLabels(promptData)
	if len(labels) == 0 {
		return encodeBreakdownJPEG(base)
	}
	if len(labels) > maxBreakdownLayers {
		labels = append([]string(nil), labels[:maxBreakdownLayers]...)
	}

	layers, err := detectBreakdownLayers(base, len(labels))
	if err != nil {
		return nil, err
	}
	plan, err := planBreakdownAnnotations(base.Bounds().Dx(), base.Bounds().Dy(), labels, layers)
	if err != nil {
		return nil, err
	}
	out := renderBreakdownOverlay(base, plan)
	if out == nil {
		return nil, fmt.Errorf("breakdown: failed to load caption font")
	}
	return encodeBreakdownJPEG(out)
}
