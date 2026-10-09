/** @jest-environment jsdom */
import React from "react";
import { render, screen } from "@testing-library/react";
import { InventorySkeleton } from "../InventorySkeleton";

describe("InventorySkeleton", () => {
  it("renders an accessible content skeleton (status + aria-busy)", () => {
    render(<InventorySkeleton />);
    const status = screen.getByRole("status");
    expect(status).toHaveAttribute("aria-busy", "true");
  });

  it("renders skeleton placeholder rows, not a bare spinner", () => {
    const { container } = render(<InventorySkeleton />);
    // Content skeletons use animate-pulse on the container, no spinning loader.
    expect(container.querySelector(".animate-pulse")).toBeTruthy();
    expect(container.querySelector(".animate-spin")).toBeNull();
  });
});
