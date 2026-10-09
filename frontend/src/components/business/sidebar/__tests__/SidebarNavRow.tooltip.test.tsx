/** @jest-environment jsdom */
import React from "react";
import { fireEvent, render, screen } from "@testing-library/react";
import { SidebarNavRow } from "../SidebarNavRow";
import { Home } from "lucide-react";

/**
 * #726 — the rail row must not repeat a label the operator can already read.
 *
 * Expanded rows print "Overview" in the row itself, so a hover bubble saying
 * "Overview — Today at a glance" is pure duplication and covers the neighbouring
 * rows during rush. The collapsed desktop rail is the one state where the label
 * is genuinely hidden (`lg:w-0 lg:opacity-0`), so the bubble survives only there.
 */

jest.mock("framer-motion", () => ({
  motion: {
    span: ({ children, ...rest }: any) => <span {...rest}>{children}</span>,
  },
}));
jest.mock("../../premium/useReducedDashboardMotion", () => ({
  useReducedDashboardMotion: () => true,
}));
jest.mock("../../premium/AnimatedNavIcon", () => ({
  AnimatedNavIcon: () => <span data-testid="nav-icon" />,
}));

function renderRow(
  extra: Partial<React.ComponentProps<typeof SidebarNavRow>> = {},
) {
  const props: React.ComponentProps<typeof SidebarNavRow> = {
    label: "Overview",
    description: "Today at a glance",
    icon: Home,
    active: false,
    collapsed: false,
    title: "Overview",
    onClick: jest.fn(),
    ...extra,
  };
  render(<SidebarNavRow {...props} />);
  return screen.getByRole("button");
}

describe("SidebarNavRow — hover tooltip (#726)", () => {
  it("renders no tooltip when the row label is already visible", () => {
    renderRow({ collapsed: false });
    expect(screen.getAllByText("Overview")).toHaveLength(1);
    expect(screen.queryByRole("tooltip", { hidden: true })).toBeNull();
  });

  it("does not point the expanded row at a description bubble", () => {
    const row = renderRow({ collapsed: false });
    expect(row).not.toHaveAttribute("aria-describedby");
    expect(screen.queryByText(/Today at a glance/)).toBeNull();
  });

  it("keeps a label-only bubble for the collapsed icon rail", () => {
    const row = renderRow({
      collapsed: true,
      title: "Tables",
      label: "Tables",
    });
    const tooltip = screen.getByRole("tooltip", { hidden: true });
    expect(tooltip).toHaveTextContent("Tables");
    expect(tooltip).not.toHaveTextContent("Today at a glance");
    expect(row).toHaveAttribute("aria-describedby", tooltip.id);
  });

  it("hides the collapsed bubble below the lg rail, where labels still render", () => {
    renderRow({ collapsed: true });
    // Collapse is desktop-only (`lg:w-16`); the mobile drawer keeps full labels,
    // so the bubble must not surface there on tap-focus.
    expect(screen.getByRole("tooltip", { hidden: true }).className).toContain(
      "max-lg:hidden",
    );
  });

  it("portals the collapsed bubble out of an overflow-hidden nav (#744)", () => {
    const { container } = render(
      <nav className="h-full overflow-hidden relative" style={{ width: 64 }}>
        <SidebarNavRow
          label="Kitchen"
          description="Live order queue"
          icon={Home}
          active={false}
          collapsed
          title="Kitchen"
          onClick={jest.fn()}
        />
      </nav>,
    );
    const tip = screen.getByRole("tooltip", { hidden: true });
    const nav = container.querySelector("nav");
    expect(nav).toBeTruthy();
    expect(nav!.contains(tip)).toBe(false);
    expect(document.body.contains(tip)).toBe(true);
    expect(tip.className).toContain("fixed");
    expect(tip.className).toContain("bg-ink-950");
    expect(tip.className).not.toContain("left-full");
  });

  it("reveals the collapsed bubble when the icon button is focused", () => {
    const row = renderRow({
      collapsed: true,
      title: "Kitchen",
      label: "Kitchen",
    });
    const tip = screen.getByRole("tooltip", { hidden: true });
    fireEvent.focus(row);
    expect(tip.className).toContain("opacity-100");
    fireEvent.blur(row);
    expect(tip.className).toContain("opacity-0");
  });
});
