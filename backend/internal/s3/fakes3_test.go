package s3

import (
	"crypto/md5"
	"encoding/hex"
	"fmt"
	"html"
	"io"
	"net/http"
	"net/http/httptest"
	"sort"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"
)

// fakeS3 is a minimal path-style S3 endpoint (PUT/GET/HEAD/DELETE object plus
// one-page ListObjectsV2) for driver conformance and the startup probes.
// Requests carrying SigV4 auth (Authorization header or X-Amz-Signature) are
// "authenticated"; anonymous reads (GET/HEAD object) succeed only when
// publicRead is set, like a bucket with a public policy or an R2 public domain.
type fakeS3 struct {
	t          *testing.T
	srv        *httptest.Server
	mu         sync.Mutex
	objects    map[string]fakeS3Object // "/bucket/key"
	publicRead bool
	// publicBuckets makes only the named buckets anonymously readable.
	publicBuckets map[string]bool
	anonGets      int
	requests      []string
	// denyList answers ListObjectsV2 with AccessDenied (no s3:ListBucket).
	denyList bool
}

type fakeS3Object struct {
	body               []byte
	contentType        string
	contentDisposition string
	meta               map[string]string
	modTime            time.Time
}

func newFakeS3(t *testing.T) *fakeS3 {
	t.Helper()
	f := &fakeS3{t: t, objects: map[string]fakeS3Object{}}
	f.srv = httptest.NewServer(http.HandlerFunc(f.serve))
	t.Cleanup(f.srv.Close)
	return f
}

func (f *fakeS3) URL() string { return f.srv.URL }

func authenticated(r *http.Request) bool {
	return strings.HasPrefix(r.Header.Get("Authorization"), "AWS4-HMAC-SHA256") ||
		r.URL.Query().Get("X-Amz-Signature") != ""
}

func (f *fakeS3) serve(w http.ResponseWriter, r *http.Request) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.requests = append(f.requests, r.Method+" "+r.URL.Path)
	path := r.URL.Path
	parts := strings.SplitN(strings.TrimPrefix(path, "/"), "/", 2)
	if len(parts) < 2 || parts[1] == "" {
		if r.Method == http.MethodGet && r.URL.Query().Get("list-type") == "2" && authenticated(r) {
			f.list(w, r, parts[0])
			return
		}
		writeS3Error(w, http.StatusBadRequest, "InvalidRequest")
		return
	}
	if !authenticated(r) {
		read := r.Method == http.MethodGet || r.Method == http.MethodHead
		if read {
			f.anonGets++
		}
		if !read || !(f.publicRead || f.publicBuckets[parts[0]]) {
			writeS3Error(w, http.StatusForbidden, "AccessDenied")
			return
		}
	}
	switch r.Method {
	case http.MethodPut:
		body, err := io.ReadAll(r.Body)
		if err != nil {
			writeS3Error(w, http.StatusBadRequest, "IncompleteBody")
			return
		}
		if strings.Contains(r.Header.Get("Content-Encoding"), "aws-chunked") {
			f.t.Errorf("fake S3 got aws-chunked upload; checksum trailers should be disabled for custom endpoints")
		}
		meta := map[string]string{}
		for name, values := range r.Header {
			if lower := strings.ToLower(name); strings.HasPrefix(lower, "x-amz-meta-") && len(values) > 0 {
				meta[strings.TrimPrefix(lower, "x-amz-meta-")] = values[0]
			}
		}
		f.objects[path] = fakeS3Object{
			body:               body,
			contentType:        r.Header.Get("Content-Type"),
			contentDisposition: r.Header.Get("Content-Disposition"),
			meta:               meta,
			modTime:            time.Now().UTC().Truncate(time.Second),
		}
		w.Header().Set("ETag", etagOf(body))
		w.WriteHeader(http.StatusOK)
	case http.MethodGet, http.MethodHead:
		obj, ok := f.objects[path]
		if !ok {
			if r.Method == http.MethodHead {
				w.WriteHeader(http.StatusNotFound)
				return
			}
			writeS3Error(w, http.StatusNotFound, "NoSuchKey")
			return
		}
		h := w.Header()
		if obj.contentType != "" {
			h.Set("Content-Type", obj.contentType)
		}
		if obj.contentDisposition != "" {
			h.Set("Content-Disposition", obj.contentDisposition)
		}
		for k, v := range obj.meta {
			h.Set("x-amz-meta-"+k, v)
		}
		h.Set("ETag", etagOf(obj.body))
		h.Set("Last-Modified", obj.modTime.Format(http.TimeFormat))
		h.Set("Content-Length", fmt.Sprint(len(obj.body)))
		w.WriteHeader(http.StatusOK)
		if r.Method == http.MethodGet {
			_, _ = w.Write(obj.body)
		}
	case http.MethodDelete:
		delete(f.objects, path)
		w.WriteHeader(http.StatusNoContent)
	default:
		writeS3Error(w, http.StatusMethodNotAllowed, "MethodNotAllowed")
	}
}

