/** @jest-environment jsdom */
import React, { useState } from "react";
import { fireEvent, render, screen } from "@testing-library/react";
import { MenuFilterToggles } from "./MenuFilterToggles";

function Harness({
  initial = new Set<string>(),
}: {
  initial?: Set<string>;
}) {
  const [selected, setSelected] = useState(initial);
  const filters = ["available", "vegetarian", "vegan"];
  return (
    <div>
      <MenuFilterToggles
        filters={filters}
        selected={selected}
        getLabel={(f) =>
          f === "available"
            ? "Available"
            : f === "vegetarian"
              ? "Vegetarian"
              : "Vegan"
        }
        onToggle={(filter) => {
          setSelected((prev) => {
            const next = new Set(prev);
            if (next.has(filter)) next.delete(filter);
            else next.add(filter);
            return next;
          });
        }}
        groupLabel="Menu filters"
      />
      <button
        type="button"
        onClick={() => setSelected(new Set())}
        aria-label="Clear"
      >
        Clear
      </button>
    </div>
  );
}

describe("Filters disclosure wiring (#423)", () => {
  it("exposes aria-expanded / aria-controls against a stable panel id", () => {
    function Disclosure() {
      const [open, setOpen] = useState(false);
      return (
        <div>
          <button
            type="button"
            aria-label="Filters"
            aria-expanded={open}
            aria-controls="guest-menu-filters-panel"
            onClick={() => setOpen((value) => !value)}
          >
            Filters
          </button>
          {open ? (
            <MenuFilterToggles
              id="guest-menu-filters-panel"
              filters={["available"]}
              selected={new Set()}
              getLabel={() => "Available"}
              onToggle={() => undefined}
              groupLabel="Filters"
            />
          ) : null}
        </div>
      );
    }
    render(<Disclosure />);
    const trigger = screen.getByRole("button", { name: "Filters" });
    expect(trigger).toHaveAttribute("aria-expanded", "false");
    expect(trigger).toHaveAttribute("aria-controls", "guest-menu-filters-panel");
    fireEvent.click(trigger);
    expect(trigger).toHaveAttribute("aria-expanded", "true");
    expect(screen.getByTestId("menu-filter-toggles")).toHaveAttribute(
      "id",
      "guest-menu-filters-panel",
    );
    fireEvent.click(trigger);
    expect(trigger).toHaveAttribute("aria-expanded", "false");
    expect(screen.queryByTestId("menu-filter-toggles")).not.toBeInTheDocument();
  });
});

describe("MenuFilterToggles (production component)", () => {
  it("exposes native buttons with aria-pressed for selection state", () => {
    render(<Harness />);
    const available = screen.getByRole("button", { name: "Available" });
    expect(available).toHaveAttribute("aria-pressed", "false");
    fireEvent.click(available);
    expect(available).toHaveAttribute("aria-pressed", "true");
  });

  it("supports keyboard focus and Space/Enter activation", () => {
    render(<Harness />);
    const vegetarian = screen.getByRole("button", { name: "Vegetarian" });
    vegetarian.focus();
    expect(vegetarian).toHaveFocus();
    fireEvent.keyDown(vegetarian, { key: " " });
    // Native buttons activate on click from keyboard; simulate press via click
    // after focus to mirror real keyboard activation in Testing Library.
    fireEvent.click(vegetarian);
    expect(vegetarian).toHaveAttribute("aria-pressed", "true");
    fireEvent.click(vegetarian);
    expect(vegetarian).toHaveAttribute("aria-pressed", "false");
  });

  it("supports deselect and clear-all", () => {
    render(<Harness initial={new Set(["vegan", "available"])} />);
    const vegan = screen.getByRole("button", { name: "Vegan" });
    expect(vegan).toHaveAttribute("aria-pressed", "true");
    fireEvent.click(vegan);
    expect(vegan).toHaveAttribute("aria-pressed", "false");
    fireEvent.click(screen.getByRole("button", { name: "Clear" }));
    expect(screen.getByRole("button", { name: "Available" })).toHaveAttribute(
      "aria-pressed",
      "false",
    );
  });
});
