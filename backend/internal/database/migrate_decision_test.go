package database

import (
	"errors"
	"testing"

	"github.com/golang-migrate/migrate/v4"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestDecideMigrationStartup(t *testing.T) {
	otherErr := errors.New("connection refused")

	cases := []struct {
		name    string
		verErr  error
		dirty   bool
		want    migrationStartupAction
		wantErr bool
	}{
		{
			name:   "clean DB applies pending (never force-baselines)",
			verErr: nil,
			dirty:  false,
			want:   migrationApplyPending,
		},
		{
			name:   "dirty DB fails loudly",
			verErr: nil,
			dirty:  true,
			want:   migrationFailDirty,
		},
		{
			// A genesis-bootstrapped DB with no applied numbered migration yet.
			name:   "no migration state applies from the genesis baseline",
			verErr: migrate.ErrNilVersion,
			dirty:  false,
			want:   migrationApplyPending,
		},
		{
			name:    "unexpected version error surfaces",
			verErr:  otherErr,
			wantErr: true,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := decideMigrationStartup(tc.verErr, tc.dirty)
			if tc.wantErr {
				require.Error(t, err)
				return
			}
			require.NoError(t, err)
			assert.Equal(t, tc.want, got)
		})
	}
}

func TestLatestMigrationVersion_EmptyDirIsZero(t *testing.T) {
	dir := t.TempDir()
	v, err := LatestMigrationVersion(dir)
	require.NoError(t, err)
	assert.Equal(t, int64(0), v)
}
