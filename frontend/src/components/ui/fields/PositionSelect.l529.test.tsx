/** @jest-environment jsdom */
import React from "react";
import { render, screen } from "@testing-library/react";
import { PositionSelect } from "./PositionSelect";

jest.mock("@/i18n/SimpleTranslationProvider", () => ({
  useSimpleLocale: () => ({ locale: "en", setLocale: jest.fn() }),
  // Seeded defaults map through dashboardSchedule.positions.server
  getTranslation: (key: string) => {
    if (key === "dashboardSchedule.positions.server") return "Mesero";
    return key;
  },
}));

describe("PositionSelect L5-29 bilingual defaults", () => {
  it("renders translated seed position names via translatePositionName", () => {
    render(
      <PositionSelect
        label="Position"
        positions={[
          {
            id: 3,
            business_id: 1,
            name: "Server",
            color_hex: "#1a6b6a",
            department: "FOH",
            is_active: true,
            sort_order: 0,
          },
        ]}
        value={3}
        onChange={() => {}}
      />,
    );
    // Trigger value + list item should show Mesero, not raw Server.
    expect(screen.getAllByText("Mesero").length).toBeGreaterThan(0);
    expect(screen.queryByText("Server")).not.toBeInTheDocument();
  });
});
