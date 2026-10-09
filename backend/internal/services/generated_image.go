package services

import (
	"fmt"
	"net/http"
	"strings"
)

// GeneratedImage is the result of a paid image generation/enhancement call.
// MIMEType is derived from the provider-returned bytes (and must be a safe
// image type). Model is the provider-served model when available, otherwise
// the configured image model.
type GeneratedImage struct {
	URL      string
	MIMEType string
	Model    string
}

// safeImageMIME maps accepted content types to file extensions.
var safeImageMIME = map[string]string{
	"image/png":  ".png",
	"image/jpeg": ".jpg",
	"image/webp": ".webp",
}

// DetectSafeImageMIME validates output bytes (and optional provider MIME) and
// returns a canonical safe MIME type or an error.
func DetectSafeImageMIME(data []byte, providerMIME string) (string, error) {
	if len(data) == 0 {
		return "", fmt.Errorf("empty image bytes")
	}
	detected := http.DetectContentType(data)
	if i := strings.IndexByte(detected, ';'); i >= 0 {
		detected = strings.TrimSpace(detected[:i])
	}
	// Go may return image/jpeg for JFIF; normalize jpg alias.
	if detected == "image/jpg" {
		detected = "image/jpeg"
	}
	if _, ok := safeImageMIME[detected]; !ok {
		return "", fmt.Errorf("unsupported detected image type %q", detected)
	}
	if providerMIME != "" {
		p := strings.ToLower(strings.TrimSpace(providerMIME))
		if i := strings.IndexByte(p, ';'); i >= 0 {
			p = strings.TrimSpace(p[:i])
		}
		if p == "image/jpg" {
			p = "image/jpeg"
		}
		if _, ok := safeImageMIME[p]; ok && p != detected {
			// Prefer bytes when they disagree; bytes are authoritative.
			return detected, nil
		}
	}
	return detected, nil
}

// ExtensionForImageMIME returns a safe file extension for a MIME type.
func ExtensionForImageMIME(mime string) string {
	if ext, ok := safeImageMIME[mime]; ok {
		return ext
	}
	return ".bin"
}
