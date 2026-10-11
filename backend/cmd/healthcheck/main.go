package main

import (
	"net/http"
	"os"
	"time"
)

func main() {
	// Probe the minimal public liveness endpoint so Docker HEALTHCHECK does
	// not depend on the detailed /health payload (#286).
	client := &http.Client{Timeout: 3 * time.Second}
	resp, err := client.Get("http://127.0.0.1:8080/api/v1/health/live")
	if err != nil {
		os.Exit(1)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode >= 200 && resp.StatusCode < 300 {
		return
	}
	os.Exit(1)
}
