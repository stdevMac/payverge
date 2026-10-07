package database

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/stdevmac/payverge/backend/schema/genesis"
)

func TestSchemaContract_OfferScheduleAndGuestRequestIdentity(t *testing.T) {
	require.Contains(t, genesis.SchemaSQL, `weekday_mask smallint DEFAULT 127 NOT NULL`)
	require.Contains(t, genesis.SchemaSQL, `start_minute integer`)
	require.Contains(t, genesis.SchemaSQL, `end_minute integer`)
	require.Contains(t, genesis.SchemaSQL, `orders_guest_request_identity_uq`)
}
