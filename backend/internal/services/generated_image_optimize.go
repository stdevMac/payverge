package services

import (
	"bytes"
	"fmt"
	"image"
	"image/color"
	"image/draw"
	"image/jpeg"
	_ "image/png"
	"math"
	"time"

	_ "golang.org/x/image/webp"
)

// Delivery-oriented caps for AI-generated menu/marketing images (NEW-8).
// Provider PNGs often land at 1.4–1.7 MB; marketing post previews load them
// into canvas and time out. Resize + JPEG cuts object size while keeping
// plate photography sharp enough for 1× and 2× UI surfaces.
const (
	maxAIGeneratedImageSide = 1280
	aiGeneratedJPEGQuality  = 78
)

// OptimizedGeneratedImage is the encode result ready for S3 upload.
type OptimizedGeneratedImage struct {
	Bytes         []byte
	MIMEType      string
	SourceBytes   int
	OutputBytes   int
	SourceWidth   int
	SourceHeight  int
	OutputWidth   int
	OutputHeight  int
	EncodeLatency time.Duration
}

// OptimizeAIGeneratedImageBytes resizes (max side 1280) and re-encodes AI image
// bytes as JPEG for public delivery. Transparent pixels are composited over an
// opaque white background before encode so alpha does not become black JPEG
// pixels (REV-2).
func OptimizeAIGeneratedImageBytes(src []byte) (*OptimizedGeneratedImage, error) {
	start := time.Now()
	if len(src) == 0 {
		return nil, fmt.Errorf("empty image bytes")
	}

	cfg, _, err := image.DecodeConfig(bytes.NewReader(src))
	if err != nil {
		return nil, fmt.Errorf("decode config: %w", err)
	}
	img, format, err := image.Decode(bytes.NewReader(src))
	if err != nil {
		return nil, fmt.Errorf("decode image: %w", err)
	}

	outW, outH := cfg.Width, cfg.Height
	if outW <= 0 || outH <= 0 {
		return nil, fmt.Errorf("invalid dimensions %dx%d", outW, outH)
	}

	if outW > maxAIGeneratedImageSide || outH > maxAIGeneratedImageSide {
		if outW >= outH {
			outH = outH * maxAIGeneratedImageSide / outW
			outW = maxAIGeneratedImageSide
		} else {
			outW = outW * maxAIGeneratedImageSide / outH
			outH = maxAIGeneratedImageSide
		}
		if outW < 1 {
			outW = 1
		}
		if outH < 1 {
			outH = 1
		}
		img = resizeImageBilinear(img, outW, outH)
	}

	if format == "jpeg" && cfg.Width == outW && cfg.Height == outH && len(src) < 350_000 {
		return &OptimizedGeneratedImage{
			Bytes: src, MIMEType: "image/jpeg",
			SourceBytes: len(src), OutputBytes: len(src),
			SourceWidth: cfg.Width, SourceHeight: cfg.Height,
			OutputWidth: outW, OutputHeight: outH,
			EncodeLatency: time.Since(start),
		}, nil
	}

	// JPEG has no alpha: composite onto opaque white so transparent regions
	// become white rather than encoder-default black.
	img = flattenOntoWhite(img)

	var buf bytes.Buffer
	if err := jpeg.Encode(&buf, img, &jpeg.Options{Quality: aiGeneratedJPEGQuality}); err != nil {
		return nil, fmt.Errorf("jpeg encode: %w", err)
	}
	out := buf.Bytes()
	mime := "image/jpeg"
	if len(out) >= len(src) && cfg.Width == outW && cfg.Height == outH {
		out = src
		switch format {
		case "png":
			mime = "image/png"
		case "jpeg", "jpg":
			mime = "image/jpeg"
		case "webp":
			mime = "image/webp"
		}
	}

	return &OptimizedGeneratedImage{
		Bytes: out, MIMEType: mime,
		SourceBytes: len(src), OutputBytes: len(out),
		SourceWidth: cfg.Width, SourceHeight: cfg.Height,
		OutputWidth: outW, OutputHeight: outH,
		EncodeLatency: time.Since(start),
	}, nil
}

// flattenOntoWhite returns an opaque RGBA copy of src with fully/partially
// transparent pixels composited over white (draw.Over).
func flattenOntoWhite(src image.Image) *image.RGBA {
	b := src.Bounds()
	dst := image.NewRGBA(b)
	draw.Draw(dst, b, &image.Uniform{C: color.White}, image.Point{}, draw.Src)
	draw.Draw(dst, b, src, b.Min, draw.Over)
	return dst
}

// resizeImageBilinear scales src to w×h with bilinear sampling (better plate
// photography quality than nearest-neighbor for downscales to 1280).
func resizeImageBilinear(src image.Image, w, h int) image.Image {
	dst := image.NewRGBA(image.Rect(0, 0, w, h))
	sb := src.Bounds()
	sw, sh := float64(sb.Dx()), float64(sb.Dy())
	if sw < 1 || sh < 1 || w < 1 || h < 1 {
		return dst
	}
	for y := 0; y < h; y++ {
		// Map output pixel centers into source continuous coords.
		sy := (float64(y)+0.5)*sh/float64(h) - 0.5 + float64(sb.Min.Y)
		for x := 0; x < w; x++ {
			sx := (float64(x)+0.5)*sw/float64(w) - 0.5 + float64(sb.Min.X)
			dst.SetRGBA(x, y, sampleBilinear(src, sx, sy, sb))
		}
	}
	return dst
}

func sampleBilinear(src image.Image, sx, sy float64, b image.Rectangle) color.RGBA {
	x0 := int(math.Floor(sx))
	y0 := int(math.Floor(sy))
	x1 := x0 + 1
	y1 := y0 + 1
	fx := sx - float64(x0)
	fy := sy - float64(y0)

	// Clamp to source bounds.
	if x0 < b.Min.X {
		x0 = b.Min.X
	}
	if y0 < b.Min.Y {
		y0 = b.Min.Y
	}
	if x1 > b.Max.X-1 {
		x1 = b.Max.X - 1
	}
	if y1 > b.Max.Y-1 {
		y1 = b.Max.Y - 1
	}
	if x0 > b.Max.X-1 {
		x0 = b.Max.X - 1
	}
	if y0 > b.Max.Y-1 {
		y0 = b.Max.Y - 1
	}

	c00 := color.RGBAModel.Convert(src.At(x0, y0)).(color.RGBA)
	c10 := color.RGBAModel.Convert(src.At(x1, y0)).(color.RGBA)
	c01 := color.RGBAModel.Convert(src.At(x0, y1)).(color.RGBA)
	c11 := color.RGBAModel.Convert(src.At(x1, y1)).(color.RGBA)

	return color.RGBA{
		R: bilinearChannel(c00.R, c10.R, c01.R, c11.R, fx, fy),
		G: bilinearChannel(c00.G, c10.G, c01.G, c11.G, fx, fy),
		B: bilinearChannel(c00.B, c10.B, c01.B, c11.B, fx, fy),
		A: bilinearChannel(c00.A, c10.A, c01.A, c11.A, fx, fy),
	}
}

func bilinearChannel(v00, v10, v01, v11 uint8, fx, fy float64) uint8 {
	a := float64(v00)*(1-fx) + float64(v10)*fx
	b := float64(v01)*(1-fx) + float64(v11)*fx
	return uint8(math.Round(a*(1-fy) + b*fy))
}
