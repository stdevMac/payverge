/** @jest-environment jsdom */
import React from "react";
import { render, screen, fireEvent } from "@testing-library/react";
import SlashPalette from "../SlashPalette";

describe("SlashPalette", () => {
  const items = [
    "Why was Tuesday slow?",
    "Plan 7-day AOV growth",
    "Top 3 retention actions",
  ];

  it("renders items when open and calls onSelect", () => {
    const onSelect = jest.fn();
    render(<SlashPalette open items={items} onSelect={onSelect} onClose={jest.fn()} />);
    fireEvent.click(screen.getByText("Plan 7-day AOV growth"));
    expect(onSelect).toHaveBeenCalledWith("Plan 7-day AOV growth");
  });

  it("Escape closes the palette", () => {
    const onClose = jest.fn();
    const root = document.createElement("div");
    document.body.appendChild(root);
    const containerRef = { current: root };
    render(
      <SlashPalette
        open
        items={items}
        onSelect={jest.fn()}
        onClose={onClose}
        containerRef={containerRef}
      />,
      { container: root },
    );
    fireEvent.keyDown(root, { key: "Escape" });
    expect(onClose).toHaveBeenCalled();
  });

  it("Arrow keys move focus and Enter selects when scoped to container (L4-12d)", () => {
    const onSelect = jest.fn();
    const root = document.createElement("div");
    document.body.appendChild(root);
    const containerRef = { current: root };
    render(
      <SlashPalette
        open
        items={items}
        onSelect={onSelect}
        onClose={jest.fn()}
        containerRef={containerRef}
      />,
      { container: root },
    );
    fireEvent.keyDown(root, { key: "ArrowDown" });
    fireEvent.keyDown(root, { key: "Enter" });
    expect(onSelect).toHaveBeenCalledWith("Plan 7-day AOV growth");
  });

  it("returns null when closed", () => {
    const { container } = render(<SlashPalette open={false} items={items} onSelect={jest.fn()} onClose={jest.fn()} />);
    expect(container.firstChild).toBeNull();
  });

  it("uses the provided ariaLabel on the listbox", () => {
    render(
      <SlashPalette
        open
        items={items}
        onSelect={jest.fn()}
        onClose={jest.fn()}
        ariaLabel="Comandos"
      />,
    );
    expect(screen.getByRole("listbox", { name: "Comandos" })).toBeInTheDocument();
  });
});
