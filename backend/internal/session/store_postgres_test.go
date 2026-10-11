//go:build integration_postgres

package session

import (
	"context"
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"

	"github.com/stdevmac/payverge/backend/internal/testperf"
)

func TestRotateSessionConcurrentCAS_PostgresIndependentConnections(t *testing.T) {
	ctx := context.Background()
	pg, err := testperf.StartPostgres(ctx)
	if err != nil {
		t.Fatalf("postgres container: %v (is Docker running?)", err)
	}
	t.Cleanup(func() { _ = pg.Terminate(ctx) })
	require.NoError(t, pg.DB.AutoMigrate(&UserSession{}, &RefreshTokenHistory{}))
	seedStore := NewStore(pg.DB)
	uid := uint(41)
	sess := seedSession(t, seedStore, &uid, "", "postgres-race-old")
	oldHash := HashToken("postgres-race-old")
	connections := make([]*gorm.DB, 2)
	for i := range connections {
		connections[i], err = gorm.Open(postgres.Open(pg.DSN), &gorm.Config{})
		require.NoError(t, err)
		sqlDB, dbErr := connections[i].DB()
		require.NoError(t, dbErr)
		t.Cleanup(func() { _ = sqlDB.Close() })
	}
	type result struct {
		matched     bool
		err         error
		refreshHash string
		sessionHash string
	}
	results := make(chan result, 2)
	start := make(chan struct{})
	var wg sync.WaitGroup
	for i := 0; i < 2; i++ {
		i := i
		wg.Add(1)
		go func() {
			defer wg.Done()
			<-start
			refreshHash := HashToken(fmt.Sprintf("postgres-race-refresh-%d", i))
			sessionHash := HashToken(fmt.Sprintf("postgres-race-session-%d", i))
			matched, rotateErr := NewStore(connections[i]).RotateSession(sess.ID, oldHash, refreshHash, sessionHash,
				time.Now().Add(7*24*time.Hour), time.Now().Add(24*time.Hour))
			results <- result{matched: matched, err: rotateErr, refreshHash: refreshHash, sessionHash: sessionHash}
		}()
	}
	close(start)
	wg.Wait()
	close(results)
	winners := 0
	var winner result
	for got := range results {
		require.NoError(t, got.err)
		if got.matched {
			winners++
			winner = got
		}
	}
	require.Equal(t, 1, winners)
	var persisted UserSession
	require.NoError(t, pg.DB.First(&persisted, sess.ID).Error)
	require.Equal(t, winner.refreshHash, persisted.RefreshToken)
	require.Equal(t, winner.sessionHash, persisted.SessionToken)
	var historyCount int64
	require.NoError(t, pg.DB.Model(&RefreshTokenHistory{}).Where("session_id = ? AND token_hash = ?", sess.ID, oldHash).
		Count(&historyCount).Error)
	require.Equal(t, int64(1), historyCount)
}
