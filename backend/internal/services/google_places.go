package services

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"time"

	"github.com/stdevmac/payverge/backend/internal/httpclientx"
)

// placesHTTPClient bounds Google Places API calls so a hung endpoint cannot
// block the caller indefinitely (http.Get uses the timeout-less DefaultClient).
var placesHTTPClient = &http.Client{Timeout: 10 * time.Second}

// GooglePlacesService handles Google Places API interactions
type GooglePlacesService struct {
	APIKey string
}

// PlaceSearchResult represents a single place from Google Places API
type PlaceSearchResult struct {
	PlaceID     string   `json:"place_id"`
	Name        string   `json:"name"`
	Address     string   `json:"formatted_address"`
	Rating      float64  `json:"rating"`
	UserRatings int      `json:"user_ratings_total"`
	Types       []string `json:"types"`
	Geometry    struct {
		Location struct {
			Lat float64 `json:"lat"`
			Lng float64 `json:"lng"`
		} `json:"location"`
	} `json:"geometry"`
}

// PlaceSearchResponse represents the response from Google Places Text Search API
type PlaceSearchResponse struct {
	Results      []PlaceSearchResult `json:"results"`
	Status       string              `json:"status"`
	ErrorMessage string              `json:"error_message,omitempty"`
}

// NewGooglePlacesService creates a new Google Places service
func NewGooglePlacesService(apiKey string) *GooglePlacesService {
	return &GooglePlacesService{
		APIKey: apiKey,
	}
}

// SearchBusinesses searches for businesses using Google Places Text Search API
func (g *GooglePlacesService) SearchBusinesses(query string) ([]PlaceSearchResult, error) {
	if g.APIKey == "" {
		return nil, fmt.Errorf("google API key not configured")
	}

	// Build the API URL
	baseURL := "https://maps.googleapis.com/maps/api/place/textsearch/json"
	params := url.Values{}
	params.Add("query", query)
	params.Add("key", g.APIKey)
	params.Add("type", "establishment") // Focus on business establishments

	fullURL := fmt.Sprintf("%s?%s", baseURL, params.Encode())

	// Make the HTTP request
	resp, err := placesHTTPClient.Get(fullURL)
	if err != nil {
		return nil, fmt.Errorf("failed to make request to Google Places API: %w", httpclientx.RedactURLError(err))
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("google Places API returned status %d", resp.StatusCode)
	}

	// Parse the response
	var searchResponse PlaceSearchResponse
	if err := json.NewDecoder(resp.Body).Decode(&searchResponse); err != nil {
		return nil, fmt.Errorf("failed to decode google Places API response: %v", err)
	}

	if searchResponse.Status != "OK" {
		if searchResponse.ErrorMessage != "" {
			return nil, fmt.Errorf("google Places API error: %s", searchResponse.ErrorMessage)
		}
		return nil, fmt.Errorf("google Places API returned status: %s", searchResponse.Status)
	}

	return searchResponse.Results, nil
}

// GenerateReviewLink generates a Google review link from a Place ID
func (g *GooglePlacesService) GenerateReviewLink(placeID string) string {
	if placeID == "" {
		return ""
	}
	return fmt.Sprintf("https://search.google.com/local/writereview?placeid=%s", placeID)
}

// GenerateBusinessURL generates a Google Maps business URL from a Place ID
func (g *GooglePlacesService) GenerateBusinessURL(placeID string) string {
	if placeID == "" {
		return ""
	}
	return fmt.Sprintf("https://maps.google.com/maps/place/?q=place_id:%s", placeID)
}

// PlaceReview represents a single review from Google Places API
type PlaceReview struct {
	AuthorName       string `json:"author_name"`
	AuthorURL        string `json:"author_url,omitempty"`
	ProfilePhotoURL  string `json:"profile_photo_url,omitempty"`
	Rating           int    `json:"rating"`
	RelativeTimeDesc string `json:"relative_time_description"`
	Text             string `json:"text"`
	Time             int64  `json:"time"`
}

// PlaceDetails represents the response from Google Places Details API
type PlaceDetails struct {
	PlaceID          string        `json:"place_id"`
	Name             string        `json:"name"`
	Rating           float64       `json:"rating"`
	UserRatingsTotal int           `json:"user_ratings_total"`
	Reviews          []PlaceReview `json:"reviews"`
	Status           string        `json:"status"`
	ErrorMessage     string        `json:"error_message,omitempty"`
}

