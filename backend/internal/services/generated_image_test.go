package services

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestDetectSafeImageMIME_AcceptsPNGJPEGWebP(t *testing.T) {
	png := []byte{0x89, 'P', 'N', 'G', '\r', '\n', 0x1a, '\n', 0, 1, 2, 3, 4, 5, 6, 7}
	jpeg := []byte{0xff, 0xd8, 0xff, 0xe0, 0x00, 0x10, 'J', 'F', 'I', 'F', 0x00, 0x01}
	webp := []byte{'R', 'I', 'F', 'F', 0, 0, 0, 0, 'W', 'E', 'B', 'P', 'V', 'P', 0, 0}

	for _, tc := range []struct {
		name string
		data []byte
		want string
		ext  string
	}{
		{"png", png, "image/png", ".png"},
		{"jpeg", jpeg, "image/jpeg", ".jpg"},
		{"webp", webp, "image/webp", ".webp"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got, err := DetectSafeImageMIME(tc.data, "")
			require.NoError(t, err)
			assert.Equal(t, tc.want, got)
			assert.Equal(t, tc.ext, ExtensionForImageMIME(got))
		})
	}
}

func TestDetectSafeImageMIME_RejectsHTML(t *testing.T) {
	_, err := DetectSafeImageMIME([]byte("<!doctype html><html>"), "image/png")
	require.Error(t, err)
}
