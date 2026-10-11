package services

import (
	"context"
	"testing"

	"github.com/stdevmac/payverge/backend/internal/llm"
)

func TestAIService_RestaurantOwnedRequestsCarryBusinessID(t *testing.T) {
	recorder := &recordingProvider{}
	service, err := NewAIService(recorder, llm.ModelConfig{Chat: "chat", Image: "image"})
	if err != nil {
		t.Fatal(err)
	}

	_, _ = service.GenerateMenuImage(context.Background(), MenuImagePrompt{
		BusinessID: 71,
		Name:       "Pizza",
	})
	if recorder.last.BusinessID != 71 || recorder.last.Feature != "image" {
		t.Fatalf("menu image budget key = (%d,%q), want (71,image)", recorder.last.BusinessID, recorder.last.Feature)
	}

	_, _ = service.GenerateMarketingCaption(context.Background(), CaptionRequest{
		BusinessID: 72,
		ItemName:   "Pizza",
	})
	if recorder.last.BusinessID != 72 || recorder.last.Feature != "marketing" {
		t.Fatalf("marketing budget key = (%d,%q), want (72,marketing)", recorder.last.BusinessID, recorder.last.Feature)
	}
}
