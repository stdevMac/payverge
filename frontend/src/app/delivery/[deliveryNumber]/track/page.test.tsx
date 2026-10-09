/** @jest-environment jsdom */
/**
 * Tracking page: 5 guest stages, terminal cards, and payment state.
 */
import React from "react";
import { act, fireEvent, render, screen, waitFor } from "@testing-library/react";
import DeliveryTrackingPage from "./page";
import { guestDeliveryApi } from "@/api/delivery";
import type { PublicDeliveryTrackingDto } from "@/api/delivery";

// ── Module mocks ──────────────────────────────────────────────────────────────

jest.mock("next/navigation", () => ({
  useParams: () => ({ deliveryNumber: "DEL-X" }),
  useSearchParams: () => ({ get: () => null }),
}));

jest.mock("@/api/delivery", () => ({
  guestDeliveryApi: {
    track: jest.fn(),
  },
}));

// The diner's selected storefront locale, mutable per-test so we can prove the
// COD money amount follows it (mock-prefixed for jest's factory hoisting).
let mockTrackLocale = "en";

// Stub GuestTranslationProvider so the page renders without i18n assets.
// t() returns the key — makes assertions stable across locales.
jest.mock("@/i18n/GuestTranslationProvider", () => ({
  GuestTranslationProvider: ({ children }: { children: React.ReactNode }) => <>{children}</>,
  useGuestTranslation: () => ({
    t: (key: string, vars?: Record<string, string | number>) => {
      if (!vars) return key;
      // Try substituting {k} placeholders into the key.
      const substituted = Object.entries(vars).reduce(
        (s, [k, v]) => s.replace(`{${k}}`, String(v)),
        key,
      );
      // If nothing was substituted (no placeholder in the key), append the
      // var values so tests can still assert on their formatted output.
      const anySubstituted = substituted !== key;
      if (!anySubstituted) {
        const extras = Object.values(vars)
          .map((v) => String(v))
          .filter(Boolean)
          .join(" ");
        return extras ? `${key} ${extras}` : key;
      }
      return substituted;
    },
    currentLanguage: mockTrackLocale,
    setLanguage: jest.fn(),
    availableLanguages: {},
    setBusinessId: jest.fn(),
  }),
}));

jest.mock("@nextui-org/react", () => ({
  Card: ({ children, className }: any) => <div className={className}>{children}</div>,
  CardBody: ({ children }: any) => <div>{children}</div>,
  Spinner: () => <div data-testid="loading-spinner" />,
  Button: ({
    children,
    onPress,
    as: As,
    href,
    isLoading,
    isDisabled,
    color: _c,
    variant: _v,
    size: _s,
    startContent: _sc,
    endContent: _ec,
    ...rest
  }: any) => {
    if (As && href) {
      return (
        <a href={href} {...rest}>
          {children}
        </a>
      );
    }
    return (
      <button type="button" onClick={onPress} disabled={isLoading || isDisabled} {...rest}>
        {children}
      </button>
    );
  },
}));

// ── Helpers ───────────────────────────────────────────────────────────────────

const mockTrack = guestDeliveryApi.track as jest.Mock;

function buildTracking(overrides: Partial<PublicDeliveryTrackingDto> = {}): PublicDeliveryTrackingDto {
  return {
    delivery_number: "DEL-X",
    status: "preparing",
    business_name: "Test Bistro",
    ...overrides,
  };
}

// ── Tests ─────────────────────────────────────────────────────────────────────

beforeEach(() => {
  jest.clearAllMocks();
  mockTrackLocale = "en";
});

