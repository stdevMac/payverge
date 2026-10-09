package server

import (
	"bytes"
	"encoding/json"
	"fmt"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/stdevmac/payverge/backend/internal/database"
	"github.com/stdevmac/payverge/backend/internal/s3"
)

// ---- UploadFileProtected tests ----

// setContractUploader swaps the package-level S3 uploader used by
// UploadFileProtected and restores it at cleanup.
func setContractUploader(t *testing.T, fn func(file *multipart.FileHeader, name string, folderPath string, opts ...s3.UploadOption) (string, error)) {
	t.Helper()
	prev := contractUploader
	contractUploader = fn
	t.Cleanup(func() { contractUploader = prev })
}

func setPublicUploader(t *testing.T, fn func(file *multipart.FileHeader, name string, folderPath string, opts ...s3.UploadOption) (string, error)) {
	t.Helper()
	prev := publicUploader
	publicUploader = fn
	t.Cleanup(func() { publicUploader = prev })
}

// newUploadContractRequest builds a multipart/form-data request for
// UploadFileProtected with the given fields and file body.
func newUploadContractRequest(t *testing.T, fields map[string]string, filename, contentType string, body []byte) *http.Request {
	t.Helper()
	var buf bytes.Buffer
	mw := multipart.NewWriter(&buf)
	for k, v := range fields {
		require.NoError(t, mw.WriteField(k, v))
	}
	if filename != "" {
		h := make(map[string][]string)
		h["Content-Disposition"] = []string{fmt.Sprintf(`form-data; name="file"; filename=%q`, filename)}
		if contentType != "" {
			h["Content-Type"] = []string{contentType}
		}
		part, err := mw.CreatePart(h)
		require.NoError(t, err)
		_, err = part.Write(body)
		require.NoError(t, err)
	}
	require.NoError(t, mw.Close())

	requestPath := "/api/v1/inside/uploads"
	if businessID := fields["businessId"]; businessID != "" {
		requestPath = fmt.Sprintf("/api/v1/inside/businesses/%s/uploads/protected", businessID)
	} else if businessID := fields["business_id"]; businessID != "" {
		requestPath = fmt.Sprintf("/api/v1/inside/businesses/%s/uploads", businessID)
	}
	req := httptest.NewRequest(http.MethodPost, requestPath, &buf)
	req.Header.Set("Content-Type", mw.FormDataContentType())
	return req
}

func newUploadRequestWithExtraFields(t *testing.T, fields map[string]string, filename, contentType string, body []byte, extraFields map[string][]byte) *http.Request {
	t.Helper()
	var buf bytes.Buffer
	mw := multipart.NewWriter(&buf)
	for k, v := range fields {
		require.NoError(t, mw.WriteField(k, v))
	}
	for k, v := range extraFields {
		part, err := mw.CreateFormField(k)
		require.NoError(t, err)
		_, err = part.Write(v)
		require.NoError(t, err)
	}
	if filename != "" {
		h := make(map[string][]string)
		h["Content-Disposition"] = []string{fmt.Sprintf(`form-data; name="file"; filename=%q`, filename)}
		if contentType != "" {
			h["Content-Type"] = []string{contentType}
		}
		part, err := mw.CreatePart(h)
		require.NoError(t, err)
		_, err = part.Write(body)
		require.NoError(t, err)
	}
	require.NoError(t, mw.Close())

	requestPath := "/api/v1/inside/uploads"
	if businessID := fields["business_id"]; businessID != "" {
		requestPath = fmt.Sprintf("/api/v1/inside/businesses/%s/uploads", businessID)
	} else if businessID := fields["businessId"]; businessID != "" {
		requestPath = fmt.Sprintf("/api/v1/inside/businesses/%s/uploads/protected", businessID)
	}
	req := httptest.NewRequest(http.MethodPost, requestPath, &buf)
	req.Header.Set("Content-Type", mw.FormDataContentType())
	return req
}

// newUploadContractContext creates a Gin test context for UploadFileProtected
// authenticated as the given Web3 owner.
func newUploadContractContext(t *testing.T, ownerAddress string, req *http.Request) (*gin.Context, *httptest.ResponseRecorder) {
	t.Helper()
	gin.SetMode(gin.TestMode)
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = req
	c.Set("address", ownerAddress)
	setUploadBusinessParamFromPath(c)
	return c, w
}

