/** @jest-environment jsdom */
/**
 * L3-44(b): scorecard must not render two close X controls.
 * hideCloseButton is required on the Modal so only the custom header X remains.
 */
import React from "react";
import { render, screen } from "@testing-library/react";

const modalProps: Record<string, unknown>[] = [];
jest.mock("@nextui-org/react", () => {
  const actual = jest.requireActual("@nextui-org/react");
  return {
    ...actual,
    Modal: ({ children, ...props }: { children: React.ReactNode }) => {
      modalProps.push(props);
      return <div data-testid="scorecard-modal">{children}</div>;
    },
    ModalContent: ({ children }: { children: React.ReactNode | (() => React.ReactNode) }) => (
      <div>{typeof children === "function" ? children() : children}</div>
    ),
    ModalHeader: ({ children }: { children: React.ReactNode }) => (
      <div data-testid="scorecard-header">{children}</div>
    ),
    ModalBody: ({ children }: { children: React.ReactNode }) => <div>{children}</div>,
    ModalFooter: ({ children }: { children: React.ReactNode }) => (
      <div data-testid="scorecard-footer">{children}</div>
    ),
  };
});

jest.mock("@/api/delivery", () => ({
  deliveryApi: {
    getDriverPerformance: jest.fn().mockResolvedValue({
      driver_name: "Ana",
      completed_today: 1,
      completed_week: 2,
      completed_all_time: 9,
      cancelled_count: 0,
      failed_count: 0,
      avg_pickup_minutes: 5,
      avg_delivery_minutes: 12,
      on_time_rate: 0.9,
      average_rating: 4.5,
      active_queue: [],
    }),
  },
}));
jest.mock("@/i18n/SimpleTranslationProvider", () => ({
  useSimpleLocale: () => ({ locale: "en" }),
}));
jest.mock("@/utils/intlLocale", () => ({
  intlLocaleFor: () => "en-US",
}));
jest.mock("@/api/currency", () => ({
  formatCurrency: (n: number) => `$${n}`,
}));

import DriverScorecardModal from "./DriverScorecardModal";

describe("L3-44 DriverScorecardModal single close control", () => {
  beforeEach(() => {
    modalProps.length = 0;
  });

  it("sets hideCloseButton so only the custom header X remains", async () => {
    render(
      <DriverScorecardModal
        isOpen
        onClose={jest.fn()}
        businessId={1}
        driverId={3}
        tString={(k) => k}
      />,
    );
    expect(modalProps.length).toBeGreaterThan(0);
    expect(modalProps[0].hideCloseButton).toBe(true);
    // Custom header close still present
    expect(
      screen.getByLabelText("performance.actions.close"),
    ).toBeInTheDocument();
  });
});
