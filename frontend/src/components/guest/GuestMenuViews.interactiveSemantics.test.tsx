/** @jest-environment jsdom */
import React from "react";
import { fireEvent, render, screen } from "@testing-library/react";

import { GuestMenuViews } from "./GuestMenuViews";
import type { Business, MenuCategory } from "../../api/business";
import { asDollars } from "@/types/money";

jest.mock("@/i18n/GuestTranslationProvider", () => ({
  useGuestTranslation: () => ({
    currentLanguage: "en",
    t: (key: string) => key,
  }),
}));

jest.mock("./ImageCarousel", () => ({
  ImageCarousel: () => (
    <button type="button" aria-label="carousel control">
      carousel
    </button>
  ),
}));

jest.mock("../common/CurrencyConverter", () => ({
  CurrencyPrice: ({ amount }: { amount: number }) => <span>{amount}</span>,
}));

jest.mock("next/image", () => ({
  __esModule: true,
  // Test stub — not a production image surface.
  // eslint-disable-next-line @next/next/no-img-element
  default: ({ alt = "", ...props }: any) => <img alt={alt} {...props} />,
}));

jest.mock("@nextui-org/react", () => {
  const React = require("react");
  return {
    Button: ({
      children,
      onClick,
      onPress,
      isDisabled,
      disabled,
      "aria-label": ariaLabel,
    }: any) => (
      <button
        type="button"
        onClick={onClick ?? onPress}
        aria-label={ariaLabel}
        disabled={Boolean(isDisabled || disabled)}
      >
        {children}
      </button>
    ),
    Badge: ({ children }: any) => <>{children}</>,
    Chip: ({ children }: any) => <span>{children}</span>,
    // Test stub for NextUI Image — not a production image surface.
    // eslint-disable-next-line @next/next/no-img-element
    Image: ({ alt = "", ...props }: any) => <img alt={alt} {...props} />,
    Modal: ({ children, isOpen }: any) => (isOpen ? <>{children}</> : null),
    ModalContent: ({ children }: any) => (
      <>{typeof children === "function" ? children(() => undefined) : children}</>
    ),
    ModalHeader: ({ children }: any) => <>{children}</>,
    ModalBody: ({ children }: any) => <>{children}</>,
    ModalFooter: ({ children }: any) => <>{children}</>,
    useDisclosure: () => {
      const [isOpen, setOpen] = React.useState(false);
      return {
        isOpen,
        onOpen: () => setOpen(true),
        onClose: () => setOpen(false),
      };
    },
  };
});

const categories = [
  {
    id: "mains",
    name: "Mains",
    description: "Main dishes",
    items: [
      {
        id: "item-1",
        name: "Harvest Bowl",
        description: "Vegetables and grains",
        price: 18.5,
        is_available: true,
        image: "https://images.example/one.jpg",
        images: [
          "https://images.example/one.jpg",
          "https://images.example/two.jpg",
        ],
      },
    ],
  },
] as unknown as MenuCategory[];

const business = { name: "Testaurant" } as unknown as Business;