describe("5-stage ladder", () => {
  it("status 'ready' maps to the preparing stage and renders the ladder", async () => {
    mockTrack.mockResolvedValue(buildTracking({ status: "ready" }));
    render(<DeliveryTrackingPage />);

    const timeline = await screen.findByTestId("status-timeline");
    expect(timeline).toBeInTheDocument();

    // Stage label key for preparing is deliveryTracking.stages.preparing
    expect(screen.getByText("deliveryTracking.stages.preparing")).toBeInTheDocument();
    // No negative terminal card
    expect(screen.queryByTestId("terminal-card")).not.toBeInTheDocument();
  });

  it("names configured courier partners on the track page", async () => {
    mockTrack.mockResolvedValue(
      buildTracking({
        external_partner_links: [
          { name: "PedidosYa", url: "https://www.pedidosya.com.ar" },
          { name: "Rappi", url: "https://www.rappi.com.ar" },
        ],
      }),
    );
    render(<DeliveryTrackingPage />);
    await screen.findByTestId("status-timeline");
    expect(screen.getByTestId("track-couriers")).toBeInTheDocument();
    expect(screen.getByText("PedidosYa")).toBeInTheDocument();
    expect(screen.getByText("Rappi")).toBeInTheDocument();
  });

  it("status 'pending' maps to received stage", async () => {
    mockTrack.mockResolvedValue(buildTracking({ status: "pending" }));
    render(<DeliveryTrackingPage />);
    await screen.findByTestId("status-timeline");
    expect(screen.getByText("deliveryTracking.stages.received")).toBeInTheDocument();
  });

  it("status 'in_transit' maps to out_for_delivery stage", async () => {
    mockTrack.mockResolvedValue(buildTracking({ status: "in_transit" }));
    render(<DeliveryTrackingPage />);
    await screen.findByTestId("status-timeline");
    expect(screen.getByText("deliveryTracking.stages.out_for_delivery")).toBeInTheDocument();
  });

  it("announces courier pickup in a polite live region", async () => {
    mockTrack
      .mockResolvedValueOnce(buildTracking({ status: "preparing" }))
      .mockResolvedValueOnce(buildTracking({ status: "picked_up" }));

    render(<DeliveryTrackingPage />);
    await screen.findByTestId("status-timeline");

    fireEvent.click(
      screen.getByRole("button", { name: "deliveryTracking.refreshStatus" }),
    );

    await waitFor(() => {
      const live = screen.getByTestId("delivery-status-live");
      expect(live).toHaveAttribute("role", "status");
      expect(live).toHaveAttribute("aria-live", "polite");
      expect(live).toHaveTextContent(
        "deliveryTracking.courierPickedUpAnnouncement",
      );
    });
  });
});

describe("terminal cards", () => {
  it("status 'failed' renders the failed terminal card with no ladder", async () => {
    mockTrack.mockResolvedValue(buildTracking({ status: "failed" }));
    render(<DeliveryTrackingPage />);

    const card = await screen.findByTestId("terminal-card");
    expect(card).toBeInTheDocument();
    // Ladder must not appear
    expect(screen.queryByTestId("status-timeline")).not.toBeInTheDocument();
    // Failed title key
    expect(screen.getByText("deliveryTracking.failedTitle")).toBeInTheDocument();
  });

  it("status 'cancelled' renders the cancelled terminal card with no ladder", async () => {
    mockTrack.mockResolvedValue(buildTracking({ status: "cancelled" }));
    render(<DeliveryTrackingPage />);

    const card = await screen.findByTestId("terminal-card");
    expect(card).toBeInTheDocument();
    expect(screen.queryByTestId("status-timeline")).not.toBeInTheDocument();
    expect(screen.getByText("deliveryTracking.cancelledTitle")).toBeInTheDocument();
  });

  it("cancelled with an operator free-text reason renders it behind the localized 'from the restaurant' label", async () => {
    mockTrack.mockResolvedValue(
      buildTracking({ status: "cancelled", cancellation_reason: "No drivers available" }),
    );
    render(<DeliveryTrackingPage />);
    const card = await screen.findByTestId("terminal-card");
    // The mock appends un-substituted vars as trailing text, so the reason
    // value appears alongside the i18n key — confirming the code path ran.
    expect(card).toHaveTextContent("deliveryTracking.cancelledReasonFromRestaurant");
    expect(card).toHaveTextContent("No drivers available");
  });

  it("maps the machine reason 'payment window expired' to a localized key — no raw English on the guest surface", async () => {
    mockTrack.mockResolvedValue(
      buildTracking({ status: "cancelled", cancellation_reason: "payment window expired" }),
    );
    render(<DeliveryTrackingPage />);
    const card = await screen.findByTestId("terminal-card");
    expect(card).toHaveTextContent("deliveryTracking.cancelledReasonPaymentExpired");
    expect(card.textContent).not.toContain("payment window expired");
  });
});

describe("localized API errors", () => {
  it("shows the localized not-found message for 404s instead of the raw backend English", async () => {
    const err = new Error("Delivery not found") as Error & { status?: number };
    err.status = 404;
    mockTrack.mockRejectedValue(err);
    render(<DeliveryTrackingPage />);

    const banner = await screen.findByTestId("error-message");
    expect(banner).toHaveTextContent("deliveryTracking.notFound");
    expect(banner.textContent).not.toContain("Delivery not found");
  });

  it("falls back to the localized loadError for other failures, never error.message", async () => {
    mockTrack.mockRejectedValue(new Error("dial tcp: connection refused"));
    render(<DeliveryTrackingPage />);

    const banner = await screen.findByTestId("error-message");
    expect(banner).toHaveTextContent("deliveryTracking.loadError");
    expect(banner.textContent).not.toContain("dial tcp");
  });
});

