/** @jest-environment jsdom */
import React from "react";
import { fireEvent, render, screen } from "@testing-library/react";
import { Users } from "lucide-react";
import { PremiumSegmentedTabs } from "../PremiumSegmentedTabs";

const tabs = [
  { key: "people", label: "People", icon: Users },
  { key: "positions", label: "Positions" },
  { key: "communication", label: "Communication", badge: 3 },
];

// Controlled harness so keyboard nav can flip the active tab (mirrors how the
// real consumers hold activeKey in state).
function Harness({ initial = "people" }: { initial?: string }) {
  const [activeKey, setActiveKey] = React.useState(initial);
  return (
    <PremiumSegmentedTabs
      tabs={tabs}
      activeKey={activeKey}
      onChange={setActiveKey}
      ariaLabel="Team"
    />
  );
}

describe("PremiumSegmentedTabs", () => {
  it("keeps tab semantics and active state", () => {
    render(
      <PremiumSegmentedTabs
        tabs={tabs}
        activeKey="positions"
        onChange={() => {}}
        ariaLabel="Team"
      />,
    );

    expect(screen.getByRole("tablist", { name: "Team" })).toBeTruthy();
    expect(
      screen.getByRole("tab", { name: /Positions/ }).getAttribute("aria-selected"),
    ).toBe("true");
    expect(
      screen.getByRole("tab", { name: /People/ }).getAttribute("aria-selected"),
    ).toBe("false");
  });

  it("emits the selected key and renders badges", () => {
    const onChange = jest.fn();
    render(
      <PremiumSegmentedTabs
        tabs={tabs}
        activeKey="people"
        onChange={onChange}
      />,
    );

    fireEvent.click(screen.getByRole("tab", { name: /Communication/ }));
    expect(onChange).toHaveBeenCalledWith("communication");
    expect(screen.getByText("3")).toBeTruthy();
  });

  it("keeps the active pill behind labels and inert to clicks", () => {
    const { container } = render(
      <PremiumSegmentedTabs
        tabs={tabs}
        activeKey="people"
        onChange={() => {}}
      />,
    );
    const pill = container.querySelector(
      '[class*="pointer-events-none"][class*="bg-white"]',
    );
    expect(pill).not.toBeNull();
    expect(pill?.className).toMatch(/pointer-events-none/);
    expect(pill?.className).toMatch(/\bz-0\b/);
    // Labels/icons sit above the pill so the spring never clips "Approved".
    expect(
      screen.getByRole("tab", { name: /People/ }).querySelector(".z-10"),
    ).not.toBeNull();
    expect(screen.getByRole("tablist").className).toMatch(/overflow-y-hidden/);
  });

  describe("WAI-ARIA keyboard navigation (roving tabindex)", () => {
    it("gives the active tab tabIndex 0 and inactive tabs tabIndex -1", () => {
      render(
        <PremiumSegmentedTabs
          tabs={tabs}
          activeKey="positions"
          onChange={() => {}}
        />,
      );
      expect(screen.getByRole("tab", { name: /Positions/ }).getAttribute("tabindex")).toBe("0");
      expect(screen.getByRole("tab", { name: /People/ }).getAttribute("tabindex")).toBe("-1");
      expect(screen.getByRole("tab", { name: /Communication/ }).getAttribute("tabindex")).toBe("-1");
    });

    it("ArrowRight selects the next tab and moves focus to it", () => {
      render(<Harness initial="people" />);
      const first = screen.getByRole("tab", { name: /People/ });
      first.focus();
      fireEvent.keyDown(first, { key: "ArrowRight" });
      const positions = screen.getByRole("tab", { name: /Positions/ });
      expect(positions).toHaveAttribute("aria-selected", "true");
      expect(document.activeElement).toBe(positions);
    });

    it("ArrowLeft wraps from the first tab to the last", () => {
      render(<Harness initial="people" />);
      const first = screen.getByRole("tab", { name: /People/ });
      first.focus();
      fireEvent.keyDown(first, { key: "ArrowLeft" });
      const last = screen.getByRole("tab", { name: /Communication/ });
      expect(last).toHaveAttribute("aria-selected", "true");
      expect(document.activeElement).toBe(last);
    });

    it("Home jumps to the first tab and End to the last", () => {
      render(<Harness initial="positions" />);
      const positions = screen.getByRole("tab", { name: /Positions/ });
      positions.focus();
      fireEvent.keyDown(positions, { key: "End" });
      expect(screen.getByRole("tab", { name: /Communication/ })).toHaveAttribute("aria-selected", "true");

      fireEvent.keyDown(screen.getByRole("tab", { name: /Communication/ }), { key: "Home" });
      expect(screen.getByRole("tab", { name: /People/ })).toHaveAttribute("aria-selected", "true");
    });
  });

  describe("scroll affordance (edge fades)", () => {
    // jsdom has no real layout, so scroll metrics are scaffolded via
    // Object.defineProperty on the scroll container (the tablist) and the
    // scroll-event path drives the state updates.
    function setScrollMetrics(
      el: HTMLElement,
      { scrollWidth, clientWidth, scrollLeft }: { scrollWidth: number; clientWidth: number; scrollLeft: number },
    ) {
      Object.defineProperty(el, "scrollWidth", { configurable: true, value: scrollWidth });
      Object.defineProperty(el, "clientWidth", { configurable: true, value: clientWidth });
      Object.defineProperty(el, "scrollLeft", { configurable: true, writable: true, value: scrollLeft });
    }

    it("shows no fades when the tabs fit within the track", () => {
      render(
        <PremiumSegmentedTabs tabs={tabs} activeKey="people" onChange={() => {}} />,
      );
      const strip = screen.getByRole("tablist");
      setScrollMetrics(strip, { scrollWidth: 343, clientWidth: 343, scrollLeft: 0 });
      fireEvent.scroll(strip);
      expect(screen.queryByTestId("segmented-tabs-fade-left")).toBeNull();
      expect(screen.queryByTestId("segmented-tabs-fade-right")).toBeNull();
    });

    it("shows only the right fade when overflowing and scrolled to the start", () => {
      render(
        <PremiumSegmentedTabs tabs={tabs} activeKey="people" onChange={() => {}} />,
      );
      const strip = screen.getByRole("tablist");
      setScrollMetrics(strip, { scrollWidth: 580, clientWidth: 343, scrollLeft: 0 });
      fireEvent.scroll(strip);
      expect(screen.queryByTestId("segmented-tabs-fade-left")).toBeNull();
      expect(screen.getByTestId("segmented-tabs-fade-right")).toBeInTheDocument();
    });

    it("shows both fades mid-scroll, then only the left fade at the end", () => {
      render(
        <PremiumSegmentedTabs tabs={tabs} activeKey="people" onChange={() => {}} />,
      );
      const strip = screen.getByRole("tablist");
      setScrollMetrics(strip, { scrollWidth: 580, clientWidth: 343, scrollLeft: 100 });
      fireEvent.scroll(strip);
      expect(screen.getByTestId("segmented-tabs-fade-left")).toBeInTheDocument();
      expect(screen.getByTestId("segmented-tabs-fade-right")).toBeInTheDocument();

      // Scrolled fully to the end: 580 - 343 = 237.
      Object.defineProperty(strip, "scrollLeft", { configurable: true, writable: true, value: 237 });
      fireEvent.scroll(strip);
      expect(screen.getByTestId("segmented-tabs-fade-left")).toBeInTheDocument();
      expect(screen.queryByTestId("segmented-tabs-fade-right")).toBeNull();
    });
  });

  describe("tablist↔tabpanel ARIA association (idPrefix)", () => {
    it("wires ids on every tab but aria-controls only on the active tab", () => {
      render(
        <PremiumSegmentedTabs
          tabs={tabs}
          activeKey="positions"
          onChange={() => {}}
          idPrefix="deck"
        />,
      );
      // Every tab carries its id (the live panel's aria-labelledby needs the
      // active one; unreferenced ids are harmless), but only the ACTIVE tab
      // points at the single mounted panel — inactive tabs must not reference
      // panel ids that don't exist.
      const people = screen.getByRole("tab", { name: /People/ });
      expect(people.getAttribute("id")).toBe("deck-tab-people");
      expect(people.getAttribute("aria-controls")).toBeNull();

      const positions = screen.getByRole("tab", { name: /Positions/ });
      expect(positions.getAttribute("id")).toBe("deck-tab-positions");
      expect(positions.getAttribute("aria-controls")).toBe("deck-panel-positions");

      const communication = screen.getByRole("tab", { name: /Communication/ });
      expect(communication.getAttribute("id")).toBe("deck-tab-communication");
      expect(communication.getAttribute("aria-controls")).toBeNull();
    });

    it("renders no id or aria-controls when idPrefix is absent (backward compatible)", () => {
      render(
        <PremiumSegmentedTabs
          tabs={tabs}
          activeKey="positions"
          onChange={() => {}}
        />,
      );
      const positions = screen.getByRole("tab", { name: /Positions/ });
      expect(positions.getAttribute("aria-controls")).toBeNull();
      expect(positions.getAttribute("id")).toBeNull();
    });
  });
});
