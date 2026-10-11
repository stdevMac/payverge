package handlers

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stdevmac/payverge/backend/internal/database"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestSplitShareConflictReturnsCode(t *testing.T) {
	gin.SetMode(gin.TestMode)
	handler := setupSplittingHandlerTestDB(t)
	bill := createSplittingHandlerBill(t, "B-err-share-conflict", database.BillStatusOpen, []database.BillItem{
		{ID: "main-1", Name: "Main", Price: 17.25, Quantity: 1, Subtotal: 17.25},
	})

	router := gin.New()
	router.POST("/guest/bill/:bill_token/split/holds", handler.CreateSplitHold)

	// First guest holds the entire remaining balance.
	firstBody, err := json.Marshal(map[string]any{
		"mode":            "custom",
		"cover_remaining": true,
	})
	require.NoError(t, err)

	firstResp := httptest.NewRecorder()
	firstReq := httptest.NewRequest(http.MethodPost, "/guest/bill/"+bill.PublicToken+"/split/holds", bytes.NewReader(firstBody))
	firstReq.Header.Set("Content-Type", "application/json")
	router.ServeHTTP(firstResp, firstReq)
	require.Equal(t, http.StatusOK, firstResp.Code, firstResp.Body.String())

	// Second guest tries to claim remaining — nothing left → share conflict.
	secondBody, err := json.Marshal(map[string]any{
		"mode":            "custom",
		"cover_remaining": true,
	})
	require.NoError(t, err)

	secondResp := httptest.NewRecorder()
	secondReq := httptest.NewRequest(http.MethodPost, "/guest/bill/"+bill.PublicToken+"/split/holds", bytes.NewReader(secondBody))
	secondReq.Header.Set("Content-Type", "application/json")
	router.ServeHTTP(secondResp, secondReq)

	require.Equal(t, http.StatusConflict, secondResp.Code, secondResp.Body.String())
	var payload map[string]any
	require.NoError(t, json.Unmarshal(secondResp.Body.Bytes(), &payload))
	assert.Equal(t, "split_share_conflict", payload["code"])
	assert.NotEmpty(t, payload["error"], "English error string must remain for logs")
}

func TestSplitNotOpenReturnsCode(t *testing.T) {
	gin.SetMode(gin.TestMode)
	handler := setupSplittingHandlerTestDB(t)
	bill := createSplittingHandlerBill(t, "B-err-not-open", database.BillStatusPaid, []database.BillItem{
		{ID: "main-1", Name: "Main", Price: 17.25, Quantity: 1, Subtotal: 17.25},
	})
	// Fully paid closed bill.
	require.NoError(t, database.GetDB().Model(bill).Updates(map[string]any{
		"paid_amount": bill.TotalAmount,
		"status":      database.BillStatusPaid,
	}).Error)

	router := gin.New()
	router.GET("/guest/bill/:bill_token/split/options", handler.GetBillSplitOptions)

	resp := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/guest/bill/"+bill.PublicToken+"/split/options", nil)
	router.ServeHTTP(resp, req)

	require.Equal(t, http.StatusBadRequest, resp.Code, resp.Body.String())
	var payload map[string]any
	require.NoError(t, json.Unmarshal(resp.Body.Bytes(), &payload))
	assert.Equal(t, "split_not_open", payload["code"])
	assert.NotEmpty(t, payload["error"], "English error string must remain for logs")
}
