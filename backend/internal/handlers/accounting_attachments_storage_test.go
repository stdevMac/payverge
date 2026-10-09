package handlers

import (
	"bytes"
	"encoding/json"
	"fmt"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/stdevmac/payverge/backend/internal/database"
	"github.com/stdevmac/payverge/backend/internal/s3"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Accounting attachment on the local storage driver (no third-party account):
// upload -> protected store -> authorized download -> delete removes the
// object, and /media never serves it.
func TestLedgerAttachmentRoundTripOnLocalStorage(t *testing.T) {
	gin.SetMode(gin.TestMode)
	db := setupAccountingHandlerDB(t)
	require.NoError(t, db.Migrator().CreateTable(&database.LedgerEntryAttachment{}))
	business := createAccountingHandlerBusiness(t, "0xAttachOwner")

	public, err := s3.NewLocalStore(t.TempDir())
	require.NoError(t, err)
	protected, err := s3.NewLocalStore(t.TempDir())
	require.NoError(t, err)
	t.Cleanup(s3.SetStores(public, protected))

	entry := database.ManualLedgerEntry{
		BusinessID: business.ID, EntryType: database.AccountingEntryTypeExpense,
		Category: "supplies", Amount: 1500, Currency: "USD",
		OccurredAt: time.Now().UTC(), Description: "Invoice with receipt",
	}
	require.NoError(t, db.Create(&entry).Error)

	h := NewAccountingHandler(database.GetDBWrapper())
	router := gin.New()
	router.Use(func(c *gin.Context) {
		c.Set("token_type", "web3")
		c.Set("address", "0xAttachOwner")
		c.Next()
	})
	base := "/inside/businesses/:id/accounting/entries/:entryId/attachments"
	router.POST(base, h.UploadEntryAttachment)
	router.GET(base+"/:attachmentId/download", h.DownloadEntryAttachment)
	router.DELETE(base+"/:attachmentId", h.DeleteEntryAttachment)
	router.GET("/media/*key", s3.MediaHandler())

	pdf := append([]byte("%PDF-1.4\n"), bytes.Repeat([]byte("x"), 2048)...)
	var buf bytes.Buffer
	mw := multipart.NewWriter(&buf)
	part, err := mw.CreateFormFile("file", "Factura proveedor (marzo).pdf")
	require.NoError(t, err)
	_, err = part.Write(pdf)
	require.NoError(t, err)
	require.NoError(t, mw.Close())

	path := fmt.Sprintf("/inside/businesses/%d/accounting/entries/%d/attachments", business.ID, entry.ID)
	req := httptest.NewRequest(http.MethodPost, path, &buf)
	req.Header.Set("Content-Type", mw.FormDataContentType())
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)
	require.Equal(t, http.StatusCreated, w.Code, w.Body.String())

	var created struct {
		Data database.LedgerEntryAttachment `json:"data"`
	}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &created))
	var row database.LedgerEntryAttachment
	require.NoError(t, db.First(&row, created.Data.ID).Error)
	require.True(t, strings.HasPrefix(row.S3Key, fmt.Sprintf("ledger/%d/%d/", business.ID, entry.ID)), row.S3Key)
	require.NoError(t, s3.ValidateKey(row.S3Key), "sanitized names must be valid local keys")

	// Authorized download returns the exact bytes.
	w = httptest.NewRecorder()
	router.ServeHTTP(w, httptest.NewRequest(http.MethodGet, fmt.Sprintf("%s/%d/download", path, row.ID), nil))
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	assert.Equal(t, pdf, w.Body.Bytes())
	assert.Equal(t, "application/pdf", w.Header().Get("Content-Type"))

	// The protected object is not in the public store and /media refuses it.
	w = httptest.NewRecorder()
	router.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/media/"+row.S3Key, nil))
	assert.Equal(t, http.StatusNotFound, w.Code)

	// Delete removes the row and the stored object.
	w = httptest.NewRecorder()
	router.ServeHTTP(w, httptest.NewRequest(http.MethodDelete, fmt.Sprintf("%s/%d", path, row.ID), nil))
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	_, err = s3.DownloadFileProtected(row.S3Key)
	require.Error(t, err)
	assert.True(t, s3.IsNotFound(err), "object must be gone after delete: %v", err)
}
