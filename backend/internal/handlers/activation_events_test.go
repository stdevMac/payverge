package handlers

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/stdevmac/payverge/backend/internal/activation"
	"github.com/stdevmac/payverge/backend/internal/database"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func TestActivationEventHandlerRejectsUnsafeAndDeduplicatesSafeClientEvent(t *testing.T) {
	gin.SetMode(gin.TestMode)
	gdb, err := gorm.Open(sqlite.Open("file:"+t.Name()+"?mode=memory&cache=shared"), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, gdb.AutoMigrate(&activation.State{}, &activation.OutboxEvent{}))
	previous := database.GetDBWrapper().GetGorm()
	database.SetTestDB(gdb)
	t.Cleanup(func() { database.SetTestDB(previous) })

	handler := NewActivationEventHandler(database.GetDBWrapper())
	serverTime := time.Date(2026, 8, 1, 11, 30, 0, 0, time.UTC)
	handler.now = func() time.Time { return serverTime }
	router := gin.New()
	router.POST("/event", handler.RecordClientEvent)

	unsafe := `{"name":"registration_started","schema_version":1,"funnel_id":"728a70ef-0e4f-49b7-9978-229b7bc2be59","idempotency_key":"client:728a70ef-0e4f-49b7-9978-229b7bc2be59:registration_started:global","email":"owner@example.test","dimensions":{"locale":"en","device_class":"desktop","elapsed_ms":0}}`
	response := performActivationRequest(router, unsafe)
	require.Equal(t, http.StatusBadRequest, response.Code)

	authoritative := `{"name":"first_paid_bill","schema_version":1,"funnel_id":"728a70ef-0e4f-49b7-9978-229b7bc2be59","idempotency_key":"client:728a70ef-0e4f-49b7-9978-229b7bc2be59:first_paid_bill:global","dimensions":{"locale":"en","device_class":"desktop","elapsed_ms":0}}`
	response = performActivationRequest(router, authoritative)
	require.Equal(t, http.StatusBadRequest, response.Code)

	safe := `{"name":"registration_started","schema_version":1,"funnel_id":"728a70ef-0e4f-49b7-9978-229b7bc2be59","idempotency_key":"client:728a70ef-0e4f-49b7-9978-229b7bc2be59:registration_started:global","dimensions":{"locale":"en","device_class":"desktop","acquisition_source":"organic","acquisition_campaign":"launch_2026","elapsed_ms":25}}`
	require.Equal(t, http.StatusCreated, performActivationRequest(router, safe).Code)
	require.Equal(t, http.StatusCreated, performActivationRequest(router, safe).Code)

	var rows []activation.OutboxEvent
	require.NoError(t, gdb.Find(&rows).Error)
	require.Len(t, rows, 1)
	require.Equal(t, serverTime, rows[0].OccurredAt)
}

func performActivationRequest(router *gin.Engine, body string) *httptest.ResponseRecorder {
	request := httptest.NewRequest(http.MethodPost, "/event", bytes.NewBufferString(body))
	request.Header.Set("Content-Type", "application/json")
	response := httptest.NewRecorder()
	router.ServeHTTP(response, request)
	return response
}
