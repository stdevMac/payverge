package services

import (
	"context"
	"strings"
	"testing"

	"github.com/stdevmac/payverge/backend/internal/llm"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// capturingMenuProvider records the request it received and returns a canned response.
type capturingMenuProvider struct {
	lastReq llm.GenerateRequest
	resp    *llm.Response
}

func (c *capturingMenuProvider) Generate(_ context.Context, req llm.GenerateRequest) (*llm.Response, error) {
	c.lastReq = req
	return c.resp, nil
}

func TestExtractMenuFromImages_TranslatesRequestAndParses(t *testing.T) {
	menuJSON := `{"restaurantName":"Test Bistro","currency":"$","categories":[{"name":"Mains","items":[{"name":"Burger","price":"$10"}]}]}`
	cap := &capturingMenuProvider{resp: &llm.Response{Text: menuJSON}}
	svc := NewMenuAIService(cap, llm.ModelConfig{Menu: "test-menu", Image: "test-image"})

	menu, err := svc.ExtractMenuFromImages(context.Background(), 41, []MenuExtractionInput{{
		Name:     "page.jpg",
		MIMEType: "image/jpeg",
		Data:     []byte("fake-jpeg-bytes"),
	}})
	require.NoError(t, err)
	require.NotNil(t, menu)

	// Request translation: menu model, one user message carrying one image, schema set.
	assert.Equal(t, "test-menu", cap.lastReq.Model)
	require.Len(t, cap.lastReq.Messages, 1)
	require.Len(t, cap.lastReq.Messages[0].Images, 1)
	assert.Equal(t, "image/jpeg", cap.lastReq.Messages[0].Images[0].MIMEType)
	require.NotNil(t, cap.lastReq.ResponseSchema)
	require.NotNil(t, cap.lastReq.Temperature)

	// Parsed result.
	assert.Equal(t, "Test Bistro", menu.RestaurantName)
	require.Len(t, menu.Categories, 1)
	assert.Equal(t, "Mains", menu.Categories[0].Name)
	require.Len(t, menu.Categories[0].Items, 1)
	assert.Equal(t, "Burger", menu.Categories[0].Items[0].Name)
}

func TestExtractMenuFromImages_PreservesVerifiedMIME(t *testing.T) {
	tests := []struct {
		name string
		mime string
		data []byte
	}{
		{"png", "image/png", []byte{0x89, 'P', 'N', 'G', '\r', '\n', 0x1a, '\n', 0, 1, 2, 3}},
		{"jpeg", "image/jpeg", []byte{0xff, 0xd8, 0xff, 0xe0, 0, 1, 2, 3, 4, 5}},
		{"webp", "image/webp", []byte("RIFF....WEBP....")},
	}

	menuJSON := `{"restaurantName":"X","currency":"$","categories":[]}`
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cap := &capturingMenuProvider{resp: &llm.Response{Text: menuJSON}}
			svc := NewMenuAIService(cap, llm.ModelConfig{Menu: "test-menu", Image: "test-image"})
			_, err := svc.ExtractMenuFromImages(context.Background(), 7, []MenuExtractionInput{{
				Name:     "page." + tt.name,
				MIMEType: tt.mime,
				Data:     tt.data,
			}})
			require.NoError(t, err)
			require.Len(t, cap.lastReq.Messages[0].Images, 1)
			assert.Equal(t, tt.mime, cap.lastReq.Messages[0].Images[0].MIMEType,
				"provider must receive the verified MIME, not a fixed jpeg default")
			assert.Equal(t, tt.data, cap.lastReq.Messages[0].Images[0].Data)
		})
	}
}

func TestRegenerateItemImage_TranslatesImageRequest(t *testing.T) {
	// Return no images so the method errors out before the S3 upload; we only
	// assert on the request the provider received.
	cap := &capturingMenuProvider{resp: &llm.Response{}}
	svc := NewMenuAIService(cap, llm.ModelConfig{Menu: "test-menu", Image: "test-image"})

	_, err := svc.RegenerateItemImage(context.Background(), 42, "Margherita Pizza", "Classic tomato and mozzarella", "", nil)
	require.Error(t, err) // no image returned — expected, S3 not reached
	// The error must be actionable: name the model so a misconfigured (text-only)
	// image model is diagnosable without a live API probe.
	assert.Contains(t, err.Error(), "test-image")

	assert.Equal(t, "test-image", cap.lastReq.Model)
	require.Len(t, cap.lastReq.Messages, 1)
	assert.Contains(t, cap.lastReq.Messages[0].Text, "Margherita Pizza")
	assert.Equal(t, []string{"image", "text"}, cap.lastReq.Modalities)
	require.NotNil(t, cap.lastReq.ImageConfig)
	assert.Equal(t, "1:1", cap.lastReq.ImageConfig.AspectRatio)
}

// The trust anchor of the whole cleanup feature is "that is actually my food".
// A model that substitutes a stock dish destroys it, so the constraint is
// pinned here rather than left to whoever next edits the prompt.
func TestBuildEnhanceImagePromptForbidsSubstitution(t *testing.T) {
	prompt := buildEnhanceImagePrompt(newWaiterMarker(), "Milanesa", "breaded beef", nil)
	for _, required := range []string{
		"Keep the same dish",
		"the same plating",
		"the same ingredients",
		"Do not invent a different dish",
		"do not add or remove food",
	} {
		if !strings.Contains(prompt, required) {
			t.Fatalf("enhance prompt lost the no-substitution constraint %q:\n%s", required, prompt)
		}
	}
}
