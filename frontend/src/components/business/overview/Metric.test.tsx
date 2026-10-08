/** @jest-environment jsdom */
import React from "react";
import { render, screen } from "@testing-library/react";
import Metric from "./Metric";

describe("Metric", () => {
  it("renders hero size with serif value", () => {
    render(<Metric size="hero" label="Revenue" value="$1,234" />);
    expect(screen.getByText("Revenue")).toBeInTheDocument();
    expect(screen.getByText("$1,234")).toBeInTheDocument();
  });

  it("renders inline size with icon and label", () => {
    render(
      <Metric
        size="inline"
        label="Bills"
        value="3"
        icon={<span data-testid="icon">i</span>}
      />,
    );
    expect(screen.getByTestId("icon")).toBeInTheDocument();
    expect(screen.getByText("Bills")).toBeInTheDocument();
    expect(screen.getByText("3")).toBeInTheDocument();
  });

  it("renders card size with hint", () => {
    render(<Metric size="card" label="Conversations" value="12" hint="All-time" />);
    expect(screen.getByText("All-time")).toBeInTheDocument();
  });

  it("shows skeleton when loading", () => {
    const { container } = render(
      <Metric size="hero" label="Revenue" value="$1,234" loading />,
    );
    expect(container.querySelector(".animate-pulse")).toBeInTheDocument();
    expect(screen.queryByText("$1,234")).not.toBeInTheDocument();
  });

  it("renders as <button> when onClick provided", () => {
    const onClick = jest.fn();
    render(<Metric size="hero" label="Revenue" value="$1,234" onClick={onClick} />);
    const btn = screen.getByRole("button");
    expect(btn).toBeInTheDocument();
    btn.click();
    expect(onClick).toHaveBeenCalled();
  });

  it("renders as <a> when href provided", () => {
    render(<Metric size="hero" label="Revenue" value="$1,234" href="/analytics" />);
    expect(screen.getByRole("link")).toHaveAttribute("href", "/analytics");
  });

  it("renders as <div> when neither onClick nor href", () => {
    const { container } = render(<Metric size="hero" label="Revenue" value="$1,234" />);
    expect(container.querySelector("button")).toBeNull();
    expect(container.querySelector("a")).toBeNull();
  });

  it("shows positive delta with brand color class", () => {
    const { container } = render(
      <Metric
        size="hero"
        label="Revenue"
        value="$100"
        delta={{ value: "+10%", positive: true }}
      />,
    );
    expect(screen.getByText("+10%")).toBeInTheDocument();
    expect(container.querySelector(".text-brand")).toBeInTheDocument();
  });

  // F-cand-7 / Root B: decorative delta arrows must not inject into the
  // accessible name of the adjacent percentage text.
  it("hides decorative delta arrow SVGs from the accessibility tree [F-cand-7]", () => {
    const { container, rerender } = render(
      <Metric
        size="hero"
        label="Revenue"
        value="$100"
        delta={{ value: "+10%", positive: true }}
      />,
    );
    const up = container.querySelector("svg");
    expect(up).not.toBeNull();
    expect(up).toHaveAttribute("aria-hidden", "true");

    rerender(
      <Metric
        size="hero"
        label="Revenue"
        value="$100"
        delta={{ value: "-5%", positive: false }}
      />,
    );
    const down = container.querySelector("svg");
    expect(down).not.toBeNull();
    expect(down).toHaveAttribute("aria-hidden", "true");
  });

  it("shows negative delta with rose color class", () => {
    const { container } = render(
      <Metric
        size="hero"
        label="Revenue"
        value="$100"
        delta={{ value: "-5%", positive: false }}
      />,
    );
    expect(screen.getByText("-5%")).toBeInTheDocument();
    expect(container.querySelector(".text-rose-700")).toBeInTheDocument();
  });

  it("applies accent variant", () => {
    const { container } = render(
      <Metric size="card" label="Revenue" value="$100" variant="accent" />,
    );
    expect(container.querySelector('[data-variant="accent"]')).toBeInTheDocument();
  });

  it("applies urgent variant", () => {
    const { container } = render(
      <Metric size="card" label="Out of stock" value="3" variant="urgent" />,
    );
    expect(container.querySelector('[data-variant="urgent"]')).toBeInTheDocument();
  });

  it("marks metric cards with premium panel data attributes", () => {
    const { container } = render(
      <Metric size="card" label="Active bills" value="4" />,
    );

    expect(container.querySelector("[data-premium-panel='true']")).toBeTruthy();
  });

  describe("animated value precision", () => {
    it("renders integer values unchanged", () => {
      render(<Metric size="card" label="Conversations" value="12" />);
      expect(screen.getByText("12")).toBeInTheDocument();
    });

    it("preserves the caller's decimal precision", () => {
      render(<Metric size="card" label="Avg messages" value="3.4" />);
      expect(screen.getByText("3.4")).toBeInTheDocument();
    });

    it("preserves trailing-zero precision from toFixed", () => {
      render(<Metric size="card" label="Avg messages" value="6.0" />);
      expect(screen.getByText("6.0")).toBeInTheDocument();
    });

    it("renders non-numeric strings raw (no animation formatting)", () => {
      render(<Metric size="card" label="Upsell rate" value="33.3%" />);
      expect(screen.getByText("33.3%")).toBeInTheDocument();
    });

    it("renders comma-grouped integers via the integer path", () => {
      render(<Metric size="card" label="Messages" value="1,234" />);
      // Commas are normalized away before animating (existing behavior).
      expect(screen.getByText("1234")).toBeInTheDocument();
    });
  });
});
