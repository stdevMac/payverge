/** @jest-environment jsdom */
/**
 * The cart already steps the cookie banner aside (CartModal.cookieBanner
 * test). The order-success modal opens right after Place Order with its
 * "View bill" / "Continue" footer on the same bottom edge, so it does too.
 */
import React, { useState } from "react";
import { act, fireEvent, render, screen } from "@testing-library/react";
import OrderSuccessModal from "./OrderSuccessModal";
import { CookieConsent } from "@/components/CookieConsent";
import { CookieConsentProvider } from "@/contexts/CookieConsentContext";
import { CONSENT_STORAGE_KEY } from "@/lib/analytics/consentGate";

jest.mock("next/navigation", () => ({
  useRouter: () => ({
    push: jest.fn(),
    replace: jest.fn(),
    prefetch: jest.fn(),
  }),
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

function Harness({ initiallyOpen }: { initiallyOpen: boolean }) {
  const [open, setOpen] = useState(initiallyOpen);
  return (
    <CookieConsentProvider>
      <button type="button" onClick={() => setOpen(true)}>
        open success
      </button>
      <OrderSuccessModal
        isOpen={open}
        onClose={() => setOpen(false)}
        message="Order placed"
        tableCode="T1"
        t={(key: string) => key}
      />
      <button type="button" onClick={() => setOpen(false)}>
        close success (test)
      </button>
      <CookieConsent />
    </CookieConsentProvider>
  );
}

const bannerTitle = /We value your privacy/i;

describe("OrderSuccessModal and the cookie banner", () => {
  beforeEach(() => {
    localStorage.clear();
  });

  it("keeps the banner hidden while open and brings it back unanswered on close", async () => {
    render(<Harness initiallyOpen />);
    expect(await screen.findByText("Order placed")).toBeInTheDocument();
    await act(async () => {
      await new Promise((r) => setTimeout(r, 50));
    });
    expect(screen.queryByText(bannerTitle)).toBeNull();

    fireEvent.click(screen.getByText("close success (test)"));
    expect(await screen.findByText(bannerTitle)).toBeInTheDocument();
    expect(localStorage.getItem(CONSENT_STORAGE_KEY)).toBeNull();
  });
});
