/** @jest-environment jsdom */
import React from "react";
import { render, screen, fireEvent } from "@testing-library/react";
import Toolbar, { ViewToggle } from "./Toolbar";

describe("Toolbar", () => {
  it("renders a search input with the placeholder and fires onChange", () => {
    const onChange = jest.fn();
    render(
      <Toolbar
        search={{
          value: "",
          onChange,
          placeholder: "Search tables",
          ariaLabel: "Search tables",
        }}
      />,
    );

    const input = screen.getByPlaceholderText("Search tables");
    expect(input).toHaveValue("");
    fireEvent.change(input, { target: { value: "patio" } });
    expect(onChange).toHaveBeenCalledWith("patio");
  });

  it("reflects the controlled search value", () => {
    render(
      <Toolbar
        search={{ value: "burger", onChange: () => {}, placeholder: "Search" }}
      />,
    );
    expect(screen.getByPlaceholderText("Search")).toHaveValue("burger");
  });

  it("hides the leading search icon from the accessibility tree", () => {
    const { container } = render(
      <Toolbar
        search={{ value: "", onChange: () => {}, placeholder: "Search" }}
      />,
    );
    expect(container.querySelector("svg")).toHaveAttribute(
      "aria-hidden",
      "true",
    );
  });

  it("renders right-slot children (filters, actions)", () => {
    render(
      <Toolbar
        search={{ value: "", onChange: () => {}, placeholder: "Search" }}
      >
        <button type="button">Refresh</button>
      </Toolbar>,
    );
    expect(screen.getByRole("button", { name: "Refresh" })).toBeInTheDocument();
  });
});

describe("ViewToggle", () => {
  it("marks the active option with the tinted treatment and aria-pressed", () => {
    render(<ViewToggle value="grid" onChange={() => {}} />);

    const grid = screen.getByRole("button", { name: "Grid view" });
    const list = screen.getByRole("button", { name: "List view" });

    expect(grid).toHaveAttribute("aria-pressed", "true");
    expect(list).toHaveAttribute("aria-pressed", "false");
    expect(grid.className).toContain("bg-brand/10");
    expect(grid.className).toContain("text-brand");
    // Regression: the active recipe must not carry the ghost idle color —
    // both utilities on one element would leave the winner to stylesheet
    // order (no tailwind-merge in this repo).
    expect(grid.className).not.toContain("text-ink-400");
    expect(list.className).not.toContain("bg-brand/10");
  });

  it("exposes labelled group semantics for the toggle pair", () => {
    render(<ViewToggle value="list" onChange={() => {}} />);
    expect(screen.getByRole("group", { name: "View" })).toBeInTheDocument();
    expect(screen.getByRole("button", { name: "List view" })).toHaveAttribute(
      "aria-pressed",
      "true",
    );
  });

  it("fires onChange with the clicked value", () => {
    const onChange = jest.fn();
    render(<ViewToggle value="grid" onChange={onChange} />);

    fireEvent.click(screen.getByRole("button", { name: "List view" }));
    expect(onChange).toHaveBeenCalledWith("list");
  });

  it("uses caller-provided labels for the option accessible names", () => {
    render(
      <ViewToggle
        value="grid"
        onChange={() => {}}
        labels={{ grid: "Cuadrícula", list: "Lista" }}
      />,
    );
    expect(
      screen.getByRole("button", { name: "Cuadrícula" }),
    ).toBeInTheDocument();
    expect(screen.getByRole("button", { name: "Lista" })).toBeInTheDocument();
  });
});
