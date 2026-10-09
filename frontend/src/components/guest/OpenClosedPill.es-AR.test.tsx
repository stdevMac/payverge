/** @jest-environment jsdom */
import React from "react";
import { render, screen } from "@testing-library/react";
import OpenClosedPill from "./OpenClosedPill";

jest.mock("@/i18n/GuestTranslationProvider", () => ({
  useGuestTranslation: () => ({
    currentLanguage: "es-AR",
    t: (key: string, params?: Record<string, string | number>) => {
      if (key === "landing.statusClosedToday") return "Cerrado hoy";
      if (key === "landing.statusOpenUntil")
        return `Abierto hasta las ${params?.time ?? ""}`;
      if (key === "landing.statusOpensAt")
        return `Abre a las ${params?.time ?? ""}`;
      return key;
    },
  }),
}));

describe("OpenClosedPill es-AR 24-hour hours chip (#949)", () => {
  it("renders 23:00, not 11:00 p. m., for a late close", () => {
    const now = new Date("2024-06-03T16:00:00Z");
    render(
      <OpenClosedPill
        hours={[
          {
            day_of_week: 1,
            open_time: "11:00",
            close_time: "23:00",
            is_closed: false,
          },
        ]}
        timezone="UTC"
        now={now}
      />,
    );
    const pill = screen.getByTestId("landing-open-pill");
    expect(pill).toHaveTextContent(/Abierto hasta las/);
    expect(pill).toHaveTextContent(/23/);
    expect(pill.textContent).not.toMatch(/p\.?\s*m/i);
    expect(pill.textContent).not.toMatch(/11:00/);
  });
});
