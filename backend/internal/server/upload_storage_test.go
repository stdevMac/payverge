package server

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/stdevmac/payverge/backend/internal/s3"
)

// useLocalStorage points the s3 package wrappers at temp-dir LocalStores (the
// zero-config self-hosted default) with relative /media URLs, and returns the
// public root so a test can "restart" by reopening it.
func useLocalStorage(t *testing.T) (publicRoot, protectedRoot string) {
	t.Helper()
	publicRoot, protectedRoot = t.TempDir(), t.TempDir()
	public, err := s3.NewLocalStore(publicRoot)
	require.NoError(t, err)
	protected, err := s3.NewLocalStore(protectedRoot)
	require.NoError(t, err)
	t.Cleanup(s3.SetStores(public, protected))
	t.Cleanup(s3.SetPublicURL(""))
	return publicRoot, protectedRoot
}

func mediaRouter() *gin.Engine {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.GET("/media/*key", s3.MediaHandler())
	r.HEAD("/media/*key", s3.MediaHandler())
	return r
}

func getMedia(t *testing.T, r http.Handler, target string) *httptest.ResponseRecorder {
	t.Helper()
	w := httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest(http.MethodGet, target, nil))
	return w
}

func setPublicDeleter(t *testing.T, fn func(string) error) {
	t.Helper()
	prev := publicDeleter
	publicDeleter = fn
	t.Cleanup(func() { publicDeleter = prev })
}

func deleteUploadRequest(t *testing.T, ownerAddress string, businessID uint, key string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(http.MethodDelete,
		fmt.Sprintf("/api/v1/inside/businesses/%d/uploads?key=%s", businessID, url.QueryEscape(key)), nil)
	c, w := newUploadFileContext(t, ownerAddress, req)
	DeleteUploadedFile(c)
	return w
}

func TestCleanUploadFolder(t *testing.T) {
	for in, want := range map[string]string{
		"":                  "",
		"/":                 "",
		"menu-item":         "menu-item",
		"/gallery/":         "gallery",
		"qr-logos":          "qr-logos",
		"menus/2026/spring": "menus/2026/spring",
		"partner_icons":     "partner_icons",
	} {
		got, err := cleanUploadFolder(in)
		require.NoError(t, err, in)
		require.Equal(t, want, got, in)
	}
	for _, bad := range []string{
		"..", "../other-business", "a/../../b", "a/./b", "a//b", `a\b`, ".hidden",
		"-dash-first", "a b", "a%2f..", "ñ", "contracts", "Contracts/x", "a/b/c/d",
		strings.Repeat("x", 65),
	} {
		_, err := cleanUploadFolder(bad)
		require.Error(t, err, bad)
	}
}

func TestStorageKeyFromInput(t *testing.T) {
	restore := s3.SetPublicURL("https://pos.example.test")
	defer restore()
	for in, want := range map[string]string{
		"businesses/7/menu-item/a.png":                                 "businesses/7/menu-item/a.png",
		"/businesses/7/menu-item/a.png":                                "businesses/7/menu-item/a.png",
		"/media/businesses/7/menu-item/a.png":                          "businesses/7/menu-item/a.png",
		"https://pos.example.test/media/businesses/7/a.png":            "businesses/7/a.png",
		"https://bucket.s3.us-east-1.amazonaws.com/businesses/7/a.png": "businesses/7/a.png",
	} {
		got, err := storageKeyFromInput(in)
		require.NoError(t, err, in)
		require.Equal(t, want, got, in)
	}
	for _, bad := range []string{
		"", "/", "/media/", "businesses/7/../8/a.png", "/media/businesses/7/%2e%2e/8/a.png",
		"businesses/7//a.png", "businesses/7/./a.png", "file:///etc/passwd", `businesses\7\a.png`,
	} {
		_, err := storageKeyFromInput(bad)
		require.Error(t, err, bad)
	}
}

