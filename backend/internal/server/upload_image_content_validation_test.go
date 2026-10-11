package server

import (
	"bytes"
	"encoding/base64"
	"encoding/binary"
	"encoding/json"
	"fmt"
	"image"
	"image/color"
	"image/jpeg"
	"image/png"
	"mime/multipart"
	"net/http"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/stdevmac/payverge/backend/internal/s3"
)

// #929 — a 3-byte "JPEG" (just the SOI/marker prefix that
// http.DetectContentType sniffs as image/jpeg) used to sail through the
// upload contract and get published to the public image host. The upload
// path must decode the bytes, not just sniff them.

// gradientImage builds a deterministic non-uniform image so encoders emit
// realistic payloads rather than a single run-length.
func gradientImage(width, height int) *image.RGBA {
	img := image.NewRGBA(image.Rect(0, 0, width, height))
	for y := 0; y < height; y++ {
		for x := 0; x < width; x++ {
			img.Set(x, y, color.RGBA{R: uint8(x * 3), G: uint8(y * 3), B: 0x40, A: 0xff})
		}
	}
	return img
}

// encodePNGBytes encodes an actual PNG of the requested size.
func encodePNGBytes(width, height int) []byte {
	var buf bytes.Buffer
	if err := png.Encode(&buf, gradientImage(width, height)); err != nil {
		panic(err)
	}
	return buf.Bytes()
}

// encodeJPEGBytes encodes an actual JPEG of the requested size.
func encodeJPEGBytes(width, height int) []byte {
	var buf bytes.Buffer
	if err := jpeg.Encode(&buf, gradientImage(width, height), nil); err != nil {
		panic(err)
	}
	return buf.Bytes()
}

// realWebPFixture is a genuine 32x32 lossy WebP; Go ships no WebP encoder, so
// the bytes are embedded.
const realWebPFixture = "UklGRpQAAABXRUJQVlA4IIgAAADQBQCdASogACAAPmUWk0ckERCN+EAGRLYATplCOBvFvyM4URsM" +
	"4ACP7reN0j2UY2L1HLlhoAQAAP79sqxqSMkJRJULMBinr8J/+ZjaAInzykkaqNQq8Tvw+usLm8RZ" +
	"SxZW4ZTN84cS23ogepQUHdMgaLozM2VmmfxE+6y/xLtaak8gMbPb37AA"

func encodeWebPBytes() []byte {
	decoded, err := base64.StdEncoding.DecodeString(realWebPFixture)
	if err != nil {
		panic(err)
	}
	return decoded
}

func rejectingPublicUploader(t *testing.T, reason string) {
	t.Helper()
	setPublicUploader(t, func(_ *multipart.FileHeader, _, _ string, _ ...s3.UploadOption) (string, error) {
		t.Fatalf("public uploader must not run: %s", reason)
		return "", nil
	})
}

func TestUploadFile_RejectsThreeByteJPEG(t *testing.T) {
	setupStaffHandlerTestDB(t)
	owner := createOwnedBusiness(t, "0xOwnerA", "biz-upload-3byte-jpeg")
	rejectingPublicUploader(t, "a 3-byte JPEG is not a decodable image")

	// Exactly the live repro: the three bytes http.DetectContentType needs to
	// call something image/jpeg.
	req := newUploadContractRequest(t, map[string]string{
		"business_id": fmt.Sprintf("%d", owner.ID),
		"folder":      "images",
	}, "tiny.jpg", "image/jpeg", []byte{0xff, 0xd8, 0xff})
	c, w := newUploadFileContext(t, "0xOwnerA", req)
	UploadFile(c)

	assert.Equal(t, http.StatusBadRequest, w.Code)
	var payload map[string]any
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &payload))
	assert.Equal(t, "invalid_image", payload["code"])
}

