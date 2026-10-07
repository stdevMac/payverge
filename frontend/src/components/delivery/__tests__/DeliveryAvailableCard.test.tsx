/** @jest-environment jsdom */
import React from "react";
import { render, screen, fireEvent, waitFor } from "@testing-library/react";
import DeliveryAvailableCard from "../DeliveryAvailableCard";
import { GuestTranslationContext } from "@/i18n/GuestTranslationProvider";
import type { DeliverySettingsDto } from "@/api/delivery";
import type { GuestDeliveryQuoteContext } from "../GuestDeliveryOrder";

// Mock child modal so tests stay focused on the card
jest.mock("../GuestDeliveryOrder", () => {
  const MockGuestDeliveryOrder = ({
    isOpen,
    onQuoteReady,
    onClose,
    initialContext,
  }: {
    isOpen: boolean;
    onQuoteReady?: (ctx: GuestDeliveryQuoteContext) => void;
    onClose: () => void;
    initialContext?: GuestDeliveryQuoteContext | null;
  }) => {
    if (!isOpen) return null;
    const address = initialContext?.delivery_address;
    return (
      <div data-testid="guest-delivery-modal">
        <span data-testid="prefill-street">{address?.street ?? ""}</span>
        <span data-testid="prefill-city">{address?.city ?? ""}</span>
        <span data-testid="prefill-state">{address?.state ?? ""}</span>
        <span data-testid="prefill-postal">{address?.postal_code ?? ""}</span>
        <span data-testid="prefill-country">{address?.country ?? ""}</span>
        <span data-testid="prefill-apartment">{address?.apartment ?? ""}</span>
        <span data-testid="prefill-instructions">
          {initialContext?.delivery_instructions ?? ""}
        </span>
        <span data-testid="prefill-contactless">
          {String(Boolean(initialContext?.contactless_delivery))}
        </span>
        <span data-testid="prefill-leave-at-door">
          {String(Boolean(initialContext?.leave_at_door))}
        </span>
        <button
          type="button"
          data-testid="mock-quote-ready"
          onClick={() =>
            onQuoteReady?.({
              delivery_address: {
                street: "1 Main St",
                city: "Dubai",
                country: "AE",
                formatted_address: "1 Main St, Dubai, AE",
              },
              contactless_delivery: false,
              leave_at_door: false,
              quote: {
                eligible: true,
                reason_code: "quote_available",
                message: "Delivery is available",
                delivery_fee: 5.99 as any,
                minimum_order_amount: 20 as any,
                free_delivery_minimum: 50 as any,
                order_subtotal: 0 as any,
                meets_minimum: true,
                estimated_prep_time: 15,
                estimated_delivery_minutes: 25,
                estimated_total_minutes: 40,
                fulfillment_mode: "delivery",
                partner_fallback_available: false,
                external_partner_links: [],
              },
            })
          }
        >
          Trigger quote ready
        </button>
        <button type="button" onClick={onClose}>
          Close modal
        </button>
      </div>
    );
  };
  MockGuestDeliveryOrder.displayName = "MockGuestDeliveryOrder";
  // Also export the interface so imports succeed
  return {
    __esModule: true,
    default: MockGuestDeliveryOrder,
  };
});

jest.mock("@nextui-org/react", () => ({
  Card: ({ children, ...props }: any) => <div {...props}>{children}</div>,
  CardBody: ({ children }: any) => <div>{children}</div>,
  Button: ({
    children,
    onPress,
    isDisabled,
    endContent: _endContent,
    classNames: _classNames,
    ...props
  }: any) => (
    <button
      type="button"
      onClick={onPress}
      disabled={isDisabled}
      {...props}
    >
      {children}
    </button>
  ),
  Chip: ({ children, ...props }: any) => <span {...props}>{children}</span>,
  Skeleton: () => <div data-testid="skeleton" />,
}));

const makeSettings = (overrides: Partial<DeliverySettingsDto> = {}): DeliverySettingsDto => ({
  business_id: 1,
  delivery_enabled: true,
  in_house_delivery_enabled: true,
  third_party_enabled: false,
  payment_mode: "cash_on_delivery",
  online_payment_available: false,
  flat_delivery_fee: 5.99 as any,
  free_delivery_minimum: 50 as any,
  minimum_order_amount: 20 as any,
  estimated_prep_time: 15,
  max_concurrent_deliveries: 10,
  delivery_hours_same_as_business: true,
  external_partner_links: [],
  zones: [],
  partner_fallback_available: false,
  ...overrides,
});

