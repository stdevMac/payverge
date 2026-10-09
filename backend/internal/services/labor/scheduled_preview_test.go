package labor

import (
	"testing"
	"time"

	"github.com/stdevmac/payverge/backend/internal/database"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type fakeShifts struct {
	lines []database.ScheduleShiftLine
	rates map[database.StaffPositionKey]int64
}

func (f *fakeShifts) GetScheduleShiftLines(_, _ uint) ([]database.ScheduleShiftLine, error) {
	return f.lines, nil
}
func (f *fakeShifts) GetStaffPositionRates(_ uint) (map[database.StaffPositionKey]int64, error) {
	return f.rates, nil
}

type fakeSettings struct {
	s database.BusinessScheduleSettings
}

func (f *fakeSettings) GetOrCreateBusinessScheduleSettings(_ uint) (*database.BusinessScheduleSettings, error) {
	return &f.s, nil
}

type fakeMinors map[uint]bool

func (f fakeMinors) IsMinor(_, staffID uint) bool { return f[staffID] }

func uptr(v uint) *uint { return &v }

func TestPreview_SumsAssignedShiftsAndPctOpenShiftUnpriced(t *testing.T) {
	ws := time.Date(2026, 7, 6, 0, 0, 0, 0, time.UTC) // Monday
	// staff 7 @ position 3: 8h shift, 30m break => 450 worked min @ $20/h (2000c)
	// open shift (StaffID nil) @ position 3: 4h, priced at $0 (no assignee)
	shifts := &fakeShifts{
		lines: []database.ScheduleShiftLine{
			{StaffID: uptr(7), PositionID: 3, StartsAt: ws.Add(9 * time.Hour), EndsAt: ws.Add(17 * time.Hour), BreakMinutes: 30},
			{StaffID: nil, PositionID: 3, StartsAt: ws.Add(18 * time.Hour), EndsAt: ws.Add(22 * time.Hour), BreakMinutes: 0},
		},
		rates: map[database.StaffPositionKey]int64{{StaffID: 7, PositionID: 3}: 2000},
	}
	set := &fakeSettings{s: database.BusinessScheduleSettings{OvertimeWeeklyMinutes: 2400, PostedLeadDays: 7}}
	calc := NewScheduledCalculator(shifts, set, fakeMinors{})

	out, err := calc.Preview(ScheduledPreviewInput{
		BusinessID: 1, ScheduleID: 9, WeekStart: ws,
		Now: ws.AddDate(0, 0, -10), Loc: time.UTC, SalesTargetCents: 100000, // $1000
	})
	require.NoError(t, err)
	// worked min: 450 (assigned) + 240 (open) = 690 total
	assert.Equal(t, 690, out.TotalMinutes)
	// cost: 450 min * 2000c / 60 = 15000c ($150); open shift adds $0
	assert.Equal(t, int64(15000), out.LaborCostCents)
	// labor% = 15000 / 100000 = 0.15
	assert.InDelta(t, 0.15, out.LaborCostPct, 1e-9)
	require.Len(t, out.Lines, 1)
	assert.Equal(t, uint(3), out.Lines[0].PositionID)
	assert.Equal(t, 2, out.Lines[0].ShiftCount)
	assert.Equal(t, 690, out.Lines[0].Minutes)
	assert.Equal(t, int64(15000), out.Lines[0].LaborCostCents)
	assert.Empty(t, out.Warnings) // 690 < 2400; posted 10d early; no minor cutoff
}

func TestOvertimeWarning_FiresOverThreshold(t *testing.T) {
	// staff 7: two 30h-ish shifts → > 2400 weekly min; staff 8: under.
	weekly := map[uint]int{7: 2520, 8: 1200}
	ws := overtimeWarnings(weekly, 2400)
	require.Len(t, ws, 1)
	assert.Equal(t, WarnOvertime, ws[0].Code)
	require.NotNil(t, ws[0].StaffID)
	assert.Equal(t, uint(7), *ws[0].StaffID)
	assert.Equal(t, 2520, ws[0].Detail)
	assert.Empty(t, overtimeWarnings(weekly, 0)) // threshold 0 = disabled
}

func TestPostedLateWarning_DraftNowInsideLeadWindow(t *testing.T) {
	ws := time.Date(2026, 7, 6, 0, 0, 0, 0, time.UTC)
	// draft (PublishedAt nil); now is 3 days out, lead = 7 → late.
	w, late := postedLateWarning(ws, nil, ws.AddDate(0, 0, -3), 7)
	require.True(t, late)
	assert.Equal(t, WarnPostedLate, w.Code)
	assert.Equal(t, 7, w.Detail)
	// now 10 days out → not late.
	_, late2 := postedLateWarning(ws, nil, ws.AddDate(0, 0, -10), 7)
	assert.False(t, late2)
	// already published 9 days out → not late even if "now" is close.
	pub := ws.AddDate(0, 0, -9)
	_, late3 := postedLateWarning(ws, &pub, ws.AddDate(0, 0, -1), 7)
	assert.False(t, late3)
	// lead 0 = disabled.
	_, late4 := postedLateWarning(ws, nil, ws, 0)
	assert.False(t, late4)
}

func TestMinorLateWarning_OnlyMinorsPastCutoff(t *testing.T) {
	d := time.Date(2026, 7, 6, 0, 0, 0, 0, time.UTC)
	cutoff := 22 * 60 // 22:00
	lines := []database.ScheduleShiftLine{
		{StaffID: uptr(7), PositionID: 3, StartsAt: d.Add(17 * time.Hour), EndsAt: d.Add(23 * time.Hour)}, // minor, ends 23:00 → late
		{StaffID: uptr(8), PositionID: 3, StartsAt: d.Add(17 * time.Hour), EndsAt: d.Add(23 * time.Hour)}, // adult, ignored
		{StaffID: uptr(9), PositionID: 3, StartsAt: d.Add(9 * time.Hour), EndsAt: d.Add(15 * time.Hour)},  // minor, ends 15:00 → ok
	}
	isMinor := func(id uint) bool { return id == 7 || id == 9 }
	ws := minorLateWarnings(lines, &cutoff, time.UTC, isMinor)
	require.Len(t, ws, 1)
	assert.Equal(t, WarnMinorLate, ws[0].Code)
	assert.Equal(t, uint(7), *ws[0].StaffID)
	assert.Empty(t, minorLateWarnings(lines, nil, time.UTC, isMinor)) // nil cutoff = off
}
