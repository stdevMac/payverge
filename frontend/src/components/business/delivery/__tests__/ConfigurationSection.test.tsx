/** @jest-environment jsdom */
import React from "react";
import { render, screen, fireEvent } from "@testing-library/react";
import { ConfigurationSection } from "../ConfigurationSection";
import { asDollars } from "@/types/money";

const t = (k: string) => k;

const baseSettings = {
  flat_delivery_fee: asDollars(3.99),
  free_delivery_minimum: asDollars(25),
  minimum_order_amount: asDollars(10),
  estimated_prep_time: 15,
  max_concurrent_deliveries: 5,
  payment_mode: "cash_on_delivery" as const,
  online_payment_available: true,
};

describe("ConfigurationSection", () => {
  it("renders card title", () => {
    render(
      <ConfigurationSection
        settings={baseSettings}
        onChange={jest.fn()}
        tString={t}
      />,
    );
    expect(
      screen.getByText("focused.configuration.cardTitle"),
    ).toBeInTheDocument();
  });

  it("renders flat fee input with correct value", () => {
    render(
      <ConfigurationSection
        settings={baseSettings}
        onChange={jest.fn()}
        tString={t}
      />,
    );
    expect(screen.getByTestId("delivery-base-fee")).toHaveValue("3.99");
  });

  it("renders estimated prep time and max concurrent inputs", () => {
    const { container } = render(
      <ConfigurationSection
        settings={baseSettings}
        onChange={jest.fn()}
        tString={t}
      />,
    );
    const inputs = container.querySelectorAll('input[type="number"]');
    const values = Array.from(inputs).map((i) => (i as HTMLInputElement).value);
    expect(values).toContain("15"); // prep time
    expect(values).toContain("5"); // max concurrent
  });

  it("does NOT render a delivery_radius input (R17 — removed)", () => {
    const { container } = render(
      <ConfigurationSection
        settings={baseSettings}
        onChange={jest.fn()}
        tString={t}
      />,
    );
    // R17: delivery_radius is gone. Money fees are DecimalInput (type=text);
    // only integer prep/capacity remain type=number (2).
    expect(container.querySelectorAll('input[type="number"]')).toHaveLength(2);
    expect(screen.getByTestId("delivery-base-fee")).toHaveAttribute(
      "type",
      "text",
    );
    expect(screen.getByTestId("delivery-minimum-order")).toHaveAttribute(
      "type",
      "text",
    );
  });

  it("fires onChange when free_delivery_minimum is blurred with a parsed value", () => {
    const onChange = jest.fn();
    render(
      <ConfigurationSection
        settings={baseSettings}
        onChange={onChange}
        tString={t}
      />,
    );
    const freeAbove = screen.getByTestId("delivery-free-above");
    fireEvent.change(freeAbove, { target: { value: "5" } });
    fireEvent.blur(freeAbove);
    expect(onChange).toHaveBeenCalledWith("free_delivery_minimum", 5);
  });

  it("renders payment mode radio group with both options", () => {
    render(
      <ConfigurationSection
        settings={baseSettings}
        onChange={jest.fn()}
        tString={t}
      />,
    );
    expect(
      screen.getByText("focused.configuration.paymentModeTitle"),
    ).toBeInTheDocument();
    // Both radio labels should be present
    expect(
      screen.getByText("focused.configuration.paymentModeOnline"),
    ).toBeInTheDocument();
    expect(
      screen.getByText("focused.configuration.paymentModeCOD"),
    ).toBeInTheDocument();
  });

  it("online radio is disabled when online_payment_available is false", () => {
    render(
      <ConfigurationSection
        settings={{ ...baseSettings, online_payment_available: false }}
        onChange={jest.fn()}
        tString={t}
      />,
    );
    // The disabled description key should render
    expect(
      screen.getByText("focused.configuration.paymentModeOnlineDisabled"),
    ).toBeInTheDocument();
  });

  it("shows mismatch banner when mode is online but payment unavailable", () => {
    render(
      <ConfigurationSection
        settings={{
          ...baseSettings,
          payment_mode: "online",
          online_payment_available: false,
        }}
        onChange={jest.fn()}
        tString={t}
      />,
    );
    expect(
      screen.getByText("focused.configuration.paymentModeMismatch"),
    ).toBeInTheDocument();
  });

  it("does not show mismatch banner when payment is available", () => {
    render(
      <ConfigurationSection
        settings={{
          ...baseSettings,
          payment_mode: "online",
          online_payment_available: true,
        }}
        onChange={jest.fn()}
        tString={t}
      />,
    );
    expect(
      screen.queryByText("focused.configuration.paymentModeMismatch"),
    ).not.toBeInTheDocument();
  });

  // payment_mode_stored follow-up (DEL-OP-2): the in-flight mismatch banner
  // is driven by REAL data — orders whose stored creation-time mode differs
  // from the current settings mode.
  it("shows in-flight mismatch banner when an order was placed under a different stored mode", () => {
    render(
      <ConfigurationSection
        settings={{ ...baseSettings, payment_mode: "online" }}
        onChange={jest.fn()}
        tString={t}
        inFlightOrders={[
          { payment_mode_stored: "cash_on_delivery" },
          { payment_mode_stored: "online" },
        ]}
      />,
    );
    expect(
      screen.getByText("focused.configuration.paymentModeInFlightMismatch"),
    ).toBeInTheDocument();
  });

  it("does not show in-flight mismatch banner when all stored modes match", () => {
    render(
      <ConfigurationSection
        settings={{ ...baseSettings, payment_mode: "cash_on_delivery" }}
        onChange={jest.fn()}
        tString={t}
        inFlightOrders={[
          { payment_mode_stored: "cash_on_delivery" },
          { payment_mode_stored: "cash_on_delivery" },
        ]}
      />,
    );
    expect(
      screen.queryByText("focused.configuration.paymentModeInFlightMismatch"),
    ).not.toBeInTheDocument();
  });

  it("treats empty stored mode as unknown — no mismatch signal (legacy rows)", () => {
    render(
      <ConfigurationSection
        settings={{ ...baseSettings, payment_mode: "online" }}
        onChange={jest.fn()}
        tString={t}
        inFlightOrders={[{ payment_mode_stored: "" }, {}]}
      />,
    );
    expect(
      screen.queryByText("focused.configuration.paymentModeInFlightMismatch"),
    ).not.toBeInTheDocument();
  });

  it("does not show in-flight mismatch banner when no orders are provided", () => {
    render(
      <ConfigurationSection
        settings={baseSettings}
        onChange={jest.fn()}
        tString={t}
      />,
    );
    expect(
      screen.queryByText("focused.configuration.paymentModeInFlightMismatch"),
    ).not.toBeInTheDocument();
  });
});
