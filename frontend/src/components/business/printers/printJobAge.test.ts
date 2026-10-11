import {
  printJobAgeChipColor,
  printJobAgeLabel,
  printJobAgeMinutes,
  printJobStatusChipColor,
} from "./RecentPrintJobs";

describe("L3-45 print job age urgency", () => {
  const now = Date.parse("2026-08-05T12:00:00.000Z");

  it("uses humanized minutes for ages >= 1m", () => {
    const iso = new Date(now - 45 * 60_000).toISOString();
    expect(printJobAgeMinutes(iso, now)).toBe(45);
    expect(printJobAgeLabel(iso, now)).toBe("45m");
  });

  it("tones warn/critical by elapsedUrgency thresholds", () => {
    const fresh = new Date(now - 60_000).toISOString();
    const warn = new Date(now - 6 * 60_000).toISOString();
    const crit = new Date(now - 20 * 60_000).toISOString();
    expect(printJobAgeChipColor(fresh, now)).toBe("success");
    expect(printJobAgeChipColor(warn, now)).toBe("warning");
    expect(printJobAgeChipColor(crit, now)).toBe("danger");
  });

  it("keeps a never-printed job loud past the 24h calm band (#833)", () => {
    // Live demo venue: kitchen tickets 1110/1111/1112 sat `routed` for
    // 57h-74h and rendered CALMER than the 21h ones, because every age past
    // `calmAfterMinutes` de-escalates to the neutral "stale" tone. A job that
    // has not printed yet gets no calm band - its age IS the problem.
    const h = (n: number) => new Date(now - n * 60 * 60_000).toISOString();
    expect(printJobAgeChipColor(h(21.8), now, "routed")).toBe("danger");
    expect(printJobAgeChipColor(h(57.4), now, "routed")).toBe("danger");
    expect(printJobAgeChipColor(h(74.6), now, "routed")).toBe("danger");
    expect(printJobAgeChipColor(h(74.6), now, "pending")).toBe("danger");
    expect(printJobAgeChipColor(h(74.6), now, "printing")).toBe("danger");
    expect(printJobAgeChipColor(h(74.6), now, "failed_retryable")).toBe(
      "danger",
    );
  });

  it("lets a settled job age into the calm band", () => {
    // A printed/cancelled/permanently-failed job's age is history, not a
    // queue problem - it must not scream in red forever.
    const h = (n: number) => new Date(now - n * 60 * 60_000).toISOString();
    expect(printJobAgeChipColor(h(180), now, "printed")).toBe("default");
    expect(printJobAgeChipColor(h(30), now, "cancelled")).toBe("default");
    expect(printJobAgeChipColor(h(30), now, "failed")).toBe("default");
    expect(printJobAgeChipColor(h(30), now, "failed_permanent")).toBe(
      "default",
    );
    // Settled but fresh keeps the existing tones.
    expect(printJobAgeChipColor(h(0.4), now, "printed")).toBe("danger");
  });

  it("defaults an unknown status to in-flight, not calm", () => {
    const old = new Date(now - 40 * 60 * 60_000).toISOString();
    expect(printJobAgeChipColor(old, now)).toBe("danger");
  });
});

describe("#833 print job status chip colour", () => {
  it("does not let a failure render like ordinary history", () => {
    expect(printJobStatusChipColor("failed_permanent")).toBe("danger");
    expect(printJobStatusChipColor("failed")).toBe("danger");
    expect(printJobStatusChipColor("failed_retryable")).toBe("warning");
  });

  it("keeps a real print quiet and in-flight work neutral", () => {
    expect(printJobStatusChipColor("printed")).toBe("success");
    expect(printJobStatusChipColor("routed")).toBe("default");
    expect(printJobStatusChipColor("pending")).toBe("default");
    expect(printJobStatusChipColor("printing")).toBe("default");
    expect(printJobStatusChipColor("cancelled")).toBe("default");
    expect(printJobStatusChipColor(null)).toBe("default");
  });
});
