/** @jest-environment jsdom */
import React from "react";
import { render, screen, fireEvent, waitFor, act } from "@testing-library/react";
import GuestDeliveryCheckout from "../GuestDeliveryCheckout";
import { guestDeliveryApi } from "@/api/delivery";
import type { CartItem } from "../GuestDeliveryCheckout";
import type { FulfillmentContext } from "@/hooks/useFulfillmentContext";
import { asDollars } from "@/types/money";

// ── Module mocks ──────────────────────────────────────────────────────────────

const mockPush = jest.fn();
jest.mock("next/navigation", () => ({
  useRouter: () => ({ push: mockPush }),
}));

jest.mock("@/api/delivery", () => ({
  guestDeliveryApi: {
    checkout: jest.fn(),
    quote: jest.fn(),
    track: jest.fn(),
    getSettings: jest.fn(),
  },
}));

const mockT = jest.fn(
  (key: string, _params?: Record<string, string | number>) => key,
);
const translationState = { currentLanguage: "en" };

jest.mock("@/i18n/GuestTranslationProvider", () => {
  const actual = jest.requireActual("@/i18n/GuestTranslationProvider");
  return {
    ...actual,
    useGuestTranslation: () => ({
      t: (key: string, params?: Record<string, string | number>) => mockT(key, params),
      get currentLanguage() {
        return translationState.currentLanguage;
      },
      setLanguage: jest.fn(),
      availableLanguages: {},
      setBusinessId: jest.fn(),
    }),
  };
});

jest.mock("@nextui-org/react", () => ({
  Modal: ({ isOpen, children }: { isOpen: boolean; children: React.ReactNode }) =>
    isOpen ? <div data-testid="checkout-modal">{children}</div> : null,
  ModalContent: ({ children }: { children: React.ReactNode }) => <div>{children}</div>,
  ModalHeader: ({ children }: { children: React.ReactNode }) => <div>{children}</div>,
  ModalBody: ({ children }: { children: React.ReactNode }) => <div>{children}</div>,
  ModalFooter: ({ children }: { children: React.ReactNode }) => <div>{children}</div>,
  Input: require("react").forwardRef(function MockInput(
    {
      label,
      onChange,
      value,
      placeholder,
      isRequired,
      isInvalid,
      errorMessage,
      id,
      startContent: _sc,
      type: _t,
      min: _min,
      step: _step,
      inputProps,
      ...rest
    }: any,
    ref: any,
  ) {
    const inputId = id || `input-${String(label)}`;
    const errorId = `${inputId}-error`;
    return (
      <div>
        <label htmlFor={inputId}>{label}{isRequired ? " *" : ""}</label>
        <input
          ref={ref}
          id={inputId}
          {...rest}
          {...inputProps}
          aria-label={label}
          aria-invalid={isInvalid ? true : undefined}
          aria-describedby={isInvalid && errorMessage ? errorId : undefined}
          value={value ?? ""}
          onChange={onChange}
          placeholder={placeholder}
        />
        {isInvalid && errorMessage ? <p id={errorId}>{errorMessage}</p> : null}
      </div>
    );
  }),
  Textarea: ({ label, onChange, value, placeholder }: any) => (
    <div>
      <label>{label}</label>
      <textarea aria-label={label} value={value ?? ""} onChange={onChange} placeholder={placeholder} />
    </div>
  ),
  Button: ({ children, onPress, isLoading, isDisabled, color: _c, variant: _v, "data-testid": testId, ...rest }: any) => (
    <button
      type="button"
      onClick={onPress}
      disabled={isDisabled || isLoading}
      data-testid={testId}
      {...rest}
    >
      {isLoading ? "Placing order..." : children}
    </button>
  ),
  Divider: () => <hr />,
  Card: ({ children, className, ...props }: any) => (
    <div className={className} {...props}>{children}</div>
  ),
  CardBody: ({ children }: any) => <div>{children}</div>,
  Chip: ({ children }: any) => <span>{children}</span>,
}));

// ── Fixtures ──────────────────────────────────────────────────────────────────

const mockFulfillmentContext: FulfillmentContext = {
  business_id: 42,
  custom_url: "test-biz",
  mode: "delivery",
  delivery_address: {
    street: "1 Main St",
    city: "Dubai",
    country: "AE",
    formatted_address: "1 Main St, Dubai, AE",
  },
  delivery_instructions: undefined,
  contactless_delivery: false,
  leave_at_door: false,
  quote: {
    eligible: true,
    reason_code: "quote_available",
    message: "Delivery available",
    delivery_fee: asDollars(5.99),
    minimum_order_amount: asDollars(20),
    free_delivery_minimum: asDollars(50),
    order_subtotal: asDollars(0),
    meets_minimum: true,
    estimated_prep_time: 15,
    estimated_delivery_minutes: 25,
    estimated_total_minutes: 40,
    fulfillment_mode: "delivery",
    partner_fallback_available: false,
    external_partner_links: [],
  },
  saved_at: new Date().toISOString(),
};

const mockCart: CartItem[] = [
  {
    key: "burger|Extra cheese",
    menu_item_name: "Classic Burger",
    menu_item_id: "item-1",
    quantity: 2,
    unit_price: 12.5,
    item_type: "menu_item",
    options: [{ name: "Extra cheese", price_change: 1.5 }],
  },
  {
    key: "fries",
    menu_item_name: "French Fries",
    menu_item_id: "item-2",
    quantity: 1,
    unit_price: 4.0,
    options: [],
  },
];

