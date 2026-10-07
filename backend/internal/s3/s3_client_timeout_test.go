package s3

import (
	"testing"
	"time"
)

// TestBoundedS3HTTPClient asserts the shared HTTP client the S3 clients are
// built with carries a non-zero operation Timeout, so a hung S3-compatible
// endpoint surfaces as a timeout instead of an indefinite block (EXT-2).
func TestBoundedS3HTTPClient(t *testing.T) {
	c := boundedS3HTTPClient()
	if c.Timeout == 0 {
		t.Fatalf("expected bounded S3 http client to have a non-zero Timeout, got 0")
	}
	if c.Timeout > 60*time.Second {
		t.Fatalf("S3 client timeout too loose: %v", c.Timeout)
	}
}
