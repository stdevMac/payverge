package cashregister_test

import (
	"context"
	"testing"
	"time"

	"github.com/stdevmac/payverge/backend/internal/services/cashregister"
)

func BenchmarkCurrentDemoHouseRailAlreadyOpen(b *testing.B) {
	db := openCashRegisterServiceTestDB(b)
	svc := cashregister.NewServiceWithClock(db, fixedCashRegisterNow)
	business := createDemoHouseRailBusiness(b, db, "bench-demo-open")
	if _, err := svc.EnsureDemoHouseCashSession(context.Background(), business.ID); err != nil {
		b.Fatal(err)
	}

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if _, err := svc.Current(context.Background(), business.ID); err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkCurrentRealVenueClosedDrawer(b *testing.B) {
	db := openCashRegisterServiceTestDB(b)
	svc := cashregister.NewServiceWithClock(db, fixedCashRegisterNow)
	business := createCashRegisterBusiness(b, db, "bench-real-closed")
	_ = createClosedCashRegisterSession(b, db, business.ID, fixedCashRegisterNow().Add(-24*time.Hour))

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if _, err := svc.Current(context.Background(), business.ID); err != nil {
			b.Fatal(err)
		}
	}
}
