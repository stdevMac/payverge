/** @jest-environment jsdom */
import React from "react";
import { render, screen } from "@testing-library/react";

import OpenClosedPill from "./OpenClosedPill";

// Identity-ish guest t() that interpolates {time} so we can assert the
// rendered time string.
jest.mock("@/i18n/GuestTranslationProvider", () => ({
  useGuestTranslation: () => ({
    currentLanguage: "en",
    t: (key: string, params?: Record<string, string | number>) => {
      if (key === "landing.statusClosedToday") return "Closed today";
      if (key === "landing.statusOpenUntil")
        return `Open until ${params?.time ?? ""}`;
      if (key === "landing.statusOpensAt")
        return `Opens at ${params?.time ?? ""}`;
      return key;
    },
  }),
}));

const SUNDAY = 0;
const MONDAY = 1;

describe("OpenClosedPill", () => {
  it("renders nothing when there are no hours", () => {
    const { container } = render(<OpenClosedPill hours={[]} />);
    expect(container).toBeEmptyDOMElement();
  });

  it("shows 'Closed today' when today is marked closed", () => {
    // 2024-06-03 is a Monday.
    const now = new Date("2024-06-03T12:00:00Z");
    render(
      <OpenClosedPill
        hours={[{ day_of_week: MONDAY, open_time: "09:00", close_time: "17:00", is_closed: true }]}
        timezone="UTC"
        now={now}
      />,
    );
    expect(screen.getByText("Closed today")).toBeInTheDocument();
  });

  it("shows 'Open until' with a localized closing time during normal hours", () => {
    const now = new Date("2024-06-03T12:00:00Z"); // Monday 12:00 UTC
    render(
      <OpenClosedPill
        hours={[{ day_of_week: MONDAY, open_time: "09:00", close_time: "17:00" }]}
        timezone="UTC"
        now={now}
      />,
    );
    // Locale-formatted (12h) close time, not raw "17:00".
    expect(screen.getByText(/Open until/)).toBeInTheDocument();
    expect(screen.getByText(/5:00\s?PM/i)).toBeInTheDocument();
    expect(screen.queryByText(/17:00/)).not.toBeInTheDocument();
  });

  it("shows 'Opens at' before opening time", () => {
    const now = new Date("2024-06-03T07:00:00Z"); // Monday 07:00 UTC, before 09:00
    render(
      <OpenClosedPill
        hours={[{ day_of_week: MONDAY, open_time: "09:00", close_time: "17:00" }]}
        timezone="UTC"
        now={now}
      />,
    );
    expect(screen.getByText(/Opens at/)).toBeInTheDocument();
    expect(screen.getByText(/9:00\s?AM/i)).toBeInTheDocument();
  });

  it("is OPEN in the evening for an overnight venue (18:00 -> 02:00)", () => {
    const now = new Date("2024-06-03T20:00:00Z"); // Monday 20:00 UTC
    render(
      <OpenClosedPill
        hours={[{ day_of_week: MONDAY, open_time: "18:00", close_time: "02:00" }]}
        timezone="UTC"
        now={now}
      />,
    );
    // Bug repro: naive logic showed "Opens at 18:00" all evening. Correct
    // logic treats the venue as open.
    expect(screen.getByText(/Open until/)).toBeInTheDocument();
  });

  it("is OPEN after midnight from the previous day's overnight window", () => {
    // 2024-06-03 00:30 UTC is a Monday; the previous day (Sunday) closes at
    // 02:00, so the venue is still open. 00:30 also exercises the midnight-hour
    // normalization (ICU can report hour "24" at midnight).
    const now = new Date("2024-06-03T00:30:00Z");
    render(
      <OpenClosedPill
        hours={[{ day_of_week: SUNDAY, open_time: "18:00", close_time: "02:00" }]}
        timezone="UTC"
        now={now}
      />,
    );
    expect(screen.getByText(/Open until/)).toBeInTheDocument();
  });

  it("uses business timezone, not device clock, so guests abroad see honest closed state", () => {
    // 2024-06-03 15:00 UTC = 11:00 America/New_York (EDT, UTC-4) = open at 11:00
    // but also = 19:00 Asia/Dubai. Without timezone, a diner in Dubai at a
    // later wall-clock could disagree with venue closed banner. Pin now to
    // 07:00 UTC = 03:00 America/New_York (closed) while 07:00 is already
    // morning-ish globally — venue hours 11:00–23:00 NY → Opens at 11:00 AM.
    const now = new Date("2024-06-03T07:00:00Z");
    render(
      <OpenClosedPill
        hours={[
          {
            day_of_week: MONDAY,
            open_time: "11:00",
            close_time: "23:00",
            is_closed: false,
          },
        ]}
        timezone="America/New_York"
        now={now}
      />,
    );
    expect(screen.getByText(/Opens at/)).toBeInTheDocument();
    expect(screen.queryByText(/Open until/)).not.toBeInTheDocument();
  });

  it("shows Open until in business TZ during venue daytime", () => {
    // 16:00 UTC = 12:00 America/New_York → inside 11:00–23:00
    const now = new Date("2024-06-03T16:00:00Z");
    render(
      <OpenClosedPill
        hours={[
          {
            day_of_week: MONDAY,
            open_time: "11:00",
            close_time: "23:00",
            is_closed: false,
          },
        ]}
        timezone="America/New_York"
        now={now}
      />,
    );
    expect(screen.getByText(/Open until/)).toBeInTheDocument();
    expect(screen.getByText(/11:00\s?PM/i)).toBeInTheDocument();
  });
});
