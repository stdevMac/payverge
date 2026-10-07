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
	require.NoError(t, checkPostgresMajor(150008, 15))
	require.ErrorContains(t, checkPostgresMajor(160004, 15), "postgres major version 16")
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
	require.Equal(t, 15, meta.PostgresMajor)
}
