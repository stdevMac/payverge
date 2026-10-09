package handlers

import (
	"bufio"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/stdevmac/payverge/backend/internal/accounting"

	"github.com/stretchr/testify/require"
)

func TestComposeProfitLoss_NetComposition(t *testing.T) {
	start := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	end := time.Date(2026, 1, 8, 0, 0, 0, 0, time.UTC)
	summary := &accounting.Summary{
		StartDate:         start,
		EndDate:           end,
		Currency:          "USD",
		AutoIncomeTotal:   1000,
		ManualIncomeTotal: 100,
		ExpenseTotal:      200,
		PayrollTotal:      150,
		IncomeBreakdown: []accounting.CategoryTotal{
			{Category: "catering", Total: 100},
		},
		ExpenseBreakdown: []accounting.CategoryTotal{
			{Category: "rent", Total: 120},
			{Category: "utilities", Total: 80},
		},
	}
	// Net = 1000 + 100 - 80 cogs - 150 labor - 200 opex = 670
	got := ComposeProfitLoss(summary, 80)
	require.Equal(t, 1000.0, got.Revenue)
	require.Equal(t, 100.0, got.OtherIncome)
	require.Equal(t, 80.0, got.COGS)
	require.Equal(t, 150.0, got.Labor)
	require.Equal(t, 200.0, got.Opex)
	require.InDelta(t, 670.0, got.Net, 0.001)
	require.Len(t, got.OtherIncomeByCategory, 1)
	require.Len(t, got.OpexByCategory, 2)
}

func TestComposeProfitLoss_NilSummary(t *testing.T) {
	got := ComposeProfitLoss(nil, 10)
	require.Equal(t, 0.0, got.Net)
}

// TestNetProfitLossHasOneDefinition locks a single Net P/L definition.
//
// GetSummary historically computed NetProfitLoss without COGS
// (auto + manual − expense − payroll). ComposeProfitLoss / GetProfitLoss
// subtracts estimated food-cost COGS too. For any business with a non-zero
// food-cost estimate those two nets disagreed by exactly the COGS amount.
// Net now lives only on the P&L path — Summary no longer carries a second net.
func TestNetProfitLossHasOneDefinition(t *testing.T) {
	// Component totals as returned by GetSummary for a window.
	auto, manual, expense, payroll := 1000.0, 100.0, 200.0, 150.0
	cogs := 80.0 // non-zero food-cost estimate (AnalyzeWindow.EstimatedCOGS)

	summary := &accounting.Summary{
		AutoIncomeTotal:   auto,
		ManualIncomeTotal: manual,
		ExpenseTotal:      expense,
		PayrollTotal:      payroll,
	}

	// Single definition: ComposeProfitLoss (GetProfitLoss.Current.Net).
	pl := ComposeProfitLoss(summary, cogs)
	wantNet := auto + manual - cogs - payroll - expense // 670
	require.InDelta(t, wantNet, pl.Net, 0.001)

	// The deleted Summary.NetProfitLoss formula (without COGS) would have been
	// auto+manual−expense−payroll = 750 — exactly COGS above the real net.
	// Assert P&L subtracts COGS so Overview/Reports cannot drift again.
	legacyWithoutCOGS := auto + manual - expense - payroll
	require.InDelta(t, cogs, legacyWithoutCOGS-pl.Net, 0.001,
		"P&L net must include COGS; legacy summary net would omit it")
}

// TestNetProfitLossStringNotReintroduced is a source-level gate: the dual
// Summary.net_profit_loss field must not grow back in accounting or the FE.
// (Handlers CSV may still label the net row; git history may still contain it.)
func TestNetProfitLossStringNotReintroduced(t *testing.T) {
	_, thisFile, _, ok := runtime.Caller(0)
	require.True(t, ok)
	// thisFile = backend/internal/handlers/accounting_pnl_test.go → repo root
	repoRoot := filepath.Clean(filepath.Join(filepath.Dir(thisFile), "..", "..", ".."))

	roots := []string{
		filepath.Join(repoRoot, "backend", "internal", "accounting"),
		filepath.Join(repoRoot, "frontend", "src"),
	}
	const banned = "net_profit_loss"
	var hits []string
	for _, root := range roots {
		err := filepath.WalkDir(root, func(path string, d os.DirEntry, walkErr error) error {
			if walkErr != nil {
				return walkErr
			}
			if d.IsDir() {
				switch d.Name() {
				case "node_modules", ".git", "testdata", "coverage":
					return filepath.SkipDir
				}
				return nil
			}
			// Current source only (skip nothing extra — tests in frontend/src must
			// also stop carrying the deleted field).
			ext := strings.ToLower(filepath.Ext(path))
			switch ext {
			case ".go", ".ts", ".tsx", ".js", ".jsx", ".json":
			default:
				return nil
			}
			f, openErr := os.Open(path)
			if openErr != nil {
				return openErr
			}
			defer func() { _ = f.Close() }()
			sc := bufio.NewScanner(f)
			buf := make([]byte, 0, 64*1024)
			sc.Buffer(buf, 1024*1024)
			lineNo := 0
			for sc.Scan() {
				lineNo++
				if strings.Contains(sc.Text(), banned) {
					rel, _ := filepath.Rel(repoRoot, path)
					hits = append(hits, filepath.ToSlash(rel)+":"+strconv.Itoa(lineNo))
				}
			}
			return sc.Err()
		})
		require.NoError(t, err, "walk %s", root)
	}
	if len(hits) > 0 {
		t.Fatalf("%q must not reappear in accounting or frontend/src (found %d):\n  %s",
			banned, len(hits), strings.Join(hits, "\n  "))
	}
}

// TestPreviousPeriodWindow_DSTSafe: the compare=prev window must be re-derived
// on business-local midnights, not by subtracting the current window's raw UTC
// duration — that skews the boundary by an hour when the prior window crosses
// a DST transition (silently gaining/losing an hour of transactions).
func TestPreviousPeriodWindow_DSTSafe(t *testing.T) {
	ny, err := time.LoadLocation("America/New_York")
	require.NoError(t, err)

	// Current window: Mar 9–15 2026 local (US DST began Mar 8 2026).
	start := time.Date(2026, 3, 9, 0, 0, 0, 0, ny).UTC() // EDT midnight
	end := time.Date(2026, 3, 16, 0, 0, 0, 0, ny).UTC()  // exclusive

	prevStart, prevEnd := previousPeriodWindow(start, end, ny)

	// Previous window must be Mar 2–8 local midnights (EST), same 7 calendar days.
	require.Equal(t, time.Date(2026, 3, 2, 0, 0, 0, 0, ny).UTC(), prevStart,
		"prev start must be local midnight Mar 2 EST, not a raw -168h jump")
	require.Equal(t, start, prevEnd)

	// The naive duration subtraction would land at Mar 1 23:00 EST — assert we
	// did NOT do that.
	require.NotEqual(t, start.Add(-(end.Sub(start))), prevStart)

	// No-DST window: plain same-length shift.
	s2 := time.Date(2026, 6, 10, 0, 0, 0, 0, ny).UTC()
	e2 := time.Date(2026, 6, 13, 0, 0, 0, 0, ny).UTC()
	p2s, p2e := previousPeriodWindow(s2, e2, ny)
	require.Equal(t, time.Date(2026, 6, 7, 0, 0, 0, 0, ny).UTC(), p2s)
	require.Equal(t, s2, p2e)
}
