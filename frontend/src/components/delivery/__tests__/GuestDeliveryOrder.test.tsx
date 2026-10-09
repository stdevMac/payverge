/** @jest-environment jsdom */
import React from "react";
import { render, screen, fireEvent, waitFor, act } from "@testing-library/react";
import GuestDeliveryOrder from "../GuestDeliveryOrder";
import { guestDeliveryApi } from "@/api/delivery";

jest.mock("@/api/delivery", () => ({
  guestDeliveryApi: {
    quote: jest.fn(),
    checkout: jest.fn(),
    track: jest.fn(),
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

// Minimal NextUI mocks sufficient for the modal content.
// Note: jest.mock factories cannot reference out-of-scope variables so we
// avoid `React.createElement` and rely on JSX (transformed by babel-jest).
jest.mock("@nextui-org/react", () => ({
  Modal: ({ isOpen, children }: { isOpen: boolean; children: React.ReactNode }) =>
    isOpen ? <div data-testid="modal">{children}</div> : null,
  ModalContent: ({ children }: { children: React.ReactNode | ((onClose: () => void) => React.ReactNode) }) => (
    <div>{typeof children === "function" ? children(() => {}) : children}</div>
  ),
  ModalHeader: ({ children }: { children: React.ReactNode }) => <div>{children}</div>,
  ModalBody: ({ children }: { children: React.ReactNode }) => <div>{children}</div>,
  ModalFooter: ({ children }: { children: React.ReactNode }) => <div>{children}</div>,
  Input: require("react").forwardRef(function MockInput(
    {
      label,
      onChange,
      value,
      isRequired,
      isInvalid,
      errorMessage,
      id,
      startContent: _sc,
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
        />
        {isInvalid && errorMessage ? <p id={errorId}>{errorMessage}</p> : null}
      </div>
    );
  }),
  Textarea: ({ label, onChange, value, description }: any) => (
    <div>
      <label>{label}</label>
      <textarea aria-label={label} value={value ?? ""} onChange={onChange} />
      {description ? <span>{description}</span> : null}
    </div>
  ),
  Button: ({ children, onPress, isLoading, isDisabled, variant: _v, color: _c, ...rest }: any) => (
    <button type="button" onClick={onPress} disabled={isDisabled || isLoading} {...rest}>
      {isLoading ? "loading" : children}
    </button>
  ),
  Switch: ({ isSelected, onValueChange, ...rest }: any) => (
    <input
      type="checkbox"
      checked={isSelected ?? false}
      onChange={(e: React.ChangeEvent<HTMLInputElement>) => onValueChange?.(e.target.checked)}
      {...rest}
    />
  ),
  Card: ({ children, className, ...props }: any) => (
    <div className={className} {...props}>{children}</div>
  ),
  CardBody: ({ children }: any) => <div>{children}</div>,
  Chip: ({ children }: any) => <span>{children}</span>,
  Divider: () => <hr />,
}));

const makeQuoteResponse = (overrides = {}) => ({
  eligible: true,
  reason_code: "quote_available",
  message: "Delivery is available",
  delivery_fee: 5.99,
  minimum_order_amount: 20,
  free_delivery_minimum: 50,
  order_subtotal: 0,
  meets_minimum: true,
  estimated_prep_time: 15,
  estimated_delivery_minutes: 25,
  estimated_total_minutes: 40,
  fulfillment_mode: "delivery",
  partner_fallback_available: false,
  external_partner_links: [],
  ...overrides,
});

const defaultProps = {
  isOpen: true,
  onClose: jest.fn(),
  businessId: 42,
  businessName: "Test Restaurant",
};

describe("GuestDeliveryOrder", () => {
  beforeEach(() => {
    jest.clearAllMocks();
    mockT.mockImplementation((key: string) => key);
    translationState.currentLanguage = "en";
  });

  it("renders the modal with address fields when open", () => {
    render(<GuestDeliveryOrder {...defaultProps} />);
    expect(screen.getByTestId("modal")).toBeInTheDocument();
    expect(screen.getByLabelText(/Street address|guestDelivery\.streetAddress/i)).toBeInTheDocument();
    expect(screen.getByLabelText(/^City$|guestDelivery\.city/i)).toBeInTheDocument();
    expect(screen.getByLabelText(/^Country$|guestDelivery\.country/i)).toBeInTheDocument();
  });

  it("does not render when isOpen is false", () => {
    const { queryByTestId } = render(
      <GuestDeliveryOrder {...defaultProps} isOpen={false} />,
    );
    expect(queryByTestId("modal")).toBeNull();
  });

  it("shows a form validation error when required fields are missing and Check address is clicked", async () => {
    render(<GuestDeliveryOrder {...defaultProps} />);
    fireEvent.click(screen.getByText(/^(Check address|businessPage\.guestDelivery\.checkAddress)$/i));
    await waitFor(() => {
      expect(
        screen.getByText(/businessPage\.deliveryQuote\.addressRequired/i),
      ).toBeInTheDocument();
    });
    expect(guestDeliveryApi.quote).not.toHaveBeenCalled();
  });

  it("calls guestDeliveryApi.quote with the correct shape on valid submission", async () => {
    (guestDeliveryApi.quote as jest.Mock).mockResolvedValue(makeQuoteResponse());

    render(<GuestDeliveryOrder {...defaultProps} />);
    fireEvent.change(screen.getByLabelText(/Street address|guestDelivery\.streetAddress/i), {
      target: { value: "1 Main St" },
    });
    fireEvent.change(screen.getByLabelText(/^City$|guestDelivery\.city/i), {
      target: { value: "Dubai" },
    });
    fireEvent.change(screen.getByLabelText(/^Country$|guestDelivery\.country/i), {
      target: { value: "AE" },
    });

    await act(async () => {
      fireEvent.click(screen.getByText(/^(Check address|businessPage\.guestDelivery\.checkAddress)$/i));
    });

    expect(guestDeliveryApi.quote).toHaveBeenCalledWith(
      42,
      expect.objectContaining({
        order_subtotal: 0,
        delivery_address: expect.objectContaining({
          street: "1 Main St",
          city: "Dubai",
          country: "AE",
        }),
      }),
    );
  });

  it("displays quote details (fee, minimum, ETA) after a successful quote", async () => {
    (guestDeliveryApi.quote as jest.Mock).mockResolvedValue(makeQuoteResponse());

    render(<GuestDeliveryOrder {...defaultProps} />);
    fireEvent.change(screen.getByLabelText(/Street address|guestDelivery\.streetAddress/i), {
      target: { value: "1 Main St" },
    });
    fireEvent.change(screen.getByLabelText(/^City$|guestDelivery\.city/i), {
      target: { value: "Dubai" },
    });
    fireEvent.change(screen.getByLabelText(/^Country$|guestDelivery\.country/i), {
      target: { value: "AE" },
    });

    await act(async () => {
      fireEvent.click(screen.getByText(/^(Check address|businessPage\.guestDelivery\.checkAddress)$/i));
    });

    await waitFor(() => {
      expect(screen.getByText(/5\.99/)).toBeInTheDocument();
      // tString returns the i18n key when not resolved; production renders "40 min".
      expect(
        screen.getByText(/40 min|guestDelivery\.minutesValue/i),
      ).toBeInTheDocument();
    });
  });

  it("shows an error card when the quote API rejects with a 422-style message", async () => {
    (guestDeliveryApi.quote as jest.Mock).mockRejectedValue(
      new Error("The delivery address is outside this business's delivery area"),
    );

    render(<GuestDeliveryOrder {...defaultProps} />);
    fireEvent.change(screen.getByLabelText(/Street address|guestDelivery\.streetAddress/i), {
      target: { value: "999 Far Away Rd" },
    });
    fireEvent.change(screen.getByLabelText(/^City$|guestDelivery\.city/i), {
      target: { value: "Nowhere" },
    });
    fireEvent.change(screen.getByLabelText(/^Country$|guestDelivery\.country/i), {
      target: { value: "US" },
    });

    await act(async () => {
      fireEvent.click(screen.getByText(/^(Check address|businessPage\.guestDelivery\.checkAddress)$/i));
    });

    await waitFor(() => {
      // Backend English must be localized, never rendered raw. (DLV-GUEST-2)
      expect(
        screen.getByText("businessPage.deliveryQuote.outsideArea"),
      ).toBeInTheDocument();
    });
    expect(
      screen.queryByText(/outside this business's delivery area/i),
    ).not.toBeInTheDocument();
  });

  it("calls onQuoteReady with the context and closes when Browse menu is clicked after a successful quote", async () => {
    const onQuoteReady = jest.fn();
    const onClose = jest.fn();
    (guestDeliveryApi.quote as jest.Mock).mockResolvedValue(makeQuoteResponse());

    render(
      <GuestDeliveryOrder
        {...defaultProps}
        onClose={onClose}
        onQuoteReady={onQuoteReady}
      />,
    );
    fireEvent.change(screen.getByLabelText(/Street address|guestDelivery\.streetAddress/i), {
      target: { value: "1 Main St" },
    });
    fireEvent.change(screen.getByLabelText(/^City$|guestDelivery\.city/i), {
      target: { value: "Dubai" },
    });
    fireEvent.change(screen.getByLabelText(/^Country$|guestDelivery\.country/i), {
      target: { value: "AE" },
    });

    await act(async () => {
      fireEvent.click(screen.getByText(/^(Check address|businessPage\.guestDelivery\.checkAddress)$/i));
    });

    await waitFor(() => {
      expect(screen.getByText(/Browse menu with delivery|guestDelivery\.browseMenu/i)).not.toBeDisabled();
    });

    fireEvent.click(screen.getByText(/Browse menu with delivery|guestDelivery\.browseMenu/i));

    expect(onQuoteReady).toHaveBeenCalledTimes(1);
    expect(onQuoteReady).toHaveBeenCalledWith(
      expect.objectContaining({
        delivery_address: expect.objectContaining({
          street: "1 Main St",
          city: "Dubai",
          country: "AE",
        }),
        quote: expect.objectContaining({ eligible: true }),
      }),
    );
    expect(onClose).toHaveBeenCalled();
  });

  it("prefills the country field from defaultCountry instead of hardcoded US (F25)", () => {
    render(<GuestDeliveryOrder {...defaultProps} defaultCountry="France" />);
    const countryInput = screen.getByLabelText(
      /^Country$|guestDelivery\.country/i,
    ) as HTMLInputElement;
    expect(countryInput.value).toBe("France");
  });

  it("leaves the country field blank when defaultCountry is unknown (F25)", () => {
    render(<GuestDeliveryOrder {...defaultProps} />);
    const countryInput = screen.getByLabelText(
      /^Country$|guestDelivery\.country/i,
    ) as HTMLInputElement;
    expect(countryInput.value).toBe("");
  });

  it("does not surface the raw machine reason_code in the quote card (F11)", async () => {
    (guestDeliveryApi.quote as jest.Mock).mockResolvedValue(makeQuoteResponse());

    render(<GuestDeliveryOrder {...defaultProps} />);
    fireEvent.change(screen.getByLabelText(/Street address|guestDelivery\.streetAddress/i), {
      target: { value: "1 Main St" },
    });
    fireEvent.change(screen.getByLabelText(/^City$|guestDelivery\.city/i), {
      target: { value: "Dubai" },
    });
    fireEvent.change(screen.getByLabelText(/^Country$|guestDelivery\.country/i), {
      target: { value: "AE" },
    });

    await act(async () => {
      fireEvent.click(screen.getByText(/^(Check address|businessPage\.guestDelivery\.checkAddress)$/i));
    });

    await waitFor(() => {
      // The headline is a localized reason-code mapping, not the raw backend
      // message and not the snake_case machine token. (F11 + DLV-GUEST-2)
      expect(
        screen.getByText("businessPage.deliveryQuote.addressDeliverable"),
      ).toBeInTheDocument();
    });
    expect(screen.queryByText(/quote_available/)).not.toBeInTheDocument();
    expect(screen.queryByText("Delivery is available")).not.toBeInTheDocument();
  });

  it("keeps Browse menu button disabled when quote reason_code is not eligible", async () => {
    (guestDeliveryApi.quote as jest.Mock).mockResolvedValue(
      makeQuoteResponse({ eligible: false, reason_code: "zone_unavailable" }),
    );

    render(<GuestDeliveryOrder {...defaultProps} />);
    fireEvent.change(screen.getByLabelText(/Street address|guestDelivery\.streetAddress/i), {
      target: { value: "1 Far Rd" },
    });
    fireEvent.change(screen.getByLabelText(/^City$|guestDelivery\.city/i), {
      target: { value: "Anywhere" },
    });
    fireEvent.change(screen.getByLabelText(/^Country$|guestDelivery\.country/i), {
      target: { value: "US" },
    });

    await act(async () => {
      fireEvent.click(screen.getByText(/^(Check address|businessPage\.guestDelivery\.checkAddress)$/i));
    });

    await waitFor(() => {
      const browseBtn = screen.getByText(/Browse menu with delivery|guestDelivery\.browseMenu/i);
      expect(browseBtn).toBeDisabled();
    });
  });

  it("hides the zeroed fee/ETA metric cards when the address is out of zone (L-2)", async () => {
    (guestDeliveryApi.quote as jest.Mock).mockResolvedValue(
      makeQuoteResponse({
        eligible: false,
        reason_code: "zone_unavailable",
        message: "The delivery address is outside this business's delivery area",
        delivery_fee: 0,
        estimated_total_minutes: 0,
        estimated_prep_time: 0,
        estimated_delivery_minutes: 0,
      }),
    );

    render(<GuestDeliveryOrder {...defaultProps} />);
    fireEvent.change(screen.getByLabelText(/Street address|guestDelivery\.streetAddress/i), {
      target: { value: "1 Far Rd" },
    });
    fireEvent.change(screen.getByLabelText(/^City$|guestDelivery\.city/i), {
      target: { value: "Anywhere" },
    });
    fireEvent.change(screen.getByLabelText(/^Country$|guestDelivery\.country/i), {
      target: { value: "US" },
    });

    await act(async () => {
      fireEvent.click(screen.getByText(/^(Check address|businessPage\.guestDelivery\.checkAddress)$/i));
    });

    await waitFor(() => {
      expect(
        screen.getByText("businessPage.deliveryQuote.outsideArea"),
      ).toBeInTheDocument();
    });
    // The zeroed fee/ETA cards must NOT render — they'd contradict the
    // out-of-area warning shown just above them. (L-2)
    expect(
      screen.queryByText(/guestDelivery\.deliveryFee/i),
    ).not.toBeInTheDocument();
    expect(
      screen.queryByText(/guestDelivery\.estimatedTime/i),
    ).not.toBeInTheDocument();
  });

  it("offers partner fallback links when the quote is ineligible (#714)", async () => {
    (guestDeliveryApi.quote as jest.Mock).mockResolvedValue(
      makeQuoteResponse({
        eligible: false,
        reason_code: "zone_unavailable",
        partner_fallback_available: true,
        external_partner_links: [
          { name: "Uber Eats", url: "https://ubereats.com", provider_key: "ubereats" },
        ],
      }),
    );

    render(<GuestDeliveryOrder {...defaultProps} />);
    fireEvent.change(screen.getByLabelText(/Street address|guestDelivery\.streetAddress/i), {
      target: { value: "18 Demo Market St" },
    });
    fireEvent.change(screen.getByLabelText(/^City$|guestDelivery\.city/i), {
      target: { value: "New York" },
    });
    fireEvent.change(screen.getByLabelText(/^Country$|guestDelivery\.country/i), {
      target: { value: "US" },
    });
    await act(async () => {
      fireEvent.click(
        screen.getByText(/^(Check address|businessPage\.guestDelivery\.checkAddress)$/i),
      );
    });
    await waitFor(() => {
      expect(screen.getByText("Uber Eats")).toBeInTheDocument();
    });
  });

  const fillAddress = () => {
    fireEvent.change(screen.getByLabelText(/Street address|guestDelivery\.streetAddress/i), {
      target: { value: "1 Main St" },
    });
    fireEvent.change(screen.getByLabelText(/^City$|guestDelivery\.city/i), {
      target: { value: "Dubai" },
    });
    fireEvent.change(screen.getByLabelText(/^Country$|guestDelivery\.country/i), {
      target: { value: "AE" },
    });
  };

  it.each([
    ["delivery_disabled", "businessPage.deliveryQuote.deliveryDisabled"],
    ["outside_delivery_hours", "businessPage.deliveryQuote.outsideHours"],
    ["capacity_reached", "businessPage.deliveryQuote.capacityFull"],
    ["delivery_unavailable", "businessPage.deliveryQuote.notAvailable"],
    ["zone_unavailable", "businessPage.deliveryQuote.outsideArea"],
    ["some_future_code", "businessPage.deliveryQuote.notAvailable"],
  ])(
    "localizes the %s reason_code instead of rendering the English backend message (DLV-GUEST-2)",
    async (reasonCode, expectedKey) => {
      (guestDeliveryApi.quote as jest.Mock).mockResolvedValue(
        makeQuoteResponse({
          eligible: false,
          reason_code: reasonCode,
          message: "Hardcoded English backend message",
        }),
      );

      render(<GuestDeliveryOrder {...defaultProps} />);
      fillAddress();
      await act(async () => {
        fireEvent.click(
          screen.getByText(/^(Check address|businessPage\.guestDelivery\.checkAddress)$/i),
        );
      });

      await waitFor(() => {
        expect(screen.getByText(expectedKey)).toBeInTheDocument();
      });
      expect(
        screen.queryByText("Hardcoded English backend message"),
      ).not.toBeInTheDocument();
    },
  );

  it("falls back to a localized quoteFailed message when the thrown error is unmappable (DLV-GUEST-2)", async () => {
    (guestDeliveryApi.quote as jest.Mock).mockRejectedValue(
      new Error("some raw backend explosion"),
    );

    render(<GuestDeliveryOrder {...defaultProps} />);
    fillAddress();
    await act(async () => {
      fireEvent.click(
        screen.getByText(/^(Check address|businessPage\.guestDelivery\.checkAddress)$/i),
      );
    });

    await waitFor(() => {
      expect(
        screen.getByText("businessPage.deliveryQuote.quoteFailed"),
      ).toBeInTheDocument();
    });
    expect(screen.queryByText(/raw backend explosion/i)).not.toBeInTheDocument();
  });

  it("shows a localized helper line explaining why Browse menu is disabled after an out-of-area result", async () => {
    (guestDeliveryApi.quote as jest.Mock).mockResolvedValue(
      makeQuoteResponse({ eligible: false, reason_code: "zone_unavailable" }),
    );

    render(<GuestDeliveryOrder {...defaultProps} />);
    // No helper before a quote result exists.
    expect(screen.queryByTestId("continue-disabled-help")).not.toBeInTheDocument();

    fillAddress();
    await act(async () => {
      fireEvent.click(
        screen.getByText(/^(Check address|businessPage\.guestDelivery\.checkAddress)$/i),
      );
    });

    await waitFor(() => {
      expect(screen.getByTestId("continue-disabled-help")).toBeInTheDocument();
    });
    expect(
      screen.getByText(/Browse menu with delivery|guestDelivery\.browseMenu/i),
    ).toBeDisabled();
  });

  it("gives the contactless and leave-at-door switches accessible names (DLV-GUEST-7)", () => {
    render(<GuestDeliveryOrder {...defaultProps} />);
    expect(
      screen.getByRole("checkbox", {
        name: /businessPage\.guestDelivery\.contactless|Contactless delivery/i,
      }),
    ).toBeInTheDocument();
    expect(
      screen.getByRole("checkbox", {
        name: /businessPage\.guestDelivery\.leaveAtDoor|Leave at door/i,
      }),
    ).toBeInTheDocument();
  });

  it("does not force a numeric keyboard on the postal code field (DLV-GUEST-9)", () => {
    render(<GuestDeliveryOrder {...defaultProps} />);
    const postal = screen.getByLabelText(/Postal code|guestDelivery\.postalCode/i);
    expect(postal).not.toHaveAttribute("inputmode", "numeric");
  });

  it("submits the address form (Enter key semantics) and runs the quote check (DLV-GUEST-10)", async () => {
    (guestDeliveryApi.quote as jest.Mock).mockResolvedValue(makeQuoteResponse());

    const { container } = render(<GuestDeliveryOrder {...defaultProps} />);
    fillAddress();

    const form = container.querySelector("form#guest-delivery-address-form");
    expect(form).not.toBeNull();
    await act(async () => {
      fireEvent.submit(form as HTMLFormElement);
    });

    expect(guestDeliveryApi.quote).toHaveBeenCalledTimes(1);
  });

  it("prefills a saved delivery context when editing", () => {
    render(
      <GuestDeliveryOrder
        {...defaultProps}
        defaultCountry="US"
        initialContext={{
          delivery_address: {
            street: "9 Market St",
            apartment: "4B",
            city: "Dubai",
            state: "DU",
            postal_code: "00000",
            country: "AE",
            formatted_address: "9 Market St, 4B, Dubai, DU, 00000, AE",
          },
          delivery_instructions: "Ring twice",
          contactless_delivery: true,
          leave_at_door: true,
        }}
      />,
    );
    expect(
      (screen.getByLabelText(/Street address|guestDelivery\.streetAddress/i) as HTMLInputElement)
        .value,
    ).toBe("9 Market St");
    expect(
      (screen.getByLabelText(/Apartment|guestDelivery\.apartment/i) as HTMLInputElement).value,
    ).toBe("4B");
    expect((screen.getByLabelText(/^City$|guestDelivery\.city/i) as HTMLInputElement).value).toBe(
      "Dubai",
    );
    expect(
      (screen.getByLabelText(/State|guestDelivery\.state/i) as HTMLInputElement).value,
    ).toBe("DU");
    expect(
      (screen.getByLabelText(/Postal code|guestDelivery\.postalCode/i) as HTMLInputElement).value,
    ).toBe("00000");
    expect(
      (screen.getByLabelText(/^Country$|guestDelivery\.country/i) as HTMLInputElement).value,
    ).toBe("AE");
    expect(
      (screen.getByLabelText(/instructionsLabel|Delivery instructions/i) as HTMLTextAreaElement)
        .value,
    ).toBe("Ring twice");
    expect(
      screen.getByRole("checkbox", {
        name: /businessPage\.guestDelivery\.contactless|Contactless delivery/i,
      }),
    ).toBeChecked();
    expect(
      screen.getByRole("checkbox", {
        name: /businessPage\.guestDelivery\.leaveAtDoor|Leave at door/i,
      }),
    ).toBeChecked();
  });

  it("does not emit a replacement context when the editor is cancelled", () => {
    const onQuoteReady = jest.fn();
    const onClose = jest.fn();
    render(
      <GuestDeliveryOrder
        {...defaultProps}
        onClose={onClose}
        onQuoteReady={onQuoteReady}
        initialContext={{
          delivery_address: {
            street: "9 Market St",
            city: "Dubai",
            country: "AE",
            formatted_address: "9 Market St, Dubai, AE",
          },
          contactless_delivery: false,
          leave_at_door: false,
        }}
      />,
    );
    fireEvent.click(screen.getByText(/^(Close|businessPage\.guestDelivery\.close)$/i));
    expect(onClose).toHaveBeenCalled();
    expect(onQuoteReady).not.toHaveBeenCalled();
  });

  it("keeps the prior context unreplaced when a re-quote fails", async () => {
    const onQuoteReady = jest.fn();
    (guestDeliveryApi.quote as jest.Mock).mockRejectedValue(new Error("quote exploded"));
    render(
      <GuestDeliveryOrder
        {...defaultProps}
        onQuoteReady={onQuoteReady}
        initialContext={{
          delivery_address: {
            street: "9 Market St",
            city: "Dubai",
            country: "AE",
            formatted_address: "9 Market St, Dubai, AE",
          },
          contactless_delivery: false,
          leave_at_door: false,
        }}
      />,
    );
    await act(async () => {
      fireEvent.click(
        screen.getByText(/^(Check address|businessPage\.guestDelivery\.checkAddress)$/i),
      );
    });
    await waitFor(() => {
      expect(screen.getByTestId("delivery-form-error")).toHaveTextContent(
        "businessPage.deliveryQuote.quoteFailed",
      );
    });
    expect(onQuoteReady).not.toHaveBeenCalled();
  });

  it("marks required address fields invalid, describes them, and focuses the first one", async () => {
    render(<GuestDeliveryOrder {...defaultProps} />);
    fireEvent.click(
      screen.getByText(/^(Check address|businessPage\.guestDelivery\.checkAddress)$/i),
    );

    const street = screen.getByLabelText(/Street address|guestDelivery\.streetAddress/i);
    const city = screen.getByLabelText(/^City$|guestDelivery\.city/i);
    const country = screen.getByLabelText(/^Country$|guestDelivery\.country/i);

    expect(street).toHaveAttribute("aria-invalid", "true");
    expect(city).toHaveAttribute("aria-invalid", "true");
    expect(country).toHaveAttribute("aria-invalid", "true");
    expect(street).toHaveAttribute("aria-describedby", "guest-delivery-street-error");
    expect(city).toHaveAttribute("aria-describedby", "guest-delivery-city-error");
    expect(country).toHaveAttribute("aria-describedby", "guest-delivery-country-error");
    expect(document.getElementById("guest-delivery-street-error")).toHaveTextContent(
      "businessPage.guestDelivery.streetRequired",
    );
    expect(screen.getByTestId("delivery-form-error")).toHaveAttribute("role", "alert");
    await waitFor(() => {
      expect(street).toHaveFocus();
    });
    expect(guestDeliveryApi.quote).not.toHaveBeenCalled();

    fireEvent.change(street, { target: { value: "1 Main St" } });
    expect(street).not.toHaveAttribute("aria-invalid", "true");
    expect(city).toHaveAttribute("aria-invalid", "true");
  });
});

describe("GuestDeliveryOrder localized address validation", () => {
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

  it.each(locales)(
    "announces required address errors in %s",
    async (lang, bundle) => {
      mockT.mockImplementation((key: string, params?: Record<string, string | number>) =>
        translateKey(bundle as Record<string, unknown>, key, params),
      );
      translationState.currentLanguage = lang;

      render(<GuestDeliveryOrder {...defaultProps} />);
      fireEvent.click(
        screen.getByRole("button", {
          name: translateKey(
            bundle as Record<string, unknown>,
            "businessPage.guestDelivery.checkAddress",
          ),
        }),
      );

      const street = screen.getByLabelText(
        translateKey(bundle as Record<string, unknown>, "businessPage.guestDelivery.streetAddress"),
      );
      expect(street).toHaveAttribute("aria-invalid", "true");
      await waitFor(() => {
        expect(street).toHaveFocus();
      });
      expect(document.getElementById("guest-delivery-street-error")).toHaveTextContent(
        translateKey(bundle as Record<string, unknown>, "businessPage.guestDelivery.streetRequired"),
      );
      expect(document.getElementById("guest-delivery-city-error")).toHaveTextContent(
        translateKey(bundle as Record<string, unknown>, "businessPage.guestDelivery.cityRequired"),
      );
      expect(document.getElementById("guest-delivery-country-error")).toHaveTextContent(
        translateKey(bundle as Record<string, unknown>, "businessPage.guestDelivery.countryRequired"),
      );
      expect(guestDeliveryApi.quote).not.toHaveBeenCalled();
    },
  );
});
