/** @jest-environment jsdom */
/**
 * Delivery pay page: guards payable window and wires PaymentSection.
 */
import React from "react";
import { act, render, screen, waitFor } from "@testing-library/react";
import DeliveryPayPage from "./page";
import { guestDeliveryApi } from "@/api/delivery";
import type { PublicDeliveryTrackingDto } from "@/api/delivery";

// ── Module mocks ──────────────────────────────────────────────────────────────

const mockRouterReplace = jest.fn();

// Allow individual tests to override the searchParams mock.
let mockSearchParamsGet: (key: string) => string | null = () => null;

jest.mock("next/navigation", () => ({
  useParams: () => ({ deliveryNumber: "DEL-X" }),
  useRouter: () => ({ replace: mockRouterReplace }),
  useSearchParams: () => ({ get: (k: string) => mockSearchParamsGet(k) }),
}));

jest.mock("@/api/delivery", () => ({
  guestDeliveryApi: {
    track: jest.fn(),
  },
}));

// Stub GuestTranslationProvider so the page renders without i18n assets.
// t() returns the key with vars substituted.
let lastGuestProviderProps: {
  initialLanguage?: string;
  preferInitialLanguage?: boolean;
} = {};
jest.mock("@/i18n/GuestTranslationProvider", () => ({
  GuestTranslationProvider: ({
    children,
    initialLanguage,
    preferInitialLanguage,
  }: {
    children: React.ReactNode;
    initialLanguage?: string;
    preferInitialLanguage?: boolean;
  }) => {
    lastGuestProviderProps = { initialLanguage, preferInitialLanguage };
    return <>{children}</>;
  },
  useGuestTranslation: () => ({
    t: (key: string, vars?: Record<string, string | number>) => {
      if (!vars) return key;
      return Object.entries(vars).reduce(
        (s, [k, v]) => s.replace(`{${k}}`, String(v)),
        key,
      );
    },
    currentLanguage: "en",
    setLanguage: jest.fn(),
    availableLanguages: {},
    setBusinessId: jest.fn(),
  }),
}));

// Stub NextUI components used by the page shell.
jest.mock("@nextui-org/react", () => ({
  Card: ({
    children,
    className,
  }: {
    children: React.ReactNode;
    className?: string;
  }) => <div className={className}>{children}</div>,
  CardBody: ({ children }: { children: React.ReactNode }) => (
    <div>{children}</div>
  ),
}));

// Stub PaymentSection — we only verify it's mounted with the correct props.
jest.mock("@/components/guest/PaymentSection", () => ({
  __esModule: true,
  default: (props: {
    billId: number;
    amount: number;
    businessId: number;
    billToken: string;
    hideTip?: boolean;
  }) => (
    <div
      data-testid="payment-section-stub"
      data-bill-id={props.billId}
      data-amount={props.amount}
      data-business-id={props.businessId}
      data-bill-number={props.billToken}
      data-hide-tip={String(!!props.hideTip)}
    />
  ),
}));

// ── Helpers ───────────────────────────────────────────────────────────────────

const mockTrack = guestDeliveryApi.track as jest.Mock;

const FUTURE_EXPIRY = new Date(Date.now() + 10 * 60 * 1000).toISOString();
const PAST_EXPIRY = new Date(Date.now() - 1000).toISOString();

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
    payment_expires_at: FUTURE_EXPIRY,
    bill: {
      id: 7,
      bill_number: "BILL-001",
      public_token: "bill-capability-001",
      status: "open",
      total_amount: 25.5 as any,
      paid: false,
      settlement_address: "0xABC",
      tipping_address: "0xDEF",
    },
    default_currency: "USD",
    display_currency: "USD",
    ...overrides,
  };
}

// ── Tests ─────────────────────────────────────────────────────────────────────

beforeEach(() => {
  jest.clearAllMocks();
  mockSearchParamsGet = () => null;
  lastGuestProviderProps = {};
});

describe("guest locale (#443)", () => {
  it("seeds GuestTranslationProvider from an explicit ?lang= query", async () => {
    mockSearchParamsGet = (key: string) => (key === "lang" ? "es-AR" : null);
    mockTrack.mockRejectedValue(new Error("not found"));
    render(<DeliveryPayPage />);
    await screen.findByText("deliveryPay.loadError");
    expect(lastGuestProviderProps.initialLanguage).toBe("es-AR");
    expect(lastGuestProviderProps.preferInitialLanguage).toBe(true);
  });
});

