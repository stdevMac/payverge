package services

import (
	"context"
	"strconv"
	"testing"
	"time"

	"github.com/stdevmac/payverge/backend/internal/database"
)

type noopPluginNotificationSender struct{}

func (noopPluginNotificationSender) SendPluginNotification(context.Context, database.PluginNotificationDelivery) PluginNotificationSendResult {
	return PluginNotificationSendResult{ProviderMessageID: "bench-provider-message"}
}

func BenchmarkPluginNotificationWorkerProcessDue(b *testing.B) {
	db, business := setupPluginNotificationOutboxTestDB(b)
	now := time.Date(2026, 5, 23, 12, 0, 0, 0, time.UTC)
	const batchSize = 25

	deliveries := make([]database.PluginNotificationDelivery, b.N*batchSize)
	for i := range deliveries {
		deliveries[i] = database.PluginNotificationDelivery{
			BusinessID:    business.ID,
			PluginName:    "telegram",
			EventType:     PluginEventOrderCreated,
			EventID:       "bench-order-event-" + strconv.Itoa(i),
			Status:        database.PluginNotificationDeliveryStatusPending,
			Payload:       map[string]interface{}{"order_number": i},
			NextAttemptAt: now,
			CreatedAt:     now,
			UpdatedAt:     now,
		}
	}
	if err := db.GetGorm().CreateInBatches(&deliveries, 500).Error; err != nil {
		b.Fatalf("create deliveries: %v", err)
	}

	worker := NewPluginNotificationWorker("telegram", noopPluginNotificationSender{})
	worker.BatchSize = batchSize
	worker.Now = func() time.Time { return now }

	ctx := context.Background()
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		processed, err := worker.ProcessDue(ctx)
		if err != nil {
			b.Fatalf("process due: %v", err)
		}
		if processed != batchSize {
			b.Fatalf("processed %d, want %d", processed, batchSize)
		}
	}
}
