package main

import (
	"context"
	"flag"
	"fmt"

	"github.com/stdevmac/payverge/backend/internal/database"
)

// runSettingsImageLimits: `server settings image-limits [--daily N]
// [--monthly-alert N]`. With no value flags it prints the effective AI image
// fair-use limits; with either flag it updates them (the other keeps its
// current value). The image hot path reads the setting on every request, so a
// change applies without a restart.
func runSettingsImageLimits(_ context.Context, env *cliEnv, args []string) error {
	fs, dbFlags := newCLIFlagSet(env, "settings image-limits")
	daily := fs.Int("daily", 0, "AI images one business may generate per UTC day (positive)")
	alert := fs.Int("monthly-alert", 0, "monthly per-business count that raises an operator alert (positive)")
	if err := parseCLIFlags(fs, args); err != nil {
		return err
	}
	set := map[string]bool{}
	fs.Visit(func(f *flag.Flag) { set[f.Name] = true })
	if set["daily"] && *daily <= 0 {
		return fmt.Errorf("%w: --daily must be a positive integer", errCLIUsage)
	}
	if set["monthly-alert"] && *alert <= 0 {
		return fmt.Errorf("%w: --monthly-alert must be a positive integer", errCLIUsage)
	}
	conn, err := env.open(*dbFlags)
	if err != nil {
		return err
	}
	store := database.NewDBWithConn(conn)
	current, err := store.GetImageLimitSettings()
	if err != nil {
		return err
	}
	if set["daily"] || set["monthly-alert"] {
		if set["daily"] {
			current.DailyLimit = *daily
		}
		if set["monthly-alert"] {
			current.MonthlyAlert = *alert
		}
		if err := store.SetImageLimitSettings(current); err != nil {
			return fmt.Errorf("update image limit settings: %w", err)
		}
		fmt.Fprintln(env.stdout, "AI image limits updated.")
	}
	fmt.Fprintf(env.stdout, "daily_limit=%d monthly_alert=%d\n", current.DailyLimit, current.MonthlyAlert)
	return nil
}