const mockCheckoutResponse = {
  bill: {
    id: 100,
    business_id: 42,
    bill_number: "BILL-100",
    subtotal: 30.5,
    tax_amount: 2.5,
    service_fee_amount: 0,
    total_amount: 39.49,
    status: "pending",
  },
  order: {
    id: 200,
    bill_id: 100,
    order_number: "ORD-200",
    status: "pending",
  },
  delivery_order: {
    id: 300,
    business_id: 42,
    delivery_number: "DEL-123",
    delivery_type: "in_house",
    status: "pending",
    customer_name: "Jane Smith",
    customer_phone: "+1 555 000 1234",
    delivery_address: {
      street: "1 Main St",
      city: "Dubai",
      country: "AE",
      formatted_address: "1 Main St, Dubai, AE",
    },
    delivery_fee: 5.99,
    contactless_delivery: false,
    leave_at_door: false,
    created_at: new Date().toISOString(),
    updated_at: new Date().toISOString(),
  },
  tracking_url: "/delivery/DEL-123/track",
};

const defaultProps = {
  isOpen: true,
  onClose: jest.fn(),
  businessId: 42,
  businessName: "Test Restaurant",
  fulfillmentContext: mockFulfillmentContext,
  cart: mockCart,
};

// ── Tests ─────────────────────────────────────────────────────────────────────

