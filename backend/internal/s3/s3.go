// Package s3 is Payverge's object-storage layer. Despite the historical name
// it is driver-agnostic: Store (store.go) has a local-filesystem driver (the
// self-host default, STORAGE_DRIVER=local) and an S3-compatible driver
// (STORAGE_DRIVER=s3). The package-level helpers in this file and
// s3Protected.go are thin wrappers over the process-wide public and protected
// stores, so callers stay unchanged across drivers.
package s3

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"mime/multipart"
	"net"
	"net/http"
	"strings"
	"time"
)

// buildLocation returns the URL to persist/serve for an uploaded object. When a
// baseURL override is configured it composes "<baseURL>/<key>" (the key maps 1:1
// to the custom-domain path); otherwise it returns fallback unchanged.
func buildLocation(baseURL, key, fallback string) string {
	if baseURL != "" {
		return strings.TrimRight(baseURL, "/") + "/" + key
	}
	return fallback
}

type UploadOptions struct {
	ContentType        string
	ContentDisposition string
}

type UploadOption func(*UploadOptions)

func WithContentType(contentType string) UploadOption {
	return func(options *UploadOptions) {
		options.ContentType = contentType
	}
}

func WithContentDisposition(disposition string) UploadOption {
	return func(options *UploadOptions) {
		options.ContentDisposition = disposition
	}
}

func ApplyUploadOptions(options ...UploadOption) UploadOptions {
	var applied UploadOptions
	for _, option := range options {
		if option != nil {
			option(&applied)
		}
	}
	return applied
}

// boundedS3HTTPClient is the shared HTTP client for all S3 SDK operations.
// The SDK defaults to no overall operation deadline; the custom endpoint is a
// third-party S3-compatible store (iDrive e2). A stall mid-body would otherwise
// block the request goroutine forever (EXT-2). 30s is the overall ceiling, and
// the transport adds finer bounds so a stalled DNS/dial/TLS handshake or a slow
// first response byte (before any body streaming) can't outlast a sensible cap.
func boundedS3HTTPClient() *http.Client {
	return &http.Client{
		Timeout: 30 * time.Second,
		Transport: &http.Transport{
			DialContext:           (&net.Dialer{Timeout: 5 * time.Second, KeepAlive: 30 * time.Second}).DialContext,
			TLSHandshakeTimeout:   5 * time.Second,
			ResponseHeaderTimeout: 15 * time.Second,
			ExpectContinueTimeout: 1 * time.Second,
			IdleConnTimeout:       90 * time.Second,
			MaxIdleConns:          100,
			MaxIdleConnsPerHost:   10,
		},
	}
}

func requirePublicStore() (Store, error) {
	store := PublicStore()
	if store == nil {
		return nil, fmt.Errorf("public storage not initialized")
	}
	return store, nil
}

// UploadFile stores a multipart upload in the public store under
// "<folderPath>/<name>" and returns its public URL (PublicURL).
func UploadFile(file *multipart.FileHeader, name string, folderPath string, options ...UploadOption) (location string, err error) {
	store, err := requirePublicStore()
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
	return PublicURL(key), nil
}

// DeleteFile deletes an object from the public store.
func DeleteFile(key string) error {
	if key == "" {
		return fmt.Errorf("empty key provided")
	}
	store, err := requirePublicStore()
	if err != nil {
		return err
	}
	if err := store.Delete(context.Background(), key); err != nil {
		return fmt.Errorf("failed to delete file: %w", err)
	}
	return nil
}

// ObjectExists reports whether an object with the given key is present in the
// public store. A missing object maps to (false, nil); other failures
// (network, auth, uninitialized store) are returned as errors so callers can
// distinguish "missing" from "unknown".
func ObjectExists(key string) (bool, error) {
	store := PublicStore()
	if store == nil {
		return false, fmt.Errorf("public storage not initialized")
	}
	if key == "" {
		return false, fmt.Errorf("empty key provided")
	}
	ok, err := store.Exists(context.Background(), key)
	if err != nil {
		return false, fmt.Errorf("failed to head object: %w", err)
	}
	return ok, nil
}

// UploadBytes stores raw bytes in the public store and returns the public URL.
func UploadBytes(data []byte, name string, folderPath string, contentType string) (location string, err error) {
	return UploadBytesWithMetadata(data, name, folderPath, contentType, nil)
}

// UploadBytesWithMetadata stores raw bytes with object metadata (S3: rendered
// as x-amz-meta-<key>; local: kept in the sidecar). Used to mark AI-generated
// images with ai-generated=true (EU AI Act Art. 50 machine-readable provenance).
func UploadBytesWithMetadata(data []byte, name, folderPath, contentType string, metadata map[string]string) (location string, err error) {
	store, err := requirePublicStore()
	if err != nil {
		return "", err
	}
	key := JoinKey(folderPath, name)
	if err := store.Put(context.Background(), key, bytes.NewReader(data), PutOptions{
		ContentType: contentType,
		Metadata:    metadata,
	}); err != nil {
		return "", fmt.Errorf("failed to upload bytes: %w", err)
	}
	return PublicURL(key), nil
}

// ReadPublicObject reads a public object fully, capped at maxBytes.
func ReadPublicObject(ctx context.Context, key string, maxBytes int64) ([]byte, ObjectInfo, error) {
	store, err := requirePublicStore()
	if err != nil {
		return nil, ObjectInfo{}, err
	}
	obj, err := store.Get(ctx, key)
	if err != nil {
		return nil, ObjectInfo{}, err
	}
	defer func() { _ = obj.Body.Close() }()
	if maxBytes > 0 && obj.Size > maxBytes {
		return nil, obj.ObjectInfo, fmt.Errorf("object exceeds %d byte cap", maxBytes)
	}
	reader := io.Reader(obj.Body)
	if maxBytes > 0 {
		reader = io.LimitReader(obj.Body, maxBytes+1)
	}
	data, err := io.ReadAll(reader)
	if err != nil {
		return nil, obj.ObjectInfo, fmt.Errorf("failed to read object: %w", err)
	}
	if maxBytes > 0 && int64(len(data)) > maxBytes {
		return nil, obj.ObjectInfo, fmt.Errorf("object exceeds %d byte cap", maxBytes)
	}
	return data, obj.ObjectInfo, nil
}

// IsNotFound reports whether err means the object does not exist.
func IsNotFound(err error) bool { return errors.Is(err, ErrNotFound) }
