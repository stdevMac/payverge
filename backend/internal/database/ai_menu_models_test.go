package database

import (
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func setupMenuExtractionClaimDB(t *testing.T) *gorm.DB {
	t.Helper()
	gormDB, err := gorm.Open(sqlite.Open(fmt.Sprintf("file:%s?mode=memory&cache=shared", t.Name())), &gorm.Config{})
	require.NoError(t, err)
	sqlDB, err := gormDB.DB()
	require.NoError(t, err)
	sqlDB.SetMaxOpenConns(1)
	SetTestDB(gormDB)
	require.NoError(t, gormDB.AutoMigrate(&Business{}, &MenuExtractionJob{}, &MenuExtractionImage{}))
	return gormDB
}

func seedExtractionJob(t *testing.T, status MenuExtractionStatus) *MenuExtractionJob {
	t.Helper()
	biz := &Business{
		BusinessId:     fmt.Sprintf("claim-biz-%s-%d", t.Name(), time.Now().UnixNano()),
		Name:           "Claim Biz",
		OwnerAddress:   "0xclaim",
		SettlementAddr: "0x1111111111111111111111111111111111111111",
		TippingAddr:    "0x2222222222222222222222222222222222222222",
		IsActive:       true,
		CreatedAt:      time.Now(),
		UpdatedAt:      time.Now(),
	}
	require.NoError(t, GetDB().Create(biz).Error)
	job := &MenuExtractionJob{
		BusinessID: biz.ID,
		Status:     status,
		CreatedAt:  time.Now(),
		UpdatedAt:  time.Now(),
	}
	require.NoError(t, GetDB().Create(job).Error)
	return job
}

func TestClaimMenuExtractionJob_OnlyOneConcurrentWinner(t *testing.T) {
	setupMenuExtractionClaimDB(t)
	job := seedExtractionJob(t, ExtractionStatusPending)
	now := time.Now().UTC()

	var wg sync.WaitGroup
	results := make(chan MenuExtractionClaim, 2)
	for i := 0; i < 2; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			claim, err := ClaimMenuExtractionJob(job.ID, fmt.Sprintf("tok-%d", i), now, DefaultMenuExtractionClaimLease)
			require.NoError(t, err)
			results <- claim
		}(i)
	}
	wg.Wait()
	close(results)

	claimed := 0
	for c := range results {
		if c.Claimed {
			claimed++
			assert.Equal(t, ExtractionStatusProcessing, c.Job.Status)
			assert.Equal(t, int64(1), c.Job.AttemptCount)
		}
	}
	assert.Equal(t, 1, claimed, "exactly one concurrent claim must win")
}

func TestClaimMenuExtractionJob_FreshProcessingAndCompletedNotReclaimed(t *testing.T) {
	setupMenuExtractionClaimDB(t)
	now := time.Now().UTC()

	processing := seedExtractionJob(t, ExtractionStatusProcessing)
	tok := "owner-token"
	claimedAt := now.Add(-time.Minute)
	require.NoError(t, GetDB().Model(processing).Updates(map[string]interface{}{
		"claim_token":   tok,
		"claimed_at":    claimedAt,
		"attempt_count": 1,
	}).Error)

	claim, err := ClaimMenuExtractionJob(processing.ID, "intruder", now, DefaultMenuExtractionClaimLease)
	require.NoError(t, err)
	assert.False(t, claim.Claimed)
	assert.Equal(t, ExtractionStatusProcessing, claim.Job.Status)
	assert.Equal(t, tok, claim.Job.ClaimToken)

	completed := seedExtractionJob(t, ExtractionStatusCompleted)
	claim2, err := ClaimMenuExtractionJob(completed.ID, "intruder", now, DefaultMenuExtractionClaimLease)
	require.NoError(t, err)
	assert.False(t, claim2.Claimed)
	assert.Equal(t, ExtractionStatusCompleted, claim2.Job.Status)
}

func TestClaimMenuExtractionJob_StaleProcessingIsReclaimable(t *testing.T) {
	setupMenuExtractionClaimDB(t)
	job := seedExtractionJob(t, ExtractionStatusProcessing)
	now := time.Now().UTC()
	stale := now.Add(-DefaultMenuExtractionClaimLease - time.Minute)
	require.NoError(t, GetDB().Model(job).Updates(map[string]interface{}{
		"claim_token":   "old",
		"claimed_at":    stale,
		"attempt_count": 2,
	}).Error)

	claim, err := ClaimMenuExtractionJob(job.ID, "fresh", now, DefaultMenuExtractionClaimLease)
	require.NoError(t, err)
	assert.True(t, claim.Claimed)
	assert.Equal(t, "fresh", claim.Job.ClaimToken)
	assert.Equal(t, int64(3), claim.Job.AttemptCount)
}

// TestMenuExtractionModels_ClaimAndMIMEFields ensures GORM models expose the
// durable claim and MIME fields required by the extraction worker.
func TestMenuExtractionModels_ClaimAndMIMEFields(t *testing.T) {
	job := MenuExtractionJob{}
	img := MenuExtractionImage{}

	// Compile-time field presence via assignment (fails if fields removed).
	job.ClaimToken = "tok"
	job.AttemptCount = 1
	_ = job.ClaimedAt
	_ = job.NextAttemptAt

	img.MIMEType = "image/png"
	img.StorageKey = "ai/menu-extraction/1/1/page.png"

	assert.Equal(t, "menu_extraction_jobs", job.TableName())
	assert.Equal(t, "menu_extraction_images", img.TableName())
}
