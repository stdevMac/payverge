/** @jest-environment jsdom */
import React from "react";
import { act, render, screen } from "@testing-library/react";
import { PaymentBlock } from "./_PaymentBlock";
import type { PublicDeliveryTrackingDto } from "@/api/delivery";

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
    awaiting_payment: true,
    payment_mode: "online",
    payment_expires_at: new Date(Date.now() + 2_000).toISOString(),
    bill: {
      id: 7,
      bill_number: "BILL-001",
      status: "open",
      total_amount: 25.5 as any,
      paid: false,
      settlement_address: "0xABC",
      tipping_address: "0xDEF",
    },
    default_currency: "USD",
    display_currency: "USD",
    ...overrides,
  } as PublicDeliveryTrackingDto;
}

const tString = (key: string) => key;

describe("PaymentBlock expiry race", () => {
  afterEach(() => {
    jest.useRealTimers();
  });

  it("hides the Pay Now CTA the moment the countdown expires, before any refetch lands", () => {
    jest.useFakeTimers();
    const onExpired = jest.fn();
    render(
      <PaymentBlock
        tracking={buildTracking()}
        deliveryNumber="DEL-X"
        onExpired={onExpired}
        tString={tString}
      />,
    );
    expect(screen.getByTestId("pay-now-card")).toBeInTheDocument();

    act(() => {
      jest.advanceTimersByTime(2_100);
    });

    expect(screen.queryByTestId("pay-now-card")).not.toBeInTheDocument();
    expect(onExpired).toHaveBeenCalledTimes(1);
  });

  it("shows the CTA again when a fresh payment window arrives", () => {
    jest.useFakeTimers();
    const { rerender } = render(
      <PaymentBlock
        tracking={buildTracking({
          payment_expires_at: new Date(Date.now() + 1_000).toISOString(),
        })}
        deliveryNumber="DEL-X"
        onExpired={jest.fn()}
        tString={tString}
      />,
    );
    act(() => {
      jest.advanceTimersByTime(1_100);
    });
    expect(screen.queryByTestId("pay-now-card")).not.toBeInTheDocument();

    rerender(
      <PaymentBlock
        tracking={buildTracking({
          payment_expires_at: new Date(Date.now() + 60_000).toISOString(),
        })}
        deliveryNumber="DEL-X"
        onExpired={jest.fn()}
        tString={tString}
      />,
    );
    expect(screen.getByTestId("pay-now-card")).toBeInTheDocument();
  });
});
