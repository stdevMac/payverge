/** @jest-environment jsdom */
/**
 * Go-live rehearsal: on a first mobile visit the cookie banner (fixed bottom,
 * z-[10000]) sat over the cart sheet's Place Order button, and a tap on the
 * banner counted as a click outside the cart modal, closing the cart without
 * recording the choice. The cart now asks the banner to step aside while it is
 * open; the banner returns, still unanswered, when the cart closes.
 */
import React, { useState } from "react";
import {
  act,
  fireEvent,
  render,
  screen,
  waitFor,
} from "@testing-library/react";
import CartModal from "./CartModal";
import type { CartItem } from "../_types";
import { CookieConsent } from "@/components/CookieConsent";
import { CookieConsentProvider } from "@/contexts/CookieConsentContext";
import { CONSENT_STORAGE_KEY } from "@/lib/analytics/consentGate";

jest.mock("next/navigation", () => ({
  useRouter: () => ({ push: jest.fn(), replace: jest.fn(), prefetch: jest.fn() }),
  useSearchParams: () => new URLSearchParams(),
  usePathname: () => "/t/T1/menu",
  useParams: () => ({}),
}));

jest.mock("@/i18n/OperatorLocaleProvider", () => {
  const actual = jest.requireActual("@/i18n/OperatorLocaleProvider");
  return {
    ...actual,
    useSimpleLocale: () => ({ locale: "en", setLocale: () => {} }),
  };
});

jest.mock("../../../../../i18n/GuestTranslationProvider", () => ({
  useGuestTranslation: () => ({ currentLanguage: "en" }),
}));

jest.mock("@/components/common/CurrencyConverter", () => {
  const CurrencyPrice = ({ amount }: { amount: number }) => (
    <span>{amount}</span>
  );
  return {
    __esModule: true,
    default: ({ amount }: { amount: number }) => <span>{amount}</span>,
    CurrencyPrice,
  };
});

const cart: CartItem[] = [
  {
    itemType: "menu_item",
    name: "Burger",
    price: 10,
    quantity: 1,
    menuItemId: "i1",
  },
];

function Harness({ initiallyOpen }: { initiallyOpen: boolean }) {
  const [open, setOpen] = useState(initiallyOpen);
  return (
    <CookieConsentProvider>
      <button type="button" onClick={() => setOpen(true)}>
        open cart
      </button>
      <CartModal
        isOpen={open}
        onClose={() => setOpen(false)}
        cart={cart}
        orderLoading={false}
        quotePending={false}
        quoteError={null}
        quoteValid
        quoteBlocked={false}
        onUpdateQuantity={jest.fn()}
        onSubmitOrder={jest.fn()}
        onClearCart={jest.fn()}
        t={(key: string) => key}
        tableCode="T1"
        promotionPreview={{
          baseSubtotal: 10,
          discountTotal: 0,
          autoDiscountTotal: 0,
          promoDiscount: 0,
          discountedSubtotal: 10,
          netSubtotal: 10,
          tax: 0,
          serviceFee: 0,
          tip: 0,
          finalTotal: 10,
          applied: [],
        }}
        appliedPromo={null}
        onPromoApplied={jest.fn()}
        onPromoRemoved={jest.fn()}
        businessCurrencies={{
          default_currency: "USD",
          display_currency: "USD",
        }}
      />
      <button type="button" onClick={() => setOpen(false)}>
        close cart (test)
      </button>
      <CookieConsent />
    </CookieConsentProvider>
  );
}

const bannerTitle = /We value your privacy/i;

describe("CartModal and the cookie banner", () => {
  beforeEach(() => {
    localStorage.clear();
  });

  it("keeps the banner off the cart sheet while the cart is open", async () => {
    render(<Harness initiallyOpen />);
    expect(await screen.findByText("Burger")).toBeInTheDocument();
    // Give the provider and the copy loader time to settle: the banner would
    // have appeared by now if nothing suppressed it.
    await act(async () => {
      await new Promise((r) => setTimeout(r, 50));
    });
    expect(screen.queryByText(bannerTitle)).toBeNull();
    expect(document.querySelector("[data-cookie-consent]")).toBeNull();
    // The banner no longer reserves page-bottom space while it steps aside.
    expect(document.documentElement.dataset.cookieBanner).toBeUndefined();
    expect(localStorage.getItem(CONSENT_STORAGE_KEY)).toBeNull();
  });

  it("brings the unanswered banner back when the cart closes, and records the choice", async () => {
    render(<Harness initiallyOpen />);
    expect(await screen.findByText("Burger")).toBeInTheDocument();
    expect(screen.queryByText(bannerTitle)).toBeNull();

    fireEvent.click(screen.getByText("close cart (test)"));
    expect(await screen.findByText(bannerTitle)).toBeInTheDocument();

    fireEvent.click(screen.getByRole("button", { name: /Decline non-essential/i }));
    await waitFor(() =>
      expect(localStorage.getItem(CONSENT_STORAGE_KEY)).not.toBeNull(),
    );
    expect(screen.queryByText(bannerTitle)).toBeNull();
  });

  it("hides a visible banner when the cart opens", async () => {
    render(<Harness initiallyOpen={false} />);
    expect(await screen.findByText(bannerTitle)).toBeInTheDocument();

    fireEvent.click(screen.getByText("open cart"));
    expect(await screen.findByText("Burger")).toBeInTheDocument();
    await waitFor(() => expect(screen.queryByText(bannerTitle)).toBeNull());
    expect(localStorage.getItem(CONSENT_STORAGE_KEY)).toBeNull();
  });
});
