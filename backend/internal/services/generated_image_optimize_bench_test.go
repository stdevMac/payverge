package services

import (
	"bytes"
	"image"
	"image/color"
	"image/png"
	"testing"
)

// BenchmarkOptimizeAIGeneratedImageBytes_LargePNG exercises the post-REV-2 path
// (bilinear resize + white flatten + JPEG) on a ≥1MB in-test PNG fixture.
func BenchmarkOptimizeAIGeneratedImageBytes_LargePNG(b *testing.B) {
	// ~1400×1400 RGBA with mild noise → PNG well over 1MB before encode.
	const side = 1400
	img := image.NewRGBA(image.Rect(0, 0, side, side))
	for y := 0; y < side; y++ {
		for x := 0; x < side; x++ {
			img.Set(x, y, color.RGBA{
				R: uint8((x*3 + y) % 251),
				G: uint8((y*5 + x) % 247),
				B: uint8((x + y*7) % 241),
				A: uint8(200 + (x+y)%55), // partial alpha to hit flatten
			})
		}
	}
	var buf bytes.Buffer
	if err := png.Encode(&buf, img); err != nil {
		b.Fatal(err)
	}
	src := buf.Bytes()
	if len(src) < 1<<20 {
		b.Fatalf("fixture PNG too small: %d bytes (need ≥1MB)", len(src))
	}
	b.ReportMetric(float64(len(src)), "src_bytes")
	b.SetBytes(int64(len(src)))
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		opt, err := OptimizeAIGeneratedImageBytes(src)
		if err != nil {
			b.Fatal(err)
		}
		if opt.MIMEType != "image/jpeg" {
			b.Fatalf("expected jpeg, got %s", opt.MIMEType)
		}
		if opt.OutputBytes == 0 {
			b.Fatal("empty output")
		}
	}
}
