import {
  buildScheduleSettingsUpdate,
  clampInt,
  formFromSettings,
  hoursToMinutes,
  minuteToTime,
  minutesToHours,
  timeToMinute,
  type ScheduleSettingsForm,
} from "./scheduleSettingsForm";
import type { ScheduleSettings } from "@/api/scheduleSettings";

describe("minute/time conversions", () => {
  test("minuteToTime pads and clamps", () => {
    expect(minuteToTime(0)).toBe("00:00");
    expect(minuteToTime(1320)).toBe("22:00");
    expect(minuteToTime(7 * 60)).toBe("07:00");
    expect(minuteToTime(9999)).toBe("23:59"); // clamped
    expect(minuteToTime(-5)).toBe("00:00"); // clamped
  });

  test("timeToMinute parses and guards", () => {
    expect(timeToMinute("00:00")).toBe(0);
    expect(timeToMinute("22:00")).toBe(1320);
    expect(timeToMinute("07:30")).toBe(450);
    expect(timeToMinute("")).toBe(0);
    expect(timeToMinute("garbage")).toBe(0);
  });

  test("round-trips minute→time→minute", () => {
    for (const m of [0, 1, 60, 450, 1320, 1439]) {
      expect(timeToMinute(minuteToTime(m))).toBe(m);
    }
  });

  test("hours ⇄ minutes", () => {
    expect(minutesToHours(480)).toBe(8);
    expect(minutesToHours(2400)).toBe(40);
    expect(minutesToHours(30)).toBe(0.5);
    expect(hoursToMinutes(8)).toBe(480);
    expect(hoursToMinutes(40)).toBe(2400);
    expect(hoursToMinutes(0.5)).toBe(30);
    expect(hoursToMinutes(9999)).toBe(10080); // clamped to backend max
  });

  test("clampInt rounds and bounds; non-finite → lo", () => {
    expect(clampInt(3.6, 0, 10)).toBe(4);
    expect(clampInt(-2, 0, 6)).toBe(0);
    expect(clampInt(99, 0, 6)).toBe(6);
    expect(clampInt(NaN, 0, 168)).toBe(0);
  });
});

const baseSettings: ScheduleSettings = {
  id: 1,
  business_id: 42,
  week_start_day: 1,
  default_shift_minutes: 480,
  reminder_lead_hours: 3,
  overtime_weekly_minutes: 2400,
  posted_lead_days: 7,
  minor_cutoff_min: null,
  quiet_hours_start_min: null,
  quiet_hours_end_min: null,
  created_at: "",
  updated_at: "",
};

describe("formFromSettings", () => {
  test("maps minutes to hours and null nullable fields to disabled+defaults", () => {
    const f = formFromSettings(baseSettings);
    expect(f.weekStartDay).toBe(1);
    expect(f.shiftHours).toBe(8);
    expect(f.overtimeHours).toBe(40);
    expect(f.quietEnabled).toBe(false);
    expect(f.quietStart).toBe("22:00"); // default seed when disabled
    expect(f.quietEnd).toBe("07:00");
    expect(f.minorEnabled).toBe(false);
    expect(f.minorCutoff).toBe("22:00");
  });

  test("enables nullable fields when the row has values", () => {
    const f = formFromSettings({
      ...baseSettings,
      quiet_hours_start_min: 1350,
      quiet_hours_end_min: 360,
      minor_cutoff_min: 1290,
    });
    expect(f.quietEnabled).toBe(true);
    expect(f.quietStart).toBe("22:30");
    expect(f.quietEnd).toBe("06:00");
    expect(f.minorEnabled).toBe(true);
    expect(f.minorCutoff).toBe("21:30");
  });
});

describe("buildScheduleSettingsUpdate", () => {
  const enabled: ScheduleSettingsForm = {
    weekStartDay: 0,
    shiftHours: 6,
    reminderLeadHours: 4,
    overtimeHours: 44,
    postedLeadDays: 14,
    quietEnabled: true,
    quietStart: "23:00",
    quietEnd: "06:30",
    minorEnabled: true,
    minorCutoff: "21:00",
  };

  test("enabled fields carry minute-of-day; hours convert to minutes", () => {
    const u = buildScheduleSettingsUpdate(enabled);
    expect(u.week_start_day).toBe(0);
    expect(u.default_shift_minutes).toBe(360);
    expect(u.reminder_lead_hours).toBe(4);
    expect(u.overtime_weekly_minutes).toBe(2640);
    expect(u.posted_lead_days).toBe(14);
    expect(u.quiet_hours_start_min).toBe(1380);
    expect(u.quiet_hours_end_min).toBe(390);
    expect(u.minor_cutoff_min).toBe(1260);
  });

  test("disabled nullable fields send -1 (clear-to-NULL sentinel)", () => {
    const u = buildScheduleSettingsUpdate({
      ...enabled,
      quietEnabled: false,
      minorEnabled: false,
    });
    expect(u.quiet_hours_start_min).toBe(-1);
    expect(u.quiet_hours_end_min).toBe(-1);
    expect(u.minor_cutoff_min).toBe(-1);
  });

  test("reminder lead is bounded to 0..168", () => {
    expect(
      buildScheduleSettingsUpdate({ ...enabled, reminderLeadHours: 999 })
        .reminder_lead_hours,
    ).toBe(168);
  });
});