// minimalPDF returns a valid-looking PDF header padded to size bytes so
// http.DetectContentType returns "application/pdf".
func minimalPDF(size int) []byte {
	head := []byte("%PDF-1.4\n%\xe2\xe3\xcf\xd3\n")
	if size <= len(head) {
		return head[:size]
	}
	out := make([]byte, size)
	copy(out, head)
	// Pad with spaces (any non-control byte works; avoid NULs that could
	// trip other sniffers).
	for i := len(head); i < size; i++ {
		out[i] = ' '
	}
	return out
}

// padImageFixture pads a real encoded image out to size so the size-cap tests
// keep working. All three decoders stop at their end-of-image marker, so
// trailing bytes are ignored — unlike a bare magic-byte header, which #929
// proved the upload path must reject.
func padImageFixture(encoded []byte, size int) []byte {
	if size <= len(encoded) {
		return encoded[:size]
	}
	out := make([]byte, size)
	copy(out, encoded)
	return out
}

func minimalPNG(size int) []byte {
	return padImageFixture(encodePNGBytes(64, 64), size)
}

func minimalJPEG(size int) []byte {
	return padImageFixture(encodeJPEGBytes(64, 64), size)
}

// minimalWebP returns a real WebP that http.DetectContentType also classifies
// as image/webp (Go's sniffer wants the masked pattern RIFF....WEBPVP, see
// net/http/sniff.go).
func minimalWebP(size int) []byte {
	return padImageFixture(encodeWebPBytes(), size)
}

func newUploadFileContext(t *testing.T, ownerAddress string, req *http.Request) (*gin.Context, *httptest.ResponseRecorder) {
	t.Helper()
	gin.SetMode(gin.TestMode)
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = req
	c.Set("address", ownerAddress)
	setUploadBusinessParamFromPath(c)
	return c, w
}

func newUploadUserIDContext(t *testing.T, userID any, req *http.Request) (*gin.Context, *httptest.ResponseRecorder) {
	t.Helper()
	gin.SetMode(gin.TestMode)
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = req
	c.Set("user_id", userID)
	setUploadBusinessParamFromPath(c)
	return c, w
}

func setUploadBusinessParamFromPath(c *gin.Context) {
	parts := strings.Split(strings.Trim(c.Request.URL.Path, "/"), "/")
	for i := 0; i+1 < len(parts); i++ {
		if parts[i] == "businesses" {
			c.Params = gin.Params{{Key: "id", Value: parts[i+1]}}
			return
		}
	}
}

func setUploadBusinessParam(c *gin.Context, businessID uint) {
	c.Params = gin.Params{{Key: "id", Value: fmt.Sprintf("%d", businessID)}}
}

func TestUploadBusinessLogo_RejectsHTML(t *testing.T) {
	setupStaffHandlerTestDB(t)
	owner := createOwnedBusiness(t, "0xOwnerA", "logo-html-owner")
	setPublicUploader(t, func(_ *multipart.FileHeader, _, _ string, _ ...s3.UploadOption) (string, error) {
		t.Fatalf("public uploader must not run for HTML logo")
		return "", nil
	})

	req := newUploadContractRequest(t, map[string]string{}, "logo.png", "text/html", []byte("<!doctype html><html></html>"))
	c, w := newUploadFileContext(t, "0xOwnerA", req)
	setUploadBusinessParam(c, owner.ID)
	UploadBusinessLogo(c)

	assert.Equal(t, http.StatusUnsupportedMediaType, w.Code)
	var payload map[string]any
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &payload))
	assert.Equal(t, "unsupported_media_type", payload["code"])
}

func TestUploadBusinessLogo_AllowsPNGAndSetsMetadata(t *testing.T) {
	setupStaffHandlerTestDB(t)
	owner := createOwnedBusiness(t, "0xOwnerA", "logo-png-owner")

	var capturedContentType, capturedDisposition string
	setPublicUploader(t, func(_ *multipart.FileHeader, name, folder string, opts ...s3.UploadOption) (string, error) {
		applied := s3.ApplyUploadOptions(opts...)
		capturedContentType = applied.ContentType
		capturedDisposition = applied.ContentDisposition
		assert.Contains(t, name, ".png")
		assert.Equal(t, fmt.Sprintf("businesses/%d/logo", owner.ID), folder)
		return fmt.Sprintf("https://s3.example/%s/%s", folder, name), nil
	})

	req := newUploadContractRequest(t, map[string]string{}, "logo.png", "image/png", minimalPNG(4096))
	c, w := newUploadFileContext(t, "0xOwnerA", req)
	setUploadBusinessParam(c, owner.ID)
	UploadBusinessLogo(c)

	assert.Equal(t, http.StatusOK, w.Code)
	assert.Equal(t, "image/png", capturedContentType)
	assert.Contains(t, capturedDisposition, `inline; filename=`)
}

