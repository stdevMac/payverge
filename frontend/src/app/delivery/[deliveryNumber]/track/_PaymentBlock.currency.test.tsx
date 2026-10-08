/** @jest-environment jsdom */
import React from "react";
import { render, screen, waitFor } from "@testing-library/react";
import { PaymentBlock } from "./_PaymentBlock";
import type { PublicDeliveryTrackingDto } from "@/api/delivery";
import { convertAmount } from "@/api/currency";

jest.mock("@/api/currency", () => {
  const actual = jest.requireActual("@/api/currency");
  return {
    ...actual,
    convertAmount: jest.fn(async (amount: number) => ({
      converted_amount: amount * 2,
    })),
  };
});

jest.mock("@nextui-org/react", () => ({
  Button: ({ children, ...props }: any) => <button {...props}>{children}</button>,
}));

function buildTracking(
  overrides: Partial<PublicDeliveryTrackingDto> = {},
): PublicDeliveryTrackingDto {
  return {
    delivery_number: "DEL-X",
    status: "confirmed",
    business_name: "Test Bistro",
    business_id: 42,
    awaiting_payment: false,
    payment_mode: "cash_on_delivery",
    bill: {
      id: 7,
      bill_number: "BILL-001",
      status: "open",
      total_amount: 10 as any,
      paid: false,
      settlement_address: "0xABC",
      tipping_address: "0xDEF",
    },
    default_currency: "ARS",
    display_currency: "USD",
    ...overrides,
  } as PublicDeliveryTrackingDto;
}

describe("PaymentBlock COD convert-then-format (#499)", () => {
  const tString = (key: string, vars?: Record<string, string | number>) =>
    vars?.amount ? `${key}:${vars.amount}` : key;

  beforeEach(() => {
    (convertAmount as jest.Mock).mockClear();
  });

  it("converts the COD amount before applying the display-currency symbol", async () => {
    render(
      <PaymentBlock
        tracking={buildTracking()}
        deliveryNumber="DEL-X"
        onExpired={jest.fn()}
        tString={tString}
        guestLocale="en"
      />,
    );

    await waitFor(() => {
      expect(screen.getByTestId("cod-row")).toHaveTextContent("$20.00");
    });
    expect(screen.getByTestId("cod-row")).not.toHaveTextContent("$10.00");
    expect(convertAmount).toHaveBeenCalled();
  });

  it("does not convert when display_currency matches default_currency", async () => {
    render(
      <PaymentBlock
        tracking={buildTracking({
          default_currency: "USD",
          display_currency: "USD",
        })}
        deliveryNumber="DEL-X"
        onExpired={jest.fn()}
        tString={tString}
        guestLocale="en"
      />,
    );

    await waitFor(() => {
      expect(screen.getByTestId("cod-row")).toHaveTextContent("$10.00");
    });
    expect(convertAmount).not.toHaveBeenCalled();
  });
});
