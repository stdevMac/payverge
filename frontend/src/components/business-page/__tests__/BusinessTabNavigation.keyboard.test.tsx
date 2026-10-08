/** @jest-environment jsdom */
import { render, screen, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import BusinessTabNavigation from "../BusinessTabNavigation";

const en = (key: string) =>
  (
    ({
      "businessPage.about": "About",
      "businessPage.menuTab": "Menu",
      "businessPage.contact": "Contact",
      "businessPage.deliveryTab": "Delivery",
      "businessPage.reservations": "Reservations",
      "businessPage.tabsAria": "Business sections",
      "businessPage.mobileNavAria": "Navigation menu",
    }) as Record<string, string>
  )[key] || key;

const es = (key: string) =>
  (
    ({
      "businessPage.about": "Acerca de",
      "businessPage.menuTab": "Menú",
      "businessPage.contact": "Contacto",
      "businessPage.deliveryTab": "Entrega",
      "businessPage.reservations": "Reservas",
      "businessPage.tabsAria": "Secciones del negocio",
      "businessPage.mobileNavAria": "Menú de navegación",
    }) as Record<string, string>
  )[key] || key;

const settings = { primary_color: "#1a6b6a", secondary_color: "#2a8b8a" };

function renderNav(
  t: (key: string) => string,
  activeTab = "about",
  onChangeTab = jest.fn(),
) {
  const view = render(
    <BusinessTabNavigation
      activeTab={activeTab}
      onChangeTab={onChangeTab}
      hasDeliveryTab
      hasReservationsTab
      designSettings={settings}
      t={t}
    />,
  );
  return { onChangeTab, tablist: screen.getByRole("tablist"), ...view };
}

describe("BusinessTabNavigation keyboard tablist", () => {
  it.each([
    ["English", en, "About", "Menu", "Contact"],
    ["Spanish", es, "Acerca de", "Menú", "Contacto"],
  ])(
    "moves focus and selection with arrows, Home, and End (%s)",
    async (_label, t, about, menu, contact) => {
      const user = userEvent.setup();
      let active = "about";
      const onChangeTab = jest.fn((next: string) => {
        active = next;
      });
      const { rerender, tablist } = renderNav(t, "about", onChangeTab);

      const refresh = () =>
        rerender(
          <BusinessTabNavigation
            activeTab={active}
            onChangeTab={onChangeTab}
            hasDeliveryTab
            hasReservationsTab
            designSettings={settings}
            t={t}
          />,
        );

      const aboutTab = within(tablist).getByRole("tab", { name: about });
      aboutTab.focus();
      expect(aboutTab).toHaveAttribute("tabindex", "0");
      within(tablist)
        .getAllByRole("tab")
        .filter((tab) => tab !== aboutTab)
        .forEach((tab) => expect(tab).toHaveAttribute("tabindex", "-1"));

      await user.keyboard("{ArrowRight}");
      refresh();
      const menuTab = within(tablist).getByRole("tab", { name: menu });
      expect(onChangeTab).toHaveBeenLastCalledWith("menu");
      expect(menuTab).toHaveFocus();
      expect(menuTab).toHaveAttribute("aria-selected", "true");
      expect(menuTab).toHaveAttribute("tabindex", "0");
      expect(aboutTab).toHaveAttribute("tabindex", "-1");

      await user.keyboard("{End}");
      refresh();
      const contactTab = within(tablist).getByRole("tab", { name: contact });
      expect(onChangeTab).toHaveBeenLastCalledWith("contact");
      expect(contactTab).toHaveFocus();
      expect(contactTab).toHaveAttribute("aria-selected", "true");

      await user.keyboard("{Home}");
      refresh();
      expect(onChangeTab).toHaveBeenLastCalledWith("about");
      expect(within(tablist).getByRole("tab", { name: about })).toHaveFocus();
    },
  );
});