func TestUploadBusinessLogo_AllowsUintUserIDAndSetsFolder(t *testing.T) {
	setupStaffHandlerTestDB(t)
	owner := createOwnedBusiness(t, "0xWalletOwner", "logo-user-id-owner")
	ownerUserID := uint(42)
	owner.UserID = &ownerUserID
	require.NoError(t, database.GetDB().Save(owner).Error)

	var capturedFolder string
	setPublicUploader(t, func(_ *multipart.FileHeader, name, folder string, opts ...s3.UploadOption) (string, error) {
		applied := s3.ApplyUploadOptions(opts...)
		assert.Equal(t, "image/png", applied.ContentType)
		assert.Contains(t, name, ".png")
		capturedFolder = folder
		return fmt.Sprintf("https://s3.example/%s/%s", folder, name), nil
	})

	req := newUploadContractRequest(t, map[string]string{}, "logo.png", "image/png", minimalPNG(4096))
	c, w := newUploadUserIDContext(t, uint(42), req)
	setUploadBusinessParam(c, owner.ID)
	UploadBusinessLogo(c)

	assert.Equal(t, http.StatusOK, w.Code)
	assert.Equal(t, fmt.Sprintf("businesses/%d/logo", owner.ID), capturedFolder)
}

func TestUploadBusinessLogo_RejectsPNGNamedHTML(t *testing.T) {
	setupStaffHandlerTestDB(t)
	owner := createOwnedBusiness(t, "0xOwnerA", "logo-extension-owner")
	setPublicUploader(t, func(_ *multipart.FileHeader, _, _ string, _ ...s3.UploadOption) (string, error) {
		t.Fatalf("public uploader must not run for mismatched logo extension")
		return "", nil
	})

	req := newUploadContractRequest(t, map[string]string{}, "logo.html", "image/png", minimalPNG(4096))
	c, w := newUploadFileContext(t, "0xOwnerA", req)
	setUploadBusinessParam(c, owner.ID)
	UploadBusinessLogo(c)

	assert.Equal(t, http.StatusUnsupportedMediaType, w.Code)
	var payload map[string]any
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &payload))
	assert.Equal(t, "unsupported_media_type", payload["code"])
}

func TestUploadBusinessLogo_AllowsPNGAtMaxFileSize(t *testing.T) {
	setupStaffHandlerTestDB(t)
	owner := createOwnedBusiness(t, "0xOwnerA", "logo-max-owner")

	setPublicUploader(t, func(_ *multipart.FileHeader, name, folder string, opts ...s3.UploadOption) (string, error) {
		applied := s3.ApplyUploadOptions(opts...)
		assert.Equal(t, "image/png", applied.ContentType)
		assert.Contains(t, name, ".png")
		assert.Equal(t, fmt.Sprintf("businesses/%d/logo", owner.ID), folder)
		return fmt.Sprintf("https://s3.example/%s/%s", folder, name), nil
	})

	req := newUploadContractRequest(t, map[string]string{}, "logo.png", "image/png", minimalPNG(maxLogoUploadBytes))
	c, w := newUploadFileContext(t, "0xOwnerA", req)
	setUploadBusinessParam(c, owner.ID)
	UploadBusinessLogo(c)

	assert.Equal(t, http.StatusOK, w.Code)
}

func TestUploadBusinessLogo_RejectsMultipartBodyOverCapBeforeParsing(t *testing.T) {
	setupStaffHandlerTestDB(t)
	owner := createOwnedBusiness(t, "0xOwnerA", "logo-body-cap-owner")
	setPublicUploader(t, func(_ *multipart.FileHeader, _, _ string, _ ...s3.UploadOption) (string, error) {
		t.Fatalf("public uploader must not run when multipart body exceeds cap")
		return "", nil
	})

	req := newUploadRequestWithExtraFields(t, map[string]string{}, "logo.png", "image/png", minimalPNG(4096), map[string][]byte{
		"padding": bytes.Repeat([]byte("x"), maxLogoUploadBytes+multipartOverheadAllowanceBytes+1),
	})
	c, w := newUploadFileContext(t, "0xOwnerA", req)
	setUploadBusinessParam(c, owner.ID)
	UploadBusinessLogo(c)

	assert.Equal(t, http.StatusRequestEntityTooLarge, w.Code)
}

