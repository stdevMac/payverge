/** @jest-environment jsdom */
import React from "react";
import { fireEvent, render, screen } from "@testing-library/react";
import { MenuPageHeader } from "./MenuPageHeader";

jest.mock("../../shared/Toolbar", () => {
  const React = jest.requireActual("react");
  function Toolbar({ children }: { children: React.ReactNode }) {
    return React.createElement("div", { "data-testid": "toolbar" }, children);
  }
  function ToolbarViewToggle() {
    return React.createElement("div", { "data-testid": "view-toggle" });
  }
  Toolbar.ViewToggle = ToolbarViewToggle;
  return {
    __esModule: true,
    default: Toolbar,
    ViewToggle: ToolbarViewToggle,
  };
});

jest.mock("@nextui-org/react", () => {
  const React = jest.requireActual("react");
  function MockSelect({
    children,
    onSelectionChange,
    "aria-label": ariaLabel,
  }: {
    children: React.ReactNode;
    onSelectionChange?: (keys: Set<string>) => void;
    "aria-label"?: string;
  }) {
    return React.createElement(
      "div",
      { "data-testid": "availability-filter" },
      React.createElement(
        "button",
        {
          type: "button",
          "aria-label": ariaLabel,
          onClick: () => onSelectionChange?.(new Set()),
        },
        "clear-selection",
      ),
      children,
    );
  }
  function MockSelectItem({ children }: { children: React.ReactNode }) {
    return React.createElement("div", null, children);
  }
  function MockChip({ children }: { children: React.ReactNode }) {
    return React.createElement("span", null, children);
  }
  return {
    Select: MockSelect,
    SelectItem: MockSelectItem,
    Chip: MockChip,
  };
});

describe("MenuPageHeader availability filter", () => {
  it("resets to all when selection is cleared (Esc), never Undefined", () => {
    const setSearchFilter = jest.fn();
    const tString = (key: string) => key;

    render(
      <MenuPageHeader
        tString={tString}
        searchQuery=""
        setSearchQuery={() => {}}
        searchFilter="available"
        setSearchFilter={setSearchFilter}
        viewMode="grid"
        setViewMode={() => {}}
        searchResultsCount={{ totalItems: 3, totalCategories: 1 }}
      />,
    );

    fireEvent.click(
      screen.getByRole("button", { name: /search\.filterByAvailability/i }),
    );
    expect(setSearchFilter).toHaveBeenCalledWith("all");
    expect(setSearchFilter).not.toHaveBeenCalledWith(undefined);
  });
});
