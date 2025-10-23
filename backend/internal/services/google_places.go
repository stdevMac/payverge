package services

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strings"
)

// GooglePlacesService handles Google Places API interactions
type GooglePlacesService struct {
	APIKey string
}

// PlaceSearchResult represents a single place from Google Places API
type PlaceSearchResult struct {
	PlaceID     string `json:"place_id"`
	Name        string `json:"name"`
	Address     string `json:"formatted_address"`
	Rating      float64 `json:"rating"`
	UserRatings int    `json:"user_ratings_total"`
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
	Results []PlaceSearchResult `json:"results"`
	Status  string             `json:"status"`
	ErrorMessage string         `json:"error_message,omitempty"`
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
		return nil, fmt.Errorf("Google API key not configured")
	}

	// Build the API URL
	baseURL := "https://maps.googleapis.com/maps/api/place/textsearch/json"
	params := url.Values{}
	params.Add("query", query)
	params.Add("key", g.APIKey)
	params.Add("type", "establishment") // Focus on business establishments
	
	fullURL := fmt.Sprintf("%s?%s", baseURL, params.Encode())

	// Make the HTTP request
	resp, err := http.Get(fullURL)
	if err != nil {
		return nil, fmt.Errorf("failed to make request to Google Places API: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("Google Places API returned status %d", resp.StatusCode)
	}

	// Parse the response
	var searchResponse PlaceSearchResponse
	if err := json.NewDecoder(resp.Body).Decode(&searchResponse); err != nil {
		return nil, fmt.Errorf("failed to decode Google Places API response: %v", err)
	}

	if searchResponse.Status != "OK" {
		if searchResponse.ErrorMessage != "" {
			return nil, fmt.Errorf("Google Places API error: %s", searchResponse.ErrorMessage)
		}
		return nil, fmt.Errorf("Google Places API returned status: %s", searchResponse.Status)
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

// ValidatePlaceID checks if a Place ID is valid by making a Place Details request
func (g *GooglePlacesService) ValidatePlaceID(placeID string) (bool, error) {
	if g.APIKey == "" || placeID == "" {
		return false, fmt.Errorf("API key or Place ID is empty")
	}

	baseURL := "https://maps.googleapis.com/maps/api/place/details/json"
	params := url.Values{}
	params.Add("place_id", placeID)
	params.Add("key", g.APIKey)
	params.Add("fields", "place_id,name") // Minimal fields for validation
	
	fullURL := fmt.Sprintf("%s?%s", baseURL, params.Encode())

	resp, err := http.Get(fullURL)
	if err != nil {
		return false, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return false, fmt.Errorf("API returned status %d", resp.StatusCode)
	}

	var result struct {
		Status string `json:"status"`
	}
	
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return false, err
	}

	return result.Status == "OK", nil
}

// FormatBusinessQuery formats a business name and address for optimal search results
func FormatBusinessQuery(businessName, address string) string {
	query := strings.TrimSpace(businessName)
	
	if address != "" {
		// Add address to improve search accuracy
		query = fmt.Sprintf("%s, %s", query, strings.TrimSpace(address))
	}
	
	return query
}
