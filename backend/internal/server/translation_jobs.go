package server

import (
	"strconv"
	"sync"
	"time"

	"github.com/google/uuid"
)

type translationJobRecord struct {
	BusinessID uint
	Status     TranslationStatus
}

var translationJobs = struct {
	mu    sync.RWMutex
	items map[string]translationJobRecord
}{
	items: make(map[string]translationJobRecord),
}

func generateJobID(businessID uint) string {
	return "translate_" + strconv.FormatUint(uint64(businessID), 10) + "_" + uuid.NewString()
}

func registerTranslationJob(businessID uint) string {
	jobID := generateJobID(businessID)
	translationJobs.mu.Lock()
	defer translationJobs.mu.Unlock()

	translationJobs.items[jobID] = translationJobRecord{
		BusinessID: businessID,
		Status: TranslationStatus{
			JobID:     jobID,
			Status:    "processing",
			Progress:  0,
			Total:     0,
			Message:   "Translation in progress",
			CreatedAt: time.Now().UTC().Format(time.RFC3339),
		},
	}

	return jobID
}

func updateTranslationJob(jobID string, updateFn func(*TranslationStatus)) {
	translationJobs.mu.Lock()
	defer translationJobs.mu.Unlock()

	record, ok := translationJobs.items[jobID]
	if !ok {
		return
	}

	updateFn(&record.Status)
	translationJobs.items[jobID] = record
}

func getTranslationJob(jobID string) (translationJobRecord, bool) {
	translationJobs.mu.RLock()
	defer translationJobs.mu.RUnlock()

	record, ok := translationJobs.items[jobID]
	return record, ok
}

func resetTranslationJobs() {
	translationJobs.mu.Lock()
	defer translationJobs.mu.Unlock()
	translationJobs.items = make(map[string]translationJobRecord)
}