func TestUploadFile_RejectsHeaderOnlyImagePadding(t *testing.T) {
	setupStaffHandlerTestDB(t)
	owner := createOwnedBusiness(t, "0xOwnerA", "biz-upload-padded-header")

	cases := []struct {
		name        string
		filename    string
		contentType string
		body        []byte
	}{
		{"png", "menu.png", "image/png", append([]byte{0x89, 'P', 'N', 'G', '\r', '\n', 0x1a, '\n'}, make([]byte, 4088)...)},
		{"jpeg", "menu.jpg", "image/jpeg", append([]byte{0xff, 0xd8, 0xff, 0xe0, 0x00, 0x10, 'J', 'F', 'I', 'F'}, make([]byte, 4086)...)},
		{"webp", "menu.webp", "image/webp", append([]byte{'R', 'I', 'F', 'F', 0, 0, 0, 0, 'W', 'E', 'B', 'P', 'V', 'P'}, make([]byte, 4082)...)},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			rejectingPublicUploader(t, "a padded magic-byte header is not a decodable image")

			req := newUploadContractRequest(t, map[string]string{
				"business_id": fmt.Sprintf("%d", owner.ID),
				"folder":      "images",
			}, tc.filename, tc.contentType, tc.body)
			c, w := newUploadFileContext(t, "0xOwnerA", req)
			UploadFile(c)

			assert.Equal(t, http.StatusBadRequest, w.Code)
			var payload map[string]any
			require.NoError(t, json.Unmarshal(w.Body.Bytes(), &payload))
			assert.Equal(t, "invalid_image", payload["code"])
		})
	}
}

func TestUploadFile_RejectsTruncatedRealImage(t *testing.T) {
	setupStaffHandlerTestDB(t)
	owner := createOwnedBusiness(t, "0xOwnerA", "biz-upload-truncated")
	rejectingPublicUploader(t, "a truncated PNG is not a decodable image")

	full := encodePNGBytes(64, 64)
	require.Greater(t, len(full), 40)

	req := newUploadContractRequest(t, map[string]string{
		"business_id": fmt.Sprintf("%d", owner.ID),
		"folder":      "images",
	}, "menu.png", "image/png", full[:len(full)/2])
	c, w := newUploadFileContext(t, "0xOwnerA", req)
	UploadFile(c)

	assert.Equal(t, http.StatusBadRequest, w.Code)
}

func TestUploadFile_RejectsImageBytesRenamedToAnotherImageType(t *testing.T) {
	setupStaffHandlerTestDB(t)
	owner := createOwnedBusiness(t, "0xOwnerA", "biz-upload-crossnamed")
	rejectingPublicUploader(t, "PNG bytes must not be published as a .jpg")

	// Sniffed type is image/png, so the .jpg extension must lose.
	req := newUploadContractRequest(t, map[string]string{
		"business_id": fmt.Sprintf("%d", owner.ID),
		"folder":      "images",
	}, "menu.jpg", "image/jpeg", encodePNGBytes(48, 48))
	c, w := newUploadFileContext(t, "0xOwnerA", req)
	UploadFile(c)

	assert.Equal(t, http.StatusUnsupportedMediaType, w.Code)
}

func TestUploadFile_AcceptsRealImages(t *testing.T) {
	setupStaffHandlerTestDB(t)
	owner := createOwnedBusiness(t, "0xOwnerA", "biz-upload-real-images")

	cases := []struct {
		name        string
		filename    string
		contentType string
		body        []byte
		wantMIME    string
	}{
		{"png", "menu.png", "image/png", encodePNGBytes(64, 64), "image/png"},
		{"jpeg", "menu.jpeg", "image/jpeg", encodeJPEGBytes(64, 64), "image/jpeg"},
		{"webp", "menu.webp", "image/webp", encodeWebPBytes(), "image/webp"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			var capturedContentType string
			setPublicUploader(t, func(_ *multipart.FileHeader, name, folder string, opts ...s3.UploadOption) (string, error) {
				capturedContentType = s3.ApplyUploadOptions(opts...).ContentType
				return fmt.Sprintf("https://s3.example/%s/%s", folder, name), nil
			})

			req := newUploadContractRequest(t, map[string]string{
				"business_id": fmt.Sprintf("%d", owner.ID),
				"folder":      "images",
			}, tc.filename, tc.contentType, tc.body)
			c, w := newUploadFileContext(t, "0xOwnerA", req)
			UploadFile(c)

			assert.Equal(t, http.StatusOK, w.Code, w.Body.String())
			assert.Equal(t, tc.wantMIME, capturedContentType)
		})
	}
}

