/** @jest-environment jsdom */
import React, { useState } from "react";
import { fireEvent, render, screen } from "@testing-library/react";
import MenuViewRadiogroup, { type MenuViewMode } from "./MenuViewRadiogroup";

const optionLabels: Record<MenuViewMode, string> = {
  detailed: "Detailed",
  compact: "Compact",
  grid: "Grid",
  "category-tabs": "Category Tabs",
};

function Harness({ initial = "detailed" as MenuViewMode }) {
  const [value, setValue] = useState<MenuViewMode>(initial);
  return (
    <div>
      <MenuViewRadiogroup
        value={value}
        onChange={setValue}
        label="Menu view"
        optionLabels={optionLabels}
      />
      <button type="button">After</button>
    </div>
  );
}

describe("MenuViewRadiogroup (#422)", () => {
  it("exposes one tab stop and moves focus + selection with arrows", () => {
    render(<Harness />);
    const radios = screen.getAllByRole("radio");
    expect(radios).toHaveLength(4);
    expect(radios[0]).toHaveAttribute("aria-checked", "true");
    expect(radios[0]).toHaveAttribute("tabIndex", "0");
    expect(radios[1]).toHaveAttribute("tabIndex", "-1");
    expect(radios[2]).toHaveAttribute("tabIndex", "-1");
    expect(radios[3]).toHaveAttribute("tabIndex", "-1");

    radios[0].focus();
    fireEvent.keyDown(screen.getByRole("radiogroup"), { key: "ArrowRight" });
    expect(radios[1]).toHaveAttribute("aria-checked", "true");
    expect(radios[1]).toHaveFocus();
    expect(radios[1]).toHaveAttribute("tabIndex", "0");
    expect(radios[0]).toHaveAttribute("tabIndex", "-1");

    fireEvent.keyDown(screen.getByRole("radiogroup"), { key: "ArrowRight" });
    fireEvent.keyDown(screen.getByRole("radiogroup"), { key: "ArrowRight" });
    fireEvent.keyDown(screen.getByRole("radiogroup"), { key: "ArrowRight" });
    expect(radios[0]).toHaveAttribute("aria-checked", "true");
    expect(radios[0]).toHaveFocus();
  });

  it("wraps backward and honors Home / End", () => {
    render(<Harness />);
    const radios = screen.getAllByRole("radio");
    radios[0].focus();
    fireEvent.keyDown(screen.getByRole("radiogroup"), { key: "ArrowLeft" });
    expect(radios[3]).toHaveAttribute("aria-checked", "true");
    expect(radios[3]).toHaveFocus();

    fireEvent.keyDown(screen.getByRole("radiogroup"), { key: "Home" });
    expect(radios[0]).toHaveAttribute("aria-checked", "true");
    expect(radios[0]).toHaveFocus();

    fireEvent.keyDown(screen.getByRole("radiogroup"), { key: "End" });
    expect(radios[3]).toHaveAttribute("aria-checked", "true");
    expect(radios[3]).toHaveFocus();
  });

  it("mirrors ArrowLeft / ArrowRight under dir=rtl", () => {
    render(
      <div dir="rtl">
        <Harness />
      </div>,
    );
    const radios = screen.getAllByRole("radio");
    radios[0].focus();
    fireEvent.keyDown(screen.getByRole("radiogroup"), { key: "ArrowLeft" });
    expect(radios[1]).toHaveAttribute("aria-checked", "true");
    expect(radios[1]).toHaveFocus();

    fireEvent.keyDown(screen.getByRole("radiogroup"), { key: "ArrowRight" });
    expect(radios[0]).toHaveAttribute("aria-checked", "true");
    expect(radios[0]).toHaveFocus();

    fireEvent.keyDown(screen.getByRole("radiogroup"), { key: "ArrowRight" });
    expect(radios[3]).toHaveAttribute("aria-checked", "true");
  });

  it("keeps pointer selection working", () => {
    render(<Harness />);
    fireEvent.click(screen.getByRole("radio", { name: "Grid" }));
    expect(screen.getByRole("radio", { name: "Grid" })).toHaveAttribute(
      "aria-checked",
      "true",
    );
  });
});