describe("i18n: aria-label + timestamps", () => {
  it("translates the delivery-progress list aria-label", async () => {
    mockTrack.mockResolvedValue(buildTracking());
    render(<DeliveryTrackingPage />);
    await screen.findByTestId("status-timeline");
    expect(
      screen.getByRole("list", { name: "deliveryTracking.progressAria" }),
    ).toBeInTheDocument();
  });

  it("formats timestamps in the guest-selected locale, not the browser locale", async () => {
    // Guest picked Japanese: ja medium date style is 2026/07/06, while the
    // browser/default locale (en) would render "Jul 6, 2026".
    mockTrackLocale = "ja";
    mockTrack.mockResolvedValue(
      buildTracking({ status: "delivered", updated_at: "2026-07-06T12:00:00Z" }),
    );
    render(<DeliveryTrackingPage />);
    await screen.findByTestId("status-timeline");

    const el = screen.getByText(/deliveryTracking\.statusUpdated/);
    expect(el.textContent).toMatch(/2026\/0?7\/0?[56]/);
    expect(el.textContent).not.toContain("Jul 6");
  });
});

describe("background polling visibility gating", () => {
  const setDocumentHidden = (hidden: boolean) => {
    Object.defineProperty(document, "hidden", {
      configurable: true,
      get: () => hidden,
    });
    Object.defineProperty(document, "visibilityState", {
      configurable: true,
      get: () => (hidden ? "hidden" : "visible"),
    });
  };

  afterEach(() => {
    setDocumentHidden(false);
    jest.useRealTimers();
  });

  it("keeps polling every 15s while the tab is visible", async () => {
    jest.useFakeTimers();
    mockTrack.mockResolvedValue(buildTracking({ status: "preparing" }));
    render(<DeliveryTrackingPage />);
    await screen.findByTestId("status-timeline");
    const initialCalls = mockTrack.mock.calls.length;

    await act(async () => {
      jest.advanceTimersByTime(15_000);
    });
    expect(mockTrack.mock.calls.length).toBe(initialCalls + 1);
  });

  it("pauses polling while the tab is hidden and refreshes immediately on visible", async () => {
    jest.useFakeTimers();
    mockTrack.mockResolvedValue(buildTracking({ status: "preparing" }));
    render(<DeliveryTrackingPage />);
    await screen.findByTestId("status-timeline");
    const initialCalls = mockTrack.mock.calls.length;

    // Guest backgrounds the tab: no polls must fire while hidden.
    act(() => {
      setDocumentHidden(true);
      document.dispatchEvent(new Event("visibilitychange"));
    });
    await act(async () => {
      jest.advanceTimersByTime(46_000);
    });
    expect(mockTrack.mock.calls.length).toBe(initialCalls);

    // Tab becomes visible again: refresh immediately, then resume polling.
    await act(async () => {
      setDocumentHidden(false);
      document.dispatchEvent(new Event("visibilitychange"));
    });
    await act(async () => {
      await Promise.resolve();
    });
    expect(mockTrack.mock.calls.length).toBe(initialCalls + 1);

    await act(async () => {
      jest.advanceTimersByTime(15_000);
    });
    expect(mockTrack.mock.calls.length).toBe(initialCalls + 2);
  });
});

