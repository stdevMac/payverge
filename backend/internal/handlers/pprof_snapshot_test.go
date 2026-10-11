package handlers

import (
	"archive/zip"
	"bytes"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
)

func TestPprofSnapshotRequiresToken(t *testing.T) {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	RegisterPprofSnapshot(r, "secret-token")

	req := httptest.NewRequest("GET", "/internal/_pprof_snapshot", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code != http.StatusUnauthorized {
		t.Fatalf("missing token: want 401, got %d", w.Code)
	}

	req = httptest.NewRequest("GET", "/internal/_pprof_snapshot", nil)
	req.Header.Set("X-Pprof-Token", "secret-token")
	w = httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("with token: want 200, got %d", w.Code)
	}
	if got := w.Header().Get("Content-Type"); got != "application/zip" {
		t.Fatalf("content-type: want application/zip, got %s", got)
	}

	zr, err := zip.NewReader(bytes.NewReader(w.Body.Bytes()), int64(w.Body.Len()))
	if err != nil {
		t.Fatalf("zip: %v", err)
	}
	names := map[string]bool{}
	for _, f := range zr.File {
		names[f.Name] = true
		rc, err := f.Open()
		if err != nil {
			t.Fatalf("open %s: %v", f.Name, err)
		}
		_, _ = io.Copy(io.Discard, rc)
		_ = rc.Close()
	}
	for _, want := range []string{"goroutine.pprof", "heap.pprof"} {
		if !names[want] {
			t.Fatalf("missing %s in zip; have %v", want, names)
		}
	}
}

func TestPprofSnapshotDisabledWhenTokenEmpty(t *testing.T) {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	RegisterPprofSnapshot(r, "") // empty token = disabled

	req := httptest.NewRequest("GET", "/internal/_pprof_snapshot", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code != http.StatusNotFound {
		t.Fatalf("disabled: want 404, got %d", w.Code)
	}
}

func TestPprofSnapshotRejectsWrongToken(t *testing.T) {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	RegisterPprofSnapshot(r, "secret-token")

	req := httptest.NewRequest("GET", "/internal/_pprof_snapshot", nil)
	req.Header.Set("X-Pprof-Token", "wrong-token")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code != http.StatusUnauthorized {
		t.Fatalf("wrong token: want 401, got %d", w.Code)
	}
}

func TestPprofSnapshotEntriesNonEmpty(t *testing.T) {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	RegisterPprofSnapshot(r, "secret-token")

	req := httptest.NewRequest("GET", "/internal/_pprof_snapshot", nil)
	req.Header.Set("X-Pprof-Token", "secret-token")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("want 200, got %d", w.Code)
	}
	zr, err := zip.NewReader(bytes.NewReader(w.Body.Bytes()), int64(w.Body.Len()))
	if err != nil {
		t.Fatalf("zip: %v", err)
	}
	for _, f := range zr.File {
		if f.UncompressedSize64 == 0 {
			t.Fatalf("zip entry %s is empty (0 bytes)", f.Name)
		}
	}
}
