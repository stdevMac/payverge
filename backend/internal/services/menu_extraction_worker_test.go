package services

import (
	"context"
	"fmt"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stdevmac/payverge/backend/internal/database"
	"github.com/stdevmac/payverge/backend/internal/llm"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func setupExtractionWorkerDB(t *testing.T) {
	t.Helper()
	gormDB, err := gorm.Open(sqlite.Open(fmt.Sprintf("file:worker-%s?mode=memory&cache=shared", t.Name())), &gorm.Config{})
	require.NoError(t, err)
	sqlDB, err := gormDB.DB()
	require.NoError(t, err)
	sqlDB.SetMaxOpenConns(1)
	database.SetTestDB(gormDB)
	require.NoError(t, gormDB.AutoMigrate(
		&database.Business{},
		&database.MenuExtractionJob{},
		&database.MenuExtractionImage{},
	))
}

func seedWorkerJob(t *testing.T, status database.MenuExtractionStatus) *database.MenuExtractionJob {
	t.Helper()
	biz := &database.Business{
		BusinessId:     "w-" + t.Name(),
		Name:           "W",
		OwnerAddress:   "0xw",
		SettlementAddr: "0x1111111111111111111111111111111111111111",
		TippingAddr:    "0x2222222222222222222222222222222222222222",
		IsActive:       true,
		CreatedAt:      time.Now(),
		UpdatedAt:      time.Now(),
	}
	require.NoError(t, database.GetDB().Create(biz).Error)
	job := &database.MenuExtractionJob{
		BusinessID: biz.ID,
		Status:     status,
		ImageCount: 1,
		CreatedAt:  time.Now(),
		UpdatedAt:  time.Now(),
	}
	require.NoError(t, database.GetDB().Create(job).Error)
	require.NoError(t, database.GetDB().Create(&database.MenuExtractionImage{
		JobID:      job.ID,
		FilePath:   "",
		PageOrder:  0,
		MIMEType:   "image/jpeg",
		StorageKey: fmt.Sprintf("ai/menu-extraction/%d/%d/page.jpg", biz.ID, job.ID),
		CreatedAt:  time.Now(),
	}).Error)
	return job
}

type countingExtractionProvider struct {
	calls atomic.Int64
	resp  *llm.Response
}

func (c *countingExtractionProvider) Generate(_ context.Context, _ llm.GenerateRequest) (*llm.Response, error) {
	c.calls.Add(1)
	return c.resp, nil
}

func TestMenuExtractionWorker_DoubleSubmitOneProviderCall(t *testing.T) {
	setupExtractionWorkerDB(t)
	job := seedWorkerJob(t, database.ExtractionStatusUploading)

	provider := &countingExtractionProvider{resp: &llm.Response{
		Text: `{"restaurantName":"R","currency":"$","categories":[]}`,
	}}
	svc := NewMenuAIService(provider, llm.ModelConfig{Menu: "m", Image: "i"})

	var cleaned atomic.Int64
	w := NewMenuExtractionWorker(svc,
		func(images []database.MenuExtractionImage) ([]MenuExtractionInput, error) {
			return []MenuExtractionInput{{
				Name:     "page",
				MIMEType: "image/jpeg",
				Data:     []byte{0xff, 0xd8, 0xff, 0xe0, 1, 2, 3},
			}}, nil
		},
		func(images []database.MenuExtractionImage) {
			cleaned.Add(1)
		},
	)

	// First process claims and completes.
	w.ProcessOne(context.Background(), job.ID)
	// Second process must observe completed and not re-call the provider.
	w.ProcessOne(context.Background(), job.ID)

	assert.Equal(t, int64(1), provider.calls.Load())
	assert.Equal(t, int64(1), cleaned.Load(), "cleanup only after terminal success")

	reloaded, err := database.GetExtractionJobByID(job.ID)
	require.NoError(t, err)
	assert.Equal(t, database.ExtractionStatusCompleted, reloaded.Status)
	assert.Contains(t, reloaded.ExtractedMenu, "restaurant_name")
}

func TestMenuExtractionWorker_RestartRecoversStaleProcessing(t *testing.T) {
	setupExtractionWorkerDB(t)
	job := seedWorkerJob(t, database.ExtractionStatusProcessing)
	stale := time.Now().UTC().Add(-database.DefaultMenuExtractionClaimLease - time.Minute)
	require.NoError(t, database.GetDB().Model(job).Updates(map[string]interface{}{
		"claim_token":   "dead-worker",
		"claimed_at":    stale,
		"attempt_count": 1,
	}).Error)

	provider := &countingExtractionProvider{resp: &llm.Response{
		Text: `{"restaurantName":"Recovered","currency":"$","categories":[]}`,
	}}
	svc := NewMenuAIService(provider, llm.ModelConfig{Menu: "m", Image: "i"})
	w := NewMenuExtractionWorker(svc,
		func(images []database.MenuExtractionImage) ([]MenuExtractionInput, error) {
			return []MenuExtractionInput{{Name: "p", MIMEType: "image/jpeg", Data: []byte{0xff, 0xd8, 0xff}}}, nil
		},
		func(images []database.MenuExtractionImage) {},
	)

	w.ProcessOne(context.Background(), job.ID)

	assert.Equal(t, int64(1), provider.calls.Load())
	reloaded, err := database.GetExtractionJobByID(job.ID)
	require.NoError(t, err)
	assert.Equal(t, database.ExtractionStatusCompleted, reloaded.Status)
	assert.Equal(t, int64(2), reloaded.AttemptCount)
}

// blockingExtractionProvider waits for ctx to end, like an in-flight provider
// call when the process shuts down.
type blockingExtractionProvider struct{ started chan struct{} }

func (b *blockingExtractionProvider) Generate(ctx context.Context, _ llm.GenerateRequest) (*llm.Response, error) {
	close(b.started)
	<-ctx.Done()
	return nil, ctx.Err()
}

// A shutdown is not the provider's fault: the in-flight job must go back to
// pending with its attempt refunded, not be marked provider_failed.
func TestMenuExtractionWorker_ShutdownReleasesInFlightJob(t *testing.T) {
	setupExtractionWorkerDB(t)
	job := seedWorkerJob(t, database.ExtractionStatusUploading)

	provider := &blockingExtractionProvider{started: make(chan struct{})}
	svc := NewMenuAIService(provider, llm.ModelConfig{Menu: "m", Image: "i"})
	w := NewMenuExtractionWorker(svc,
		func(images []database.MenuExtractionImage) ([]MenuExtractionInput, error) {
			return []MenuExtractionInput{{Name: "p", MIMEType: "image/jpeg", Data: []byte{0xff, 0xd8, 0xff}}}, nil
		},
		func(images []database.MenuExtractionImage) {},
	)
	w.Start(context.Background())
	w.Enqueue(job.ID)
	select {
	case <-provider.started:
	case <-time.After(10 * time.Second):
		t.Fatal("provider was never called")
	}
	w.Stop()

	reloaded, err := database.GetExtractionJobByID(job.ID)
	require.NoError(t, err)
	assert.Equal(t, database.ExtractionStatusPending, reloaded.Status)
	assert.Empty(t, reloaded.ErrorMessage, "shutdown must not record a provider failure")
	assert.Equal(t, int64(0), reloaded.AttemptCount, "shutdown must not spend an attempt")

	ids, err := database.ListMenuExtractionJobsNeedingWork(time.Now().UTC(), database.DefaultMenuExtractionClaimLease, 10)
	require.NoError(t, err)
	assert.Contains(t, ids, job.ID, "restart recovery must pick the job up immediately")
}