describe("payable state", () => {
  it("renders PaymentSection with the bill amount when awaiting_payment and within expiry", async () => {
    mockTrack.mockResolvedValue(buildTracking());
    render(<DeliveryPayPage />);

    const section = await screen.findByTestId("payment-section-stub");
    expect(section).toBeInTheDocument();
    expect(section).toHaveAttribute("data-bill-id", "7");
    expect(section).toHaveAttribute("data-amount", "25.5");
    expect(section).toHaveAttribute("data-business-id", "42");
    expect(section).toHaveAttribute("data-bill-number", "bill-capability-001");
  });

  it("suppresses the pay-page tip selector (driver tip already in the bill)", async () => {
    // The driver tip entered at checkout is baked into bill.total_amount, so the
    // pay page must pass hideTip to avoid a second gratuity prompt that would
    // double-charge a tip on top of the authoritative bill total.
    mockTrack.mockResolvedValue(buildTracking());
    render(<DeliveryPayPage />);

    const section = await screen.findByTestId("payment-section-stub");
    expect(section).toHaveAttribute("data-hide-tip", "true");
  });

  it("shows the page title key while payable", async () => {
    mockTrack.mockResolvedValue(buildTracking());
    render(<DeliveryPayPage />);

    await screen.findByTestId("payment-section-stub");
    expect(screen.getByText("deliveryPay.title")).toBeInTheDocument();
  });

  it("does not crash when bill lacks public_token — recoverable error + track link", async () => {
    // Backend historically omitted public_token from TrackDelivery; guestBillRef
    // used to throw and the error boundary swallowed the pay form. Soft-fail instead.
    mockTrack.mockResolvedValue(
      buildTracking({
        bill: {
          id: 7,
          bill_number: "BILL-001",
          public_token: "",
          status: "open",
          total_amount: 25.5 as any,
          paid: false,
        },
      }),
    );
    render(<DeliveryPayPage />);

    expect(await screen.findByText("deliveryPay.loadError")).toBeInTheDocument();
    expect(screen.getByText("deliveryPay.trackOrder")).toBeInTheDocument();
    expect(screen.queryByTestId("payment-section-stub")).not.toBeInTheDocument();
  });
});

describe("redirect guards", () => {
  it("redirects to track page when awaiting_payment is false", async () => {
    mockTrack.mockResolvedValue(buildTracking({ awaiting_payment: false }));
    render(<DeliveryPayPage />);

    await waitFor(() => {
      expect(mockRouterReplace).toHaveBeenCalledWith("/delivery/DEL-X/track");
    });
    expect(
      screen.queryByTestId("payment-section-stub"),
    ).not.toBeInTheDocument();
  });

  it("redirects to track page when payment_expires_at is in the past", async () => {
    mockTrack.mockResolvedValue(
      buildTracking({ payment_expires_at: PAST_EXPIRY }),
    );
    render(<DeliveryPayPage />);

    await waitFor(() => {
      expect(mockRouterReplace).toHaveBeenCalledWith("/delivery/DEL-X/track");
    });
    expect(
      screen.queryByTestId("payment-section-stub"),
    ).not.toBeInTheDocument();
  });

  it("redirects to track page when bill is null", async () => {
    mockTrack.mockResolvedValue(buildTracking({ bill: null }));
    render(<DeliveryPayPage />);

    await waitFor(() => {
      expect(mockRouterReplace).toHaveBeenCalledWith("/delivery/DEL-X/track");
    });
    expect(
      screen.queryByTestId("payment-section-stub"),
    ).not.toBeInTheDocument();
  });

  it("redirects to track page when business_id is missing", async () => {
    mockTrack.mockResolvedValue(buildTracking({ business_id: undefined }));
    render(<DeliveryPayPage />);

    await waitFor(() => {
      expect(mockRouterReplace).toHaveBeenCalledWith("/delivery/DEL-X/track");
    });
    expect(
      screen.queryByTestId("payment-section-stub"),
    ).not.toBeInTheDocument();
  });

  it("redirects to track page on ?payment=success without rendering PaymentSection", async () => {
    // Guest returns from a plugin redirect with ?payment=success.
    // The page should route to track immediately so the polling page can
    // confirm the payment once the webhook lands — no form re-shown.
    mockSearchParamsGet = (k: string) => (k === "payment" ? "success" : null);
    // API may or may not have resolved yet — either way the redirect fires.
    mockTrack.mockResolvedValue(buildTracking());
    render(<DeliveryPayPage />);

    await waitFor(() => {
      expect(mockRouterReplace).toHaveBeenCalledWith("/delivery/DEL-X/track");
    });
    expect(
      screen.queryByTestId("payment-section-stub"),
    ).not.toBeInTheDocument();
  });
});

describe("remaining amount (partial payments)", () => {
  it("charges total_amount minus paid_amount, not the full bill total", async () => {
    // Staff recorded a $10 alternative payment on a $25.50 bill — the guest
    // must only be offered the $15.50 remainder (crypto is irreversible).
    const base = buildTracking();
    mockTrack.mockResolvedValue({
      ...base,
      bill: {
        ...base.bill!,
        total_amount: 25.5 as any,
        paid_amount: 10 as any,
      },
    });
    render(<DeliveryPayPage />);

    const section = await screen.findByTestId("payment-section-stub");
    expect(section).toHaveAttribute("data-amount", "15.5");
  });

  it("clamps the charge at zero when paid_amount exceeds the total", async () => {
    const base = buildTracking();
    mockTrack.mockResolvedValue({
      ...base,
      bill: {
        ...base.bill!,
        total_amount: 25.5 as any,
        paid_amount: 30 as any,
      },
    });
    render(<DeliveryPayPage />);

    const section = await screen.findByTestId("payment-section-stub");
    expect(section).toHaveAttribute("data-amount", "0");
  });
});

