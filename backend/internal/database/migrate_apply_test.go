package database

import (
	"testing"

	"github.com/golang-migrate/migrate/v4"
	"github.com/stretchr/testify/require"
)

// fakeMigrator records Up and reports a different ledger after Up runs.
type fakeMigrator struct {
	version     uint
	dirty       bool
	verErr      error
	upErr       error
	upCalled    bool
	afterVer    uint
	afterDirty  bool
	afterVerErr error
}

func (f *fakeMigrator) Up() error {
	f.upCalled = true
	return f.upErr
}

func (f *fakeMigrator) Version() (uint, bool, error) {
	if f.upCalled {
		return f.afterVer, f.afterDirty, f.afterVerErr
	}
	return f.version, f.dirty, f.verErr
}

func TestApplyMigrations_VersionBelowLatest(t *testing.T) {
	f := &fakeMigrator{version: 1, afterVer: 1}
	_, err := applyMigrations(f, 3)
	require.EqualError(t, err, "migration version 1 after Up does not match latest migration file 3")
	require.True(t, f.upCalled)
}

func TestApplyMigrations_DirtyAfterUp(t *testing.T) {
	f := &fakeMigrator{version: 2, afterVer: 3, afterDirty: true}
	_, err := applyMigrations(f, 3)
	require.Error(t, err)
	require.Contains(t, err.Error(), "dirty")
	require.True(t, f.upCalled)
}

func TestApplyMigrations_SucceedsAtLatest(t *testing.T) {
	f := &fakeMigrator{version: 1, afterVer: 4}
	got, err := applyMigrations(f, 4)
	require.NoError(t, err)
	require.Equal(t, uint(4), got)
	require.True(t, f.upCalled)
}

func TestApplyMigrations_ErrNoChangeAtLatest(t *testing.T) {
	f := &fakeMigrator{version: 4, afterVer: 4, upErr: migrate.ErrNoChange}
	got, err := applyMigrations(f, 4)
	require.NoError(t, err)
	require.Equal(t, uint(4), got)
	require.True(t, f.upCalled)
}

func TestApplyMigrations_DirtyBeforeUp(t *testing.T) {
	f := &fakeMigrator{version: 2, dirty: true, afterVer: 9}
	_, err := applyMigrations(f, 3)
	require.Error(t, err)
	require.Contains(t, err.Error(), "dirty")
	require.False(t, f.upCalled)
}

func TestApplyMigrations_LatestVersionZeroSkipsUp(t *testing.T) {
	f := &fakeMigrator{verErr: migrate.ErrNilVersion, afterVer: 9}
	got, err := applyMigrations(f, 0)
	require.NoError(t, err)
	require.Equal(t, uint(0), got)
	require.False(t, f.upCalled)
}

func TestApplyMigrations_NilVersionAfterUp(t *testing.T) {
	f := &fakeMigrator{version: 1, afterVerErr: migrate.ErrNilVersion}
	_, err := applyMigrations(f, 2)
	require.Error(t, err)
	require.ErrorIs(t, err, migrate.ErrNilVersion)
	require.True(t, f.upCalled)
}
