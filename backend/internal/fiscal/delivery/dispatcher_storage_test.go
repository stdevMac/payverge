package delivery

import (
	"context"
	"strings"
	"testing"

	"github.com/stdevmac/payverge/backend/internal/s3"
)

// The production dispatcher stores fiscal artifacts in the protected store; on
// the local driver (zero-config self-hosting) they round-trip by the returned
// location and never land in the public store served at /media.
func TestDispatcherUploadProtectedRoundTripOnLocalStorage(t *testing.T) {
	public, err := s3.NewLocalStore(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	protected, err := s3.NewLocalStore(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(s3.SetStores(public, protected))

	pdf := []byte("%PDF-1.4\nfiscal receipt body")
	loc, err := New(nil).UploadProtected(pdf, "receipt-7.pdf", "fiscal-receipts/3", "application/pdf")
	if err != nil {
		t.Fatalf("UploadProtected: %v", err)
	}
	if loc != "fiscal-receipts/3/receipt-7.pdf" {
		t.Fatalf("location = %q, want the bare protected key", loc)
	}
	got, err := s3.DownloadFileProtected(loc)
	if err != nil || string(got) != string(pdf) {
		t.Fatalf("DownloadFileProtected = %q, %v", got, err)
	}
	if ok, _ := public.Exists(context.Background(), loc); ok {
		t.Fatal("fiscal PDF must never be written to the public store")
	}
	if !s3.IsProtectedMediaKey(loc) || !strings.HasPrefix(loc, "fiscal-receipts/") {
		t.Fatalf("%q must be in a /media-denied key space", loc)
	}
}
