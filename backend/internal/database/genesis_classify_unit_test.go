package database

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestClassifyEmptyLedger(t *testing.T) {
	require.Equal(t, DBVersioned, classifyEmptyLedger(true), "a head-0 install keeps an empty ledger; any later binary must still migrate it")
	require.Equal(t, DBLegacy, classifyEmptyLedger(false), "missing genesis tables")
}

func TestCheckPostgresMajor(t *testing.T) {
	require.NoError(t, checkPostgresMajor(180001, 18))
	require.ErrorContains(t, checkPostgresMajor(150008, 18), "postgres major version 15 does not match the genesis baseline's 18")

	older := checkPostgresMajor(150008, 18)
	require.ErrorContains(t, older, "deploy/upgrade-postgres.sh", "an older server must point self-hosters at the upgrade script")
	require.ErrorContains(t, older, "managed database", "an older server must tell managed-DB operators to upgrade the DB")
	require.ErrorContains(t, older, "PostgreSQL 18")
	require.NotContains(t, older.Error(), "restore the backup")

	newer := checkPostgresMajor(190002, 18)
	require.ErrorContains(t, newer, "postgres major version 19 does not match the genesis baseline's 18")
	require.ErrorContains(t, newer, "older Payverge", "a newer server means the binary is behind the database")
	require.ErrorContains(t, newer, "restore the backup")
	require.NotContains(t, newer.Error(), "deploy/upgrade-postgres.sh", "upgrading Postgres again cannot fix a binary that is too old")
	require.ErrorContains(t, checkPostgresMajor(150008, 0), "postgres_major is required")
}

func TestCheckFingerprint(t *testing.T) {
	require.NoError(t, checkFingerprint("abc", "abc"))
	require.ErrorContains(t, checkFingerprint("abc", "def"), "schema drift")
	require.ErrorContains(t, checkFingerprint("abc", ""), "fingerprint_sha256 is required")
}

func TestEmbeddedGenesisRecordsFingerprintAndMajor(t *testing.T) {
	meta, err := loadGenesisMeta()
	require.NoError(t, err)
	require.Len(t, meta.FingerprintSHA256, 64)
	require.Equal(t, 18, meta.PostgresMajor)
}