describe.each(["detailed", "compact", "grid", "category-tabs"] as const)(
  "GuestMenuViews %s interaction structure",
  (viewMode) => {
    it("marks an inventory-out Harvest Bowl as not sellable", () => {
      render(
        <GuestMenuViews
          categories={[
            {
              ...categories[0],
              items: [
                {
                  ...categories[0].items[0],
                  is_available: true,
                  inventory_status: "out_of_stock",
                  dietary_tags: ["vegetarian"],
                  allergens: ["gluten", "sesame"],
                },
              ],
            },
          ]}
          business={business}
          tableCode="T1"
          currentBill={null}
          viewMode={viewMode}
          activeCategory={0}
          onAddToCart={jest.fn()}
          onItemClick={jest.fn()}
          isOrderingEnabled
          itemOrderability={{
            "item-1": { state: "inventory_out", orderable: false },
          }}
        />,
      );

      expect(screen.getAllByText("menu.outOfStock").length).toBeGreaterThan(0);
      expect(screen.queryByText("menu.unavailable")).not.toBeInTheDocument();
      const add = screen
        .queryAllByRole("button")
        .find((control) =>
          (control.getAttribute("aria-label") || "").includes("addItemToCart"),
        );
      if (add) {
        expect(add).toBeDisabled();
      }
    });

    it("surfaces an orderable inventory warning as low stock", () => {
      render(
        <GuestMenuViews
          categories={categories}
          business={business}
          tableCode="T1"
          currentBill={null}
          viewMode={viewMode}
          activeCategory={0}
          onAddToCart={jest.fn()}
          onItemClick={jest.fn()}
          isOrderingEnabled
          itemOrderability={{
            "item-1": { state: "inventory_warning", orderable: true },
          }}
        />,
      );

      expect(screen.getAllByText("menu.lowStock").length).toBeGreaterThan(0);
      expect(
        screen.queryByText("menu.filters.available"),
      ).not.toBeInTheDocument();
    });

    it("keeps details, carousel, and add controls out of button-like ancestors", () => {
      const onAddToCart = jest.fn();
      const onItemClick = jest.fn();
      render(
        <GuestMenuViews
          categories={categories}
          business={business}
          tableCode="T1"
          currentBill={null}
          viewMode={viewMode}
          activeCategory={0}
          onAddToCart={onAddToCart}
          onItemClick={onItemClick}
          isOrderingEnabled
        />,
      );

      const details = screen
        .getAllByRole("button", { name: "accessibility.openItem" })
        .filter((control) => control.tagName === "BUTTON");
      expect(details).toHaveLength(viewMode === "detailed" ? 2 : 1);

      const controls = screen.getAllByRole("button");
      for (const control of controls) {
        expect(
          control.parentElement?.closest('button, [role="button"]'),
        ).toBeNull();
      }

      const add = controls.find((control) => {
        const name =
          control.getAttribute("aria-label") || control.textContent || "";
        return (
          name === "menu.add" ||
          name === "menu.addToCart" ||
          name === "accessibility.addToOrder" ||
          name === "accessibility.addItemToCart" ||
          name === "accessibility.addItemToOrder" ||
          name.includes("addItemToCart") ||
          name.includes("addItemToOrder")
        );
      });
      expect(add).toBeDefined();
      fireEvent.click(add!);
      expect(onAddToCart).toHaveBeenCalledTimes(1);
      expect(onItemClick).not.toHaveBeenCalled();
    });
  },
);

function renderItemWithOption(isOrderingEnabled: boolean) {
  render(
    <GuestMenuViews
      categories={[
        {
          ...categories[0],
          items: [
            {
              ...categories[0].items[0],
              options: [
                {
                  id: "opt-cheese",
                  name: "Extra cheese",
                  price_change: asDollars(1.5),
                  is_required: false,
                },
              ],
            },
          ],
        },
      ]}
      business={business}
      tableCode="T1"
      currentBill={null}
      viewMode="detailed"
      activeCategory={0}
      onAddToCart={jest.fn()}
      onItemClick={jest.fn()}
      isOrderingEnabled={isOrderingEnabled}
    />,
  );
}

describe("GuestMenuViews modifier toggle buttons", () => {
  it("reports aria-pressed and flips it when ordering is enabled", () => {
    renderItemWithOption(true);
    fireEvent.click(
      screen.getAllByRole("button", { name: "accessibility.openItem" })[0],
    );
    const option = screen.getByRole("button", { name: /Extra cheese/ });
    expect(option).toHaveAttribute("aria-pressed", "false");
    fireEvent.click(option);
    expect(option).toHaveAttribute("aria-pressed", "true");
  });

  it("disables the option button when ordering is disabled", () => {
    renderItemWithOption(false);
    fireEvent.click(
      screen.getAllByRole("button", { name: "accessibility.openItem" })[0],
    );
    const option = screen.getByRole("button", { name: /Extra cheese/ });
    expect(option).toBeDisabled();
    expect(option).not.toHaveAttribute("aria-pressed");
  });
});