func TestUploadBusinessLogo_RejectsThreeByteJPEG(t *testing.T) {
	setupStaffHandlerTestDB(t)
	owner := createOwnedBusiness(t, "0xOwnerA", "logo-3byte-jpeg")
	rejectingPublicUploader(t, "a 3-byte JPEG is not a decodable logo")

	req := newUploadContractRequest(t, map[string]string{}, "logo.jpg", "image/jpeg", []byte{0xff, 0xd8, 0xff})
	c, w := newUploadFileContext(t, "0xOwnerA", req)
	setUploadBusinessParam(c, owner.ID)
	UploadBusinessLogo(c)

	assert.Equal(t, http.StatusBadRequest, w.Code)
	var payload map[string]any
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &payload))
	assert.Equal(t, "invalid_image", payload["code"])
}

func TestUploadFileProtected_RejectsThreeByteJPEGButKeepsPDFs(t *testing.T) {
	setupStaffHandlerTestDB(t)
	owner := createOwnedBusiness(t, "0xOwnerA", "protected-3byte-jpeg")

	t.Run("garbage jpeg", func(t *testing.T) {
		setContractUploader(t, func(_ *multipart.FileHeader, _, _ string, _ ...s3.UploadOption) (string, error) {
			t.Fatalf("contract uploader must not run for a 3-byte JPEG")
			return "", nil
		})

		req := newUploadContractRequest(t, map[string]string{
			"businessId": fmt.Sprintf("%d", owner.ID),
		}, "contract.jpg", "image/jpeg", []byte{0xff, 0xd8, 0xff})
		c, w := newUploadContractContext(t, "0xOwnerA", req)
		UploadFileProtected(c)

		assert.Equal(t, http.StatusBadRequest, w.Code)
	})

	t.Run("pdf still allowed", func(t *testing.T) {
		setContractUploader(t, func(_ *multipart.FileHeader, name, folder string, _ ...s3.UploadOption) (string, error) {
			return fmt.Sprintf("https://s3.example/%s/%s", folder, name), nil
		})

		req := newUploadContractRequest(t, map[string]string{
			"businessId": fmt.Sprintf("%d", owner.ID),
		}, "contract.pdf", "application/pdf", minimalPDF(4096))
		c, w := newUploadContractContext(t, "0xOwnerA", req)
		UploadFileProtected(c)

		assert.Equal(t, http.StatusOK, w.Code, w.Body.String())
	})
}

// #929 repair — the decode gate must never reject an image that uploaded fine
// before it existed. Animated WebP is the sharp edge: golang.org/x/image/webp
// returns a Config straight off the VP8X chunk, so image.DecodeConfig succeeds,
// but a full image.Decode walks the RIFF container looking for a top-level
// "VP8 "/"VP8L" chunk. An animation keeps its frames inside ANMF chunks, so the
// walk runs to io.EOF and reports "webp: invalid format". Every upload surface
// sends accept="image/*", so a GIF logo re-exported as WebP lands here.

// riffChunk frames a payload as a RIFF chunk (FourCC, little-endian length,
// payload, pad byte to an even boundary).
func riffChunk(fourCC string, payload []byte) []byte {
	out := make([]byte, 0, 8+len(payload)+1)
	out = append(out, fourCC...)
	var size [4]byte
	binary.LittleEndian.PutUint32(size[:], uint32(len(payload)))
	out = append(out, size[:]...)
	out = append(out, payload...)
	if len(payload)%2 == 1 {
		out = append(out, 0)
	}
	return out
}

func riffContainer(body []byte) []byte {
	out := make([]byte, 0, 12+len(body))
	out = append(out, "RIFF"...)
	var size [4]byte
	binary.LittleEndian.PutUint32(size[:], uint32(len(body)+4))
	out = append(out, size[:]...)
	out = append(out, "WEBP"...)
	return append(out, body...)
}

func putUint24LE(dst []byte, v int) {
	dst[0] = byte(v)
	dst[1] = byte(v >> 8)
	dst[2] = byte(v >> 16)
}

