package main

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"
	"os"
	"path"
	"strings"
	"time"
)

type syntheticEvent struct {
	EventID     string            `json:"event_id"`
	Timestamp   string            `json:"timestamp"`
	Platform    string            `json:"platform"`
	Level       string            `json:"level"`
	Logger      string            `json:"logger"`
	Message     string            `json:"message"`
	Environment string            `json:"environment"`
	Tags        map[string]string `json:"tags"`
}

type sentryStoreResponse struct {
	ID string `json:"id"`
}

func sentryStoreEndpoint(rawDSN string) (string, string, error) {
	parsed, err := url.Parse(strings.TrimSpace(rawDSN))
	if err != nil {
		return "", "", fmt.Errorf("parse Sentry DSN: %w", err)
	}
	if parsed.Scheme != "https" && parsed.Scheme != "http" {
		return "", "", errors.New("Sentry DSN must use http or https")
	}
	if parsed.Host == "" || parsed.User == nil || parsed.User.Username() == "" {
		return "", "", errors.New("Sentry DSN must include a host and public key")
	}

	segments := strings.Split(strings.Trim(parsed.Path, "/"), "/")
	if len(segments) == 0 || segments[len(segments)-1] == "" {
		return "", "", errors.New("Sentry DSN must include a project ID")
	}
	projectID := segments[len(segments)-1]
	for _, char := range projectID {
		if char < '0' || char > '9' {
			return "", "", errors.New("Sentry DSN project ID must be numeric")
		}
	}

	endpointSegments := append(segments[:len(segments)-1], "api", projectID, "store")
	endpointURL := url.URL{
		Scheme: parsed.Scheme,
		Host:   parsed.Host,
		Path:   path.Join(append([]string{"/"}, endpointSegments...)...) + "/",
	}
	return endpointURL.String(), parsed.User.Username(), nil
}

func sendSentrySynthetic(ctx context.Context, client *http.Client, rawDSN string, now time.Time, random io.Reader) (string, error) {
	endpoint, publicKey, err := sentryStoreEndpoint(rawDSN)
	if err != nil {
		return "", err
	}

	eventIDBytes := make([]byte, 16)
	if _, err := io.ReadFull(random, eventIDBytes); err != nil {
		return "", fmt.Errorf("generate event ID: %w", err)
	}
	eventID := hex.EncodeToString(eventIDBytes)
	event := syntheticEvent{
		EventID:     eventID,
		Timestamp:   now.UTC().Format(time.RFC3339Nano),
		Platform:    "other",
		Level:       "info",
		Logger:      "payverge.synthetic",
		Message:     "Payverge production synthetic event",
		Environment: "production",
		Tags: map[string]string{
			"synthetic": "true",
			"source":    "github-actions",
			"service":   "production-synthetics",
		},
	}
	payload, err := json.Marshal(event)
	if err != nil {
		return "", fmt.Errorf("encode Sentry event: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(payload))
	if err != nil {
		return "", fmt.Errorf("create Sentry request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("User-Agent", "payverge-production-synthetic/1.0")
	req.Header.Set("X-Sentry-Auth", fmt.Sprintf("Sentry sentry_version=7, sentry_client=payverge-production-synthetic/1.0, sentry_key=%s", publicKey))

	resp, err := client.Do(req)
	if err != nil {
		return "", fmt.Errorf("send Sentry event: %w", err)
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(io.LimitReader(resp.Body, 4096))
	if err != nil {
		return "", fmt.Errorf("read Sentry response: %w", err)
	}
	if resp.StatusCode < http.StatusOK || resp.StatusCode >= http.StatusMultipleChoices {
		return "", fmt.Errorf("Sentry returned status %d", resp.StatusCode)
	}

	var accepted sentryStoreResponse
	if err := json.Unmarshal(body, &accepted); err != nil {
		return "", fmt.Errorf("decode Sentry response: %w", err)
	}
	if accepted.ID == "" {
		return "", errors.New("Sentry response did not include an event ID")
	}
	return accepted.ID, nil
}

func main() {
	dsn := strings.TrimSpace(os.Getenv("SENTRY_DSN"))
	if dsn == "" {
		log.Fatal("SENTRY_DSN is required")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	eventID, err := sendSentrySynthetic(ctx, &http.Client{Timeout: 20 * time.Second}, dsn, time.Now(), rand.Reader)
	if err != nil {
		log.Fatalf("Sentry synthetic failed: %v", err)
	}
	fmt.Printf("Sentry accepted synthetic event %s\n", eventID)
}