describe("DeliveryAvailableCard", () => {
  it("wraps a long Spanish CTA instead of clipping mid-word at 390px (#495)", () => {
    const { container } = render(
      <GuestTranslationContext.Provider
        value={
          {
            currentLanguage: "es",
            t: (key: string) =>
              key === "businessPage.delivery.checkAddressCta"
                ? "Verifica la dirección y comienza el pedido"
                : key,
          } as unknown as React.ContextType<typeof GuestTranslationContext>
        }
      >
        <div style={{ width: 390 }}>
          <DeliveryAvailableCard
            businessId={1}
            businessName="Test Restaurant"
            deliverySettings={makeSettings()}
          />
        </div>
      </GuestTranslationContext.Provider>,
    );

    const cta = screen.getByTestId("delivery-start-cta");
    expect(cta).toHaveTextContent("Verifica la dirección y comienza el pedido");
    expect(`${cta.className} ${container.innerHTML}`).toMatch(/whitespace-normal/);
    expect(cta.className).not.toMatch(/whitespace-nowrap/);
  });

  it("renders the card when in-house delivery is enabled", () => {
    render(
      <DeliveryAvailableCard
        businessId={1}
        businessName="Test Restaurant"
        deliverySettings={makeSettings()}
      />,
    );
    expect(screen.getByText(/Delivery available/i)).toBeInTheDocument();
    expect(screen.getByText(/5\.99/)).toBeInTheDocument();
  });

  it("formats the delivery fee in the diner's locale", () => {
    render(
      <GuestTranslationContext.Provider
        value={
          {
            currentLanguage: "de",
          } as unknown as React.ContextType<typeof GuestTranslationContext>
        }
      >
        <DeliveryAvailableCard
          businessId={1}
          businessName="Test Restaurant"
          deliverySettings={makeSettings()}
        />
      </GuestTranslationContext.Provider>,
    );
    // German uses a comma decimal ("5,99 $"); en-US ("$5.99") never produces "5,99".
    expect(screen.getByText(/5,99/)).toBeInTheDocument();
    expect(screen.queryByText(/5\.99/)).not.toBeInTheDocument();
  });

  it("returns null when delivery is disabled", () => {
    const { container } = render(
      <DeliveryAvailableCard
        businessId={1}
        businessName="Test Restaurant"
        deliverySettings={makeSettings({ delivery_enabled: false })}
      />,
    );
    expect(container.firstChild).toBeNull();
  });

  it("returns null when in_house_delivery_enabled is false", () => {
    const { container } = render(
      <DeliveryAvailableCard
        businessId={1}
        businessName="Test Restaurant"
        deliverySettings={makeSettings({ in_house_delivery_enabled: false })}
      />,
    );
    expect(container.firstChild).toBeNull();
  });

  it("shows skeleton placeholders while loading", () => {
    render(
      <DeliveryAvailableCard
        businessId={1}
        businessName="Test Restaurant"
        deliverySettings={makeSettings()}
        loading={true}
      />,
    );
    expect(screen.getAllByTestId("skeleton").length).toBeGreaterThan(0);
  });

  it("opens the GuestDeliveryOrder modal when the CTA button is pressed", () => {
    render(
      <DeliveryAvailableCard
        businessId={1}
        businessName="Test Restaurant"
        deliverySettings={makeSettings()}
      />,
    );
    expect(screen.queryByTestId("guest-delivery-modal")).toBeNull();
    fireEvent.click(screen.getByText(/Check address/i));
    expect(screen.getByTestId("guest-delivery-modal")).toBeInTheDocument();
  });

  it("calls onQuoteReady and closes the modal when a quote context is emitted", async () => {
    const onQuoteReady = jest.fn();
    render(
      <DeliveryAvailableCard
        businessId={1}
        businessName="Test Restaurant"
        deliverySettings={makeSettings()}
        onQuoteReady={onQuoteReady}
      />,
    );
    fireEvent.click(screen.getByText(/Check address/i));
    fireEvent.click(screen.getByTestId("mock-quote-ready"));

    await waitFor(() => {
      expect(onQuoteReady).toHaveBeenCalledTimes(1);
    });
    // Modal should close after onQuoteReady fires
    expect(screen.queryByTestId("guest-delivery-modal")).toBeNull();
  });

  it("does not present default fee, ETA, or Downtown as a live quote before an address", () => {
    render(
      <DeliveryAvailableCard
        businessId={1}
        businessName="Test Restaurant"
        deliverySettings={makeSettings({
          zones: [
            {
              id: 1,
              name: "Downtown",
              delivery_fee: 3 as any,
              minimum_order_amount: 15 as any,
              estimated_time: 20,
              priority: 1,
              cutoff_buffer_minutes: 5,
              is_active: true,
            },
          ],
        })}
      />,
    );
    expect(screen.getByTestId("delivery-quote-chip")).toHaveTextContent(/Starting estimate/i);
    expect(screen.queryByText(/^Live quote$/i)).not.toBeInTheDocument();
    expect(screen.queryByText(/Downtown/i)).not.toBeInTheDocument();
    expect(screen.getByTestId("delivery-area-value")).toHaveTextContent(
      /confirm your area after you enter an address/i,
    );
    expect(screen.getByText(/Starting estimated time/i)).toBeInTheDocument();
    expect(screen.getByText(/Typical starting fee/i)).toBeInTheDocument();
  });

  it("hides the free-above / minimum-order line entirely when both thresholds are unset (DLV-GUEST-8)", () => {
    render(
      <DeliveryAvailableCard
        businessId={1}
        businessName="Test Restaurant"
        deliverySettings={makeSettings({
          free_delivery_minimum: 0 as any,
          minimum_order_amount: 0 as any,
        })}
      />,
    );
    expect(screen.queryByText(/Free above/i)).not.toBeInTheDocument();
    expect(screen.queryByText(/Minimum order/i)).not.toBeInTheDocument();
    expect(screen.queryByText(/\$0\.00/)).not.toBeInTheDocument();
  });

  it("shows only the configured threshold segment when the other is zero (DLV-GUEST-8)", () => {
    render(
      <DeliveryAvailableCard
        businessId={1}
        businessName="Test Restaurant"
        deliverySettings={makeSettings({
          free_delivery_minimum: 0 as any,
          minimum_order_amount: 20 as any,
        })}
      />,
    );
    expect(screen.queryByText(/Free above/i)).not.toBeInTheDocument();
    expect(screen.getByText((c) => /Minimum order/i.test(c) && c.includes("20"))).toBeInTheDocument();
  });

  it("surfaces the published delivery window on the card (DLV-GUEST-6)", () => {
    render(
      <DeliveryAvailableCard
        businessId={1}
        businessName="Test Restaurant"
        deliverySettings={makeSettings({
          delivery_hours_same_as_business: false,
          delivery_start_time: "11:00",
          delivery_end_time: "22:00",
        })}
      />,
    );
    expect(screen.getByTestId("delivery-hours")).toHaveTextContent("11:00");
    expect(screen.getByTestId("delivery-hours")).toHaveTextContent("22:00");
  });

  it("shows an outside-hours notice instead of pretending 24/7 delivery (DLV-GUEST-6)", () => {
    jest.useFakeTimers().setSystemTime(new Date("2026-07-06T02:00:00"));
    try {
      render(
        <DeliveryAvailableCard
          businessId={1}
          businessName="Test Restaurant"
          deliverySettings={makeSettings({
            delivery_hours_same_as_business: false,
            delivery_start_time: "11:00",
            delivery_end_time: "22:00",
          })}
        />,
      );
      expect(screen.getByTestId("outside-hours-notice")).toHaveTextContent("11:00");
    } finally {
      jest.useRealTimers();
    }
  });

  it("shows no outside-hours notice while within the delivery window (DLV-GUEST-6)", () => {
    jest.useFakeTimers().setSystemTime(new Date("2026-07-06T13:00:00"));
    try {
      render(
        <DeliveryAvailableCard
          businessId={1}
          businessName="Test Restaurant"
          deliverySettings={makeSettings({
            delivery_hours_same_as_business: false,
            delivery_start_time: "11:00",
            delivery_end_time: "22:00",
          })}
        />,
      );
      expect(screen.queryByTestId("outside-hours-notice")).not.toBeInTheDocument();
    } finally {
      jest.useRealTimers();
    }
  });

  it("shows no hours line when delivery hours mirror business hours", () => {
    render(
      <DeliveryAvailableCard
        businessId={1}
        businessName="Test Restaurant"
        deliverySettings={makeSettings({ delivery_hours_same_as_business: true })}
      />,
    );
    expect(screen.queryByTestId("delivery-hours")).not.toBeInTheDocument();
    expect(screen.queryByTestId("outside-hours-notice")).not.toBeInTheDocument();
  });

  it("does not claim Delivery available when business is closed and hours match business (Closed Mode)", () => {
    render(
      <DeliveryAvailableCard
        businessId={1}
        businessName="Test Restaurant"
        deliverySettings={makeSettings({ delivery_hours_same_as_business: true })}
        isBusinessOpen={false}
      />,
    );
    expect(screen.queryByText(/^Delivery available$/i)).not.toBeInTheDocument();
    const notice = screen.getByTestId("outside-hours-notice");
    expect(notice).toHaveTextContent(/not available at this time/i);
    // Title + notice both use closed copy (honest Closed Mode).
    expect(
      screen.getAllByText(/Delivery is not available at this time/i).length,
    ).toBeGreaterThanOrEqual(1);
    // Browse path stays open — CTA softens to check-address only.
    expect(screen.getByText(/^Check address$/i)).toBeInTheDocument();
  });

  it("keeps Delivery available when business is open and hours match business", () => {
    render(
      <DeliveryAvailableCard
        businessId={1}
        businessName="Test Restaurant"
        deliverySettings={makeSettings({ delivery_hours_same_as_business: true })}
        isBusinessOpen={true}
      />,
    );
    expect(screen.getByText(/Delivery available/i)).toBeInTheDocument();
    expect(screen.queryByTestId("outside-hours-notice")).not.toBeInTheDocument();
  });

  it("uses guest-voice copy, not developer jargon (DLV-GUEST-1)", () => {
    render(
      <DeliveryAvailableCard
        businessId={1}
        businessName="Test Restaurant"
        deliverySettings={makeSettings()}
      />,
    );
    expect(screen.queryByText(/serviceability/i)).not.toBeInTheDocument();
    expect(screen.queryByText(/delivery context attached/i)).not.toBeInTheDocument();
    expect(screen.queryByText(/We only create the delivery order/i)).not.toBeInTheDocument();
    expect(
      screen.getByText(/Enter your address to see delivery fees and timing/i),
    ).toBeInTheDocument();
    expect(
      screen.getByText(/nothing is ordered yet/i),
    ).toBeInTheDocument();
  });
});

