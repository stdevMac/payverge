/** @jest-environment jsdom */
/**
 * M5 — styled sidebar tooltips replace native `title` chrome: consistent
 * ink-950 bubble, keyboard-visible (focus-within), pointer-transparent, and
 * exposed to AT via role="tooltip" + aria-describedby on the trigger.
 */
import React from "react";
import { fireEvent, render, screen, waitFor } from "@testing-library/react";
import { SidebarTooltip } from "../SidebarTooltip";

describe("SidebarTooltip", () => {
  it("renders the trigger and a tooltip bubble with role=tooltip", () => {
    render(
      <SidebarTooltip label="Kitchen">
        <button type="button">K</button>
      </SidebarTooltip>,
    );
    expect(screen.getByRole("button", { name: "K" })).toBeInTheDocument();
    expect(screen.getByRole("tooltip")).toHaveTextContent("Kitchen");
  });

  it("links trigger and bubble with aria-describedby", () => {
    render(
      <SidebarTooltip label="Kitchen">
        <button type="button">K</button>
      </SidebarTooltip>,
    );
    const trigger = screen.getByRole("button", { name: "K" });
    const tip = screen.getByRole("tooltip");
    expect(tip).toHaveAttribute("id");
    expect(trigger).toHaveAttribute("aria-describedby", tip.id);
  });

  it("appends the description when provided", () => {
    render(
      <SidebarTooltip label="Kitchen" description="Live order queue">
        <button type="button">K</button>
      </SidebarTooltip>,
    );
    expect(screen.getByRole("tooltip")).toHaveTextContent(
      "Kitchen — Live order queue",
    );
  });

  it("keeps the bubble hover/focus-revealed (opacity-0 base) and pointer-transparent", () => {
    render(
      <SidebarTooltip label="Kitchen">
        <button type="button">K</button>
      </SidebarTooltip>,
    );
    const tip = screen.getByRole("tooltip");
    expect(tip.className).toContain("opacity-0");
    expect(tip.className).toContain("group-hover/tip:opacity-100");
    expect(tip.className).toContain("group-focus-within/tip:opacity-100");
    expect(tip.className).toContain("group-has-[[aria-expanded=true]]/tip:!opacity-0");
    expect(tip.className).toContain("group-has-[[data-open=true]]/tip:!opacity-0");
    expect(tip.className).toContain("pointer-events-none");
  });

  it("supports a right-side placement for the collapsed rail", () => {
    render(
      <SidebarTooltip label="Kitchen" side="right">
        <button type="button">K</button>
      </SidebarTooltip>,
    );
    const tip = screen.getByRole("tooltip", { hidden: true });
    expect(tip.className).toContain("fixed");
    expect(tip.className).not.toContain("left-full");
    expect(tip).toHaveAttribute("data-sidebar-tooltip", "portal");
  });

  it("portals the right-side bubble outside overflow-hidden ancestors (#744)", () => {
    const { container } = render(
      <nav className="h-full overflow-hidden" style={{ width: 64 }}>
        <SidebarTooltip label="Kitchen" side="right">
          <button type="button">K</button>
        </SidebarTooltip>
      </nav>,
    );
    const tip = screen.getByRole("tooltip", { hidden: true });
    const nav = container.querySelector("nav");
    expect(nav).toBeTruthy();
    expect(nav!.contains(tip)).toBe(false);
    expect(document.body.contains(tip)).toBe(true);
    expect(tip.className).toContain("bg-ink-950");
    expect(tip.className).toContain("invisible");
  });

  it("reveals the portaled right-side bubble on hover and hides it on leave", () => {
    render(
      <SidebarTooltip label="Kitchen" side="right">
        <button type="button">K</button>
      </SidebarTooltip>,
    );
    const wrap = screen.getByTestId("sidebar-tooltip-wrap");
    const tip = screen.getByRole("tooltip", { hidden: true });
    expect(tip.className).toContain("opacity-0");
    expect(tip.className).toContain("invisible");

    fireEvent.mouseEnter(wrap);
    expect(tip.className).toContain("opacity-100");
    expect(tip.className).toContain("visible");
    expect(tip.className).not.toContain("invisible");

    fireEvent.mouseLeave(wrap);
    expect(tip.className).toContain("opacity-0");
    expect(tip.className).toContain("invisible");
  });

  it("reveals the portaled right-side bubble when the icon button is focused", () => {
    render(
      <SidebarTooltip label="Kitchen" side="right">
        <button type="button">K</button>
      </SidebarTooltip>,
    );
    const button = screen.getByRole("button", { name: "K" });
    const tip = screen.getByRole("tooltip", { hidden: true });
    fireEvent.focus(button);
    expect(tip.className).toContain("opacity-100");
    fireEvent.blur(button);
    expect(tip.className).toContain("opacity-0");
  });

  it("does not reveal the portaled bubble when only the wrap is focused", () => {
    render(
      <SidebarTooltip label="Kitchen" side="right">
        <button type="button">K</button>
      </SidebarTooltip>,
    );
    const wrap = screen.getByTestId("sidebar-tooltip-wrap");
    const tip = screen.getByRole("tooltip", { hidden: true });
    fireEvent.focus(wrap);
    expect(tip.className).toContain("opacity-0");
    expect(tip.className).toContain("invisible");
  });

  it("composes the trigger onFocus so callers still receive it", () => {
    const onFocus = jest.fn();
    render(
      <SidebarTooltip label="Kitchen" side="right">
        <button type="button" onFocus={onFocus}>
          K
        </button>
      </SidebarTooltip>,
    );
    fireEvent.focus(screen.getByRole("button", { name: "K" }));
    expect(onFocus).toHaveBeenCalledTimes(1);
    expect(screen.getByRole("tooltip", { hidden: true }).className).toContain(
      "opacity-100",
    );
  });

  it("hides the portaled bubble when a descendant flyout is open", async () => {
    render(
      <SidebarTooltip label="Kitchen" side="right">
        <button type="button" aria-expanded="false">
          K
        </button>
      </SidebarTooltip>,
    );
    const button = screen.getByRole("button", { name: "K" });
    const tip = screen.getByRole("tooltip", { hidden: true });
    fireEvent.focus(button);
    expect(tip.className).toContain("opacity-100");

    button.setAttribute("aria-expanded", "true");
    await waitFor(() => {
      expect(tip.className).toContain("opacity-0");
      expect(tip.className).toContain("invisible");
    });
  });

  it("hides the portaled bubble when data-open is set on a descendant", async () => {
    render(
      <SidebarTooltip label="Alerts" side="right">
        <button type="button" data-open="false">
          A
        </button>
      </SidebarTooltip>,
    );
    const button = screen.getByRole("button", { name: "A" });
    const tip = screen.getByRole("tooltip", { hidden: true });
    fireEvent.mouseEnter(screen.getByTestId("sidebar-tooltip-wrap"));
    expect(tip.className).toContain("opacity-100");

    button.setAttribute("data-open", "true");
    await waitFor(() => {
      expect(tip.className).toContain("invisible");
    });
  });
});
