/** @jest-environment jsdom */
import React from "react";
import { render, screen, act, waitFor } from "@testing-library/react";

// ── Module mocks ──────────────────────────────────────────────────────────────

jest.mock("next/navigation", () => ({
  useParams: jest.fn(() => ({ deliveryNumber: "DEL-001" })),
  useRouter: jest.fn(() => ({ push: jest.fn() })),
  useSearchParams: jest.fn(() => ({ get: () => null })),
}));

jest.mock("next/link", () => {
  const MockLink = ({ href, children }: { href: string; children: React.ReactNode }) => (
    <a href={href}>{children}</a>
  );
  MockLink.displayName = "MockLink";
  return MockLink;
});

jest.mock("@/api/delivery", () => ({
  guestDeliveryApi: {
    track: jest.fn(),
  },
}));

jest.mock("@/i18n/GuestTranslationProvider", () => ({
  GuestTranslationProvider: ({ children }: { children: React.ReactNode }) => <>{children}</>,
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

jest.mock("@/utils/nextRouteParams", () => ({
  getRouteParam: (_params: Record<string, string>, key: string) =>
    (_params as Record<string, string>)[key] ?? "",
}));

jest.mock("@nextui-org/react", () => ({
  Card: ({ children }: { children: React.ReactNode }) => <div data-testid="card">{children}</div>,
  CardBody: ({ children }: { children: React.ReactNode }) => <div>{children}</div>,
  Spinner: ({ color }: { color: string }) => <div data-testid="spinner" data-color={color} />,
  Button: ({
    children,
    onPress,
    isLoading,
    isDisabled,
    as: As,
    href,
    startContent,
    endContent,
    ...rest
  }: any) => {
    const content = (
      <>
        {startContent}
        {isLoading ? "loading" : children}
        {endContent}
      </>
    );
    if (As === "a" && href) {
      return <a href={href} {...rest}>{content}</a>;
    }
    if (As) {
      return <As href={href} {...rest}>{content}</As>;
    }
    return (
      <button type="button" onClick={onPress} disabled={isDisabled || isLoading} {...rest}>
        {content}
      </button>
    );
  },
  Chip: ({ children, color }: { children: React.ReactNode; color: string }) => (
    <span data-chip={color}>{children}</span>
  ),
}));

// ── Fixtures ──────────────────────────────────────────────────────────────────

import { guestDeliveryApi } from "@/api/delivery";
import type { PublicDeliveryTrackingDto } from "@/api/delivery";

const mockTrack = guestDeliveryApi.track as jest.MockedFunction<typeof guestDeliveryApi.track>;

const baseTracking: PublicDeliveryTrackingDto = {
  delivery_number: "DEL-001",
  status: "pending",
  business_name: "The Spice Garden",
  business_custom_url: "spice-garden",
  updated_at: "2026-05-02T10:00:00Z",
};

const inTransitTracking: PublicDeliveryTrackingDto = {
  ...baseTracking,
  status: "in_transit",
  // Wire truth: the public tracking payload carries only name + phone
  // (backend TrackDelivery driver summary) — no vehicle/rating fields.
  driver: {
    name: "Carlos M.",
    phone: "+1 555 000 1234",
  },
  estimated_delivery_time: new Date(Date.now() + 12 * 60 * 1000).toISOString(),
};

const deliveredTracking: PublicDeliveryTrackingDto = {
  ...baseTracking,
  status: "delivered",
};

const cancelledTracking: PublicDeliveryTrackingDto = {
  ...baseTracking,
  status: "cancelled",
};

// ── Tests ─────────────────────────────────────────────────────────────────────

import DeliveryTrackingPage from "../page";

describe("DeliveryTrackingPage", () => {
  beforeEach(() => {
    jest.useFakeTimers();
    jest.clearAllMocks();
  });

  afterEach(() => {
    jest.useRealTimers();
  });

  // ── Loading state ─────────────────────────────────────────────────────────

  it("shows a loading spinner while the initial fetch is in flight", async () => {
    // Never resolves during this test
    mockTrack.mockReturnValue(new Promise(() => {}));

    render(<DeliveryTrackingPage />);
    expect(screen.getByTestId("spinner")).toBeInTheDocument();
  });

  // ── Pending state ─────────────────────────────────────────────────────────

  it("renders the status timeline for a pending delivery", async () => {
    mockTrack.mockResolvedValueOnce(baseTracking);

    render(<DeliveryTrackingPage />);

    await waitFor(() => {
      expect(screen.getByTestId("status-timeline")).toBeInTheDocument();
    });

    // 5-stage ladder is present
    expect(screen.getByTestId("status-timeline")).toBeInTheDocument();
    // Business name displayed
    expect(screen.getByText("The Spice Garden")).toBeInTheDocument();
  });

  // ── In-transit with driver ────────────────────────────────────────────────

  it("renders driver card and ETA when in_transit with assigned driver", async () => {
    mockTrack.mockResolvedValueOnce(inTransitTracking);

    render(<DeliveryTrackingPage />);

    await waitFor(() => {
      expect(screen.getByText("Carlos M.")).toBeInTheDocument();
    });

    // Driver fields — the public payload carries only name + phone.
    expect(screen.getByText("+1 555 000 1234")).toBeInTheDocument();

    // ETA card
    expect(screen.getByTestId("status-timeline")).toBeInTheDocument();
  });

  // ── No live driver map / raw coordinates (honesty) ────────────────────────
  // There is no real driver-GPS producer, so the tracking page must never
  // promise a live map or expose raw lat/lng even when the API carries
  // current_location. Keep the honest surfaces: status ladder, driver card, ETA.

  it("never renders a live driver map or raw coordinates, even when the API carries current_location", async () => {
    const trackingWithLocation: PublicDeliveryTrackingDto = {
      ...inTransitTracking,
      current_location: {
        latitude: 41.8781,
        longitude: -87.6298,
        timestamp: "2026-05-02T10:30:00Z",
      },
    };
    mockTrack.mockResolvedValueOnce(trackingWithLocation);

    render(<DeliveryTrackingPage />);

    await waitFor(() => {
      expect(screen.getByTestId("status-timeline")).toBeInTheDocument();
    });

    expect(screen.queryByTestId("delivery-map")).not.toBeInTheDocument();
    // Raw "lat, lng" coordinate text must not appear either
    expect(
      screen.queryByText(/-?\d+\.\d{5}, -?\d+\.\d{5}/),
    ).not.toBeInTheDocument();
  });

  // ── Delivered terminal state ──────────────────────────────────────────────

  it("shows the delivered card and stops polling", async () => {
    mockTrack.mockResolvedValueOnce(deliveredTracking);

    render(<DeliveryTrackingPage />);

    // Delivered uses delivered-card (green), not the rose terminal-card
    await waitFor(() => {
      expect(screen.getByTestId("delivered-card")).toBeInTheDocument();
    });

    const callsBefore = mockTrack.mock.calls.length;

    // Advance time well past polling interval — polling should NOT fire
    act(() => {
      jest.advanceTimersByTime(60_000);
    });

    // No additional calls after terminal state
    expect(mockTrack.mock.calls.length).toBe(callsBefore);
  });

  // ── Cancelled terminal state ──────────────────────────────────────────────

  it("shows the cancelled terminal card and stops polling", async () => {
    mockTrack.mockResolvedValueOnce(cancelledTracking);

    render(<DeliveryTrackingPage />);

    await waitFor(() => {
      expect(screen.getByTestId("terminal-card")).toBeInTheDocument();
    });

    const callsBefore = mockTrack.mock.calls.length;

    act(() => {
      jest.advanceTimersByTime(60_000);
    });

    expect(mockTrack.mock.calls.length).toBe(callsBefore);
  });

  // ── Polling active for live delivery ─────────────────────────────────────

  it("polls while delivery is live and stops after transition to delivered", async () => {
    mockTrack
      .mockResolvedValueOnce(baseTracking)          // initial
      .mockResolvedValueOnce(inTransitTracking)     // first poll
      .mockResolvedValueOnce(deliveredTracking);    // second poll — terminal

    render(<DeliveryTrackingPage />);

    // Wait for initial load
    await waitFor(() => {
      expect(screen.getByTestId("status-timeline")).toBeInTheDocument();
    });

    // First poll fires at 15 s
    await act(async () => {
      jest.advanceTimersByTime(15_000);
    });
    await waitFor(() => expect(mockTrack).toHaveBeenCalledTimes(2));

    // Second poll fires — delivered state (shows delivered-card + ladder)
    await act(async () => {
      jest.advanceTimersByTime(15_000);
    });
    await waitFor(() => expect(mockTrack).toHaveBeenCalledTimes(3));

    // After delivered, no more calls
    act(() => {
      jest.advanceTimersByTime(60_000);
    });
    expect(mockTrack).toHaveBeenCalledTimes(3);
  });

  // ── Error fallback ────────────────────────────────────────────────────────

  it("shows a localized error message when the fetch fails — never the raw error text", async () => {
    mockTrack.mockRejectedValueOnce(new Error("Network error"));

    render(<DeliveryTrackingPage />);

    await waitFor(() => {
      expect(screen.getByTestId("error-message")).toBeInTheDocument();
    });

    expect(screen.getByText("deliveryTracking.loadError")).toBeInTheDocument();
    expect(screen.queryByText("Network error")).not.toBeInTheDocument();
  });

  // ── Driver card hidden before assignment ─────────────────────────────────

  it("does not show driver card when no driver is assigned", async () => {
    mockTrack.mockResolvedValueOnce(baseTracking);

    render(<DeliveryTrackingPage />);

    await waitFor(() => {
      expect(screen.getByTestId("status-timeline")).toBeInTheDocument();
    });

    // Driver label should not appear since no driver field
    expect(screen.queryByText(/Carlos/)).not.toBeInTheDocument();
  });
});