describe("payment state", () => {
  it("awaiting_payment renders the pay-now link to /delivery/DEL-X/pay", async () => {
    mockTrack.mockResolvedValue(
      buildTracking({
        status: "confirmed",
        awaiting_payment: true,
        payment_expires_at: new Date(Date.now() + 5 * 60 * 1000).toISOString(),
      }),
    );
    render(<DeliveryTrackingPage />);

    await screen.findByTestId("pay-now-card");
    const link = screen.getByTestId("pay-now-link") as HTMLAnchorElement;
    expect(link).toBeInTheDocument();
    expect(link.href).toContain("/delivery/DEL-X/pay");
  });

  it("bill.paid renders the paid chip", async () => {
    mockTrack.mockResolvedValue(
      buildTracking({
        status: "delivered",
        bill: { id: 1, bill_number: "B-1", status: "paid", total_amount: 25.0 as any, paid: true },
      }),
    );
    render(<DeliveryTrackingPage />);

    const chip = await screen.findByTestId("paid-chip");
    expect(chip).toBeInTheDocument();
    expect(chip).toHaveTextContent("deliveryTracking.paid");
  });

  it("cash_on_delivery while preparing shows the pay-driver-cash row with amount", async () => {
    mockTrack.mockResolvedValue(
      buildTracking({
        status: "preparing",
        payment_mode: "cash_on_delivery",
        bill: { id: 2, bill_number: "B-2", status: "open", total_amount: 17 as any, paid: false },
      }),
    );
    render(<DeliveryTrackingPage />);

    const row = await screen.findByTestId("cod-row");
    expect(row).toBeInTheDocument();
    // The i18n key must appear (confirms the right code path ran)
    expect(row.textContent).toContain("deliveryTracking.payDriverCash");
    // The formatted total must appear — Intl.NumberFormat('en', currency USD) → "$17.00"
    expect(row.textContent).toContain("$17.00");
  });

  it("cash_on_delivery uses the business currency, not a hardcoded USD", async () => {
    mockTrack.mockResolvedValue(
      buildTracking({
        status: "preparing",
        payment_mode: "cash_on_delivery",
        default_currency: "EUR",
        display_currency: "EUR",
        bill: { id: 4, bill_number: "B-4", status: "open", total_amount: 17 as any, paid: false },
      }),
    );
    render(<DeliveryTrackingPage />);

    const row = await screen.findByTestId("cod-row");
    // Intl with currency EUR formats with the euro sign, never the dollar sign.
    expect(row.textContent).toContain("€");
    expect(row.textContent).not.toContain("$");
  });

  it("cash_on_delivery formats the amount in the diner's selected locale, not the browser locale", async () => {
    // A German-speaking diner: money groups with '.' and decimates with ',' —
    // 1.234,56 €. The browser/default locale (en) would render €1,234.56, so the
    // German grouping proves the COD amount follows currentLanguage.
    mockTrackLocale = "de";
    mockTrack.mockResolvedValue(
      buildTracking({
        status: "preparing",
        payment_mode: "cash_on_delivery",
        default_currency: "EUR",
        display_currency: "EUR",
        bill: { id: 6, bill_number: "B-6", status: "open", total_amount: 1234.56 as any, paid: false },
      }),
    );
    render(<DeliveryTrackingPage />);

    const row = await screen.findByTestId("cod-row");
    expect(row.textContent).toContain("1.234,56");
    expect(row.textContent).not.toContain("1,234.56");
  });

  it("cash_on_delivery falls back to default_currency when display_currency is absent", async () => {
    mockTrack.mockResolvedValue(
      buildTracking({
        status: "preparing",
        payment_mode: "cash_on_delivery",
        default_currency: "EUR",
        bill: { id: 5, bill_number: "B-5", status: "open", total_amount: 17 as any, paid: false },
      }),
    );
    render(<DeliveryTrackingPage />);

    const row = await screen.findByTestId("cod-row");
    expect(row.textContent).toContain("€");
    expect(row.textContent).not.toContain("$");
  });

  it("cash_on_delivery is NOT shown after delivery is delivered", async () => {
    mockTrack.mockResolvedValue(
      buildTracking({
        status: "delivered",
        payment_mode: "cash_on_delivery",
        bill: { id: 3, bill_number: "B-3", status: "open", total_amount: 32.5 as any, paid: false },
      }),
    );
    render(<DeliveryTrackingPage />);
    await screen.findByTestId("status-timeline");
    expect(screen.queryByTestId("cod-row")).not.toBeInTheDocument();
  });

  it("awaiting_payment with expiry: the live countdown is inlined — literal __COUNTDOWN__ never rendered", async () => {
    mockTrack.mockResolvedValue(
      buildTracking({
        status: "confirmed",
        awaiting_payment: true,
        payment_expires_at: new Date(Date.now() + 5 * 60 * 1000).toISOString(),
      }),
    );
    render(<DeliveryTrackingPage />);

    const card = await screen.findByTestId("pay-now-card");
    // The live countdown element must be present
    expect(card.querySelector("[data-testid='payment-countdown']")).toBeInTheDocument();
    // The raw placeholder must never appear as text
    expect(card.textContent).not.toContain("__COUNTDOWN__");
  });
});

describe("driver card visibility", () => {
  it("driver card shown only while stage is out_for_delivery", async () => {
    mockTrack.mockResolvedValue(
      buildTracking({
        status: "in_transit",
        driver: { name: "Alex", phone: "+1234567890" },
      }),
    );
    render(<DeliveryTrackingPage />);
    await screen.findByTestId("status-timeline");
    expect(screen.getByText("Alex")).toBeInTheDocument();
  });

  it("driver card hidden when stage is preparing (even if driver present)", async () => {
    mockTrack.mockResolvedValue(
      buildTracking({
        status: "preparing",
        driver: { name: "Alex", phone: "+1234567890" },
      }),
    );
    render(<DeliveryTrackingPage />);
    await screen.findByTestId("status-timeline");
    expect(screen.queryByText("Alex")).not.toBeInTheDocument();
  });
});
