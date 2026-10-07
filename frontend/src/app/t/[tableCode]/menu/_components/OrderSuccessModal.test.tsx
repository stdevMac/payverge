/** @jest-environment jsdom */
import React from "react";
import { render, screen } from "@testing-library/react";
import OrderSuccessModal from "./OrderSuccessModal";

describe("OrderSuccessModal", () => {
  it("renders the success message", () => {
    render(
      <OrderSuccessModal
        isOpen={true}
        onClose={jest.fn()}
        message="Your order has been sent to the kitchen"
        tableCode="ABC123"
        t={(key: string) => key}
      />,
    );
    expect(
      screen.getByText("Your order has been sent to the kitchen"),
    ).toBeInTheDocument();
    expect(screen.getByText("menu.orderSubmitted")).toBeInTheDocument();
  });

  it("renders a close button", () => {
    render(
      <OrderSuccessModal
        isOpen={true}
        onClose={jest.fn()}
        message="Done"
        tableCode="ABC123"
        t={(key: string) => key}
      />,
    );
    expect(screen.getAllByRole("button").length).toBeGreaterThan(0);
  });
});
