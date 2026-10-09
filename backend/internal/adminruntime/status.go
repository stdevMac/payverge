package adminruntime

import (
	"sync"
	"time"
)

// WorkerStatus is a named background worker the admin health panel surfaces.
type WorkerStatus struct {
	Name    string `json:"name"`
	Status  string `json:"status"` // running | stopped | disabled
	Detail  string `json:"detail,omitempty"`
	Started bool   `json:"started"`
}

var (
	mu      sync.RWMutex
	workers = make(map[string]WorkerStatus)
)

// SetWorker records the latest known state for a background worker.
func SetWorker(name, status, detail string, started bool) {
	mu.Lock()
	defer mu.Unlock()
	workers[name] = WorkerStatus{
		Name:    name,
		Status:  status,
		Detail:  detail,
		Started: started,
	}
}

// ListWorkers returns a stable snapshot of registered workers.
func ListWorkers() []WorkerStatus {
	mu.RLock()
	defer mu.RUnlock()
	out := make([]WorkerStatus, 0, len(workers))
	for _, w := range workers {
		out = append(out, w)
	}
	return out
}

// StartedAt is process start time for uptime reporting.
var StartedAt = time.Now()
