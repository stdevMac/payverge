/** @jest-environment jsdom */
import React from "react";
import { render, screen, waitFor } from "@testing-library/react";
import { computeAccessibleName } from "dom-accessibility-api";
import { NamedSwitch } from "./NamedSwitch";
import { announcedSwitchName, labelledByTargetsHaveText } from "./namedControl";

describe("NamedSwitch (#446)", () => {
  it("exposes the purpose name plus checked state", () => {
    render(<NamedSwitch name="Auto-assign tables" isSelected />);
    const sw = screen.getByRole("switch", { name: "Auto-assign tables" });
    expect(announcedSwitchName(sw)).toBe("Auto-assign tables, on");
    expect(computeAccessibleName(sw)).toBe("Auto-assign tables");
    expect(labelledByTargetsHaveText(sw)).toBe(true);
  });

  it("announces off when the switch is not selected", () => {
    render(<NamedSwitch name="Show Item Images" isSelected={false} />);
    const sw = screen.getByRole("switch", { name: "Show Item Images" });
    expect(announcedSwitchName(sw)).toBe("Show Item Images, off");
  });

  it("associates the visible title and description", async () => {
    render(
      <div>
        <p id="low-stock-label">Low stock warnings</p>
        <p id="low-stock-desc">Show warning badges near the threshold.</p>
        <NamedSwitch
          name="Low stock warnings"
          labelId="low-stock-label"
          descriptionId="low-stock-desc"
          isSelected
        />
      </div>,
    );
    const sw = screen.getByRole("switch", { name: "Low stock warnings" });
    expect(sw.getAttribute("aria-labelledby")).toBe("low-stock-label");
    // react-aria re-renders the input once its description slot resolves and
    // drops the stamped describedby; the observer restores it a microtask later.
    await waitFor(() =>
      expect(sw.getAttribute("aria-describedby")).toBe("low-stock-desc"),
    );
    expect(labelledByTargetsHaveText(sw)).toBe(true);
    expect(announcedSwitchName(sw)).toBe("Low stock warnings, on");
  });
});
