package main

import (
	"strings"

	"github.com/stdevmac/payverge/backend/internal/config"
	"github.com/stdevmac/payverge/backend/internal/logger"
	"github.com/stdevmac/payverge/backend/internal/server"
)

// runInstanceStartupChecks validates the instance identity env (PUBLIC_URL,
// brand, contacts). Errors are fatal in production and logged otherwise;
// warnings are always logged. A missing operator inbox is reported once by
// warnOperatorRoutingGaps (emails.WarnIfAdminEmailsUnset), not here.
func runInstanceStartupChecks(productionMode bool) {
	errs, warns := config.ValidateInstance(productionMode)
	for _, w := range warns {
		logger.Logger.Warnf("INSTANCE CONFIG WARNING — %s: %s", w.Field, w.Message)
	}
	if len(errs) == 0 {
		return
	}
	if productionMode {
		logger.Logger.Fatal(config.FormatErrors(errs))
	}
	for _, e := range errs {
		logger.Logger.Warnf("INSTANCE CONFIG ERROR (ignored outside production) — %s: %s", e.Field, e.Message)
	}
}

// instanceRuntimeFromBoot converts the flag+env values main.go resolved at
// boot into the facts GET /api/v1/instance reports. Compose passes RPC_URL as
// --rpc-url, so the handler cannot read these from env alone. emailProvider is
// the transport buildEmailServer actually wired (a hosted provider without a
// key has already been downgraded to "log" there), so it is reported as-is.
func instanceRuntimeFromBoot(rpcURL string, telegramEnabled bool, emailProvider string) server.InstanceRuntime {
	return server.InstanceRuntime{
		EmailProvider:   strings.ToLower(strings.TrimSpace(emailProvider)),
		CryptoEnabled:   strings.TrimSpace(rpcURL) != "",
		TelegramEnabled: telegramEnabled,
	}
}
