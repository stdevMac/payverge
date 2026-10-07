package demo

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/stdevmac/payverge/backend/internal/database"
)

var guessableDemoPayDeliveryNumber = regexp.MustCompile(`^DEMO-PAY-[0-9]+$`)
var productionDeliveryNumber = regexp.MustCompile(`^DEL-[A-Z0-9]{16}$`)

func TestDemoGeneratorSourceDoesNotEmitGuessableDeliveryNumbers(t *testing.T) {
	dir := "."
	generatorSrc, err := os.ReadFile(filepath.Join(dir, "generator.go"))
	require.NoError(t, err)
	daySrc, err := os.ReadFile(filepath.Join(dir, "day_generator.go"))
	require.NoError(t, err)

	require.NotContains(t, string(generatorSrc), `DEMO-PAY-%d`,
		"ensureAwaitingPaymentDelivery must not format DEMO-PAY-<business_id>")
	require.NotContains(t, string(generatorSrc), `"DEMO-PAY-"`,
		"demo generator must not keep the DEMO-PAY- prefix")
	require.NotContains(t, string(daySrc), `fmt.Sprintf("DEL-%s", bill.BillNumber)`,
		"day_generator must not derive the public track id from the bill number")
}

func TestSeededDeliveryNumbersAreUnguessable(t *testing.T) {
	db := newDemoServiceTestDB(t)
	admin := seedAdmin(t, db, "delivery-enum-admin@example.com")

	svc := NewService(db, Options{Now: fixedNow, SeedVersion: "honesty-seed", BaselineDays: 5})
	require.NoError(t, mustEnsure(svc, admin.ID))

	type row struct {
		BusinessID     uint
		DeliveryNumber string
	}
	var rows []row
	require.NoError(t, db.Model(&database.DeliveryOrder{}).
		Select("delivery_orders.business_id, delivery_orders.delivery_number").
		Joins("JOIN businesses ON businesses.id = delivery_orders.business_id").
		Where("businesses.demo_owner_user_id = ?", admin.ID).
		Scan(&rows).Error)
	require.NotEmpty(t, rows, "seed must create at least one delivery")

	seen := map[string]struct{}{}
	for _, r := range rows {
		require.Falsef(t, guessableDemoPayDeliveryNumber.MatchString(r.DeliveryNumber),
			"delivery_number %q matches DEMO-PAY-<digits>", r.DeliveryNumber)
		require.NotEqualf(t, strconv.FormatUint(uint64(r.BusinessID), 10), r.DeliveryNumber,
			"delivery_number must not be the raw business id")
		require.NotEqualf(t, fmt.Sprintf("DEMO-PAY-%d", r.BusinessID), r.DeliveryNumber,
			"delivery_number must not be DEMO-PAY-<business_id>")
		require.Falsef(t, strings.HasPrefix(r.DeliveryNumber, fmt.Sprintf("DEL-B%d-", r.BusinessID)),
			"delivery_number %q embeds business id %d as the public identifier", r.DeliveryNumber, r.BusinessID)
		require.Regexpf(t, productionDeliveryNumber, r.DeliveryNumber,
			"delivery_number %q must use the production opaque DEL- + 16 hex shape", r.DeliveryNumber)
		_, dup := seen[r.DeliveryNumber]
		require.Falsef(t, dup, "duplicate delivery_number %q", r.DeliveryNumber)
		seen[r.DeliveryNumber] = struct{}{}
	}
}
