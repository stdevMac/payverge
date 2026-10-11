/**
 * @jest-environment jsdom
 */
import React from "react";
import { render, screen } from "@testing-library/react";
import GuestClosedBanner from "./GuestClosedBanner";

jest.mock("@/i18n/GuestTranslationProvider", () => ({
  useGuestTranslation: () => ({
    t: (key: string) =>
      ({
        "menu.businessClosed": "Restaurant is currently closed",
        "menu.businessClosedDescription":
          "You can still browse the menu and pay an open bill.",
      })[key] ?? key,
  }),
}));

describe("GuestClosedBanner", () => {
  it("exposes the closed-hours status region for QA and a11y", () => {
    render(<GuestClosedBanner />);
    const banner = screen.getByTestId("guest-business-closed-banner");
    expect(banner).toHaveAttribute("role", "status");
    expect(banner).toHaveTextContent("Restaurant is currently closed");
    expect(banner).toHaveTextContent(
      "You can still browse the menu and pay an open bill.",
    );
  });
});