func TestUploadFile_AllowsPDFAndSetsMetadataUnderBusinessFolder(t *testing.T) {
	setupStaffHandlerTestDB(t)
	owner := createOwnedBusiness(t, "0xOwnerA", "biz-upload-public-pdf")

	var capturedName, capturedFolder, capturedContentType, capturedDisposition string
	setPublicUploader(t, func(_ *multipart.FileHeader, name, folder string, opts ...s3.UploadOption) (string, error) {
		applied := s3.ApplyUploadOptions(opts...)
		capturedName = name
		capturedFolder = folder
		capturedContentType = applied.ContentType
		capturedDisposition = applied.ContentDisposition
		return fmt.Sprintf("https://s3.example/%s/%s", folder, name), nil
	})

	req := newUploadContractRequest(t, map[string]string{
		"business_id": fmt.Sprintf("%d", owner.ID),
		"folder":      "menus",
	}, "menu.pdf", "application/pdf", minimalPDF(4096))
	c, w := newUploadFileContext(t, "0xOwnerA", req)
	UploadFile(c)

	assert.Equal(t, http.StatusOK, w.Code)
	assert.Contains(t, capturedName, ".pdf")
	assert.Equal(t, fmt.Sprintf("businesses/%d/menus", owner.ID), capturedFolder)
	assert.Equal(t, "application/pdf", capturedContentType)
	assert.Contains(t, capturedDisposition, `inline; filename=`)
}

func TestUploadFile_AllowsPNGAndSetsMetadataUnderBusinessFolder(t *testing.T) {
	setupStaffHandlerTestDB(t)
	owner := createOwnedBusiness(t, "0xOwnerA", "biz-upload-public-png")

	var capturedFolder, capturedContentType string
	setPublicUploader(t, func(_ *multipart.FileHeader, name, folder string, opts ...s3.UploadOption) (string, error) {
		applied := s3.ApplyUploadOptions(opts...)
		capturedFolder = folder
		capturedContentType = applied.ContentType
		assert.Contains(t, name, ".png")
		assert.Contains(t, applied.ContentDisposition, `inline; filename=`)
		return fmt.Sprintf("https://s3.example/%s/%s", folder, name), nil
	})

	req := newUploadContractRequest(t, map[string]string{
		"business_id": fmt.Sprintf("%d", owner.ID),
		"folder":      "images",
	}, "menu.png", "image/png", minimalPNG(4096))
	c, w := newUploadFileContext(t, "0xOwnerA", req)
	UploadFile(c)

	assert.Equal(t, http.StatusOK, w.Code)
	assert.Equal(t, fmt.Sprintf("businesses/%d/images", owner.ID), capturedFolder)
	assert.Equal(t, "image/png", capturedContentType)
}

func TestUploadFile_AllowsJPEGAndSetsMetadataUnderBusinessFolder(t *testing.T) {
	setupStaffHandlerTestDB(t)
	owner := createOwnedBusiness(t, "0xOwnerA", "biz-upload-public-jpeg")

	var capturedFolder, capturedContentType string
	setPublicUploader(t, func(_ *multipart.FileHeader, name, folder string, opts ...s3.UploadOption) (string, error) {
		applied := s3.ApplyUploadOptions(opts...)
		capturedFolder = folder
		capturedContentType = applied.ContentType
		assert.Contains(t, name, ".jpeg")
		assert.Contains(t, applied.ContentDisposition, `inline; filename=`)
		return fmt.Sprintf("https://s3.example/%s/%s", folder, name), nil
	})

	req := newUploadContractRequest(t, map[string]string{
		"business_id": fmt.Sprintf("%d", owner.ID),
		"folder":      "images",
	}, "menu.jpeg", "image/jpeg", minimalJPEG(4096))
	c, w := newUploadFileContext(t, "0xOwnerA", req)
	UploadFile(c)

	assert.Equal(t, http.StatusOK, w.Code)
	assert.Equal(t, fmt.Sprintf("businesses/%d/images", owner.ID), capturedFolder)
	assert.Equal(t, "image/jpeg", capturedContentType)
}

