/** @jest-environment jsdom */
/**
 * D1 / L1-12: enable/disable confirmation ModalContent must be opaque white
 * (bg-white), not translucent bg-white/95 or /85 — structural ghosting was Root B.
 * Assert the mounted modal class string, not a source grep alone.
 */
import React from "react";
import { render, screen, fireEvent } from "@testing-library/react";

jest.mock("react-hot-toast", () => ({
  __esModule: true,
  default: { error: jest.fn(), success: jest.fn() },
}));

jest.mock("@/i18n/SimpleTranslationProvider", () => ({
  useSimpleLocale: () => ({ locale: "en", setLocale: jest.fn() }),
  getTranslation: (key: string) => key,
}));

jest.mock("@/api/kitchenOrders", () => ({
  getKitchenOrdersStatus: jest.fn(() =>
    Promise.resolve({ kitchen_enabled: true, orders_enabled: true }),
  ),
  toggleKitchenAndOrders: jest.fn(() => Promise.resolve()),
}));

// Forward ModalContent className so opacity classes are observable in DOM.
jest.mock("@nextui-org/react", () => {
  const React = require("react");
  return {
    Modal: ({
      isOpen,
      children,
    }: {
      isOpen: boolean;
      children: React.ReactNode;
    }) =>
      isOpen ? (
        <div role="dialog" data-testid="kitchen-orders-modal">
          {children}
        </div>
      ) : null,
    ModalContent: ({
      className,
      children,
    }: {
      className?: string;
      children: React.ReactNode | (() => React.ReactNode);
    }) => (
      <div data-testid="kitchen-orders-modal-content" className={className}>
        {typeof children === "function" ? children() : children}
      </div>
    ),
    ModalHeader: ({ children }: { children: React.ReactNode }) => (
      <div>{children}</div>
    ),
    ModalBody: ({ children }: { children: React.ReactNode }) => (
      <div>{children}</div>
    ),
    ModalFooter: ({ children }: { children: React.ReactNode }) => (
      <div>{children}</div>
    ),
    Button: ({
      children,
      onPress,
      isLoading: _isLoading,
      ...rest
    }: {
      children: React.ReactNode;
      onPress?: () => void;
      isLoading?: boolean;
    }) => (
      <button type="button" onClick={onPress} {...rest}>
        {children}
      </button>
    ),
  };
});

import { KitchenOrdersToggle } from "@/components/business/KitchenOrdersToggle";

describe("KitchenOrdersToggle L1-12 opaque ModalContent (D1)", () => {
  it("enable modal content is opaque bg-white, not bg-white/9x", async () => {
    render(
      <KitchenOrdersToggle
        businessId={1}
        isLocked={false}
        variant="button"
        externalEnabled={false}
        externalLoading={false}
      />,
    );

    fireEvent.click(
      screen.getByText("kitchenOrdersToggle.button.enableOrdering"),
    );

    const content = await screen.findByTestId("kitchen-orders-modal-content");
    const cls = content.className || "";
    expect(cls).toMatch(/\bbg-white\b/);
    expect(cls).not.toMatch(/bg-white\/9[05]/);
    expect(cls).not.toMatch(/bg-white\/85/);
  });

  it("disable modal content is opaque bg-white when ordering is on", async () => {
    render(
      <KitchenOrdersToggle
        businessId={1}
        isLocked={false}
        variant="button"
        externalEnabled={true}
        externalLoading={false}
      />,
    );

    fireEvent.click(
      screen.getByText("kitchenOrdersToggle.button.disableOrdering"),
    );

    const content = await screen.findByTestId("kitchen-orders-modal-content");
    const cls = content.className || "";
    expect(cls).toMatch(/\bbg-white\b/);
    expect(cls).not.toMatch(/bg-white\/9[05]/);
  });
});
