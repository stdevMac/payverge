package s3

import "testing"

// TestNormalizeRegionForEndpoint locks the region-normalization rule that keeps
// Cloudflare R2 from rejecting an AWS region name (e.g. us-west-2) with
// InvalidRegionName. R2 only accepts auto/wnam/enam/weur/eeur/apac/oc; when the
// write endpoint is an R2 host and the configured region is not one of those,
// the region is coerced to "auto". Every other endpoint (real AWS, MinIO, etc.)
// passes the region through unchanged.
func TestNormalizeRegionForEndpoint(t *testing.T) {
	cases := []struct {
		name     string
		region   string
		endpoint string
		want     string
	}{
		{
			name:     "R2 endpoint with AWS region normalizes to auto",
			region:   "us-west-2",
			endpoint: "https://abc123def.r2.cloudflarestorage.com",
			want:     "auto",
		},
		{
			name:     "R2 endpoint with accepted region wnam is preserved",
			region:   "wnam",
			endpoint: "https://abc123def.r2.cloudflarestorage.com",
			want:     "wnam",
		},
		{
			name:     "R2 endpoint with auto is preserved",
			region:   "auto",
			endpoint: "https://abc123def.r2.cloudflarestorage.com",
			want:     "auto",
		},
		{
			name:     "AWS (empty endpoint) keeps its region",
			region:   "us-west-2",
			endpoint: "",
			want:     "us-west-2",
		},
		{
			name:     "non-R2 custom endpoint (minio) keeps its region",
			region:   "us-east-1",
			endpoint: "https://minio.internal.example.com",
			want:     "us-east-1",
		},
		{
			name:     "R2 endpoint without scheme still normalizes",
			region:   "us-west-2",
			endpoint: "abc123def.r2.cloudflarestorage.com",
			want:     "auto",
		},
		{
			name:     "R2 endpoint http scheme normalizes",
			region:   "eu-central-1",
			endpoint: "http://acct.r2.cloudflarestorage.com",
			want:     "auto",
		},
		{
			name:     "R2 endpoint with accepted region enam is preserved",
			region:   "enam",
			endpoint: "https://acct.r2.cloudflarestorage.com/bucket",
			want:     "enam",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := normalizeRegionForEndpoint(tc.region, tc.endpoint)
			if got != tc.want {
				t.Fatalf("normalizeRegionForEndpoint(%q, %q) = %q, want %q",
					tc.region, tc.endpoint, got, tc.want)
			}
		})
	}
}
