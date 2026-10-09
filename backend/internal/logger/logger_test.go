package logger

import (
	"testing"

	"github.com/sirupsen/logrus"
)

func TestInitLoggerUsesCanonicalProductionEnvAliases(t *testing.T) {
	t.Setenv("NODE_ENV", "")
	t.Setenv("ENV", "")
	t.Setenv("APP_ENV", "prod")

	InitLogger()

	if Logger.GetLevel() != logrus.InfoLevel {
		t.Fatalf("expected production log level info for APP_ENV=prod, got %s", Logger.GetLevel())
	}
}
