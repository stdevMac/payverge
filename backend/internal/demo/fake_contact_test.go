package demo

import (
	"context"
	"regexp"
	"strconv"
	"strings"
	"testing"

	"github.com/stdevmac/payverge/backend/internal/database"

	"github.com/stretchr/testify/require"
)

var (
	// "+54 11 0000-0148": the printed venue landline form.
	fakeDisplayPhone = regexp.MustCompile(`^\+54 11 0000-\d{4}$`)
	// "+5491100002300": the compact mobile form rows store.
	fakeMobilePhone = regexp.MustCompile(`^\+549110000\d{4}$`)
	firstNumber     = regexp.MustCompile(`\d+`)
)

func TestFakeContactHelpers(t *testing.T) {
	require.Equal(t, "+54 11 0000-0148", demoFakePhoneDisplay(148))
	require.Equal(t, "+54 11 0000-5602", demoFakePhoneDisplay(5602))
	require.Equal(t, "+5491100002300", demoFakeMobile(2300))
	require.Equal(t, "+5491100000999", demoFakeMobile(999))
	require.Equal(t, "Defensa 9148", demoFakeStreet("Defensa", 148))
	require.Equal(t, "Estados Unidos 9740", demoFakeStreet("Estados Unidos", 740))
	require.Equal(t, "https://bodegon-mesa-larga.example", demoFakeWebsite(profile{Key: "primary"}))
	require.Equal(t, "https://parrilla-quebracho-azul.example", demoFakeWebsite(profile{Key: "secondary"}))
}

func requireFakeHouseNumber(t *testing.T, what, street string) {
	t.Helper()
	m := firstNumber.FindString(street)
	require.NotEmpty(t, m, "%s: street %q must carry a house number", what, street)
	n, err := strconv.Atoi(m)
	require.NoError(t, err)
	require.GreaterOrEqual(t, n, demoFakeHouseNumberBase,
		"%s: street %q must use a fictional house number (>= %d)", what, street, demoFakeHouseNumberBase)
}

// A self-hosted showroom must never print, dial or map a real line or
// address. Every phone and street the seed writes must come from the fake
// blocks in fake_contact.go, and no venue may route to an upstream website or
// social account.
func TestSeededDemoContactDataIsFictional(t *testing.T) {
	db := newDemoServiceTestDB(t)
	admin := seedAdmin(t, db, "demo-admin-fake-contact@example.com")

	svc := NewService(db, Options{Now: fixedNow, SeedVersion: "test-seed-fake-contact", BaselineDays: 7})
	_, err := svc.EnsureForAdmin(context.Background(), admin.ID)
	require.NoError(t, err)

	businesses := demoBusinesses(t, db, admin.ID)
	require.Len(t, businesses, 2)
	ids := make([]uint, 0, len(businesses))
	for _, b := range businesses {
		ids = append(ids, b.ID)
		require.Regexp(t, fakeDisplayPhone, b.Phone, "business %d phone", b.ID)
		requireFakeHouseNumber(t, "business address", b.Address.Street)
		require.True(t, strings.HasSuffix(b.Website, ".example"), "business %d website %q must use the reserved .example TLD", b.ID, b.Website)
		require.Equal(t, demoNoSocialMedia, b.SocialMedia, "business %d must carry no social handles", b.ID)
		require.NotContains(t, strings.ToLower(b.Website+b.SocialMedia), "payverge")
	}

	var drivers []database.DeliveryDriver
	require.NoError(t, db.Where("business_id IN ?", ids).Find(&drivers).Error)
	require.NotEmpty(t, drivers)
	for _, d := range drivers {
		require.Regexp(t, fakeMobilePhone, d.Phone, "driver %q phone", d.Name)
	}

	var customers []database.Customer
	require.NoError(t, db.Find(&customers).Error)
	require.NotEmpty(t, customers)
	for _, c := range customers {
		if c.Phone == "" {
			continue
		}
		require.Regexp(t, fakeMobilePhone, c.Phone, "customer %q phone", c.Name)
	}

	var addresses []database.CustomerAddress
	require.NoError(t, db.Where("business_id IN ?", ids).Find(&addresses).Error)
	require.NotEmpty(t, addresses)
	for _, a := range addresses {
		requireFakeHouseNumber(t, "customer address", a.Street)
		require.Regexp(t, fakeMobilePhone, a.ContactPhone)
	}

	var deliveries []database.DeliveryOrder
	require.NoError(t, db.Where("business_id IN ?", ids).Find(&deliveries).Error)
	require.NotEmpty(t, deliveries)
	for _, d := range deliveries {
		require.Regexp(t, fakeMobilePhone, d.CustomerPhone, "delivery %s phone", d.DeliveryNumber)
		requireFakeHouseNumber(t, "delivery address", d.DeliveryAddress.Street)
	}

	var reservations []database.TableReservation
	require.NoError(t, db.Where("business_id IN ?", ids).Find(&reservations).Error)
	require.NotEmpty(t, reservations)
	for _, r := range reservations {
		require.Regexp(t, fakeMobilePhone, r.CustomerPhone, "reservation %s phone", r.ConfirmationCode)
	}
}