describe("live payment-window gate", () => {
  afterEach(() => {
    jest.useRealTimers();
  });

  it("redirects away the moment the payment window expires, without a re-fetch", async () => {
    jest.useFakeTimers();
    mockTrack.mockResolvedValue(
      buildTracking({
        payment_expires_at: new Date(Date.now() + 3_000).toISOString(),
      }),
    );
    render(<DeliveryPayPage />);

    const section = await screen.findByTestId("payment-section-stub");
    expect(section).toBeInTheDocument();
    expect(mockRouterReplace).not.toHaveBeenCalled();

    // Cross the deadline: the gate must flip live and tear the form down
    // before any wallet interaction can start.
    await act(async () => {
      jest.advanceTimersByTime(5_000);
    });

    expect(mockRouterReplace).toHaveBeenCalledWith("/delivery/DEL-X/track");
    expect(
      screen.queryByTestId("payment-section-stub"),
    ).not.toBeInTheDocument();
  });

  it("re-fetches tracking on an interval and redirects when the order stops being payable", async () => {
    jest.useFakeTimers();
    // Flag-driven server state: payable until the "sweeper" cancels it.
    let serverCancelled = false;
    mockTrack.mockImplementation(async () =>
      serverCancelled
        ? buildTracking({ awaiting_payment: false, status: "cancelled" })
        : buildTracking(),
    );
    render(<DeliveryPayPage />);

    // Initial load resolves with a payable order.
    await act(async () => {
      await Promise.resolve();
    });
    const initialCalls = mockTrack.mock.calls.length;
    expect(initialCalls).toBeGreaterThanOrEqual(1);
    expect(mockRouterReplace).not.toHaveBeenCalled();
    expect(screen.getByTestId("payment-section-stub")).toBeInTheDocument();

    // The stale-page scenario: the sweeper cancels the order server-side;
    // the page must notice on its own within a poll cycle.
    serverCancelled = true;
    await act(async () => {
      jest.advanceTimersByTime(16_000);
    });

    expect(mockTrack.mock.calls.length).toBeGreaterThan(initialCalls);
    await act(async () => {
      await Promise.resolve();
    });
    expect(mockRouterReplace).toHaveBeenCalledWith("/delivery/DEL-X/track");
    expect(
      screen.queryByTestId("payment-section-stub"),
    ).not.toBeInTheDocument();
  });

  it("a background re-fetch failure does not nuke a loaded pay form into the error state", async () => {
    jest.useFakeTimers();
    mockTrack
      .mockResolvedValueOnce(buildTracking())
      .mockRejectedValue(new Error("network blip"));
    render(<DeliveryPayPage />);

    await screen.findByTestId("payment-section-stub");

    await act(async () => {
      jest.advanceTimersByTime(16_000);
    });

    expect(screen.getByTestId("payment-section-stub")).toBeInTheDocument();
    expect(screen.queryByText("deliveryPay.loadError")).not.toBeInTheDocument();
  });
});

describe("error state", () => {
  it("shows the loadError key when the API rejects", async () => {
    mockTrack.mockRejectedValue(new Error("network error"));
    render(<DeliveryPayPage />);

    await screen.findByText("deliveryPay.loadError");
    expect(
      screen.queryByTestId("payment-section-stub"),
    ).not.toBeInTheDocument();
  });

  it("exposes a page heading on the invalid-record recovery view", async () => {
    mockTrack.mockRejectedValue(new Error("not found"));
    render(<DeliveryPayPage />);

    await screen.findByText("deliveryPay.loadError");
    const heading =
      screen.queryByRole("heading", { level: 1 }) ??
      screen.getByRole("heading", { level: 2 });
    expect(heading).toBeVisible();
    expect(heading).toHaveTextContent("deliveryPay.title");
  });
});

describe("load error state", () => {
  it("renders retry + track CTAs on load error and recovers on retry", async () => {
    mockTrack
      .mockRejectedValueOnce(new Error("network"))
      .mockResolvedValueOnce(buildTracking());
    render(<DeliveryPayPage />);

    await screen.findByText("deliveryPay.loadError");
    const trackLink = screen.getByRole("link", {
      name: "deliveryPay.trackOrder",
    });
    expect(trackLink).toHaveAttribute("href", "/delivery/DEL-X/track");

    await act(async () => {
      screen.getByRole("button", { name: "deliveryPay.retry" }).click();
    });

    expect(
      await screen.findByTestId("payment-section-stub"),
    ).toBeInTheDocument();
    expect(screen.queryByText("deliveryPay.loadError")).not.toBeInTheDocument();
  });
});
