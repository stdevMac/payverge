package fiscal

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

// Task 5: authorization paths must not invoke email/upload/print INLINE after
// terminal authorization. Delivery is via durable tasks + DeliveryWorker only.
// This is a source gate over the fiscal package (excluding tests and the
// delivery worker / legacy deliverReceipt implementation).
func TestNoInlineDeliveryAfterAuthorization(t *testing.T) {
	_, thisFile, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("runtime.Caller failed")
	}
	dir := filepath.Dir(thisFile)

	// Files that own the authorization → delivery handoff.
	// They must enqueue tasks, never call DeliverReceipt / ForceDeliverReceipt /
	// dispatcher methods directly after authorization.
	forbiddenInAuthPaths := []string{
		"DeliverReceipt(",
		"ForceDeliverReceipt(",
		"deliverAuthorizedReceipt(",
		".SendReceiptEmail(",
		".UploadProtected(",
		".EnqueueReceiptPrint(",
	}
	// Auth path files under scrutiny.
	authFiles := []string{
		"service_jobs.go",
		"repository.go", // SaveReceiptForJob
		"service.go",    // ResendReceipt / enqueue helpers — catch a future inline-delivery regression here too
	}
	// Allowed files that still implement the dispatcher interface / legacy
	// deliverReceipt (used by worker + legacy sweep drain only).
	// service_jobs must not call deliverAuthorizedReceipt (removed).

	for _, name := range authFiles {
		path := filepath.Join(dir, name)
		body, err := os.ReadFile(path)
		if err != nil {
			t.Fatalf("read %s: %v", name, err)
		}
		src := string(body)
		// Strip comments roughly so doc comments don't false-positive.
		for _, line := range strings.Split(src, "\n") {
			trim := strings.TrimSpace(line)
			if strings.HasPrefix(trim, "//") {
				continue
			}
			for _, needle := range forbiddenInAuthPaths {
				if strings.Contains(line, needle) {
					t.Errorf("%s must not invoke %s after authorization (Wave 4 durable delivery); found: %s",
						name, needle, strings.TrimSpace(line))
				}
			}
		}
	}

	// Positive control: delivery_worker.go must still call dispatcher methods.
	workerPath := filepath.Join(dir, "delivery_worker.go")
	wbody, err := os.ReadFile(workerPath)
	if err != nil {
		t.Fatalf("read delivery_worker.go: %v", err)
	}
	if !strings.Contains(string(wbody), "SendReceiptEmail") &&
		!strings.Contains(string(wbody), "UploadProtected") {
		t.Error("delivery_worker.go should execute delivery channels via dispatcher")
	}
}