// PlaceDetailsResponse represents the full response from Google Places Details API
type PlaceDetailsResponse struct {
	Result       PlaceDetails `json:"result"`
	Status       string       `json:"status"`
	ErrorMessage string       `json:"error_message,omitempty"`
}

// GetPlaceReviews fetches reviews for a specific place ID
func (g *GooglePlacesService) GetPlaceReviews(placeID string, language ...string) ([]PlaceReview, error) {
	if g.APIKey == "" || placeID == "" {
		return nil, fmt.Errorf("api key or Place ID is empty")
	}

	baseURL := "https://maps.googleapis.com/maps/api/place/details/json"
	params := url.Values{}
	params.Add("place_id", placeID)
	params.Add("key", g.APIKey)
	params.Add("fields", "reviews,rating,user_ratings_total") // Request reviews and rating info

	// Add language parameter if provided
	if len(language) > 0 && language[0] != "" {
		params.Add("language", language[0])
	}

	fullURL := fmt.Sprintf("%s?%s", baseURL, params.Encode())

	resp, err := placesHTTPClient.Get(fullURL)
	if err != nil {
		return nil, fmt.Errorf("failed to make request to google Places API: %w", httpclientx.RedactURLError(err))
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("google Places API returned status %d", resp.StatusCode)
	}

	var detailsResponse PlaceDetailsResponse
	if err := json.NewDecoder(resp.Body).Decode(&detailsResponse); err != nil {
		return nil, fmt.Errorf("failed to decode google Places API response: %v", err)
	}

	if detailsResponse.Status != "OK" {
		if detailsResponse.ErrorMessage != "" {
			return nil, fmt.Errorf("google Places API error: %s", detailsResponse.ErrorMessage)
		}
		return nil, fmt.Errorf("google Places API returned status: %s", detailsResponse.Status)
	}

	return detailsResponse.Result.Reviews, nil
}

// GetPlaceDetails fetches detailed information including reviews for a place
func (g *GooglePlacesService) GetPlaceDetails(placeID string) (*PlaceDetails, error) {
	if g.APIKey == "" || placeID == "" {
		return nil, fmt.Errorf("API key or Place ID is empty")
	}

	baseURL := "https://maps.googleapis.com/maps/api/place/details/json"
	params := url.Values{}
	params.Add("place_id", placeID)
	params.Add("key", g.APIKey)
	params.Add("fields", "place_id,name,rating,user_ratings_total,reviews")

	fullURL := fmt.Sprintf("%s?%s", baseURL, params.Encode())

	resp, err := placesHTTPClient.Get(fullURL)
	if err != nil {
		return nil, fmt.Errorf("failed to make request to google Places API: %w", httpclientx.RedactURLError(err))
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("google Places API returned status %d", resp.StatusCode)
	}

	var detailsResponse PlaceDetailsResponse
	if err := json.NewDecoder(resp.Body).Decode(&detailsResponse); err != nil {
		return nil, fmt.Errorf("failed to decode google Places API response: %v", err)
	}

	if detailsResponse.Status != "OK" {
		if detailsResponse.ErrorMessage != "" {
			return nil, fmt.Errorf("google Places API error: %s", detailsResponse.ErrorMessage)
		}
		return nil, fmt.Errorf("google Places API returned status: %s", detailsResponse.Status)
	}

	return &detailsResponse.Result, nil
}

// ValidatePlaceID checks if a Place ID is valid by making a Place Details request
func (g *GooglePlacesService) ValidatePlaceID(placeID string) (bool, error) {
	if g.APIKey == "" || placeID == "" {
		return false, fmt.Errorf("api key or Place ID is empty")
	}

	baseURL := "https://maps.googleapis.com/maps/api/place/details/json"
	params := url.Values{}
	params.Add("place_id", placeID)
	params.Add("key", g.APIKey)
	params.Add("fields", "place_id,name") // Minimal fields for validation

	fullURL := fmt.Sprintf("%s?%s", baseURL, params.Encode())

	resp, err := placesHTTPClient.Get(fullURL)
	if err != nil {
		return false, httpclientx.RedactURLError(err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		return false, fmt.Errorf("api returned status %d", resp.StatusCode)
	}

	var result struct {
		Status string `json:"status"`
	}

	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return false, err
	}

	return result.Status == "OK", nil
}