func TestUploadFile_AllowsUintUserIDBusiness(t *testing.T) {
	setupStaffHandlerTestDB(t)
	ownerUserID := uint(77)
	business := &database.Business{
		BusinessId:     "biz-upload-user-id",
		Name:           "biz-upload-user-id",
		OwnerAddress:   "0xOwnerUserID",
		UserID:         &ownerUserID,
		SettlementAddr: "0x1111111111111111111111111111111111111111",
		TippingAddr:    "0x2222222222222222222222222222222222222222",
	}
	require.NoError(t, database.GetDB().Create(business).Error)

	var capturedFolder string
	setPublicUploader(t, func(_ *multipart.FileHeader, name, folder string, opts ...s3.UploadOption) (string, error) {
		applied := s3.ApplyUploadOptions(opts...)
		assert.Equal(t, "application/pdf", applied.ContentType)
		assert.Contains(t, name, ".pdf")
		capturedFolder = folder
		return fmt.Sprintf("https://s3.example/%s/%s", folder, name), nil
	})

	req := newUploadContractRequest(t, map[string]string{
		"business_id": fmt.Sprintf("%d", business.ID),
	}, "menu.pdf", "application/pdf", minimalPDF(4096))
	c, w := newUploadUserIDContext(t, ownerUserID, req)
	UploadFile(c)

	assert.Equal(t, http.StatusOK, w.Code)
	assert.Equal(t, fmt.Sprintf("businesses/%d", business.ID), capturedFolder)
}

func TestUploadFile_AllowsPDFAtMaxFileSize(t *testing.T) {
	setupStaffHandlerTestDB(t)
	owner := createOwnedBusiness(t, "0xOwnerA", "biz-upload-public-max-pdf")

	setPublicUploader(t, func(_ *multipart.FileHeader, name, folder string, opts ...s3.UploadOption) (string, error) {
		applied := s3.ApplyUploadOptions(opts...)
		assert.Equal(t, "application/pdf", applied.ContentType)
		assert.Contains(t, name, ".pdf")
		assert.Equal(t, fmt.Sprintf("businesses/%d", owner.ID), folder)
		return fmt.Sprintf("https://s3.example/%s/%s", folder, name), nil
	})

	req := newUploadContractRequest(t, map[string]string{
		"business_id": fmt.Sprintf("%d", owner.ID),
	}, "menu.pdf", "application/pdf", minimalPDF(maxProtectedUploadBytes))
	c, w := newUploadFileContext(t, "0xOwnerA", req)
	UploadFile(c)

	assert.Equal(t, http.StatusOK, w.Code)
}

func TestUploadFile_RejectsMultipartBodyOverCapBeforeParsing(t *testing.T) {
	setupStaffHandlerTestDB(t)
	owner := createOwnedBusiness(t, "0xOwnerA", "biz-upload-body-cap")
	setPublicUploader(t, func(_ *multipart.FileHeader, _, _ string, _ ...s3.UploadOption) (string, error) {
		t.Fatalf("public uploader must not run when multipart body exceeds cap")
		return "", nil
	})

	req := newUploadRequestWithExtraFields(t, map[string]string{
		"business_id": fmt.Sprintf("%d", owner.ID),
	}, "menu.pdf", "application/pdf", minimalPDF(4096), map[string][]byte{
		"padding": bytes.Repeat([]byte("x"), maxProtectedUploadBytes+multipartOverheadAllowanceBytes+1),
	})
	c, w := newUploadFileContext(t, "0xOwnerA", req)
	UploadFile(c)

	assert.Equal(t, http.StatusRequestEntityTooLarge, w.Code)
}

func TestUploadFile_RejectsHTML(t *testing.T) {
	setupStaffHandlerTestDB(t)
	owner := createOwnedBusiness(t, "0xOwnerA", "biz-upload-html")
	setPublicUploader(t, func(_ *multipart.FileHeader, _, _ string, _ ...s3.UploadOption) (string, error) {
		t.Fatalf("public uploader must not run for HTML")
		return "", nil
	})

	req := newUploadContractRequest(t, map[string]string{
		"business_id": fmt.Sprintf("%d", owner.ID),
	}, "menu.pdf", "text/html", []byte("<!doctype html><html></html>"))
	c, w := newUploadFileContext(t, "0xOwnerA", req)
	UploadFile(c)

	assert.Equal(t, http.StatusUnsupportedMediaType, w.Code)
}

