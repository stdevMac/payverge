/** @jest-environment jsdom */
import { renderHook } from "@testing-library/react";
import { useRailBadges } from "../useRailBadges";
import type { RailAlertCounts } from "../useRailBadges";
import type { Reservation } from "@/api/reservations";

/**
 * M3 — rail badge derivation, consolidated. Rules pinned here:
 * Bills = tickets waiting for approval (#793 — open checks are tab copy,
 * never a red pip), Kitchen = tickets waiting on the cook, pending + approved
 * (#792 — must match the tab, not kitchen_order_ready alert pings),
 * Reservations = seated-window count, Tables = occupancy (#774) with
 * service-call fallback, Delivery = alerts-only, Accounting/Fiscal = failed
 * invoices, Staff = unread chat.
 */

jest.mock("@/hooks/useFailedInvoiceCount", () => ({
  useFailedInvoiceCount: jest.fn(() => 0),
}));
jest.mock("@/hooks/useChatUnreadCount", () => ({
  useChatUnreadCount: jest.fn(() => 0),
}));

const { useFailedInvoiceCount } = jest.requireMock(
  "@/hooks/useFailedInvoiceCount",
) as { useFailedInvoiceCount: jest.Mock };
const { useChatUnreadCount } = jest.requireMock(
  "@/hooks/useChatUnreadCount",
) as { useChatUnreadCount: jest.Mock };

const BASE = {
  businessId: 7,
  hasAccess: true,
  alertCounts: null as RailAlertCounts | null,
  occupiedTablesCount: undefined as number | undefined,
  globalOrders: {} as Record<number, any[]>,
  globalOrdersLoaded: undefined as boolean | undefined,
  upcomingReservations: [] as Reservation[],
};

function renderBadges(overrides: Partial<typeof BASE> = {}) {
  return renderHook(() => useRailBadges({ ...BASE, ...overrides }));
}

beforeEach(() => {
  jest.clearAllMocks();
  useFailedInvoiceCount.mockReturnValue(0);
  useChatUnreadCount.mockReturnValue(0);
});

describe("useRailBadges — bills (#793 pending approvals, not open checks)", () => {
  it("counts pending-approval tickets from the loaded orders feed", () => {
    const { result } = renderBadges({
      globalOrdersLoaded: true,
      globalOrders: {
        1143: [{ status: "pending" }, { status: "pending" }],
        761: [{ status: "in_kitchen" }],
      },
    });
    expect(result.current.bills).toBe(2);
  });

  it("is null with open checks but an empty approval queue (#793 repro)", () => {
    const { result } = renderBadges({
      globalOrdersLoaded: true,
      // Two open checks' worth of live tickets, none pending — the QA night.
      globalOrders: {
        1143: [{ status: "approved" }, { status: "approved" }],
        761: [{ status: "in_kitchen" }],
      },
      alertCounts: {
        bills: 2,
        kitchen: 0,
        reservations: 0,
        delivery: 0,
        tables: 0,
      },
    });
    expect(result.current.bills).toBeNull();
  });

  it("bridges with alertCounts.bills before the first orders reconcile", () => {
    const { result } = renderBadges({
      globalOrdersLoaded: false,
      alertCounts: {
        bills: 3,
        kitchen: 0,
        reservations: 0,
        delivery: 0,
        tables: 0,
      },
    });
    expect(result.current.bills).toBe(3);
  });
});

describe("useRailBadges — kitchen (#792 tickets waiting on the cook)", () => {
  it("counts pending + approved from the loaded feed, matching the tab", () => {
    const { result } = renderBadges({
      globalOrdersLoaded: true,
      // Approved=2, In Kitchen=3, All=5 — the badge must read 2, not the
      // kitchen_order_ready alert count (1 or blank on the QA night).
      globalOrders: {
        1143: [{ status: "approved" }, { status: "approved" }],
        847: [{ status: "in_kitchen" }],
        848: [{ status: "in_kitchen" }],
        850: [{ status: "in_kitchen" }],
      },
      alertCounts: {
        bills: 0,
        kitchen: 1,
        reservations: 0,
        delivery: 0,
        tables: 0,
      },
    });
    expect(result.current.kitchen).toBe(2);
  });

  it("includes needs-approval tickets so the rail is never blank pre-approve", () => {
    const { result } = renderBadges({
      globalOrdersLoaded: true,
      globalOrders: {
        1143: [{ status: "pending" }, { status: "pending" }],
      },
      alertCounts: {
        bills: 0,
        kitchen: 0,
        reservations: 0,
        delivery: 0,
        tables: 0,
      },
    });
    expect(result.current.kitchen).toBe(2);
  });

  it("bridges with alertCounts.kitchen before the first orders reconcile", () => {
    const { result } = renderBadges({
      globalOrdersLoaded: false,
      alertCounts: {
        bills: 0,
        kitchen: 4,
        reservations: 0,
        delivery: 0,
        tables: 0,
      },
    });
    expect(result.current.kitchen).toBe(4);
  });

  it("counts from globalOrders without alerts or the loaded flag (tests)", () => {
    const { result } = renderBadges({
      globalOrders: {
        1: [{ status: "approved" }, { status: "pending" }],
        2: [{ status: "approved" }],
      },
    });
    expect(result.current.kitchen).toBe(3);
  });

  it("returns null on a quiet queue", () => {
    const { result } = renderBadges();
    expect(result.current.kitchen).toBeNull();
  });
});