func TestDeleteUploadedFile_RejectsTraversalAndProtectedKeys(t *testing.T) {
	setupStaffHandlerTestDB(t)
	owner := createOwnedBusiness(t, "0xOwnerA", "biz-delete-guards")
	other := createOwnedBusiness(t, "0xOwnerB", "biz-delete-guards-other")

	var deleted []string
	setPublicDeleter(t, func(key string) error {
		deleted = append(deleted, key)
		return nil
	})

	cases := map[string]int{
		// traversal: rejected before any tenant comparison
		fmt.Sprintf("businesses/%d/../%d/menu-item/a.png", owner.ID, other.ID):    http.StatusBadRequest,
		fmt.Sprintf("/media/businesses/%d/%%2e%%2e/%d/a.png", owner.ID, other.ID): http.StatusBadRequest,
		fmt.Sprintf("businesses/%d//a.png", owner.ID):                             http.StatusBadRequest,
		"": http.StatusBadRequest,
		// outside the tenant prefix, or the tenant prefix itself
		fmt.Sprintf("businesses/%d/menu-item/a.png", other.ID):  http.StatusForbidden,
		fmt.Sprintf("businesses/%d0/menu-item/a.png", owner.ID): http.StatusForbidden,
		fmt.Sprintf("businesses/%d", owner.ID):                  http.StatusForbidden,
		"demo-arg/assets/carta/flan.jpg":                        http.StatusForbidden,
		// protected namespace is never deletable through the public endpoint
		fmt.Sprintf("businesses/%d/contracts/nda.pdf", owner.ID):        http.StatusForbidden,
		fmt.Sprintf("/media/businesses/%d/contracts/nda.pdf", owner.ID): http.StatusForbidden,
	}
	for key, want := range cases {
		w := deleteUploadRequest(t, "0xOwnerA", owner.ID, key)
		assert.Equal(t, want, w.Code, "key %q: %s", key, w.Body.String())
	}
	require.Empty(t, deleted, "no rejected key may reach the store")

	// Every accepted URL shape resolves to the same tenant key.
	for _, input := range []string{
		fmt.Sprintf("businesses/%d/menu-item/a.png", owner.ID),
		fmt.Sprintf("/media/businesses/%d/menu-item/a.png", owner.ID),
		fmt.Sprintf("https://bucket.s3.us-east-1.amazonaws.com/businesses/%d/menu-item/a.png", owner.ID),
	} {
		w := deleteUploadRequest(t, "0xOwnerA", owner.ID, input)
		require.Equal(t, http.StatusOK, w.Code, input)
	}
	require.Equal(t, []string{
		fmt.Sprintf("businesses/%d/menu-item/a.png", owner.ID),
		fmt.Sprintf("businesses/%d/menu-item/a.png", owner.ID),
		fmt.Sprintf("businesses/%d/menu-item/a.png", owner.ID),
	}, deleted)
}

func TestUploadFileProtected_StoresUniqueKeysAndNeverOverwrites(t *testing.T) {
	setupStaffHandlerTestDB(t)
	owner := createOwnedBusiness(t, "0xOwnerA", "biz-protected-unique")

	var names []string
	setContractUploader(t, func(_ *multipart.FileHeader, name, _ string, _ ...s3.UploadOption) (string, error) {
		names = append(names, name)
		return "ok", nil
	})
	for i := 0; i < 2; i++ {
		req := newUploadContractRequest(t, map[string]string{
			"businessId": fmt.Sprintf("%d", owner.ID),
		}, "contract.pdf", "application/pdf", minimalPDF(4096))
		c, w := newUploadContractContext(t, "0xOwnerA", req)
		UploadFileProtected(c)
		require.Equal(t, http.StatusOK, w.Code, w.Body.String())

		var body map[string]any
		require.NoError(t, json.Unmarshal(w.Body.Bytes(), &body))
		assert.Equal(t, "contract.pdf", body["filename"], "response keeps the caller's display name")
	}
	require.Len(t, names, 2)
	assert.NotEqual(t, names[0], names[1], "two uploads of one filename must not share a key")
	for _, name := range names {
		assert.True(t, strings.HasSuffix(name, ".pdf"), name)
		assert.NotEqual(t, "contract.pdf", name)
		require.NoError(t, s3.ValidateKey(name))
	}
}

