// Package testperf provides shared test-only PostgreSQL and fixture helpers.
// StartIsolatedPostgres gives concurrency and migration tests their own database
// on TEST_DATABASE_URL, or their own Testcontainers server when no DSN is set.
// The package is not used by production code paths.
package testperf
