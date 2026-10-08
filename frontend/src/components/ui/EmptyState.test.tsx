/** @jest-environment jsdom */
import React from "react";
import { render, screen } from "@testing-library/react";
import { Receipt } from "lucide-react";
import { EmptyState } from "./EmptyState";

// Panel-mode covers the behavior formerly owned by TabEmptyState (the white
// panel + custom action node + hint). Default mode covers the bare centered
// stack the primitive has always rendered.
describe("EmptyState (panel mode)", () => {
  it("renders title, subtitle, action and hint", () => {
    render(
      <EmptyState
        panel
        icon={Receipt}
        title="No bills yet"
        subtitle="Bills are created automatically when guests start ordering."
        action={<button type="button">Create a bill</button>}
        hint="You can also seat guests from Tables."
      />,
    );
    expect(
      screen.getByRole("heading", { name: "No bills yet" }),
    ).toBeInTheDocument();
    expect(screen.getByRole("heading", { name: "No bills yet" }).tagName).toBe(
      "H3",
    );
    expect(
      screen.getByText(
        "Bills are created automatically when guests start ordering.",
      ),
    ).toBeInTheDocument();
    expect(
      screen.getByRole("button", { name: "Create a bill" }),
    ).toBeInTheDocument();
    expect(
      screen.getByText("You can also seat guests from Tables."),
    ).toBeInTheDocument();
  });

  it("uses a sans-serif title (no font-title serif class)", () => {
    render(
      <EmptyState panel icon={Receipt} title="Empty" subtitle="Nothing here." />,
    );
    const heading = screen.getByRole("heading", { name: "Empty" });
    expect(heading.className).not.toContain("font-title");
    expect(heading.className).toContain("font-semibold");
  });

  it("tightens vertical padding in compact mode", () => {
    render(
      <EmptyState
        panel
        icon={Receipt}
        title="Empty"
        subtitle="Nothing."
        compact
        data-testid="empty"
      />,
    );
    expect(screen.getByTestId("empty").className).toContain("py-8");
  });

  it("uses the md icon tile in compact mode", () => {
    const { container } = render(
      <EmptyState
        panel
        icon={Receipt}
        title="Empty"
        subtitle="Nothing."
        compact
      />,
    );
    const tile = container.querySelector("svg")?.parentElement;
    expect(tile?.className).toContain("h-10 w-10");
    expect(tile?.className).toContain("rounded-xl");
  });
});

describe("EmptyState (default mode)", () => {
  it("renders the built-in button CTA from actionLabel + onAction", () => {
    const onAction = jest.fn();
    render(
      <EmptyState
        icon={Receipt}
        title="No data"
        subtitle="Nothing to show yet."
        actionLabel="Do the thing"
        onAction={onAction}
      />,
    );
    const btn = screen.getByRole("button", { name: "Do the thing" });
    expect(btn).toBeInTheDocument();
    btn.click();
    expect(onAction).toHaveBeenCalledTimes(1);
  });

  it("renders a link CTA from actionLabel + actionHref", () => {
    render(
      <EmptyState
        icon={Receipt}
        title="No data"
        subtitle="Nothing to show yet."
        actionLabel="Go"
        actionHref="/somewhere"
      />,
    );
    const link = screen.getByRole("link", { name: "Go" });
    expect(link).toHaveAttribute("href", "/somewhere");
  });

  it("renders the hint at AA-passing contrast (ink-500, not ink-400)", () => {
    render(
      <EmptyState
        icon={Receipt}
        title="T"
        subtitle="S"
        hint="A quiet footnote"
      />,
    );
    const hint = screen.getByText("A quiet footnote");
    expect(hint.className).toContain("text-ink-500");
    expect(hint.className).not.toContain("text-ink-400");
  });

  it("prefers a custom action node over the built-in button", () => {
    render(
      <EmptyState
        icon={Receipt}
        title="No data"
        subtitle="Nothing to show yet."
        actionLabel="Built-in"
        onAction={() => {}}
        action={<button type="button">Custom</button>}
      />,
    );
    expect(
      screen.getByRole("button", { name: "Custom" }),
    ).toBeInTheDocument();
    expect(
      screen.queryByRole("button", { name: "Built-in" }),
    ).not.toBeInTheDocument();
  });
});
