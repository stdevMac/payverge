import type { StaffAvailability, TimeOffRequest } from "@/api/availability";
import {
  parseTimeToMinutes,
  weekdayForDayKey,
  dayAvailability,
  dayApprovedTimeOff,
  cellPosture,
  detectShiftConflicts,
  formatMinuteRange,
  type TeamAvailability,
} from "./scheduleAvailability";

// Weekday anchors (local): 2024-01-01 is a Monday (getDay 1), so 2024-01-06 is
// Saturday (6) and 2024-01-07 is Sunday (0).
const MON = "2024-01-01";
const TUE = "2024-01-02";
const SAT = "2024-01-06";

function avail(
  staff_id: number,
  weekday: number,
  start_min: number,
  end_min: number,
  kind: "preferred" | "unavailable",
): StaffAvailability {
  return {
    id: Math.floor(start_min + end_min + weekday),
    business_id: 1,
    staff_id,
    weekday,
    start_min,
    end_min,
    kind,
    created_at: "",
    updated_at: "",
  };
}

function timeOff(
  staff_id: number,
  starts_at: string,
  ends_at: string,
  status: TimeOffRequest["status"] = "approved",
): TimeOffRequest {
  return {
    id: 1,
    business_id: 1,
    staff_id,
    starts_at,
    ends_at,
    reason: "vacation",
    status,
    decided_by_staff_id: null,
    decided_at: null,
    created_at: "",
    updated_at: "",
  };
}

describe("parseTimeToMinutes", () => {
  it("parses valid HH:MM", () => {
    expect(parseTimeToMinutes("00:00")).toBe(0);
    expect(parseTimeToMinutes("09:30")).toBe(570);
    expect(parseTimeToMinutes("23:59")).toBe(1439);
  });
  it("rejects malformed or out-of-range input", () => {
    expect(parseTimeToMinutes("24:00")).toBeNull();
    expect(parseTimeToMinutes("9:5")).toBeNull();
    expect(parseTimeToMinutes("bad")).toBeNull();
    expect(parseTimeToMinutes("")).toBeNull();
  });
});

describe("weekdayForDayKey", () => {
  it("maps a day key to JS getDay (0=Sun..6=Sat)", () => {
    expect(weekdayForDayKey(MON)).toBe(1);
    expect(weekdayForDayKey(SAT)).toBe(6);
    expect(weekdayForDayKey("2024-01-07")).toBe(0);
  });
});

describe("dayAvailability", () => {
  const team: TeamAvailability = {
    10: [
      avail(10, 1, 540, 720, "preferred"), // Mon 9–12 preferred
      avail(10, 1, 840, 960, "unavailable"), // Mon 14–16 unavailable
      avail(10, 6, 0, 1440, "unavailable"), // Sat all day off
    ],
  };

  it("returns the windows on that weekday with posture flags", () => {
    const mon = dayAvailability(10, MON, team);
    expect(mon.windows).toHaveLength(2);
    expect(mon.hasPreferred).toBe(true);
    expect(mon.hasUnavailable).toBe(true);
    expect(mon.allDayUnavailable).toBe(false);
  });

  it("detects an all-day unavailable window", () => {
    expect(dayAvailability(10, SAT, team).allDayUnavailable).toBe(true);
  });

  it("is empty for a weekday with no windows and for an open row", () => {
    expect(dayAvailability(10, TUE, team).windows).toHaveLength(0);
    expect(dayAvailability(null, MON, team).windows).toHaveLength(0);
  });
});

describe("dayApprovedTimeOff", () => {
  // Local mid-day instants (no Z) so the day-overlap is TZ-stable against the
  // local dayBounds used by the helper.
  const approved = [timeOff(10, "2024-01-05T12:00:00", "2024-01-08T12:00:00")];

  it("matches a day inside an approved range", () => {
    expect(dayApprovedTimeOff(10, SAT, approved)).not.toBeNull();
  });
  it("does not match a day outside the range", () => {
    expect(dayApprovedTimeOff(10, MON, approved)).toBeNull();
  });
  it("ignores other staff and non-approved requests", () => {
    expect(dayApprovedTimeOff(11, SAT, approved)).toBeNull();
    const pending = [
      timeOff(10, "2024-01-05T12:00:00", "2024-01-08T12:00:00", "pending"),
    ];
    expect(dayApprovedTimeOff(10, SAT, pending)).toBeNull();
  });
});

describe("cellPosture", () => {
  const team: TeamAvailability = {
    10: [
      avail(10, 1, 540, 720, "preferred"),
      avail(10, 2, 840, 960, "unavailable"),
    ],
  };
  const approved = [timeOff(10, "2024-01-06T00:00:00", "2024-01-07T00:00:00")];

  it("prioritizes time-off over availability", () => {
    // Give staff a preferred window on Saturday too; time-off must still win.
    const t2: TeamAvailability = {
      10: [...team[10], avail(10, 6, 540, 720, "preferred")],
    };
    expect(cellPosture(10, SAT, t2, approved).kind).toBe("timeoff");
  });
  it("reports unavailable, then preferred, then none", () => {
    expect(cellPosture(10, TUE, team, []).kind).toBe("unavailable");
    expect(cellPosture(10, MON, team, []).kind).toBe("preferred");
    expect(cellPosture(10, "2024-01-03", team, []).kind).toBe("none"); // Wed: nothing
  });
  it("is 'none' for an open (unassigned) row", () => {
    expect(cellPosture(null, MON, team, approved).kind).toBe("none");
  });
});