describe("GuestDeliveryCheckout", () => {
  beforeEach(() => {
    jest.clearAllMocks();
    mockT.mockImplementation((key: string) => key);
    translationState.currentLanguage = "en";
  });

  it("renders the checkout modal when open", () => {
    render(<GuestDeliveryCheckout {...defaultProps} />);
    expect(screen.getByTestId("checkout-modal")).toBeInTheDocument();
    expect(screen.getByLabelText(/fullName|Full name/i)).toBeInTheDocument();
    expect(screen.getByLabelText(/phoneNumber|Phone number/i)).toBeInTheDocument();
  });

  it("names configured courier partners on an eligible checkout", () => {
    const ctx = {
      ...mockFulfillmentContext,
      quote: {
        ...mockFulfillmentContext.quote,
        partner_fallback_available: true,
        external_partner_links: [
          { name: "PedidosYa", url: "https://www.pedidosya.com.ar", provider_key: "pedidosya" },
          { name: "Rappi", url: "https://www.rappi.com.ar", provider_key: "rappi" },
        ],
      },
    };
    render(<GuestDeliveryCheckout {...defaultProps} fulfillmentContext={ctx} />);
    expect(screen.getByTestId("checkout-couriers")).toBeInTheDocument();
    expect(screen.getByText("PedidosYa")).toBeInTheDocument();
    expect(screen.getByText("Rappi")).toBeInTheDocument();
  });

  // #897 security: partner URLs are operator-configured data rendered on an
  // unauthenticated guest surface. A javascript:/data: URL must never become an
  // href.
  it("never renders an unsafe partner URL as a link", () => {
    const ctx = {
      ...mockFulfillmentContext,
      quote: {
        ...mockFulfillmentContext.quote,
        partner_fallback_available: true,
        external_partner_links: [
          { name: "Evil", url: "javascript:alert(1)", provider_key: "pedidosya" },
          {
            name: "DataEvil",
            url: "data:text/html,<script>alert(1)</script>",
            provider_key: "rappi",
          },
          { name: "Rappi", url: "https://www.rappi.com.ar", provider_key: "rappi" },
        ],
      },
    };
    render(<GuestDeliveryCheckout {...defaultProps} fulfillmentContext={ctx} />);

    const block = screen.getByTestId("checkout-couriers");
    const anchors = Array.from(block.querySelectorAll("a"));
    expect(anchors).toHaveLength(1);
    expect(anchors[0]).toHaveAttribute("href", "https://www.rappi.com.ar");
    expect(
      anchors.some((a) => (a.getAttribute("href") || "").startsWith("javascript:")),
    ).toBe(false);
    // The unsafe rows degrade to plain text rather than disappearing silently.
    expect(screen.getByText("Evil").tagName).toBe("SPAN");
    expect(screen.getByText("DataEvil").tagName).toBe("SPAN");
  });

  it("does not render when isOpen is false", () => {
    render(<GuestDeliveryCheckout {...defaultProps} isOpen={false} />);
    expect(screen.queryByTestId("checkout-modal")).toBeNull();
  });

  it("shows cart items with correct line totals", () => {
    render(<GuestDeliveryCheckout {...defaultProps} />);
    // Classic Burger: (12.50 + 1.50) * 2 = 28.00
    expect(screen.getByText("Classic Burger")).toBeInTheDocument();
    expect(screen.getAllByText((content) => content.includes("$28.00")).length).toBeGreaterThan(0);
    // French Fries: 4.00 * 1 = 4.00
    expect(screen.getByText("French Fries")).toBeInTheDocument();
    expect(screen.getAllByText((content) => content.includes("$4.00")).length).toBeGreaterThan(0);
  });

  it("shows delivery address from fulfillment context", () => {
    render(<GuestDeliveryCheckout {...defaultProps} />);
    expect(screen.getByText(/1 Main St, Dubai, AE/)).toBeInTheDocument();
  });

  it("validates required fields before calling checkout API", async () => {
    render(<GuestDeliveryCheckout {...defaultProps} />);
    const placeBtn = screen.getByTestId("place-order-btn");
    await act(async () => {
      fireEvent.click(placeBtn);
    });
    await waitFor(() => {
      expect(screen.getByText(/businessPage\.checkout\.nameRequired/i)).toBeInTheDocument();
    });
    expect(guestDeliveryApi.checkout).not.toHaveBeenCalled();
  });

  it("validates phone required after name is filled", async () => {
    render(<GuestDeliveryCheckout {...defaultProps} />);

    fireEvent.change(screen.getByLabelText(/fullName|Full name/i), {
      target: { value: "Jane Smith" },
    });

    const placeBtn = screen.getByTestId("place-order-btn");
    await act(async () => {
      fireEvent.click(placeBtn);
    });

    await waitFor(() => {
      expect(screen.getByText(/businessPage\.checkout\.phoneRequired/i)).toBeInTheDocument();
    });
    expect(guestDeliveryApi.checkout).not.toHaveBeenCalled();
  });

  it("shows error when email is empty and does NOT call checkout API", async () => {
    render(<GuestDeliveryCheckout {...defaultProps} />);

    fireEvent.change(screen.getByLabelText(/fullName|Full name/i), {
      target: { value: "Jane Smith" },
    });
    fireEvent.change(screen.getByLabelText(/phoneNumber|Phone number/i), {
      target: { value: "+1 555 000 1234" },
    });
    // email left empty

    await act(async () => {
      fireEvent.click(screen.getByTestId("place-order-btn"));
    });

    await waitFor(() => {
      expect(screen.getByText("businessPage.guestDelivery.emailRequired")).toBeInTheDocument();
    });
    expect(guestDeliveryApi.checkout).not.toHaveBeenCalled();
  });

  it("posts the correct payload shape (including customer_email) to checkout API on success", async () => {
    (guestDeliveryApi.quote as jest.Mock).mockResolvedValue({
      eligible: true,
      reason_code: "quote_available",
      message: "Delivery available",
      delivery_fee: asDollars(5.99),
      minimum_order_amount: asDollars(20),
      free_delivery_minimum: asDollars(50),
      order_subtotal: asDollars(32),
      meets_minimum: true,
      estimated_prep_time: 15,
      estimated_delivery_minutes: 25,
      estimated_total_minutes: 40,
      fulfillment_mode: "delivery",
      partner_fallback_available: false,
      external_partner_links: [],
    });
    (guestDeliveryApi.checkout as jest.Mock).mockResolvedValue(mockCheckoutResponse);

    render(<GuestDeliveryCheckout {...defaultProps} />);

    fireEvent.change(screen.getByLabelText(/fullName|Full name/i), {
      target: { value: "Jane Smith" },
    });
    fireEvent.change(screen.getByLabelText(/phoneNumber|Phone number/i), {
      target: { value: "+1 555 000 1234" },
    });
    fireEvent.change(screen.getByLabelText(/businessPage\.checkout\.emailLabel|Email/i), {
      target: { value: "jane@example.com" },
    });

    await act(async () => {
      fireEvent.click(screen.getByTestId("place-order-btn"));
    });

    await waitFor(() => {
      expect(guestDeliveryApi.checkout).toHaveBeenCalledWith(
        42,
        expect.objectContaining({
          customer_name: "Jane Smith",
          customer_phone: "+1 555 000 1234",
          customer_email: "jane@example.com",
          delivery_address: expect.objectContaining({
            street: "1 Main St",
            city: "Dubai",
          }),
          items: expect.arrayContaining([
            expect.objectContaining({
              menu_item_name: "Classic Burger",
              quantity: 2,
              price: 14, // 12.50 + 1.50
            }),
            expect.objectContaining({
              menu_item_name: "French Fries",
              quantity: 1,
              price: 4.0,
            }),
          ]),
        }),
        { idempotencyKey: expect.any(String) },
      );
    });
  });

  it("redirects to tracking page after successful checkout", async () => {
    (guestDeliveryApi.quote as jest.Mock).mockResolvedValue({
      eligible: true,
      reason_code: "quote_available",
      message: "Delivery available",
      delivery_fee: asDollars(5.99),
      minimum_order_amount: asDollars(20),
      free_delivery_minimum: asDollars(50),
      order_subtotal: asDollars(32),
      meets_minimum: true,
      estimated_prep_time: 15,
      estimated_delivery_minutes: 25,
      estimated_total_minutes: 40,
      fulfillment_mode: "delivery",
      partner_fallback_available: false,
      external_partner_links: [],
    });
    (guestDeliveryApi.checkout as jest.Mock).mockResolvedValue(mockCheckoutResponse);
    const onSuccess = jest.fn();

    render(
      <GuestDeliveryCheckout
        {...defaultProps}
        onSuccess={onSuccess}
      />
    );

    fireEvent.change(screen.getByLabelText(/fullName|Full name/i), {
      target: { value: "Jane Smith" },
    });
    fireEvent.change(screen.getByLabelText(/phoneNumber|Phone number/i), {
      target: { value: "+1 555 000 1234" },
    });
    fireEvent.change(screen.getByLabelText(/businessPage\.checkout\.emailLabel|Email/i), {
      target: { value: "jane@example.com" },
    });

    await act(async () => {
      fireEvent.click(screen.getByTestId("place-order-btn"));
    });

    await waitFor(() => {
      expect(mockPush).toHaveBeenCalledWith("/delivery/DEL-123/track");
    });
    expect(onSuccess).toHaveBeenCalled();
  });

  it("routes to the pay page when the checkout order awaits online payment", async () => {
    (guestDeliveryApi.quote as jest.Mock).mockResolvedValue({
      eligible: true,
      reason_code: "quote_available",
      message: "Delivery available",
      delivery_fee: asDollars(5.99),
      minimum_order_amount: asDollars(20),
      free_delivery_minimum: asDollars(50),
      order_subtotal: asDollars(32),
      meets_minimum: true,
      estimated_prep_time: 15,
      estimated_delivery_minutes: 25,
      estimated_total_minutes: 40,
      fulfillment_mode: "delivery",
      partner_fallback_available: false,
      external_partner_links: [],
    });
    (guestDeliveryApi.checkout as jest.Mock).mockResolvedValue({
      ...mockCheckoutResponse,
      delivery_order: {
        ...mockCheckoutResponse.delivery_order,
        payment_mode_stored: "online",
      },
    });

    render(<GuestDeliveryCheckout {...defaultProps} />);

    fireEvent.change(screen.getByLabelText(/fullName|Full name/i), {
      target: { value: "Jane Smith" },
    });
    fireEvent.change(screen.getByLabelText(/phoneNumber|Phone number/i), {
      target: { value: "+1 555 000 1234" },
    });
    fireEvent.change(screen.getByLabelText(/businessPage\.checkout\.emailLabel|Email/i), {
      target: { value: "jane@example.com" },
    });

    await act(async () => {
      fireEvent.click(screen.getByTestId("place-order-btn"));
    });

    await waitFor(() => {
      expect(mockPush).toHaveBeenCalledWith("/delivery/DEL-123/pay");
    });
    expect(mockPush).not.toHaveBeenCalledWith("/delivery/DEL-123/track");
  });

  it("shows localized error message when checkout API rejects with a capacity error", async () => {
    (guestDeliveryApi.quote as jest.Mock).mockResolvedValue({
      eligible: true,
      reason_code: "quote_available",
      message: "Delivery available",
      delivery_fee: asDollars(5.99),
      minimum_order_amount: asDollars(20),
      free_delivery_minimum: asDollars(50),
      order_subtotal: asDollars(32),
      meets_minimum: true,
      estimated_prep_time: 15,
      estimated_delivery_minutes: 25,
      estimated_total_minutes: 40,
      fulfillment_mode: "delivery",
      partner_fallback_available: false,
      external_partner_links: [],
    });
    (guestDeliveryApi.checkout as jest.Mock).mockRejectedValue(
      new Error("delivery capacity exceeded")
    );

    render(<GuestDeliveryCheckout {...defaultProps} />);

    fireEvent.change(screen.getByLabelText(/fullName|Full name/i), {
      target: { value: "Jane Smith" },
    });
    fireEvent.change(screen.getByLabelText(/phoneNumber|Phone number/i), {
      target: { value: "+1 555 000 1234" },
    });
    fireEvent.change(screen.getByLabelText(/businessPage\.checkout\.emailLabel|Email/i), {
      target: { value: "jane@example.com" },
    });

    await act(async () => {
      fireEvent.click(screen.getByTestId("place-order-btn"));
    });

    await waitFor(() => {
      // Localized key returned by our t() mock
      expect(
        screen.getByText("businessPage.deliveryQuote.capacityFull")
      ).toBeInTheDocument();
    });
  });

  it("normalizes a comma-decimal driver tip into the checkout total (F23)", async () => {
    (guestDeliveryApi.quote as jest.Mock).mockResolvedValue({
      eligible: true,
      reason_code: "quote_available",
      message: "Delivery available",
      delivery_fee: asDollars(5.99),
      minimum_order_amount: asDollars(20),
      free_delivery_minimum: asDollars(50),
      order_subtotal: asDollars(32),
      meets_minimum: true,
      estimated_prep_time: 15,
      estimated_delivery_minutes: 25,
      estimated_total_minutes: 40,
      fulfillment_mode: "delivery",
      partner_fallback_available: false,
      external_partner_links: [],
    });
    (guestDeliveryApi.checkout as jest.Mock).mockResolvedValue(mockCheckoutResponse);

    render(<GuestDeliveryCheckout {...defaultProps} />);

    fireEvent.change(screen.getByLabelText(/fullName|Full name/i), {
      target: { value: "Jane Smith" },
    });
    fireEvent.change(screen.getByLabelText(/phoneNumber|Phone number/i), {
      target: { value: "+1 555 000 1234" },
    });
    fireEvent.change(screen.getByLabelText(/businessPage\.checkout\.emailLabel|Email/i), {
      target: { value: "jane@example.com" },
    });
    // Comma-decimal locale entry: "2,50" must parse to 2.5, not be truncated.
    fireEvent.change(screen.getByLabelText(/businessPage\.checkout\.driverTip|Driver tip/i), {
      target: { value: "2,50" },
    });

    await act(async () => {
      fireEvent.click(screen.getByTestId("place-order-btn"));
    });

    await waitFor(() => {
      expect(guestDeliveryApi.checkout).toHaveBeenCalledWith(
        42,
        expect.objectContaining({ driver_tip: 2.5 }),
        { idempotencyKey: expect.any(String) },
      );
    });
  });

  it("shows correct tax and service fee rows when rates are percents and total agrees (R8)", () => {
    // subtotal = $10.00 (1 item @ $10, no options)
    // taxRate=8 → taxAmount = Math.round(10 * 8) / 100 = $0.80
    // serviceFeeRate=5 → serviceFeeAmount = Math.round(10 * 5) / 100 = $0.50
    // deliveryFee = $5.99 (from mockFulfillmentContext quote)
    // total = 10.00 + 0.80 + 0.50 + 5.99 = $17.29
    const tenDollarCart: CartItem[] = [
      {
        key: "item-10",
        menu_item_name: "Flat Item",
        menu_item_id: "item-10",
        quantity: 1,
        unit_price: 10.0,
        options: [],
      },
    ];

    render(
      <GuestDeliveryCheckout
        {...defaultProps}
        cart={tenDollarCart}
        taxRate={8}
        serviceFeeRate={5}
      />
    );

    // Tax row: $0.80
    expect(screen.getAllByText((content) => content.includes("$0.80")).length).toBeGreaterThan(0);
    // Service fee row: $0.50
    expect(screen.getAllByText((content) => content.includes("$0.50")).length).toBeGreaterThan(0);
    // Total: subtotal + tax + service fee + delivery fee = 10 + 0.80 + 0.50 + 5.99 = 17.29
    expect(screen.getAllByText((content) => content.includes("$17.29")).length).toBeGreaterThan(0);
  });

  it("pre-normalizes subtotal before computing tax to match backend ComputeBillTotals (cent-divergence regression)", () => {
    // Regression for the IEEE-754 cent-divergence bug:
    //
    // cart: item1 (unit_price=0.50, option price_change=0.10, qty=1) + item2 (unit_price=0.70, qty=1)
    //   raw subtotal in JS = (0.50 + 0.10) * 1 + 0.70 = 0.6 + 0.7 = 1.2999999999999998 (fp noise)
    //
    // WITHOUT normalization: Math.round(1.2999... * 15) / 100 = Math.round(19.4999...) / 100 = 0.19 (wrong)
    // WITH normalization:    normalizedSubtotal = Math.round(1.2999... * 100)/100 = 1.30
    //                        Math.round(1.30 * 15) / 100 = Math.round(19.5) / 100 = 0.20 (correct — matches backend)
    //
    // Backend ComputeBillTotals: normalizes subtotal first via math.Round(subtotal*100)/100,
    // then computes tax as math.Round(normalizedSubtotal * taxRate / 100 * 100) / 100.
    // For subtotal=1.30, taxRate=15: tax = round(1.30*15)/100 = round(19.5)/100 = 0.20.
    const fpNoiseCart: CartItem[] = [
      {
        key: "item-fp1",
        menu_item_name: "Item with option",
        menu_item_id: "item-fp1",
        quantity: 1,
        unit_price: 0.5,
        options: [{ name: "Add-on", price_change: 0.1 }],
      },
      {
        key: "item-fp2",
        menu_item_name: "Side item",
        menu_item_id: "item-fp2",
        quantity: 1,
        unit_price: 0.7,
        options: [],
      },
    ];

    // taxRate=15 (15%), no service fee, delivery fee = $5.99 from mockFulfillmentContext
    // Normalized subtotal = $1.30
    // Tax = $0.20 (backend/fixed value), NOT $0.19 (unfixed fp result)
    // Total = 1.30 + 0.20 + 5.99 = $7.49
    render(
      <GuestDeliveryCheckout
        {...defaultProps}
        cart={fpNoiseCart}
        taxRate={15}
        serviceFeeRate={0}
      />
    );

    // Tax row must show $0.20 (backend value), not $0.19 (unfixed floating-point result)
    expect(screen.getAllByText((content) => content.includes("$0.20")).length).toBeGreaterThan(0);
    // Total: 1.30 + 0.20 + 5.99 = $7.49
    expect(screen.getAllByText((content) => content.includes("$7.49")).length).toBeGreaterThan(0);
  });

  it("disables place order button when cart subtotal is below delivery minimum", () => {
    const smallCart: CartItem[] = [
      {
        key: "tea",
        menu_item_name: "Green Tea",
        quantity: 1,
        unit_price: 2.0,
        options: [],
      },
    ];
    render(
      <GuestDeliveryCheckout
        {...defaultProps}
        cart={smallCart}
      />
    );
    // subtotal 2.00 < minimum 20.00 → button disabled
    expect(screen.getByTestId("place-order-btn")).toBeDisabled();
  });

  it("shows $0 delivery fee immediately when cart subtotal meets free_delivery_minimum from quote context", () => {
    // free_delivery_minimum = $50; mockCart subtotal = (12.50+1.50)*2 + 4.00 = 32.00 → fee shown
    // Override cart so subtotal = $55.00 ≥ free_delivery_minimum $50 → fee must display as $0
    const freeCart: CartItem[] = [
      {
        key: "big-item",
        menu_item_name: "Premium Platter",
        menu_item_id: "item-big",
        quantity: 1,
        unit_price: 55.0,
        options: [],
      },
    ];

    // fulfillmentContext has free_delivery_minimum = $50; quote.delivery_fee = $5.99 (stale zero-subtotal quote)
    render(<GuestDeliveryCheckout {...defaultProps} cart={freeCart} />);

    // The fee row must show $0.00 immediately — not $5.99 — because 55 >= 50
    expect(screen.getAllByText((content) => content.includes("$0.00")).length).toBeGreaterThan(0);
    // $5.99 must NOT appear as the fee amount
    expect(screen.queryByText((content) => content.includes("$5.99"))).not.toBeInTheDocument();
  });

  it("does NOT show fee-changed notice or block when re-quote fee only drops (spurious second-click suppressed)", async () => {
    // Initial displayed fee from fulfillmentContext.quote = $5.99 (quoted at subtotal=0)
    // Re-quote returns $0 (cart now meets free_delivery_minimum)
    (guestDeliveryApi.quote as jest.Mock).mockResolvedValue({
      eligible: true,
      reason_code: "quote_available",
      message: "Delivery available",
      delivery_fee: 0,
      minimum_order_amount: 20,
      free_delivery_minimum: 50,
      order_subtotal: 55,
      meets_minimum: true,
      estimated_prep_time: 15,
      estimated_delivery_minutes: 25,
      estimated_total_minutes: 40,
      fulfillment_mode: "delivery",
      partner_fallback_available: false,
      external_partner_links: [],
    });
    (guestDeliveryApi.checkout as jest.Mock).mockResolvedValue(mockCheckoutResponse);

    // Cart subtotal = $55 (meets free_delivery_minimum=$50); initial quote fee = $5.99
    const freeCart: CartItem[] = [
      {
        key: "big-item",
        menu_item_name: "Premium Platter",
        menu_item_id: "item-big",
        quantity: 1,
        unit_price: 55.0,
        options: [],
      },
    ];

    render(<GuestDeliveryCheckout {...defaultProps} cart={freeCart} />);

    fireEvent.change(screen.getByLabelText(/fullName|Full name/i), {
      target: { value: "Jane Smith" },
    });
    fireEvent.change(screen.getByLabelText(/phoneNumber|Phone number/i), {
      target: { value: "+1 555 000 1234" },
    });
    fireEvent.change(screen.getByLabelText(/businessPage\.checkout\.emailLabel|Email/i), {
      target: { value: "jane@example.com" },
    });

    // Single submit — fee dropped from $5.99 → $0, should NOT show notice, must call checkout directly
    await act(async () => {
      fireEvent.click(screen.getByTestId("place-order-btn"));
    });

    await waitFor(() => {
      expect(guestDeliveryApi.checkout).toHaveBeenCalledTimes(1);
    });
    // No fee-changed notice should be shown
    expect(screen.queryByText(/feeChangedNotice/)).not.toBeInTheDocument();
  });

  it("re-quote returns different fee → first submit shows feeChangedNotice and blocks checkout; second submit calls checkout", async () => {
    // Re-quote returns a higher fee than the original 5.99
    (guestDeliveryApi.quote as jest.Mock).mockResolvedValue({
      eligible: true,
      reason_code: "quote_available",
      message: "Delivery available",
      delivery_fee: 8.99,
      minimum_order_amount: 20,
      free_delivery_minimum: 50,
      order_subtotal: 32,
      meets_minimum: true,
      estimated_prep_time: 15,
      estimated_delivery_minutes: 25,
      estimated_total_minutes: 40,
      fulfillment_mode: "delivery",
      partner_fallback_available: false,
      external_partner_links: [],
    });
    (guestDeliveryApi.checkout as jest.Mock).mockResolvedValue(mockCheckoutResponse);

    render(<GuestDeliveryCheckout {...defaultProps} />);

    fireEvent.change(screen.getByLabelText(/fullName|Full name/i), {
      target: { value: "Jane Smith" },
    });
    fireEvent.change(screen.getByLabelText(/phoneNumber|Phone number/i), {
      target: { value: "+1 555 000 1234" },
    });
    fireEvent.change(screen.getByLabelText(/businessPage\.checkout\.emailLabel|Email/i), {
      target: { value: "jane@example.com" },
    });

    // FIRST submit — fee changed notice shown, checkout NOT called
    await act(async () => {
      fireEvent.click(screen.getByTestId("place-order-btn"));
    });

    await waitFor(() => {
      expect(
        screen.getByText("businessPage.guestDelivery.feeChangedNotice"),
      ).toBeInTheDocument();
    });
    expect(guestDeliveryApi.checkout).not.toHaveBeenCalled();

    // Place order button should still be enabled for user to confirm
    expect(screen.getByTestId("place-order-btn")).not.toBeDisabled();

    // SECOND submit — should bypass re-quote and call checkout
    await act(async () => {
      fireEvent.click(screen.getByTestId("place-order-btn"));
    });

    await waitFor(() => {
      expect(guestDeliveryApi.checkout).toHaveBeenCalledTimes(1);
    });
  });

  it("clamps a negative driver tip to zero so the displayed total is never understated (DLV-GUEST-4)", () => {
    render(<GuestDeliveryCheckout {...defaultProps} />);
    const tipInput = screen.getByLabelText(
      /businessPage\.checkout\.driverTip|Driver tip/i,
    ) as HTMLInputElement;

    fireEvent.change(tipInput, { target: { value: "-5" } });

    // The minus sign is stripped at the input, so the field can never hold a
    // negative value...
    expect(tipInput.value).toBe("5");

    // ...and even if a negative value somehow reached state, the math clamps.
    // Subtotal 32 + fee 5.99 + tip 5 => 42.99; an understated 32.99 (tip -5)
    // must never render.
    expect(screen.getByText((c) => c.includes("businessPage.checkout.placeOrder") && c.includes("$42.99"))).toBeInTheDocument();
    expect(screen.queryByText(/\$32\.99/)).not.toBeInTheDocument();
  });

  it("never sends a negative driver_tip to the checkout API (DLV-GUEST-4)", async () => {
    (guestDeliveryApi.checkout as jest.Mock).mockResolvedValue(mockCheckoutResponse);
    (guestDeliveryApi.quote as jest.Mock).mockResolvedValue({
      ...mockFulfillmentContext.quote,
      order_subtotal: 32,
    });

    render(<GuestDeliveryCheckout {...defaultProps} />);
    fireEvent.change(screen.getByLabelText(/businessPage\.checkout\.fullName|Full name/i), {
      target: { value: "Jane Smith" },
    });
    fireEvent.change(screen.getByLabelText(/businessPage\.checkout\.phoneNumber|Phone number/i), {
      target: { value: "+1 555 000 1234" },
    });
    fireEvent.change(screen.getByLabelText(/businessPage\.checkout\.emailLabel|Email/i), {
      target: { value: "jane@example.com" },
    });
    const tipInput = screen.getByLabelText(
      /businessPage\.checkout\.driverTip|Driver tip/i,
    ) as HTMLInputElement;
    fireEvent.change(tipInput, { target: { value: "-5" } });

    await act(async () => {
      fireEvent.click(screen.getByTestId("place-order-btn"));
    });

    await waitFor(() => {
      expect(guestDeliveryApi.checkout).toHaveBeenCalledTimes(1);
    });
    const payload = (guestDeliveryApi.checkout as jest.Mock).mock.calls[0][1];
    // "-5" was sanitized to "5" at the input, so the tip is positive.
    expect(payload.driver_tip).toBe(5);
  });

  it("discloses online payment mode before the order is placed (DLV-GUEST-5)", async () => {
    (guestDeliveryApi.getSettings as jest.Mock).mockResolvedValue({
      payment_mode: "online",
      online_payment_available: true,
    });

    render(<GuestDeliveryCheckout {...defaultProps} />);

    await waitFor(() => {
      expect(screen.getByTestId("payment-mode-disclosure")).toBeInTheDocument();
    });
    expect(
      screen.getByText("businessPage.checkout.payOnlineNotice"),
    ).toBeInTheDocument();
    expect(guestDeliveryApi.getSettings).toHaveBeenCalledWith(42);
  });

  it("discloses cash-on-delivery payment mode before the order is placed (DLV-GUEST-5)", async () => {
    (guestDeliveryApi.getSettings as jest.Mock).mockResolvedValue({
      payment_mode: "cash_on_delivery",
      online_payment_available: false,
    });

    render(<GuestDeliveryCheckout {...defaultProps} />);

    await waitFor(() => {
      expect(
        screen.getByText("businessPage.checkout.payCashNotice"),
      ).toBeInTheDocument();
    });
  });

  it("shows a retryable notice when the payment-mode settings fetch fails", async () => {
    (guestDeliveryApi.getSettings as jest.Mock)
      .mockRejectedValueOnce(new Error("network"))
      .mockResolvedValueOnce({ payment_mode: "online" });

    render(<GuestDeliveryCheckout {...defaultProps} />);

    expect(
      await screen.findByTestId("payment-mode-unavailable"),
    ).toBeInTheDocument();
    expect(screen.queryByTestId("payment-mode-disclosure")).not.toBeInTheDocument();

    await act(async () => {
      fireEvent.click(
        screen.getByRole("button", {
          name: /paymentModeRetry|Try again/i,
        }),
      );
    });

    expect(await screen.findByTestId("payment-mode-disclosure")).toBeInTheDocument();
    expect(screen.queryByTestId("payment-mode-unavailable")).not.toBeInTheDocument();
  });

  it("marks required contact fields invalid, describes them, and focuses the first one", async () => {
    render(<GuestDeliveryCheckout {...defaultProps} />);
    await act(async () => {
      fireEvent.click(screen.getByTestId("place-order-btn"));
    });

    const name = screen.getByLabelText(/fullName|Full name/i);
    const phone = screen.getByLabelText(/phoneNumber|Phone number/i);
    const email = screen.getByLabelText(/emailLabel|Email/i);

    expect(name).toHaveAttribute("aria-invalid", "true");
    expect(phone).toHaveAttribute("aria-invalid", "true");
    expect(email).toHaveAttribute("aria-invalid", "true");
    expect(name).toHaveAttribute("aria-describedby", "guest-delivery-checkout-name-error");
    expect(document.getElementById("guest-delivery-checkout-name-error")).toHaveTextContent(
      "businessPage.checkout.nameRequired",
    );
    expect(document.getElementById("guest-delivery-checkout-phone-error")).toHaveTextContent(
      "businessPage.checkout.phoneRequired",
    );
    expect(document.getElementById("guest-delivery-checkout-email-error")).toHaveTextContent(
      "businessPage.guestDelivery.emailRequired",
    );
    await waitFor(() => {
      expect(name).toHaveFocus();
    });
    expect(guestDeliveryApi.checkout).not.toHaveBeenCalled();

    fireEvent.change(name, { target: { value: "Jane Smith" } });
    expect(name).not.toHaveAttribute("aria-invalid", "true");
    expect(phone).toHaveAttribute("aria-invalid", "true");
  });

  it("marks an invalid email and does not place the order", async () => {
    render(<GuestDeliveryCheckout {...defaultProps} />);
    fireEvent.change(screen.getByLabelText(/fullName|Full name/i), {
      target: { value: "Jane Smith" },
    });
    fireEvent.change(screen.getByLabelText(/phoneNumber|Phone number/i), {
      target: { value: "+1 555 000 1234" },
    });
    fireEvent.change(screen.getByLabelText(/emailLabel|Email/i), {
      target: { value: "not-an-email" },
    });

    await act(async () => {
      fireEvent.click(screen.getByTestId("place-order-btn"));
    });

    const email = screen.getByLabelText(/emailLabel|Email/i);
    expect(email).toHaveAttribute("aria-invalid", "true");
    await waitFor(() => {
      expect(email).toHaveFocus();
    });
    expect(document.getElementById("guest-delivery-checkout-email-error")).toHaveTextContent(
      "businessPage.checkout.emailInvalid",
    );
    expect(guestDeliveryApi.checkout).not.toHaveBeenCalled();
  });

  it("submits contact fields with Enter without placing an invalid order", async () => {
    const { container } = render(<GuestDeliveryCheckout {...defaultProps} />);
    const form = container.querySelector("form#guest-delivery-checkout-form");
    expect(form).not.toBeNull();
    await act(async () => {
      fireEvent.submit(form as HTMLFormElement);
    });
    expect(guestDeliveryApi.checkout).not.toHaveBeenCalled();
    expect(screen.getByLabelText(/fullName|Full name/i)).toHaveAttribute("aria-invalid", "true");
  });

  it("exposes async checkout failures as a live alert", async () => {
    (guestDeliveryApi.quote as jest.Mock).mockResolvedValue({
      ...mockFulfillmentContext.quote,
      order_subtotal: 32,
    });
    (guestDeliveryApi.checkout as jest.Mock).mockRejectedValue(
      new Error("delivery capacity exceeded"),
    );

    render(<GuestDeliveryCheckout {...defaultProps} />);
    fireEvent.change(screen.getByLabelText(/fullName|Full name/i), {
      target: { value: "Jane Smith" },
    });
    fireEvent.change(screen.getByLabelText(/phoneNumber|Phone number/i), {
      target: { value: "+1 555 000 1234" },
    });
    fireEvent.change(screen.getByLabelText(/emailLabel|Email/i), {
      target: { value: "jane@example.com" },
    });

    await act(async () => {
      fireEvent.click(screen.getByTestId("place-order-btn"));
    });

    await waitFor(() => {
      expect(screen.getByTestId("delivery-checkout-error")).toHaveAttribute("role", "alert");
    });
  });
});