// Menu image: upload handler -> /media -> restart -> delete handler -> 404, on
// the local driver with no third-party account.
func TestMenuImageUploadServeDeleteRoundTripOnLocalStorage(t *testing.T) {
	setupStaffHandlerTestDB(t)
	publicRoot, protectedRoot := useLocalStorage(t)
	owner := createOwnedBusiness(t, "0xOwnerA", "biz-local-roundtrip")
	image := minimalPNG(2048)

	req := newUploadContractRequest(t, map[string]string{
		"business_id": fmt.Sprintf("%d", owner.ID),
		"folder":      "menu-item",
	}, "milanesa.png", "image/png", image)
	c, w := newUploadFileContext(t, "0xOwnerA", req)
	UploadFile(c)
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())

	var resp struct {
		Location string `json:"location"`
		Folder   string `json:"folder"`
	}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	prefix := fmt.Sprintf("/media/businesses/%d/menu-item/", owner.ID)
	require.True(t, strings.HasPrefix(resp.Location, prefix), resp.Location)
	require.True(t, s3.IsOwnMediaURL(resp.Location))

	router := mediaRouter()
	got := getMedia(t, router, resp.Location)
	require.Equal(t, http.StatusOK, got.Code)
	assert.Equal(t, image, got.Body.Bytes())
	assert.Equal(t, "image/png", got.Header().Get("Content-Type"))
	assert.Equal(t, "nosniff", got.Header().Get("X-Content-Type-Options"))
	assert.Equal(t, s3.MediaCacheImmutable, got.Header().Get("Cache-Control"))
	etag := got.Header().Get("ETag")
	require.NotEmpty(t, etag)

	// Restart: a fresh LocalStore over the same directory serves the object.
	public, err := s3.NewLocalStore(publicRoot)
	require.NoError(t, err)
	protected, err := s3.NewLocalStore(protectedRoot)
	require.NoError(t, err)
	s3.SetStores(public, protected)
	again := getMedia(t, router, resp.Location)
	require.Equal(t, http.StatusOK, again.Code)
	assert.Equal(t, etag, again.Header().Get("ETag"))

	// Delete through the tenant-scoped handler using the stored URL.
	dw := deleteUploadRequest(t, "0xOwnerA", owner.ID, resp.Location)
	require.Equal(t, http.StatusOK, dw.Code, dw.Body.String())
	assert.Equal(t, http.StatusNotFound, getMedia(t, router, resp.Location).Code)
}

// Protected contract upload on the local driver: unique key, never served by
// /media, readable through the authorized protected download path.
func TestProtectedUploadRoundTripOnLocalStorage(t *testing.T) {
	setupStaffHandlerTestDB(t)
	useLocalStorage(t)
	owner := createOwnedBusiness(t, "0xOwnerA", "biz-local-protected")

	upload := func(body []byte) string {
		req := newUploadContractRequest(t, map[string]string{
			"businessId": fmt.Sprintf("%d", owner.ID),
		}, "nda.pdf", "application/pdf", body)
		c, w := newUploadContractContext(t, "0xOwnerA", req)
		UploadFileProtected(c)
		require.Equal(t, http.StatusOK, w.Code, w.Body.String())
		var resp struct {
			Location string `json:"location"`
		}
		require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
		return resp.Location
	}
	first, second := minimalPDF(4096), bytes.Repeat([]byte("%PDF-1.4 second "), 300)
	loc1 := upload(first)
	loc2 := upload(second)
	require.NotEqual(t, loc1, loc2)
	require.True(t, strings.HasPrefix(loc1, fmt.Sprintf("businesses/%d/contracts/", owner.ID)), loc1)

	got1, err := s3.DownloadFileProtected(loc1)
	require.NoError(t, err)
	assert.Equal(t, first, got1, "second upload of the same filename must not overwrite the first")
	got2, err := s3.DownloadFileProtected(loc2)
	require.NoError(t, err)
	assert.Equal(t, second, got2)

	// Neither the protected key nor its /media form is publicly served.
	router := mediaRouter()
	assert.Equal(t, http.StatusNotFound, getMedia(t, router, "/media/"+loc1).Code)
	ok, err := s3.PublicStore().Exists(context.Background(), loc1)
	require.NoError(t, err)
	assert.False(t, ok, "protected uploads never land in the public store")
}

// Fiscal PDF on the local driver: the delivery dispatcher's protected upload
// (s3.UploadBytesProtected) round-trips through the guest PDF endpoint and is
// never reachable via /media.
func TestGuestFiscalReceiptPDFRoundTripOnLocalStorage(t *testing.T) {
	gin.SetMode(gin.TestMode)
	setupGuestFiscalReceiptTestDB(t)
	enableGuestFiscalRuntimeControl(t)
	useLocalStorage(t)
	bill := seedGuestFiscalBill(t, "tok-local-pdf-1")

	pdf := minimalPDF(8192)
	loc, err := s3.UploadBytesProtected(pdf, "receipt-1.pdf", fmt.Sprintf("fiscal-receipts/%d", bill.BusinessID), "application/pdf")
	require.NoError(t, err)
	seedAuthorizedGuestFiscalReceipt(t, bill, "00000043", "71234567890124",
		"https://www.arca.gob.ar/fe/qr/?p=local", loc)

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(http.MethodGet, "/guest/bill/tok-local-pdf-1/fiscal-receipt/pdf", nil)
	c.Params = gin.Params{{Key: "bill_token", Value: "tok-local-pdf-1"}}
	GetGuestFiscalReceiptPDF(c)
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	assert.Equal(t, "application/pdf", w.Header().Get("Content-Type"))
	assert.Equal(t, pdf, w.Body.Bytes())

	assert.Equal(t, http.StatusNotFound, getMedia(t, mediaRouter(), "/media/"+loc).Code)
}
