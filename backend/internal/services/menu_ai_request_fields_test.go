package services

import (
	"context"
	"testing"

	"github.com/stdevmac/payverge/backend/internal/llm"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestExtractMenuFromImages_SetsFeatureAndMaxTokens(t *testing.T) {
	cap := &capturingMenuProvider{resp: &llm.Response{Text: `{"restaurantName":"X","categories":[]}`}}
	svc := NewMenuAIService(cap, llm.ModelConfig{Menu: "m", Image: "i"})

	_, err := svc.ExtractMenuFromImages(context.Background(), 81, []MenuExtractionInput{{
		Name:     "p.jpg",
		MIMEType: "image/jpeg",
		Data:     []byte{0xff, 0xd8, 0xff, 0xe0, 0, 1, 2, 3},
	}})
	require.NoError(t, err)
	assert.Equal(t, "extraction", cap.lastReq.Feature)
	assert.Equal(t, 32768, cap.lastReq.MaxTokens)
	require.NotNil(t, cap.lastReq.Temperature)
	assert.InDelta(t, 0.1, float64(*cap.lastReq.Temperature), 0.001)
	assert.Equal(t, uint(81), cap.lastReq.BusinessID)
}

func TestRegenerateItemImage_SetsImageFeature(t *testing.T) {
	cap := &capturingMenuProvider{resp: &llm.Response{}}
	svc := NewMenuAIService(cap, llm.ModelConfig{Menu: "m", Image: "i"})
	_, _ = svc.RegenerateItemImage(context.Background(), 82, "Pizza", "d", "", nil)
	assert.Equal(t, "image", cap.lastReq.Feature)
	assert.Equal(t, uint(82), cap.lastReq.BusinessID)
}

func TestEnhanceItemImage_SetsBusinessBudgetKey(t *testing.T) {
	cap := &capturingMenuProvider{resp: &llm.Response{}}
	svc := NewMenuAIService(cap, llm.ModelConfig{Menu: "m", Image: "i"})
	_, _ = svc.EnhanceItemImage(context.Background(), 83, []byte("image"), "image/png", "Pizza", "d", "1:1", nil)
	assert.Equal(t, "image", cap.lastReq.Feature)
	assert.Equal(t, uint(83), cap.lastReq.BusinessID)
}
