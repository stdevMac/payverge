/** @jest-environment jsdom */
import { render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import BusinessTabNavigation from "../BusinessTabNavigation";

const t = (k: string) => k;

describe("BusinessTabNavigation", () => {
   
  const settings = { primary_color: "#1a6b6a", secondary_color: "#2a8b8a" };
   

  it("renders all baseline tabs (about, menu, contact)", () => {
    render(
      <BusinessTabNavigation
        activeTab="menu"
        onChangeTab={() => {}}
        hasDeliveryTab={false}
        hasReservationsTab={false}
        designSettings={settings}
        t={t}
      />,
    );
    expect(screen.getAllByText("businessPage.about").length).toBeGreaterThan(0);
    expect(screen.getAllByText("businessPage.menuTab").length).toBeGreaterThan(0);
    expect(screen.getAllByText("businessPage.contact").length).toBeGreaterThan(0);
  });

  it("contains the desktop tab row so 768px cannot expand the document (#442)", () => {
    const { container } = render(
      <BusinessTabNavigation
        activeTab="menu"
        onChangeTab={() => {}}
        hasDeliveryTab
        hasReservationsTab
        designSettings={settings}
        t={t}
        rightSlot={<button type="button">ES-AR</button>}
      />,
    );

    const tablist = screen.getByRole("tablist");
    expect(tablist.className).toMatch(/\bmin-w-0\b/);
    expect(tablist.className).toMatch(/\boverflow-x-auto\b/);
    expect(tablist.className).toMatch(/\boverflow-y-hidden\b/);
    expect(tablist.getAttribute("aria-label")).toMatch(/tabsAria|Business sections/);

    const bar = container.querySelector("[data-desktop-tab-bar]");
    expect(bar).not.toBeNull();
    expect(bar?.className).toMatch(/\bmin-w-0\b/);
    expect(bar?.className).toMatch(/\boverflow-hidden\b/);

    const slot = container.querySelector("[data-tab-bar-right-slot]");
    expect(slot).not.toBeNull();
    expect(slot?.className).toMatch(/\bshrink-0\b/);
    expect(screen.getByRole("button", { name: "ES-AR" })).toBeInTheDocument();
  });

  it("does not use scale-110 or shadow-3xl on the desktop tabs", () => {
    const { container } = render(
      <BusinessTabNavigation
        activeTab="menu"
        onChangeTab={() => {}}
        hasDeliveryTab
        hasReservationsTab
        designSettings={settings}
        t={t}
      />,
    );
    expect(container.innerHTML).not.toMatch(/\bscale-110\b/);
    expect(container.innerHTML).not.toMatch(/\bshadow-3xl\b/);
  });

  it("calls onChangeTab when a desktop tab is clicked", async () => {
    const onChangeTab = jest.fn();
    const user = userEvent.setup();
    render(
      <BusinessTabNavigation
        activeTab="about"
        onChangeTab={onChangeTab}
        hasDeliveryTab
        hasReservationsTab
        designSettings={settings}
        t={t}
      />,
    );
    const menuTabs = screen.getAllByRole("tab", { name: /menuTab/i });
    await user.click(menuTabs[0]);
    expect(onChangeTab).toHaveBeenCalledWith("menu");
  });

  it("uses a single scrollable tablist (no mobile bottom-sheet picker)", () => {
    render(
      <BusinessTabNavigation
        activeTab="menu"
        onChangeTab={() => {}}
        hasDeliveryTab
        hasReservationsTab
        designSettings={settings}
        t={t}
      />,
    );

    expect(screen.getAllByRole("tablist")).toHaveLength(1);
    expect(screen.queryByRole("dialog")).toBeNull();
    expect(screen.getByRole("tablist").className).toMatch(/\bsnap-x\b/);
  });
});