// list answers one ListObjectsV2 page (prefix and max-keys honored; never
// truncated beyond max-keys, which is all the probes need).
func (f *fakeS3) list(w http.ResponseWriter, r *http.Request, bucket string) {
	if f.denyList {
		writeS3Error(w, http.StatusForbidden, "AccessDenied")
		return
	}
	q := r.URL.Query()
	prefix := q.Get("prefix")
	maxKeys := 1000
	if v, err := strconv.Atoi(q.Get("max-keys")); err == nil && v >= 0 {
		maxKeys = v
	}
	var keys []string
	for p := range f.objects {
		if key, ok := strings.CutPrefix(p, "/"+bucket+"/"); ok && strings.HasPrefix(key, prefix) {
			keys = append(keys, key)
		}
	}
	sort.Strings(keys)
	truncated := len(keys) > maxKeys
	if truncated {
		keys = keys[:maxKeys]
	}
	var b strings.Builder
	fmt.Fprintf(&b, `<?xml version="1.0" encoding="UTF-8"?><ListBucketResult xmlns="http://s3.amazonaws.com/doc/2006-03-01/"><Name>%s</Name><Prefix>%s</Prefix><KeyCount>%d</KeyCount><MaxKeys>%d</MaxKeys><IsTruncated>%t</IsTruncated>`,
		html.EscapeString(bucket), html.EscapeString(prefix), len(keys), maxKeys, truncated)
	for _, key := range keys {
		obj := f.objects["/"+bucket+"/"+key]
		fmt.Fprintf(&b, `<Contents><Key>%s</Key><Size>%d</Size><ETag>%s</ETag><LastModified>%s</LastModified></Contents>`,
			html.EscapeString(key), len(obj.body), html.EscapeString(etagOf(obj.body)), obj.modTime.Format(time.RFC3339))
	}
	b.WriteString(`</ListBucketResult>`)
	w.Header().Set("Content-Type", "application/xml")
	w.WriteHeader(http.StatusOK)
	_, _ = io.WriteString(w, b.String())
}

// seed stores an object directly (as if uploaded before this process ran).
func (f *fakeS3) seed(path string, body []byte) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.objects[path] = fakeS3Object{body: body, contentType: "image/png", modTime: time.Now().UTC().Truncate(time.Second)}
}

// writes lists the PUT and DELETE requests the fake received.
func (f *fakeS3) writes() []string {
	f.mu.Lock()
	defer f.mu.Unlock()
	var out []string
	for _, req := range f.requests {
		if strings.HasPrefix(req, http.MethodPut+" ") || strings.HasPrefix(req, http.MethodDelete+" ") {
			out = append(out, req)
		}
	}
	return out
}

func (f *fakeS3) has(path string) bool {
	f.mu.Lock()
	defer f.mu.Unlock()
	_, ok := f.objects[path]
	return ok
}

func (f *fakeS3) count() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return len(f.objects)
}

func etagOf(body []byte) string {
	sum := md5.Sum(body)
	return `"` + hex.EncodeToString(sum[:]) + `"`
}

func writeS3Error(w http.ResponseWriter, status int, code string) {
	w.Header().Set("Content-Type", "application/xml")
	w.WriteHeader(status)
	_, _ = fmt.Fprintf(w, `<?xml version="1.0" encoding="UTF-8"?><Error><Code>%s</Code><Message>%s</Message></Error>`, code, code)
}

// newFakeS3Store returns an S3Store against the fake in path-style mode.
func newFakeS3Store(t *testing.T, f *fakeS3, bucket, prefix string, reserved ...string) *S3Store {
	t.Helper()
	client, err := newS3Client(bucket, "test-access-key", "test-secret-key", "us-east-1", f.URL(), true)
	if err != nil {
		t.Fatalf("newS3Client: %v", err)
	}
	return newS3Store(client, bucket, prefix, reserved...)
}
