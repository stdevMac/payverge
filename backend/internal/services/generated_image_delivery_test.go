package services

import (
	"bytes"
	"context"
	"encoding/binary"
	"errors"
	"fmt"
	"hash/crc32"
	"image"
	"net/http"
	"os"
	"path/filepath"
	"testing"

	"github.com/stdevmac/payverge/backend/internal/s3"

	"github.com/stretchr/testify/require"
)

func TestValidateGeneratedImageDelivery(t *testing.T) {
	tests := []struct {
		name        string
		status      int
		contentType string
		fixture     string
		body        []byte
		wantErr     string
	}{
		{name: "delivered png", status: http.StatusOK, contentType: "image/png", fixture: "valid-1x1.png"},
		{name: "delivered jpeg", status: http.StatusOK, contentType: "image/jpeg", fixture: "valid-1x1.jpg"},
		{name: "delivered webp", status: http.StatusOK, contentType: "image/webp", fixture: "valid-1x1.webp"},
		{name: "cdn miss", status: http.StatusNotFound, contentType: "text/plain", body: []byte("missing"), wantErr: "status 404"},
		{name: "html error body", status: http.StatusOK, contentType: "text/html", body: []byte("<html>no</html>"), wantErr: "unsafe delivered image"},
		{name: "mime mismatch", status: http.StatusOK, contentType: "image/jpeg", fixture: "valid-1x1.png", wantErr: "delivered content type mismatch"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			body := tc.body
			if tc.fixture != "" {
				body = mustGeneratedImageFixture(t, tc.fixture)
			}
			fetch := func(context.Context, string) ([]byte, string, error) {
				if tc.status != http.StatusOK {
					return nil, "", fmt.Errorf("asset fetch returned status %d", tc.status)
				}
				return body, tc.contentType, nil
			}
			image := &GeneratedImage{
				URL:      "https://images.payverge.io/menu_items/ai_generated/dish",
				MIMEType: tc.contentType,
			}
			err := validateGeneratedImageDelivery(context.Background(), image, fetch)
			if tc.wantErr == "" {
				require.NoError(t, err)
				require.Equal(t, tc.contentType, image.MIMEType)
				return
			}
			require.ErrorContains(t, err, tc.wantErr)
		})
	}
}

func TestValidateGeneratedImageDeliveryRejectsTruncatedOrCorruptImages(t *testing.T) {
	tests := []struct {
		name string
		mime string
		file string
	}{
		{name: "png", mime: "image/png", file: "valid-1x1.png"},
		{name: "jpeg", mime: "image/jpeg", file: "valid-1x1.jpg"},
		{name: "webp", mime: "image/webp", file: "valid-1x1.webp"},
	}

	for _, tc := range tests {
		t.Run(tc.name+"/truncated", func(t *testing.T) {
			valid := mustGeneratedImageFixture(t, tc.file)
			truncated := append([]byte(nil), valid[:16]...)
			requireDeliverySignatureStillAccepted(t, truncated, tc.mime)
			require.ErrorContains(t, validateDeliveredBytes(t, tc.mime, truncated), "invalid delivered image")
		})

		t.Run(tc.name+"/corrupt", func(t *testing.T) {
			corrupt := append([]byte(nil), mustGeneratedImageFixture(t, tc.file)...)
			for i := 16; i < len(corrupt); i++ {
				corrupt[i] = 0
			}
			requireDeliverySignatureStillAccepted(t, corrupt, tc.mime)
			require.ErrorContains(t, validateDeliveredBytes(t, tc.mime, corrupt), "invalid delivered image")
		})
	}
}

func TestValidateGeneratedImageDeliveryRejectsOversizedImageHeadersBeforeDecode(t *testing.T) {
	const oversizedDimension = 9000
	tests := []struct {
		name string
		mime string
		file string
	}{
		{name: "png", mime: "image/png", file: "valid-1x1.png"},
		{name: "jpeg", mime: "image/jpeg", file: "valid-1x1.jpg"},
		{name: "webp", mime: "image/webp", file: "valid-1x1.webp"},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			bomb := generatedImageHeaderFixture(t, tc.file, tc.mime, oversizedDimension, oversizedDimension)
			config, _, err := image.DecodeConfig(bytes.NewReader(bomb))
			require.NoError(t, err, "bomb fixture must have a parseable header")
			require.Equal(t, oversizedDimension, config.Width)
			require.Equal(t, oversizedDimension, config.Height)

			err = validateDeliveredBytes(t, tc.mime, bomb)
			require.ErrorContains(t, err, "dimensions exceed safe limit")
			require.NotContains(t, err.Error(), "decode complete image",
				"dimension rejection must happen before the allocation-heavy full decode")
		})
	}
}