// webpChunkPayload pulls one top-level chunk out of a WebP container.
func webpChunkPayload(t *testing.T, container []byte, fourCC string) []byte {
	t.Helper()
	require.Greater(t, len(container), 12)
	for off := 12; off+8 <= len(container); {
		size := int(binary.LittleEndian.Uint32(container[off+4 : off+8]))
		require.LessOrEqual(t, off+8+size, len(container))
		if string(container[off:off+4]) == fourCC {
			return container[off+8 : off+8+size]
		}
		off += 8 + size + size%2
	}
	t.Fatalf("chunk %q not present", fourCC)
	return nil
}

// synthesizeAnimatedWebP hand-builds the smallest RIFF/VP8X/ANIM/ANMF file that
// reproduces the exact seam: DecodeConfig reads the VP8X canvas, image.Decode
// cannot find a top-level frame chunk.
func synthesizeAnimatedWebP(width, height int, frameFourCC string, frame []byte) []byte {
	vp8x := make([]byte, 10)
	vp8x[0] = 1 << 1 // animation bit
	putUint24LE(vp8x[4:7], width-1)
	putUint24LE(vp8x[7:10], height-1)

	anim := make([]byte, 6) // background colour + loop count

	anmf := make([]byte, 16)
	putUint24LE(anmf[0:3], 0) // frame x
	putUint24LE(anmf[3:6], 0) // frame y
	putUint24LE(anmf[6:9], width-1)
	putUint24LE(anmf[9:12], height-1)
	putUint24LE(anmf[12:15], 100) // duration ms
	anmf[15] = 0                  // blend + dispose
	anmf = append(anmf, riffChunk(frameFourCC, frame)...)

	body := riffChunk("VP8X", vp8x)
	body = append(body, riffChunk("ANIM", anim)...)
	body = append(body, riffChunk("ANMF", anmf)...)
	return riffContainer(body)
}

// TestAnimatedWebPReproducesTheDecodeSeam pins the library behaviour the repair
// is built on, so a future x/image bump that starts decoding animations makes
// this obvious rather than silently changing the fix's meaning.
func TestAnimatedWebPReproducesTheDecodeSeam(t *testing.T) {
	still := encodeWebPBytes()
	animated := synthesizeAnimatedWebP(32, 32, "VP8 ", webpChunkPayload(t, still, "VP8 "))

	assert.Equal(t, "image/webp", http.DetectContentType(animated))

	config, format, err := image.DecodeConfig(bytes.NewReader(animated))
	require.NoError(t, err, "DecodeConfig must succeed off the VP8X chunk")
	assert.Equal(t, "webp", format)
	assert.Equal(t, 32, config.Width)
	assert.Equal(t, 32, config.Height)

	_, _, err = image.Decode(bytes.NewReader(animated))
	require.Error(t, err, "full decode cannot succeed for an animation")
	assert.Contains(t, err.Error(), "webp: invalid format")
}

func TestUploadFile_AcceptsSynthesizedAnimatedWebP(t *testing.T) {
	setupStaffHandlerTestDB(t)
	owner := createOwnedBusiness(t, "0xOwnerA", "biz-upload-animated-webp")

	still := encodeWebPBytes()
	animated := synthesizeAnimatedWebP(32, 32, "VP8 ", webpChunkPayload(t, still, "VP8 "))

	var uploaded bool
	setPublicUploader(t, func(_ *multipart.FileHeader, name, folder string, _ ...s3.UploadOption) (string, error) {
		uploaded = true
		return fmt.Sprintf("https://s3.example/%s/%s", folder, name), nil
	})

	req := newUploadContractRequest(t, map[string]string{
		"business_id": fmt.Sprintf("%d", owner.ID),
		"folder":      "images",
	}, "logo.webp", "image/webp", animated)
	c, w := newUploadFileContext(t, "0xOwnerA", req)
	UploadFile(c)

	assert.Equal(t, http.StatusOK, w.Code, w.Body.String())
	assert.True(t, uploaded)
}