func TestUploadFile_RejectsPDFNamedHTML(t *testing.T) {
	setupStaffHandlerTestDB(t)
	owner := createOwnedBusiness(t, "0xOwnerA", "biz-upload-pdf-named-html")
	setPublicUploader(t, func(_ *multipart.FileHeader, _, _ string, _ ...s3.UploadOption) (string, error) {
		t.Fatalf("public uploader must not run for mismatched PDF extension")
		return "", nil
	})

	req := newUploadContractRequest(t, map[string]string{
		"business_id": fmt.Sprintf("%d", owner.ID),
	}, "menu.html", "application/pdf", minimalPDF(4096))
	c, w := newUploadFileContext(t, "0xOwnerA", req)
	UploadFile(c)

	assert.Equal(t, http.StatusUnsupportedMediaType, w.Code)
}

func TestUploadFile_RejectsSVG(t *testing.T) {
	setupStaffHandlerTestDB(t)
	owner := createOwnedBusiness(t, "0xOwnerA", "biz-upload-svg")
	setPublicUploader(t, func(_ *multipart.FileHeader, _, _ string, _ ...s3.UploadOption) (string, error) {
		t.Fatalf("public uploader must not run for SVG")
		return "", nil
	})

	req := newUploadContractRequest(t, map[string]string{
		"business_id": fmt.Sprintf("%d", owner.ID),
	}, "logo.svg", "image/svg+xml", []byte(`<svg xmlns="http://www.w3.org/2000/svg"><script>alert(1)</script></svg>`))
	c, w := newUploadFileContext(t, "0xOwnerA", req)
	UploadFile(c)

	assert.Equal(t, http.StatusUnsupportedMediaType, w.Code)
}

func TestUploadFile_RejectsOverSizedBody(t *testing.T) {
	setupStaffHandlerTestDB(t)
	owner := createOwnedBusiness(t, "0xOwnerA", "biz-upload-big")
	setPublicUploader(t, func(_ *multipart.FileHeader, _, _ string, _ ...s3.UploadOption) (string, error) {
		t.Fatalf("public uploader must not run for oversized upload")
		return "", nil
	})

	req := newUploadContractRequest(t, map[string]string{
		"business_id": fmt.Sprintf("%d", owner.ID),
	}, "huge.pdf", "application/pdf", minimalPDF(11<<20))
	c, w := newUploadFileContext(t, "0xOwnerA", req)
	UploadFile(c)

	assert.Equal(t, http.StatusRequestEntityTooLarge, w.Code)
}

func TestUploadFile_RejectsUnsafeFilename(t *testing.T) {
	setupStaffHandlerTestDB(t)
	owner := createOwnedBusiness(t, "0xOwnerA", "biz-upload-unsafe-name")
	setPublicUploader(t, func(_ *multipart.FileHeader, _, _ string, _ ...s3.UploadOption) (string, error) {
		t.Fatalf("public uploader must not run for unsafe filename")
		return "", nil
	})

	req := newUploadContractRequest(t, map[string]string{
		"business_id": fmt.Sprintf("%d", owner.ID),
	}, `..\evil.pdf`, "application/pdf", minimalPDF(4096))
	c, w := newUploadFileContext(t, "0xOwnerA", req)
	UploadFile(c)

	assert.Equal(t, http.StatusBadRequest, w.Code)
}

func TestUploadFile_RejectsRawForwardSlashFilename(t *testing.T) {
	setupStaffHandlerTestDB(t)
	owner := createOwnedBusiness(t, "0xOwnerA", "biz-upload-forward-slash-name")
	setPublicUploader(t, func(_ *multipart.FileHeader, _, _ string, _ ...s3.UploadOption) (string, error) {
		t.Fatalf("public uploader must not run for raw path filename")
		return "", nil
	})

	req := newUploadContractRequest(t, map[string]string{
		"business_id": fmt.Sprintf("%d", owner.ID),
	}, "dir/menu.pdf", "application/pdf", minimalPDF(4096))
	c, w := newUploadFileContext(t, "0xOwnerA", req)
	UploadFile(c)

	assert.Equal(t, http.StatusBadRequest, w.Code)
}

