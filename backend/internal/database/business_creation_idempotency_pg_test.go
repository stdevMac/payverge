//go:build integration_postgres

package database

import (
	"strings"
	"sync"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestWorkspaceIdempotencyAndQRMilestone_PostgresConcurrency(t *testing.T) {
	pg := startGenesisPostgres(t)

	createdAfter := &Business{
		BusinessId:   "created-after-qr-milestone",
		Name:         "New Cafe",
		OwnerAddress: "0x2222222222222222222222222222222222222222",
		IsActive:     true,
	}
	require.NoError(t, pg.DB.Create(createdAfter).Error)
	require.Nil(t, createdAfter.QRPreviewedAt,
		"new workspaces must earn the QR preview milestone")

	previousDB := db
	SetTestDB(pg.DB)
	t.Cleanup(func() { SetTestDB(previousDB) })

	type createResult struct {
		business *Business
		replayed bool
		err      error
	}
	results := make([]createResult, 2)
	start := make(chan struct{})
	var wg sync.WaitGroup
	for i := range results {
		wg.Add(1)
		go func(index int) {
			defer wg.Done()
			<-start
			candidate := &Business{
				BusinessId:   "concurrent-response-loss-cafe",
				Name:         "Concurrent Response Loss Cafe",
				OwnerAddress: "0x3333333333333333333333333333333333333333",
				IsActive:     true,
			}
			results[index].business, results[index].replayed, results[index].err =
				CreateBusinessIdempotently(
					candidate,
					strings.Repeat("a", 64),
					strings.Repeat("b", 64),
					strings.Repeat("c", 64),
				)
		}(i)
	}
	close(start)
	wg.Wait()

	require.NoError(t, results[0].err)
	require.NoError(t, results[1].err)
	require.NotNil(t, results[0].business)
	require.NotNil(t, results[1].business)
	require.Equal(t, results[0].business.ID, results[1].business.ID)
	require.NotEqual(t, results[0].replayed, results[1].replayed,
		"one concurrent request must create and the other must replay")

	var workspaceCount, requestCount int64
	require.NoError(t, pg.DB.Model(&Business{}).
		Where("business_id = ?", "concurrent-response-loss-cafe").
		Count(&workspaceCount).Error)
	require.Equal(t, int64(1), workspaceCount)
	require.NoError(t, pg.DB.Model(&BusinessCreationRequest{}).
		Where("owner_scope_hash = ? AND idempotency_key_hash = ?",
			strings.Repeat("a", 64), strings.Repeat("b", 64)).
		Count(&requestCount).Error)
	require.Equal(t, int64(1), requestCount)
}