// TestUploadFile_AcceptsRealAnimatedWebPFiles runs genuine libwebp output
// (img2webp, 40x40, three frames, lossy and lossless) through the handler.
func TestUploadFile_AcceptsRealAnimatedWebPFiles(t *testing.T) {
	setupStaffHandlerTestDB(t)
	owner := createOwnedBusiness(t, "0xOwnerA", "biz-upload-animated-real")

	for _, name := range []string{"animated_lossy.webp", "animated_lossless.webp"} {
		t.Run(name, func(t *testing.T) {
			body, err := os.ReadFile(filepath.Join("testdata", name))
			require.NoError(t, err)
			require.Equal(t, "image/webp", http.DetectContentType(body))

			var uploaded bool
			setPublicUploader(t, func(_ *multipart.FileHeader, n, folder string, _ ...s3.UploadOption) (string, error) {
				uploaded = true
				return fmt.Sprintf("https://s3.example/%s/%s", folder, n), nil
			})

			req := newUploadContractRequest(t, map[string]string{
				"business_id": fmt.Sprintf("%d", owner.ID),
				"folder":      "images",
			}, name, "image/webp", body)
			c, w := newUploadFileContext(t, "0xOwnerA", req)
			UploadFile(c)

			assert.Equal(t, http.StatusOK, w.Code, w.Body.String())
			assert.True(t, uploaded)
		})
	}
}

// Accepting animations must not reopen #929: a VP8X header that claims an
// animation but carries no decodable frame is still garbage.
func TestUploadFile_RejectsAnimationHeaderWithoutFrames(t *testing.T) {
	setupStaffHandlerTestDB(t)
	owner := createOwnedBusiness(t, "0xOwnerA", "biz-upload-animated-garbage")

	vp8x := make([]byte, 10)
	vp8x[0] = 1 << 1
	putUint24LE(vp8x[4:7], 63)
	putUint24LE(vp8x[7:10], 63)
	header := riffChunk("VP8X", vp8x)
	header = append(header, riffChunk("ANIM", make([]byte, 6))...)

	truncatedANMF := append([]byte("ANMF"), 0xff, 0x00, 0x00, 0x00)
	truncatedANMF = append(truncatedANMF, make([]byte, 8)...)

	cases := []struct {
		name string
		body []byte
	}{
		{"no frames at all", riffContainer(header)},
		{"padded with zeros", riffContainer(append(append([]byte{}, header...), make([]byte, 512)...))},
		{"frame chunk runs past the buffer", riffContainer(append(append([]byte{}, header...), truncatedANMF...))},
		{"frame carries no image sub-chunk", riffContainer(append(append([]byte{}, header...), riffChunk("ANMF", make([]byte, 16))...))},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			rejectingPublicUploader(t, "an animation header without a frame is not an image")

			req := newUploadContractRequest(t, map[string]string{
				"business_id": fmt.Sprintf("%d", owner.ID),
				"folder":      "images",
			}, "logo.webp", "image/webp", tc.body)
			c, w := newUploadFileContext(t, "0xOwnerA", req)
			UploadFile(c)

			assert.Equal(t, http.StatusBadRequest, w.Code, w.Body.String())
			var payload map[string]any
			require.NoError(t, json.Unmarshal(w.Body.Bytes(), &payload))
			assert.Equal(t, "invalid_image", payload["code"])
		})
	}
}

// #929 said nothing about a minimum size, and these upload fine on base
// f92388231 — favicon-class artwork and 1x1 spacers must keep working.
func TestUploadFile_AcceptsTinyButRealImages(t *testing.T) {
	setupStaffHandlerTestDB(t)
	owner := createOwnedBusiness(t, "0xOwnerA", "biz-upload-tiny-real")

	cases := []struct {
		name string
		body []byte
	}{
		{"1x1 png", encodePNGBytes(1, 1)},
		{"8x8 png", encodePNGBytes(8, 8)},
		{"15x15 png", encodePNGBytes(15, 15)},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			var uploaded bool
			setPublicUploader(t, func(_ *multipart.FileHeader, name, folder string, _ ...s3.UploadOption) (string, error) {
				uploaded = true
				return fmt.Sprintf("https://s3.example/%s/%s", folder, name), nil
			})

			req := newUploadContractRequest(t, map[string]string{
				"business_id": fmt.Sprintf("%d", owner.ID),
				"folder":      "images",
			}, "icon.png", "image/png", tc.body)
			c, w := newUploadFileContext(t, "0xOwnerA", req)
			UploadFile(c)

			assert.Equal(t, http.StatusOK, w.Code, w.Body.String())
			assert.True(t, uploaded)
		})
	}
}
