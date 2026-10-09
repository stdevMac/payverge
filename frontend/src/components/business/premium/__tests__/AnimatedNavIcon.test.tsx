/** @jest-environment jsdom */
import React from "react";
import { render, screen } from "@testing-library/react";
import { Home } from "lucide-react";
import { AnimatedNavIcon } from "../AnimatedNavIcon";

describe("AnimatedNavIcon", () => {
  it("renders the icon with a stable accessible presentation", () => {
    render(
      <AnimatedNavIcon
        icon={Home}
        active
        label="Overview"
        data-testid="overview-icon"
      />,
    );

    const icon = screen.getByTestId("overview-icon");
    expect(icon).toHaveAttribute("aria-hidden", "true");
    expect(icon).toHaveAttribute("data-nav-icon", "true");
    expect(icon).toHaveClass("text-brand");
  });

  it("uses the quiet inactive treatment", () => {
    render(
      <AnimatedNavIcon
        icon={Home}
        active={false}
        label="Overview"
        data-testid="overview-icon"
      />,
    );

    expect(screen.getByTestId("overview-icon")).toHaveClass("text-ink-400");
  });
});