func TestUploadFile_RejectsRawBackslashFilename(t *testing.T) {
	setupStaffHandlerTestDB(t)
	owner := createOwnedBusiness(t, "0xOwnerA", "biz-upload-backslash-name")
	setPublicUploader(t, func(_ *multipart.FileHeader, _, _ string, _ ...s3.UploadOption) (string, error) {
		t.Fatalf("public uploader must not run for raw path filename")
		return "", nil
	})

	req := newUploadContractRequest(t, map[string]string{
		"business_id": fmt.Sprintf("%d", owner.ID),
	}, `dir\menu.pdf`, "application/pdf", minimalPDF(4096))
	c, w := newUploadFileContext(t, "0xOwnerA", req)
	UploadFile(c)

	assert.Equal(t, http.StatusBadRequest, w.Code)
}

func TestUploadFile_RejectsCustomFolderTraversal(t *testing.T) {
	setupStaffHandlerTestDB(t)
	owner := createOwnedBusiness(t, "0xOwnerA", "biz-upload-folder-traversal")
	setPublicUploader(t, func(_ *multipart.FileHeader, _, _ string, _ ...s3.UploadOption) (string, error) {
		t.Fatalf("public uploader must not run for folder traversal")
		return "", nil
	})

	req := newUploadContractRequest(t, map[string]string{
		"business_id": fmt.Sprintf("%d", owner.ID),
		"folder":      "../other-business",
	}, "menu.pdf", "application/pdf", minimalPDF(4096))
	c, w := newUploadFileContext(t, "0xOwnerA", req)
	UploadFile(c)

	assert.Equal(t, http.StatusBadRequest, w.Code)
}

func TestUploadFileProtected_AllowsOwnerPDFAndDerivesKey(t *testing.T) {
	setupStaffHandlerTestDB(t)
	owner := createOwnedBusiness(t, "0xOwnerA", "biz-up-owner-pdf")

	var capturedName, capturedFolder string
	setContractUploader(t, func(file *multipart.FileHeader, name, folder string, _ ...s3.UploadOption) (string, error) {
		capturedName = name
		capturedFolder = folder
		return fmt.Sprintf("https://s3.example/%s/%s", folder, name), nil
	})

	body := minimalPDF(1 << 20) // 1 MiB
	req := newUploadContractRequest(t, map[string]string{
		"businessId": fmt.Sprintf("%d", owner.ID),
	}, "nda.pdf", "application/pdf", body)

	c, w := newUploadContractContext(t, "0xOwnerA", req)
	UploadFileProtected(c)

	assert.Equal(t, http.StatusOK, w.Code)
	// Stored under a unique name (never overwrites); the extension is kept.
	assert.NotEqual(t, "nda.pdf", capturedName)
	assert.True(t, strings.HasSuffix(capturedName, ".pdf"), capturedName)
	assert.Equal(t, fmt.Sprintf("businesses/%d/contracts", owner.ID), capturedFolder)
}

func TestUploadFileProtected_RejectsCrossTenantUpload(t *testing.T) {
	setupStaffHandlerTestDB(t)
	_ = createOwnedBusiness(t, "0xOwnerA", "biz-up-owner-a")
	other := createOwnedBusiness(t, "0xOwnerB", "biz-up-owner-b")

	setContractUploader(t, func(_ *multipart.FileHeader, _, _ string, _ ...s3.UploadOption) (string, error) {
		t.Fatalf("uploader must not run for cross-tenant attempt")
		return "", nil
	})

	body := minimalPDF(4096)
	req := newUploadContractRequest(t, map[string]string{
		"businessId": fmt.Sprintf("%d", other.ID),
	}, "steal.pdf", "application/pdf", body)

	c, w := newUploadContractContext(t, "0xOwnerA", req)
	UploadFileProtected(c)

	assert.Equal(t, http.StatusForbidden, w.Code)
}

