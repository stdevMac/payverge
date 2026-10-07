/** @jest-environment jsdom */
import React from "react";
import { render, screen } from "@testing-library/react";
import { SelectItem } from "@nextui-org/react";
import { computeAccessibleName } from "dom-accessibility-api";
import { NamedSelect } from "./NamedSelect";
import { labelledByTargetsHaveText } from "./namedControl";

describe("NamedSelect (#446)", () => {
  it("names the trigger by purpose and current value, not value alone", () => {
    render(
      <NamedSelect
        name="Font Family"
        valueLabel="Sans (DM Sans)"
        selectedKeys={["Inter"]}
        disallowEmptySelection
      >
        <SelectItem key="Inter" value="Inter">
          Sans (DM Sans)
        </SelectItem>
        <SelectItem key="Serif" value="Serif">
          Serif (DM Serif Display)
        </SelectItem>
      </NamedSelect>,
    );
    const trigger = screen.getByRole("button", {
      name: "Font Family, Sans (DM Sans)",
    });
    expect(computeAccessibleName(trigger)).toBe("Font Family, Sans (DM Sans)");
    expect(computeAccessibleName(trigger)).not.toBe("Sans (DM Sans)");
    expect(labelledByTargetsHaveText(trigger)).toBe(true);
  });

  it("updates the accessible name when the selected value changes", () => {
    const { rerender } = render(
      <NamedSelect
        name="Menu layout"
        valueLabel="Grid"
        selectedKeys={["grid"]}
        disallowEmptySelection
      >
        <SelectItem key="grid" value="grid">
          Grid
        </SelectItem>
        <SelectItem key="list" value="list">
          List
        </SelectItem>
      </NamedSelect>,
    );
    expect(
      screen.getByRole("button", { name: "Menu layout, Grid" }),
    ).toBeInTheDocument();

    rerender(
      <NamedSelect
        name="Menu layout"
        valueLabel="List"
        selectedKeys={["list"]}
        disallowEmptySelection
      >
        <SelectItem key="grid" value="grid">
          Grid
        </SelectItem>
        <SelectItem key="list" value="list">
          List
        </SelectItem>
      </NamedSelect>,
    );
    expect(
      screen.getByRole("button", { name: "Menu layout, List" }),
    ).toBeInTheDocument();
  });
});