const liveQuoteContext: GuestDeliveryQuoteContext = {
  delivery_address: {
    street: "9 Market St",
    apartment: "4B",
    city: "Dubai",
    state: "DU",
    postal_code: "00000",
    country: "AE",
    formatted_address: "9 Market St, Dubai, AE",
  },
  delivery_instructions: "Ring twice",
  contactless_delivery: true,
  leave_at_door: true,
  quote: {
    eligible: true,
    reason_code: "quote_available",
    message: "Delivery is available",
    delivery_fee: 7.5 as any,
    minimum_order_amount: 25 as any,
    free_delivery_minimum: 60 as any,
    order_subtotal: 0 as any,
    meets_minimum: true,
    estimated_prep_time: 15,
    estimated_delivery_minutes: 20,
    estimated_total_minutes: 35,
    fulfillment_mode: "delivery",
    partner_fallback_available: false,
    external_partner_links: [],
    zone: {
      id: 2,
      name: "Marina",
      delivery_fee: 7.5 as any,
      minimum_order_amount: 25 as any,
      estimated_time: 20,
      priority: 1,
      cutoff_buffer_minutes: 5,
      is_active: true,
    },
  },
};

describe("DeliveryAvailableCard address-specific quote and edit", () => {
  it("labels a successful address quote as live and shows its zone, fee, and ETA", () => {
    render(
      <DeliveryAvailableCard
        businessId={1}
        businessName="Test Restaurant"
        deliverySettings={makeSettings({
          flat_delivery_fee: 4.99 as any,
          zones: [
            {
              id: 1,
              name: "Downtown",
              delivery_fee: 3 as any,
              minimum_order_amount: 15 as any,
              estimated_time: 20,
              priority: 1,
              cutoff_buffer_minutes: 5,
              is_active: true,
            },
          ],
        })}
        fulfillmentContext={liveQuoteContext}
      />,
    );
    expect(screen.getByTestId("delivery-quote-chip")).toHaveTextContent(/^Live quote$/i);
    expect(screen.getByTestId("delivery-area-value")).toHaveTextContent("Marina");
    expect(screen.queryByText(/Downtown/i)).not.toBeInTheDocument();
    expect(screen.getByText(/7\.5/)).toBeInTheDocument();
    expect(screen.getByText(/About 35 minutes/i)).toBeInTheDocument();
    expect(screen.getByTestId("delivery-quote-meta")).toHaveTextContent(/Quoted for 9 Market St/i);
    expect(screen.getByTestId("delivery-quote-meta")).toHaveTextContent(/Recheck your address/i);
  });

  it("does not treat an out-of-zone saved quote as live eligibility", () => {
    render(
      <DeliveryAvailableCard
        businessId={1}
        businessName="Test Restaurant"
        deliverySettings={makeSettings({
          zones: [
            {
              id: 1,
              name: "Downtown",
              delivery_fee: 3 as any,
              minimum_order_amount: 15 as any,
              estimated_time: 20,
              priority: 1,
              cutoff_buffer_minutes: 5,
              is_active: true,
            },
          ],
        })}
        fulfillmentContext={{
          ...liveQuoteContext,
          quote: {
            ...liveQuoteContext.quote,
            eligible: false,
            reason_code: "zone_unavailable",
          },
        }}
      />,
    );
    expect(screen.getByTestId("delivery-quote-chip")).toHaveTextContent(/Starting estimate/i);
    expect(screen.queryByText(/Downtown/i)).not.toBeInTheDocument();
    expect(screen.queryByText(/Marina/i)).not.toBeInTheDocument();
  });

  it("updates live quote values when the address context is replaced and reverts when cleared", () => {
    const { rerender } = render(
      <DeliveryAvailableCard
        businessId={1}
        businessName="Test Restaurant"
        deliverySettings={makeSettings()}
        fulfillmentContext={liveQuoteContext}
      />,
    );
    expect(screen.getByTestId("delivery-area-value")).toHaveTextContent("Marina");

    rerender(
      <DeliveryAvailableCard
        businessId={1}
        businessName="Test Restaurant"
        deliverySettings={makeSettings()}
        fulfillmentContext={{
          ...liveQuoteContext,
          delivery_address: {
            ...liveQuoteContext.delivery_address,
            street: "80 Harbor",
            formatted_address: "80 Harbor, Dubai, AE",
          },
          quote: {
            ...liveQuoteContext.quote,
            delivery_fee: 9.25 as any,
            estimated_total_minutes: 48,
            zone: {
              ...liveQuoteContext.quote.zone!,
              name: "Harbor",
            },
          },
        }}
      />,
    );
    expect(screen.getByTestId("delivery-quote-chip")).toHaveTextContent(/^Live quote$/i);
    expect(screen.getByTestId("delivery-area-value")).toHaveTextContent("Harbor");
    expect(screen.getByText(/9\.25/)).toBeInTheDocument();
    expect(screen.getByText(/About 48 minutes/i)).toBeInTheDocument();

    rerender(
      <DeliveryAvailableCard
        businessId={1}
        businessName="Test Restaurant"
        deliverySettings={makeSettings()}
        fulfillmentContext={null}
      />,
    );
    expect(screen.getByTestId("delivery-quote-chip")).toHaveTextContent(/Starting estimate/i);
    expect(screen.queryByText(/Harbor/i)).not.toBeInTheDocument();
    expect(screen.queryByTestId("delivery-quote-meta")).not.toBeInTheDocument();
  });

  it("opens the address editor immediately on Edit and prefills the saved context", () => {
    render(
      <DeliveryAvailableCard
        businessId={1}
        businessName="Test Restaurant"
        deliverySettings={makeSettings()}
        fulfillmentContext={liveQuoteContext}
        openAddressEditorToken={1}
      />,
    );
    expect(screen.getByTestId("guest-delivery-modal")).toBeInTheDocument();
    expect(screen.getByTestId("prefill-street")).toHaveTextContent("9 Market St");
    expect(screen.getByTestId("prefill-city")).toHaveTextContent("Dubai");
    expect(screen.getByTestId("prefill-state")).toHaveTextContent("DU");
    expect(screen.getByTestId("prefill-postal")).toHaveTextContent("00000");
    expect(screen.getByTestId("prefill-country")).toHaveTextContent("AE");
    expect(screen.getByTestId("prefill-apartment")).toHaveTextContent("4B");
    expect(screen.getByTestId("prefill-instructions")).toHaveTextContent("Ring twice");
    expect(screen.getByTestId("prefill-contactless")).toHaveTextContent("true");
    expect(screen.getByTestId("prefill-leave-at-door")).toHaveTextContent("true");
  });

  it("does not replace the saved context when the editor is cancelled", () => {
    const onQuoteReady = jest.fn();
    render(
      <DeliveryAvailableCard
        businessId={1}
        businessName="Test Restaurant"
        deliverySettings={makeSettings()}
        fulfillmentContext={liveQuoteContext}
        openAddressEditorToken={1}
        onQuoteReady={onQuoteReady}
      />,
    );
    fireEvent.click(screen.getByText("Close modal"));
    expect(onQuoteReady).not.toHaveBeenCalled();
    expect(screen.getByTestId("delivery-quote-chip")).toHaveTextContent(/^Live quote$/i);
    expect(screen.getByTestId("delivery-area-value")).toHaveTextContent("Marina");
  });
});