func TestUploadFileProtected_IgnoresCallerSuppliedFolder_PreventsTraversal(t *testing.T) {
	setupStaffHandlerTestDB(t)
	owner := createOwnedBusiness(t, "0xOwnerA", "biz-up-folder")

	var capturedFolder string
	setContractUploader(t, func(_ *multipart.FileHeader, _, folder string, _ ...s3.UploadOption) (string, error) {
		capturedFolder = folder
		return "ok", nil
	})

	body := minimalPDF(4096)
	req := newUploadContractRequest(t, map[string]string{
		"businessId": fmt.Sprintf("%d", owner.ID),
		"folder":     "../../../escape",
	}, "nda.pdf", "application/pdf", body)

	c, w := newUploadContractContext(t, "0xOwnerA", req)
	UploadFileProtected(c)

	assert.Equal(t, http.StatusOK, w.Code)
	assert.Equal(t, fmt.Sprintf("businesses/%d/contracts", owner.ID), capturedFolder)
	assert.NotContains(t, capturedFolder, "..")
}

func TestUploadFileProtected_RejectsHTMLContentType(t *testing.T) {
	setupStaffHandlerTestDB(t)
	owner := createOwnedBusiness(t, "0xOwnerA", "biz-up-html")

	setContractUploader(t, func(_ *multipart.FileHeader, _, _ string, _ ...s3.UploadOption) (string, error) {
		t.Fatalf("uploader must not run for disallowed content type")
		return "", nil
	})

	// Body sniffs to text/html — http.DetectContentType keys on leading
	// "<!doctype html" / "<html".
	body := []byte("<!DOCTYPE html><html><body>not a pdf</body></html>")
	req := newUploadContractRequest(t, map[string]string{
		"businessId": fmt.Sprintf("%d", owner.ID),
	}, "evil.pdf", "text/html", body)

	c, w := newUploadContractContext(t, "0xOwnerA", req)
	UploadFileProtected(c)

	assert.Equal(t, http.StatusUnsupportedMediaType, w.Code)
	var payload map[string]any
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &payload))
	assert.Equal(t, "unsupported_media_type", payload["code"])
}

func TestUploadFileProtected_RejectsPDFNamedHTML(t *testing.T) {
	setupStaffHandlerTestDB(t)
	owner := createOwnedBusiness(t, "0xOwnerA", "biz-up-pdf-named-html")

	setContractUploader(t, func(_ *multipart.FileHeader, _, _ string, _ ...s3.UploadOption) (string, error) {
		t.Fatalf("uploader must not run for mismatched protected extension")
		return "", nil
	})

	req := newUploadContractRequest(t, map[string]string{
		"businessId": fmt.Sprintf("%d", owner.ID),
	}, "contract.html", "application/pdf", minimalPDF(4096))

	c, w := newUploadContractContext(t, "0xOwnerA", req)
	UploadFileProtected(c)

	assert.Equal(t, http.StatusUnsupportedMediaType, w.Code)
}

func TestUploadFileProtected_RejectsOverSizedBody(t *testing.T) {
	setupStaffHandlerTestDB(t)
	owner := createOwnedBusiness(t, "0xOwnerA", "biz-up-big")

	setContractUploader(t, func(_ *multipart.FileHeader, _, _ string, _ ...s3.UploadOption) (string, error) {
		t.Fatalf("uploader must not run for oversized body")
		return "", nil
	})

	body := minimalPDF(11 << 20) // 11 MiB > 10 MiB cap
	req := newUploadContractRequest(t, map[string]string{
		"businessId": fmt.Sprintf("%d", owner.ID),
	}, "huge.pdf", "application/pdf", body)

	c, w := newUploadContractContext(t, "0xOwnerA", req)
	UploadFileProtected(c)

	assert.Equal(t, http.StatusRequestEntityTooLarge, w.Code)
	var payload map[string]any
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &payload))
	assert.Equal(t, "payload_too_large", payload["code"])
}

func TestUploadFileProtected_RequiresBusinessID(t *testing.T) {
	setupStaffHandlerTestDB(t)
	_ = createOwnedBusiness(t, "0xOwnerA", "biz-up-missing-id")

	setContractUploader(t, func(_ *multipart.FileHeader, _, _ string, _ ...s3.UploadOption) (string, error) {
		t.Fatalf("uploader must not run when businessId is missing")
		return "", nil
	})

	body := minimalPDF(4096)
	req := newUploadContractRequest(t, map[string]string{}, "nda.pdf", "application/pdf", body)

	c, w := newUploadContractContext(t, "0xOwnerA", req)
	UploadFileProtected(c)

	assert.Equal(t, http.StatusBadRequest, w.Code)
	assert.Contains(t, strings.ToLower(w.Body.String()), "business id")
}
