package handlers

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"time"

	"github.com/stdevmac/payverge/backend/internal/activation"
	"github.com/stdevmac/payverge/backend/internal/database"

	"github.com/gin-gonic/gin"
)

type ActivationEventHandler struct {
	recorder *activation.Recorder
	now      func() time.Time
}

func NewActivationEventHandler(db *database.DB) *ActivationEventHandler {
	return &ActivationEventHandler{recorder: activation.NewRecorder(db.GetGorm()), now: time.Now}
}

type clientActivationEventRequest struct {
	Name           activation.Name       `json:"name"`
	SchemaVersion  int                   `json:"schema_version"`
	FunnelID       string                `json:"funnel_id"`
	IdempotencyKey string                `json:"idempotency_key"`
	Dimensions     activation.Dimensions `json:"dimensions"`
}

func (h *ActivationEventHandler) RecordClientEvent(c *gin.Context) {
	decoder := json.NewDecoder(io.LimitReader(c.Request.Body, 16<<10))
	decoder.DisallowUnknownFields()
	var request clientActivationEventRequest
	if err := decoder.Decode(&request); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"code": "invalid_activation_event"})
		return
	}
	if err := ensureSingleJSONValue(decoder); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"code": "invalid_activation_event"})
		return
	}
	if request.IdempotencyKey == "" || len(request.IdempotencyKey) > 255 {
		c.JSON(http.StatusBadRequest, gin.H{"code": "invalid_activation_event"})
		return
	}
	event := activation.Event{
		Name: request.Name, SchemaVersion: request.SchemaVersion,
		FunnelID: request.FunnelID, IdempotencyKey: request.IdempotencyKey,
		Dimensions: request.Dimensions, OccurredAt: h.now().UTC(),
	}
	inserted, err := h.recorder.RecordClient(c.Request.Context(), event)
	if err != nil {
		status := http.StatusInternalServerError
		if errors.Is(err, activation.ErrInvalidEvent) {
			status = http.StatusBadRequest
		}
		c.JSON(status, gin.H{"code": "invalid_activation_event"})
		return
	}
	c.JSON(http.StatusCreated, gin.H{"accepted": true, "duplicate": !inserted})
}

func ensureSingleJSONValue(decoder *json.Decoder) error {
	var trailing any
	err := decoder.Decode(&trailing)
	if errors.Is(err, io.EOF) {
		return nil
	}
	if err == nil {
		return errors.New("multiple JSON values")
	}
	return err
}