func TestValidateGeneratedImageDeliveryRejectsOversizedPixelAreaBeforeDecode(t *testing.T) {
	const largeDimension = 6000
	tests := []struct {
		name string
		mime string
		file string
	}{
		{name: "png", mime: "image/png", file: "valid-1x1.png"},
		{name: "jpeg", mime: "image/jpeg", file: "valid-1x1.jpg"},
		{name: "webp", mime: "image/webp", file: "valid-1x1.webp"},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			bomb := generatedImageHeaderFixture(t, tc.file, tc.mime, largeDimension, largeDimension)
			config, _, err := image.DecodeConfig(bytes.NewReader(bomb))
			require.NoError(t, err, "bomb fixture must have a parseable header")
			require.Equal(t, largeDimension, config.Width)
			require.Equal(t, largeDimension, config.Height)

			err = validateDeliveredBytes(t, tc.mime, bomb)
			require.ErrorContains(t, err, "pixel area exceeds safe limit")
			require.NotContains(t, err.Error(), "decode complete image",
				"pixel-area rejection must happen before the allocation-heavy full decode")
		})
	}
}

func generatedImageHeaderFixture(t *testing.T, file, mime string, width, height int) []byte {
	t.Helper()
	data := append([]byte(nil), mustGeneratedImageFixture(t, file)...)

	switch mime {
	case "image/png":
		require.GreaterOrEqual(t, len(data), 33)
		binary.BigEndian.PutUint32(data[16:20], uint32(width))
		binary.BigEndian.PutUint32(data[20:24], uint32(height))
		binary.BigEndian.PutUint32(data[29:33], crc32.ChecksumIEEE(data[12:29]))
	case "image/jpeg":
		marker := bytes.Index(data, []byte{0xff, 0xc0})
		require.NotEqual(t, -1, marker, "fixture must contain a baseline SOF marker")
		require.LessOrEqual(t, width, 65535)
		require.LessOrEqual(t, height, 65535)
		binary.BigEndian.PutUint16(data[marker+5:marker+7], uint16(height))
		binary.BigEndian.PutUint16(data[marker+7:marker+9], uint16(width))
	case "image/webp":
		frameHeader := bytes.Index(data, []byte{0x9d, 0x01, 0x2a})
		require.NotEqual(t, -1, frameHeader, "fixture must contain a VP8 frame header")
		require.LessOrEqual(t, width, 0x3fff)
		require.LessOrEqual(t, height, 0x3fff)
		binary.LittleEndian.PutUint16(data[frameHeader+3:frameHeader+5], uint16(width))
		binary.LittleEndian.PutUint16(data[frameHeader+5:frameHeader+7], uint16(height))
	default:
		t.Fatalf("unsupported fixture MIME %q", mime)
	}
	return data
}

func mustGeneratedImageFixture(t *testing.T, name string) []byte {
	t.Helper()
	data, err := os.ReadFile(filepath.Join("testdata", "generated_images", name))
	require.NoError(t, err)
	return data
}

func requireDeliverySignatureStillAccepted(t *testing.T, data []byte, mime string) {
	t.Helper()
	detected, err := DetectSafeImageMIME(data, mime)
	require.NoError(t, err, "fixture must pass the pre-fix signature/MIME gate")
	require.Equal(t, mime, detected)
}

func validateDeliveredBytes(t *testing.T, mime string, data []byte) error {
	t.Helper()
	return validateGeneratedImageDelivery(
		context.Background(),
		&GeneratedImage{URL: "https://images.payverge.io/menu_items/ai_generated/dish", MIMEType: mime},
		func(context.Context, string) ([]byte, string, error) {
			return bytes.Clone(data), mime, nil
		},
	)
}

func TestValidateGeneratedImageDeliveryRejectsNonCanonicalHost(t *testing.T) {
	err := validateGeneratedImageDelivery(
		context.Background(),
		&GeneratedImage{URL: "https://attacker.example/dish.png", MIMEType: "image/png"},
		func(_ context.Context, rawURL string) ([]byte, string, error) {
			if err := s3.ValidatePublicAssetURL(rawURL, "images.payverge.io"); err != nil {
				return nil, "", err
			}
			return nil, "", errors.New("unexpected fetch")
		},
	)
	require.ErrorIs(t, err, s3.ErrDisallowedAssetURL)
}
