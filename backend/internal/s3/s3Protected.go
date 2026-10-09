package s3

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"mime/multipart"
	"strings"
)

func requireProtectedStore() (Store, error) {
	store := ProtectedStore()
	if store == nil {
		return nil, fmt.Errorf("protected storage not initialized")
	}
	return store, nil
}

// UploadFileProtected stores a multipart upload in the protected store and
// returns its location (the bare key, or "<S3_PROTECTED_BASE_URL>/<key>").
// Callers persist the location and read it back with DownloadFileProtected.
func UploadFileProtected(file *multipart.FileHeader, name string, folderPath string, options ...UploadOption) (location string, err error) {
	store, err := requireProtectedStore()
	if err != nil {
		return "", err
	}
	f, err := file.Open()
	if err != nil {
		return "", fmt.Errorf("failed to open file: %v", err)
	}
	defer func() {
		if closeErr := f.Close(); closeErr != nil && err == nil {
			location = ""
			err = fmt.Errorf("failed to close file: %v", closeErr)
		}
	}()
	key := JoinKey(folderPath, name)
	applied := ApplyUploadOptions(options...)
	if err := store.Put(context.Background(), key, f, PutOptions{
		ContentType:        applied.ContentType,
		ContentDisposition: applied.ContentDisposition,
	}); err != nil {
		return "", fmt.Errorf("failed to upload file: %w", err)
	}
	return protectedLocation(key), nil
}

// UploadBytesProtected stores raw in-memory bytes in the protected store and
// returns the stored location. Mirrors the public UploadBytes helper for
// callers (e.g. fiscal receipt delivery) that have bytes rather than a
// multipart file.
func UploadBytesProtected(data []byte, name, folderPath, contentType string) (location string, err error) {
	store, err := requireProtectedStore()
	if err != nil {
		return "", err
	}
	key := JoinKey(folderPath, name)
	if err := store.Put(context.Background(), key, bytes.NewReader(data), PutOptions{ContentType: contentType}); err != nil {
		return "", fmt.Errorf("failed to upload bytes to protected storage: %w", err)
	}
	return protectedLocation(key), nil
}

// extractKeyFromURL extracts the object key from a full S3 URL
func extractKeyFromURL(url string) (string, error) {
	if url == "" {
		return "", fmt.Errorf("empty URL provided")
	}

	// Remove the protocol (http:// or https://) if present
	url = strings.TrimPrefix(strings.TrimPrefix(url, "https://"), "http://")

	// Split URL by '/' to separate domain and path
	parts := strings.SplitN(url, "/", 2)
	if len(parts) < 2 {
		return "", fmt.Errorf("invalid URL format: no path found")
	}

	// Return everything after the domain as the key
	return parts[1], nil
}

// protectedKey resolves a persisted protected location to its key: a bare
// key, "<S3_PROTECTED_BASE_URL>/<key>", or a legacy full S3 URL (host
// stripped). Query strings (e.g. stale presign parameters) are dropped.
func protectedKey(keyOrURL string) (string, error) {
	keyOrURL = strings.TrimSpace(keyOrURL)
	if !strings.HasPrefix(keyOrURL, "http") {
		return strings.TrimPrefix(keyOrURL, "/"), nil
	}
	if base := snapshot().protectedBaseURL; base != "" && strings.HasPrefix(keyOrURL, base+"/") {
		return strings.TrimPrefix(keyOrURL, base+"/"), nil
	}
	key, err := extractKeyFromURL(keyOrURL)
	if err != nil {
		return "", err
	}
	if i := strings.IndexAny(key, "?#"); i >= 0 {
		key = key[:i]
	}
	return key, nil
}

// DownloadFileProtected reads a protected object by key or legacy URL.
func DownloadFileProtected(keyOrURL string) ([]byte, error) {
	if keyOrURL == "" {
		return nil, fmt.Errorf("empty input provided")
	}
	store, err := requireProtectedStore()
	if err != nil {
		return nil, err
	}
	key, err := protectedKey(keyOrURL)
	if err != nil {
		return nil, err
	}
	obj, err := store.Get(context.Background(), key)
	if err != nil {
		return nil, fmt.Errorf("failed to get protected object: %w", err)
	}
	defer func() { _ = obj.Body.Close() }()
	body, err := io.ReadAll(obj.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read object body: %v", err)
	}
	return body, nil
}

// DeleteFileProtected deletes a protected object by key or legacy URL. Used
// to clean extraction page objects after a job reaches a terminal state.
func DeleteFileProtected(keyOrURL string) error {
	if keyOrURL == "" {
		return fmt.Errorf("empty key provided")
	}
	store, err := requireProtectedStore()
	if err != nil {
		return err
	}
	key, err := protectedKey(keyOrURL)
	if err != nil {
		return err
	}
	if err := store.Delete(context.Background(), key); err != nil {
		return fmt.Errorf("failed to delete protected file: %w", err)
	}
	return nil
}
