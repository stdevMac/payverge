package services

import (
	"bytes"
	"image"
	"image/color"
	"image/jpeg"
	"image/png"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestOptimizeAIGeneratedImageBytes_ResizesAndShrinks(t *testing.T) {
	img := image.NewRGBA(image.Rect(0, 0, 2400, 2400))
	for y := 0; y < 2400; y++ {
		for x := 0; x < 2400; x++ {
			img.Set(x, y, color.RGBA{
				R: uint8(40 + x*80/2400),
				G: uint8(30 + y*90/2400),
				B: uint8(20 + (x+y)*40/4800),
				A: 255,
			})
		}
	}
	var buf bytes.Buffer
	require.NoError(t, png.Encode(&buf, img))
	src := buf.Bytes()

	opt, err := OptimizeAIGeneratedImageBytes(src)
	require.NoError(t, err)
	require.Equal(t, "image/jpeg", opt.MIMEType)
	require.Equal(t, maxAIGeneratedImageSide, opt.OutputWidth)
	require.Equal(t, maxAIGeneratedImageSide, opt.OutputHeight)
	require.Less(t, opt.OutputBytes, opt.SourceBytes)
	require.Less(t, opt.OutputBytes, 250_000)
	require.Greater(t, opt.EncodeLatency, time.Duration(0))
	t.Logf("NEW-8 metrics: before=%dB after=%dB encode=%s",
		opt.SourceBytes, opt.OutputBytes, opt.EncodeLatency)
}

func TestOptimizeAIGeneratedImageBytes_SmallImageKeepsSafeMIME(t *testing.T) {
	img := image.NewRGBA(image.Rect(0, 0, 64, 64))
	for y := 0; y < 64; y++ {
		for x := 0; x < 64; x++ {
			img.Set(x, y, color.RGBA{R: 10, G: 20, B: 30, A: 255})
		}
	}
	var buf bytes.Buffer
	require.NoError(t, png.Encode(&buf, img))
	opt, err := OptimizeAIGeneratedImageBytes(buf.Bytes())
	require.NoError(t, err)
	require.Contains(t, []string{"image/jpeg", "image/png"}, opt.MIMEType)
	require.Equal(t, 64, opt.OutputWidth)
}

// REV-2: transparent PNG regions must become white in the JPEG, not black.
// Source is wider than maxAIGeneratedImageSide so resize always re-encodes
// (the keep-original guard only applies when dimensions are unchanged).
func TestOptimizeAIGeneratedImageBytes_TransparentRegionBecomesWhite(t *testing.T) {
	const w, h = 1600, 400
	img := image.NewRGBA(image.Rect(0, 0, w, h))
	// Left half fully opaque teal; right half fully transparent (would become
	// black under jpeg.Encode without flattenOntoWhite).
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			if x < w/2 {
				img.Set(x, y, color.RGBA{R: 26, G: 107, B: 106, A: 255})
			} else {
				img.Set(x, y, color.RGBA{R: 255, G: 255, B: 255, A: 0})
			}
		}
	}
	var pngBuf bytes.Buffer
	require.NoError(t, png.Encode(&pngBuf, img))

	opt, err := OptimizeAIGeneratedImageBytes(pngBuf.Bytes())
	require.NoError(t, err)
	require.Equal(t, "image/jpeg", opt.MIMEType, "expected JPEG after resize+encode")
	require.Equal(t, maxAIGeneratedImageSide, opt.OutputWidth)

	decoded, err := jpeg.Decode(bytes.NewReader(opt.Bytes))
	require.NoError(t, err)
	db := decoded.Bounds()
	// Transparent half maps to the right side of the scaled output.
	tx := db.Max.X - 4
	ty := db.Min.Y + db.Dy()/2
	c := color.RGBAModel.Convert(decoded.At(tx, ty)).(color.RGBA)
	require.GreaterOrEqual(t, int(c.R), 240, "transparent region R should be white, got %v", c)
	require.GreaterOrEqual(t, int(c.G), 240, "transparent region G should be white, got %v", c)
	require.GreaterOrEqual(t, int(c.B), 240, "transparent region B should be white, got %v", c)

	// Opaque half should stay teal-ish (not flattened to white).
	ox := db.Min.X + 4
	opaque := color.RGBAModel.Convert(decoded.At(ox, ty)).(color.RGBA)
	require.Less(t, int(opaque.R)+int(opaque.G)+int(opaque.B), 700,
		"opaque teal should not flatten to near-white: %v", opaque)
}
