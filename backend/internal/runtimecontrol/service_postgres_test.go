//go:build integration_postgres

package runtimecontrol

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/stdevmac/payverge/backend/internal/testperf/genesisdb"

	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func TestPostgresConcurrentInviteAdmissionNeverExceedsCap(t *testing.T) {
	ctx := context.Background()
	pg, err := genesisdb.Start(ctx)
	require.NoError(t, err)
	t.Cleanup(func() { _ = pg.Terminate(ctx) })
	svc := New(pg.DB)
	_, code, err := svc.CreateInviteBatch(ctx, CreateInviteBatchInput{Name: "parallel", CohortCap: 3, Owner: "launch", Reason: "race proof", ExpiresAt: time.Now().Add(time.Hour), Actor: "test"})
	require.NoError(t, err)

	start := make(chan struct{})
	errs := make(chan error, 12)
	var wg sync.WaitGroup
	for i := 0; i < 12; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			<-start
			errs <- svc.RegisterWithInvite(ctx, string(rune('a'+i))+"@example.test", code, func(tx *gorm.DB) error { return nil })
		}(i)
	}
	close(start)
	wg.Wait()
	close(errs)
	success := 0
	for err := range errs {
		if err == nil {
			success++
		} else {
			require.True(t, errors.Is(err, ErrCohortFull), err)
		}
	}
	require.Equal(t, 3, success)

}
