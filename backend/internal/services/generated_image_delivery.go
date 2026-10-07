package services

import (
	"bytes"
	"context"
	"fmt"
	"image"
	_ "image/jpeg"
	_ "image/png"
	"strings"

	"github.com/stdevmac/payverge/backend/internal/s3"

	_ "golang.org/x/image/webp"
)

type generatedImageFetcher func(context.Context, string) ([]byte, string, error)

const (
	// Generated providers currently return images no larger than 4K. Keep enough
	// headroom for future providers while rejecting compact decompression bombs
	// before image.Decode allocates their advertised pixel buffers.
	maxGeneratedImageDimension = 8192
	maxGeneratedImagePixelArea = int64(32 << 20) // 32 megapixels
)

func canonicalImageMIME(value string) string {
	value = strings.ToLower(strings.TrimSpace(value))
	if i := strings.IndexByte(value, ';'); i >= 0 {
		value = strings.TrimSpace(value[:i])
	}
	if value == "image/jpg" {
		return "image/jpeg"
	}
	return value
}

func decodedImageMIME(format string) string {
	switch strings.ToLower(strings.TrimSpace(format)) {
	case "png":
		return "image/png"
	case "jpeg", "jpg":
		return "image/jpeg"
	case "webp":
		return "image/webp"
	default:
		return ""
	}
}

func validateGeneratedImageBytes(data []byte, detectedMIME string) error {
	config, format, err := image.DecodeConfig(bytes.NewReader(data))
	if err != nil {
		return fmt.Errorf("decode image configuration: %w", err)
	}
	if config.Width <= 0 || config.Height <= 0 {
		return fmt.Errorf("image must have positive dimensions, got %dx%d", config.Width, config.Height)
	}
	if config.Width > maxGeneratedImageDimension || config.Height > maxGeneratedImageDimension {
		return fmt.Errorf("image dimensions exceed safe limit: got %dx%d, maximum side is %d",
			config.Width, config.Height, maxGeneratedImageDimension)
	}
	pixelArea := int64(config.Width) * int64(config.Height)
	if pixelArea > maxGeneratedImagePixelArea {
		return fmt.Errorf("image pixel area exceeds safe limit: got %d pixels, maximum is %d",
			pixelArea, maxGeneratedImagePixelArea)
	}
	if decodedMIME := decodedImageMIME(format); decodedMIME == "" || decodedMIME != detectedMIME {
		return fmt.Errorf("decoded image type mismatch: format %q, bytes %q", format, detectedMIME)
	}

	decoded, format, err := image.Decode(bytes.NewReader(data))
	if err != nil {
		return fmt.Errorf("decode complete image: %w", err)
	}
	bounds := decoded.Bounds()
	if bounds.Dx() <= 0 || bounds.Dy() <= 0 {
		return fmt.Errorf("image must have positive decoded dimensions, got %dx%d", bounds.Dx(), bounds.Dy())
	}
	if bounds.Dx() != config.Width || bounds.Dy() != config.Height {
		return fmt.Errorf("decoded image dimensions mismatch: header %dx%d, decoded %dx%d",
			config.Width, config.Height, bounds.Dx(), bounds.Dy())
	}
	if decodedMIME := decodedImageMIME(format); decodedMIME == "" || decodedMIME != detectedMIME {
		return fmt.Errorf("fully decoded image type mismatch: format %q, bytes %q", format, detectedMIME)
	}
	return nil
}

func validateGeneratedImageDelivery(ctx context.Context, image *GeneratedImage, fetch generatedImageFetcher) error {
	if image == nil || strings.TrimSpace(image.URL) == "" {
		return fmt.Errorf("generated image has no delivery URL")
	}
	data, deliveredMIME, err := fetch(ctx, image.URL)
	if err != nil {
		return err
	}
	detectedMIME, err := DetectSafeImageMIME(data, deliveredMIME)
	if err != nil {
		return fmt.Errorf("unsafe delivered image: %w", err)
	}
	if deliveredMIME = canonicalImageMIME(deliveredMIME); deliveredMIME != "" && deliveredMIME != detectedMIME {
		return fmt.Errorf("delivered content type mismatch: header %q, bytes %q", deliveredMIME, detectedMIME)
	}
	if expected := canonicalImageMIME(image.MIMEType); expected != "" && expected != detectedMIME {
		return fmt.Errorf("generated image mime mismatch: expected %q, delivered %q", expected, detectedMIME)
	}
	if err := validateGeneratedImageBytes(data, detectedMIME); err != nil {
		return fmt.Errorf("invalid delivered image: %w", err)
	}
	image.MIMEType = detectedMIME
	return nil
}

// ValidateGeneratedImageDelivery verifies that the uploaded image is available
// from Payverge's canonical public asset host and still contains safe image bytes.
func ValidateGeneratedImageDelivery(ctx context.Context, image *GeneratedImage) error {
	return validateGeneratedImageDelivery(ctx, image, s3.DownloadPublicAsset)
}
