package database

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestBusinessHasPaidBill(t *testing.T) {
	setupTestDB(t)

	has, err := BusinessHasPaidBill(4242)
	require.NoError(t, err)
	assert.False(t, has, "no bills → false")

	require.NoError(t, db.Create(&Bill{BusinessID: 4242, Status: BillStatusOpen, BillNumber: "OPEN-1"}).Error)
	has, err = BusinessHasPaidBill(4242)
	require.NoError(t, err)
	assert.False(t, has, "open bills don't count")

	require.NoError(t, db.Create(&Bill{BusinessID: 4242, Status: BillStatusPaid, BillNumber: "PAID-1"}).Error)
	has, err = BusinessHasPaidBill(4242)
	require.NoError(t, err)
	assert.True(t, has)
}
