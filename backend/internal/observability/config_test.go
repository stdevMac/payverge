package observability

import (
	"testing"

	"github.com/sirupsen/logrus"
	"github.com/stretchr/testify/require"
)

func clearConfigEnv(t *testing.T) {
	t.Helper()

	for _, key := range []string{
		"SENTRY_DSN",
		"SENTRY_ENVIRONMENT",
		"APP_ENV",
		"ENV",
		"NODE_ENV",
		"SENTRY_ENABLED",
		"SENTRY_RELEASE",
		"NEXT_PUBLIC_RELEASE_SHA",
		"SENTRY_ENABLE_TRACING",
		"SENTRY_TRACES_SAMPLE_RATE",
		"SENTRY_ENABLE_LOGS",
		"SENTRY_LOG_LEVELS",
	} {
		t.Setenv(key, "")
	}
}

func TestLoadConfigFromEnv_DefaultsDisabledWithoutDSN(t *testing.T) {
	clearConfigEnv(t)
	t.Setenv("SENTRY_ENVIRONMENT", "production")

	cfg := LoadConfigFromEnv("backend")

	require.False(t, cfg.Enabled)
	require.Equal(t, "backend", cfg.Service)
	require.Equal(t, "production", cfg.Environment)
}

func TestLoadConfigFromEnv_EnablesForProductionWithDSN(t *testing.T) {
	clearConfigEnv(t)
	t.Setenv("SENTRY_DSN", "https://public@example.com/1")
	t.Setenv("SENTRY_ENVIRONMENT", "production")

	cfg := LoadConfigFromEnv("backend")

	require.True(t, cfg.Enabled)
	require.Equal(t, "https://public@example.com/1", cfg.DSN)
	require.InDelta(t, 0.05, cfg.TracesSampleRate, 0.0001)
}

func TestLoadConfigFromEnv_ExplicitEnabledOverridesEnvironment(t *testing.T) {
	clearConfigEnv(t)
	t.Setenv("SENTRY_DSN", "https://public@example.com/1")
	t.Setenv("SENTRY_ENVIRONMENT", "development")
	t.Setenv("SENTRY_ENABLED", "true")

	cfg := LoadConfigFromEnv("backend")

	require.True(t, cfg.Enabled)
}

func TestLoadConfigFromEnv_NormalizesProdAlias(t *testing.T) {
	clearConfigEnv(t)
	t.Setenv("SENTRY_DSN", "https://public@example.com/1")
	t.Setenv("ENV", "prod")

	cfg := LoadConfigFromEnv("backend")

	// ENV=prod must both enable Sentry and be reported as "production".
	require.True(t, cfg.Enabled)
	require.Equal(t, "production", cfg.Environment)
}

func TestLoadConfigFromEnv_ExplicitEnabledRequiresDSN(t *testing.T) {
	clearConfigEnv(t)
	t.Setenv("SENTRY_ENVIRONMENT", "production")
	t.Setenv("SENTRY_ENABLED", "true")

	cfg := LoadConfigFromEnv("backend")

	require.False(t, cfg.Enabled)
}

func TestLoadConfigFromEnv_EnvironmentFallbackOrder(t *testing.T) {
	tests := []struct {
		name string
		set  func(t *testing.T)
		want string
	}{
		{
			name: "defaults to development",
			want: "development",
		},
		{
			name: "uses APP_ENV when SENTRY_ENVIRONMENT unset",
			set: func(t *testing.T) {
				t.Setenv("APP_ENV", "staging")
			},
			want: "staging",
		},
		{
			name: "uses ENV when SENTRY_ENVIRONMENT and APP_ENV unset",
			set: func(t *testing.T) {
				t.Setenv("ENV", "qa")
			},
			want: "qa",
		},
		{
			name: "uses NODE_ENV when higher priority vars unset",
			set: func(t *testing.T) {
				t.Setenv("NODE_ENV", "test")
			},
			want: "test",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			clearConfigEnv(t)
			if tt.set != nil {
				tt.set(t)
			}

			cfg := LoadConfigFromEnv("backend")

			require.Equal(t, tt.want, cfg.Environment)
		})
	}
}

func TestLoadConfigFromEnv_ReleaseFallbackOrder(t *testing.T) {
	tests := []struct {
		name string
		set  func(t *testing.T)
		want string
	}{
		{
			name: "SENTRY_RELEASE wins over NEXT_PUBLIC_RELEASE_SHA",
			set: func(t *testing.T) {
				t.Setenv("SENTRY_RELEASE", "backend-release")
				t.Setenv("NEXT_PUBLIC_RELEASE_SHA", "frontend-sha")
			},
			want: "backend-release",
		},
		{
			name: "uses NEXT_PUBLIC_RELEASE_SHA when SENTRY_RELEASE unset",
			set: func(t *testing.T) {
				t.Setenv("NEXT_PUBLIC_RELEASE_SHA", "frontend-sha")
			},
			want: "frontend-sha",
		},
		{
			name: "defaults to dev",
			want: "dev",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			clearConfigEnv(t)
			if tt.set != nil {
				tt.set(t)
			}

			cfg := LoadConfigFromEnv("backend")

			require.Equal(t, tt.want, cfg.Release)
		})
	}
}

func TestLoadConfigFromEnv_ParsesTracingLogsAndLevels(t *testing.T) {
	clearConfigEnv(t)
	t.Setenv("SENTRY_DSN", "https://public@example.com/1")
	t.Setenv("SENTRY_ENVIRONMENT", "staging")
	t.Setenv("SENTRY_ENABLE_TRACING", "false")
	t.Setenv("SENTRY_ENABLE_LOGS", "true")
	t.Setenv("SENTRY_TRACES_SAMPLE_RATE", "0.25")
	t.Setenv("SENTRY_LOG_LEVELS", "warn,error")

	cfg := LoadConfigFromEnv("backend")

	require.True(t, cfg.Enabled)
	require.False(t, cfg.EnableTracing)
	require.True(t, cfg.EnableLogs)
	require.InDelta(t, 0.25, cfg.TracesSampleRate, 0.0001)
	require.Equal(t, []logrus.Level{logrus.WarnLevel, logrus.ErrorLevel}, cfg.LogLevels)
}

func TestLoadConfigFromEnv_InvalidSampleRateFallsBack(t *testing.T) {
	clearConfigEnv(t)
	t.Setenv("SENTRY_DSN", "https://public@example.com/1")
	t.Setenv("SENTRY_ENVIRONMENT", "production")
	t.Setenv("SENTRY_TRACES_SAMPLE_RATE", "NaN")

	cfg := LoadConfigFromEnv("backend")

	require.InDelta(t, 0.05, cfg.TracesSampleRate, 0.0001)
}