describe("GuestDeliveryCheckout localized contact validation", () => {
  const { translateKey } = jest.requireActual("@/i18n/GuestTranslationProvider") as {
    translateKey: (
      translations: Record<string, unknown>,
      key: string,
      params?: Record<string, string | number>,
    ) => string;
  };
  const locales = [
    ["en", require("@/i18n/guest-messages/en.json")],
    ["es", require("@/i18n/guest-messages/es.json")],
    ["es-AR", require("@/i18n/guest-messages/es-AR.json")],
  ] as const;

  beforeEach(() => {
    jest.clearAllMocks();
  });

  it.each(locales)("announces required contact errors in %s", async (lang, bundle) => {
    mockT.mockImplementation((key: string, params?: Record<string, string | number>) =>
      translateKey(bundle as Record<string, unknown>, key, params),
    );
    translationState.currentLanguage = lang;

    render(<GuestDeliveryCheckout {...defaultProps} />);
    await act(async () => {
      fireEvent.click(screen.getByTestId("place-order-btn"));
    });

    expect(document.getElementById("guest-delivery-checkout-name-error")).toHaveTextContent(
      translateKey(bundle as Record<string, unknown>, "businessPage.checkout.nameRequired"),
    );
    expect(document.getElementById("guest-delivery-checkout-phone-error")).toHaveTextContent(
      translateKey(bundle as Record<string, unknown>, "businessPage.checkout.phoneRequired"),
    );
    expect(document.getElementById("guest-delivery-checkout-email-error")).toHaveTextContent(
      translateKey(bundle as Record<string, unknown>, "businessPage.guestDelivery.emailRequired"),
    );
    await waitFor(() => {
      expect(
        screen.getByLabelText(
          translateKey(bundle as Record<string, unknown>, "businessPage.checkout.fullName"),
        ),
      ).toHaveFocus();
    });
    expect(guestDeliveryApi.checkout).not.toHaveBeenCalled();
  });
});