describe("useRailBadges — reservations (seated window)", () => {
  const mkReservation = (minutesFromNow: number, status: string) =>
    ({
      reservation_time: new Date(
        Date.now() + minutesFromNow * 60 * 1000,
      ).toISOString(),
      status,
    }) as Reservation;

  it("counts confirmed/pending reservations in the -15/+30 window", () => {
    const { result } = renderBadges({
      upcomingReservations: [
        mkReservation(-10, "confirmed"),
        mkReservation(20, "pending"),
        mkReservation(45, "confirmed"), // outside window
        mkReservation(10, "cancelled"), // invalid status
      ],
    });
    expect(result.current.reservations).toBe(2);
  });

  it("prefers alertCounts.reservations when alerts are wired", () => {
    const { result } = renderBadges({
      alertCounts: {
        bills: 0,
        kitchen: 0,
        reservations: 8,
        delivery: 0,
        tables: 0,
      },
      upcomingReservations: [mkReservation(5, "confirmed")],
    });
    expect(result.current.reservations).toBe(8);
  });
});

describe("useRailBadges — tables occupancy (#774) and alerts", () => {
  it("keeps Mesas at leftover-kitchen occupancy while Cuentas is approvals (#774/#793)", () => {
    const { result } = renderBadges({
      globalOrdersLoaded: true,
      globalOrders: { 1143: [{ status: "pending" }] },
      occupiedTablesCount: 5,
      alertCounts: {
        bills: 2,
        kitchen: 0,
        reservations: 0,
        delivery: 0,
        tables: 0,
      },
    });
    expect(result.current.bills).toBe(1);
    expect(result.current.tables).toBe(5);
  });

  it("prefers dashboard-polled occupancy over service-call alerts", () => {
    const { result } = renderBadges({
      occupiedTablesCount: 5,
      alertCounts: {
        bills: 0,
        kitchen: 0,
        reservations: 0,
        delivery: 2,
        tables: 1,
      },
    });
    expect(result.current.tables).toBe(5);
    expect(result.current.delivery).toBe(2);
  });

  it("falls back to service-call alerts when occupancy is zero", () => {
    const { result } = renderBadges({
      occupiedTablesCount: 0,
      alertCounts: {
        bills: 0,
        kitchen: 0,
        reservations: 0,
        delivery: 0,
        tables: 3,
      },
    });
    expect(result.current.tables).toBe(3);
  });

  it("tables and delivery are null without occupancy or alerts", () => {
    const { result } = renderBadges();
    expect(result.current.tables).toBeNull();
    expect(result.current.delivery).toBeNull();
  });

  it("accounting and fiscal mirror the failed-invoice count", () => {
    useFailedInvoiceCount.mockReturnValue(3);
    const { result } = renderBadges();
    expect(result.current.accounting).toBe(3);
    expect(result.current.fiscal).toBe(3);
  });

  it("staff mirrors the unread chat count", () => {
    useChatUnreadCount.mockReturnValue(9);
    const { result } = renderBadges();
    expect(result.current.staff).toBe(9);
  });

  it("gates the failed-invoice fetch on tier access", () => {
    renderBadges({ hasAccess: false });
    expect(useFailedInvoiceCount).toHaveBeenCalledWith(7, false);
  });

  it("leaves badge-less tabs out of the map (absent = no badge)", () => {
    const { result } = renderBadges();
    expect(result.current.overview).toBeUndefined();
    expect(result.current.menu).toBeUndefined();
    expect(result.current.settings).toBeUndefined();
  });
});
