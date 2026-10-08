/** @jest-environment jsdom */
import React from "react";
import { render, screen } from "@testing-library/react";
import OperatingHoursEditor from "@/components/business/OperatingHoursEditor";

jest.mock("@/i18n/SimpleTranslationProvider", () => {
  const actual = jest.requireActual("@/i18n/getTranslation");
  return {
    useSimpleLocale: () => ({ locale: "en", setLocale: jest.fn() }),
    getTranslation: actual.getTranslation,
  };
});

// getDayName is incidental to this behavior; keep it deterministic.
jest.mock("@/components/business/operatingHoursDayName", () => ({
  getDayName: (d: number) => `Day ${d}`,
}));

type Hour = {
  day_of_week: number;
  open_time: string;
  close_time: string;
  is_closed: boolean;
};

function renderEditor(hours: Hour[]) {
  return render(
    <OperatingHoursEditor
      businessId={1}
      hours={hours as any}
      onHoursChange={jest.fn()}
    />,
  );
}

describe("OperatingHoursEditor — overnight affordance", () => {
  it("shows a '+1 next day' badge when close_time is at/before open_time", () => {
    // A bar open 18:00 → 02:00 stores close <= open (the canonical overnight
    // encoding). The editor should confirm that's an after-midnight close.
    renderEditor([
      { day_of_week: 1, open_time: "18:00", close_time: "02:00", is_closed: false },
    ]);
    expect(screen.getByText(/\+1 next day/i)).toBeInTheDocument();
  });

  it("does not show the badge for a normal same-day window", () => {
    renderEditor([
      { day_of_week: 1, open_time: "09:00", close_time: "17:00", is_closed: false },
    ]);
    expect(screen.queryByText(/\+1 next day/i)).not.toBeInTheDocument();
  });

  it("does not show the badge for a closed day", () => {
    renderEditor([
      { day_of_week: 1, open_time: "18:00", close_time: "02:00", is_closed: true },
    ]);
    // Closed day renders the closed-all-day note, no time inputs/badge.
    expect(screen.queryByText(/\+1 next day/i)).not.toBeInTheDocument();
  });
});
