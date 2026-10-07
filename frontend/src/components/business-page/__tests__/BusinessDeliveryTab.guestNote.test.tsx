/** @jest-environment jsdom */
import React from "react";
import { render, screen } from "@testing-library/react";
import BusinessDeliveryTab from "../BusinessDeliveryTab";
import type { Dollars } from "@/types/money";

jest.mock("@/components/delivery/DeliveryAvailableCard", () => ({
  __esModule: true,
  default: () => <div data-testid="delivery-available-card" />,
}));

const t = (key: string) => key;

const designSettings = {
  primary_color: "#1a6b6a",
  secondary_color: "#2a8b8a",
};

describe("BusinessDeliveryTab guest note (#718)", () => {
  it("does not render the English dispatch-board seed on the public partner path", () => {
    render(
      <BusinessDeliveryTab
        businessId={86}
        businessName="Demo"
        customUrl="payverge-ai-pro-demo-lounge"
        deliveryEnabled={false}
        deliverySettings={{
          business_id: 86,
          delivery_enabled: true,
          in_house_delivery_enabled: false,
          third_party_enabled: true,
          payment_mode: "online",
          online_payment_available: true,
          flat_delivery_fee: 4.99 as Dollars,
          free_delivery_minimum: 50 as Dollars,
          minimum_order_amount: 15 as Dollars,
          estimated_prep_time: 24,
          max_concurrent_deliveries: 6,
          delivery_hours_same_as_business: true,
          delivery_instructions: "Use the delivery dispatch board for assignment.",
          external_partner_links: [
            { name: "Uber Eats", url: "https://ubereats.com" },
          ],
          zones: [],
          partner_fallback_available: true,
        }}
        deliveryPartnerLinks={[{ name: "Uber Eats", url: "https://ubereats.com" }]}
        designSettings={designSettings}
        onQuoteReady={jest.fn()}
        t={t}
      />,
    );

    expect(
      screen.queryByText(/Use the delivery dispatch board/i),
    ).not.toBeInTheDocument();
    expect(screen.getByText("Uber Eats")).toBeInTheDocument();
  });
});