describe("detectShiftConflicts", () => {
  const team: TeamAvailability = {
    10: [
      avail(10, 1, 840, 960, "unavailable"), // Mon 14:00–16:00 off
      avail(10, 1, 1380, 1440, "unavailable"), // Mon 23:00–24:00 off
    ],
  };
  const approved = [timeOff(10, "2024-01-06T00:00:00", "2024-01-07T00:00:00")];

  it("returns no conflict when the shift avoids unavailable windows", () => {
    const c = detectShiftConflicts(
      { staffId: 10, dayKey: MON, startTime: "08:00", endTime: "12:00" },
      team,
      approved,
    );
    expect(c).toHaveLength(0);
  });

  it("flags a shift overlapping an unavailable window", () => {
    const c = detectShiftConflicts(
      { staffId: 10, dayKey: MON, startTime: "15:00", endTime: "18:00" },
      team,
      approved,
    );
    expect(c.map((x) => x.kind)).toContain("unavailable");
  });

  it("flags a shift on an approved time-off day", () => {
    const c = detectShiftConflicts(
      { staffId: 10, dayKey: SAT, startTime: "09:00", endTime: "17:00" },
      team,
      approved,
    );
    expect(c.map((x) => x.kind)).toContain("timeoff");
  });

  it("treats an overnight shift as running to end-of-day for weekday overlap", () => {
    const c = detectShiftConflicts(
      { staffId: 10, dayKey: MON, startTime: "23:00", endTime: "01:00" },
      team,
      approved,
    );
    expect(c.map((x) => x.kind)).toContain("unavailable");
  });

  it("returns no conflicts for an open (unassigned) shift", () => {
    expect(
      detectShiftConflicts(
        { staffId: null, dayKey: SAT, startTime: "09:00", endTime: "17:00" },
        team,
        approved,
      ),
    ).toHaveLength(0);
  });

  it("flags a same-staff overlapping existing shift as a double-booking", () => {
    const existing = [
      {
        id: 501,
        schedule_id: 1,
        business_id: 1,
        position_id: 2,
        staff_id: 10,
        starts_at: "2024-01-01T10:00:00",
        ends_at: "2024-01-01T14:00:00",
        break_minutes: 0,
        notes: "",
        created_at: "",
        updated_at: "",
      },
    ] as unknown as Parameters<typeof detectShiftConflicts>[0]["existingShifts"];
    const c = detectShiftConflicts(
      {
        staffId: 10,
        dayKey: MON,
        startTime: "12:00",
        endTime: "16:00",
        existingShifts: existing,
      },
      team,
      approved,
    );
    expect(c.map((x) => x.kind)).toContain("doublebook");
  });

  it("does not double-book against the shift being edited (ignoreShiftId)", () => {
    const existing = [
      {
        id: 501,
        schedule_id: 1,
        business_id: 1,
        position_id: 2,
        staff_id: 10,
        starts_at: "2024-01-01T10:00:00",
        ends_at: "2024-01-01T14:00:00",
        break_minutes: 0,
        notes: "",
        created_at: "",
        updated_at: "",
      },
    ] as unknown as Parameters<typeof detectShiftConflicts>[0]["existingShifts"];
    const c = detectShiftConflicts(
      {
        staffId: 10,
        dayKey: MON,
        startTime: "12:00",
        endTime: "16:00",
        existingShifts: existing,
        ignoreShiftId: 501,
      },
      team,
      approved,
    );
    expect(c.map((x) => x.kind)).not.toContain("doublebook");
  });

  it("ignores a different staff member's overlapping shift", () => {
    const existing = [
      {
        id: 501,
        schedule_id: 1,
        business_id: 1,
        position_id: 2,
        staff_id: 99,
        starts_at: "2024-01-01T10:00:00",
        ends_at: "2024-01-01T14:00:00",
        break_minutes: 0,
        notes: "",
        created_at: "",
        updated_at: "",
      },
    ] as unknown as Parameters<typeof detectShiftConflicts>[0]["existingShifts"];
    const c = detectShiftConflicts(
      {
        staffId: 10,
        dayKey: MON,
        startTime: "12:00",
        endTime: "16:00",
        existingShifts: existing,
      },
      team,
      approved,
    );
    expect(c.map((x) => x.kind)).not.toContain("doublebook");
  });
});

describe("formatMinuteRange", () => {
  it("formats a minute-of-day range", () => {
    const s = formatMinuteRange(540, 1020, "en-US");
    expect(s).toContain("–");
    expect(s.length).toBeGreaterThan(0);
  });
});
