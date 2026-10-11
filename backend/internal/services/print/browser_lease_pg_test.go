//go:build integration_postgres

package print

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/stdevmac/payverge/backend/internal/database"
	"github.com/stdevmac/payverge/backend/internal/testperf"
)

// TestQueue_ClaimBrowserJob_PostgresRace proves SKIP LOCKED yields exactly one
// winner when two clients race on Postgres.
func TestQueue_ClaimBrowserJob_PostgresRace(t *testing.T) {
	ctx := context.Background()
	pg, err := testperf.StartPostgres(ctx)
	if err != nil {
		t.Skipf("postgres unavailable: %v", err)
	}
	t.Cleanup(func() { _ = pg.Terminate(ctx) })

	db := pg.DB
	if err := db.AutoMigrate(
		&database.Business{},
		&database.Printer{},
		&database.PrintJob{},
	); err != nil {
		t.Fatalf("automigrate: %v", err)
	}

	biz := &database.Business{
		BusinessId:     "pg-print-race",
		OwnerAddress:   "0x0000000000000000000000000000000000000001",
		Name:           "PG Race",
		SettlementAddr: "0x0000000000000000000000000000000000000002",
		TippingAddr:    "0x0000000000000000000000000000000000000003",
	}
	if err := db.Create(biz).Error; err != nil {
		t.Fatal(err)
	}
	printer := database.Printer{
		BusinessID: biz.ID, Name: "browser", Role: "bill",
		Transport: "browser", PaperWidthMM: 80, Enabled: true,
	}
	if err := db.Create(&printer).Error; err != nil {
		t.Fatal(err)
	}
	html := "<html>race</html>"
	job := database.PrintJob{
		BusinessID: biz.ID, PrinterID: &printer.ID, Kind: database.PrintJobKindBill,
		SourceType: "bill", SourceID: 1, Status: database.PrintJobStatusRouted,
		PayloadHTML: &html, MaxAttempts: 6,
	}
	if err := db.Create(&job).Error; err != nil {
		t.Fatal(err)
	}

	q := NewQueue(db)
	now := time.Now()
	var wg sync.WaitGroup
	results := make(chan *database.PrintJob, 2)
	for _, client := range []string{"tab-a", "tab-b"} {
		wg.Add(1)
		go func(clientID string) {
			defer wg.Done()
			claimed, err := q.ClaimBrowserJob(ctx, biz.ID, clientID, now, time.Minute)
			if err != nil {
				t.Errorf("claim %s: %v", clientID, err)
				return
			}
			results <- claimed
		}(client)
	}
	wg.Wait()
	close(results)

	var winners int
	for c := range results {
		if c != nil {
			winners++
		}
	}
	if winners != 1 {
		t.Fatalf("expected exactly one claim winner, got %d", winners)
	}
}
