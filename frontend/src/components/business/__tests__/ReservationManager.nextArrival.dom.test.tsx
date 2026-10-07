/** @jest-environment jsdom */
/**
 * #716: next-arrival KPI must not say Ninguna while a confirmed tomorrow
 * booking is on Próximas. Device TZ ≠ venue TZ; leftover today hydrate
 * must not hide the upcoming row.
 */
process.env.TZ = "America/Argentina/Buenos_Aires";

import React from "react";
import { fireEvent, render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { businessDateKey } from "@/utils/businessTime";
import { wallTimeToInstant } from "@/utils/zonedDateTime";
import { getDateFilterSelect } from "./_reservationManagerTestUtils";

const VENUE_TZ = "America/New_York";

function venueTomorrowNineteen(): string {
  const today = businessDateKey(new Date(), VENUE_TZ);
  const [year, month, day] = today.split("-").map(Number);
  const next = new Date(Date.UTC(year, month - 1, day + 1));
  const key = next.toISOString().slice(0, 10);
  return wallTimeToInstant(`${key}T19:00`, VENUE_TZ).toISOString();
}

const TOMORROW_ISO = venueTomorrowNineteen();
const TODAY_ISO = wallTimeToInstant(
  `${businessDateKey(new Date(), VENUE_TZ)}T12:00`,
  VENUE_TZ,
).toISOString();

const franky = {
  id: 262,
  business_id: 1,
  customer_name: "QA Test Franky",
  customer_phone: "",
  party_size: 2,
  reservation_time: TOMORROW_ISO,
  duration: 90,
  status: "confirmed" as const,
  table: { id: 5, name: "Table 5" },
  created_at: TOMORROW_ISO,
  updated_at: TOMORROW_ISO,
};

const seatedToday = {
  id: 11,
  business_id: 1,
  customer_name: "Seated Today",
  customer_phone: "",
  party_size: 2,
  reservation_time: TODAY_ISO,
  duration: 90,
  status: "seated" as const,
  table: { id: 2, name: "Table 2" },
  created_at: TODAY_ISO,
  updated_at: TODAY_ISO,
};

/** Today is a single-day window (start === end). Próximas spans many days. */
function listForRange(start?: string, end?: string) {
  const isSingleDay = Boolean(start && end && start === end);
  return isSingleDay ? [seatedToday] : [franky];
}

const mockGetReservations = jest.fn(
  (_id: number, start?: string, end?: string) => {
    const rows = listForRange(start, end);
    return Promise.resolve({
      reservations: rows,
      total: rows.length,
      page: 1,
      page_size: 25,
      total_pages: 1,
    });
  },
);

const mockGetAllUpcoming = jest.fn(() =>
  Promise.resolve({
    items: [seatedToday],
    metadata: { total: 1, page: 1, page_size: 100, total_pages: 1 },
    capped: false,
  }),
);

const mockGetUpcomingReservations = jest.fn(() =>
  Promise.resolve({
    reservations: [franky],
    total: 1,
  }),
);

jest.mock("@/i18n/SimpleTranslationProvider", () => ({
  useSimpleLocale: () => ({ locale: "es", setLocale: jest.fn() }),
  getTranslation: (key: string) => {
    const leaf = key.replace(/^businessDashboard\.reservations\./, "");
    const map: Record<string, string> = {
      "insights.nextArrival": "Próxima llegada",
      "insights.none": "Ninguna",
      "insights.coversToday": "Comensales hoy",
      "insights.waitlist": "Lista de espera",
      "insights.needsTable": "Sin mesa asignada",
      "filters.upcoming": "Próximas",
      "filters.today": "Hoy",
      dateFilter: "Filtro de fecha",
      guests: "comensales",
      "serviceDetails.unassigned": "Sin asignar",
      "viewToggle.list": "Lista",
      "viewToggle.board": "Tablero",
    };
    return map[leaf] ?? key;
  },
}));

jest.mock("@/hooks/useBusinessAccess", () => ({
  useBusinessAccess: () => ({
    access: null,
    loading: false,
    error: null,
    hasAccess: true,
    isSuspended: false,
    lockState: "active",
    aiConfigured: false,
    refetch: jest.fn(),
  }),
}));

jest.mock("@/api/reservations", () => ({
  reservationAPI: {
    getReservations: (...args: unknown[]) =>
      mockGetReservations(
        ...(args as Parameters<typeof mockGetReservations>),
      ),
    getSettings: () =>
      Promise.resolve({
        enabled: true,
        min_party_size: 1,
        max_party_size: 10,
        max_advance_days: 30,
        default_duration: 90,
        min_advance_minutes: 30,
        slot_interval_minutes: 15,
        max_covers_per_slot: 50,
        hold_duration_minutes: 15,
        service_buffer_minutes: 0,
        no_show_grace_minutes: 15,
        reminder_hours_before: 2,
        cancellation_deadline: 2,
        allow_cancellation: true,
        allow_waitlist: true,
        auto_assign_tables: false,
        approval_mode: "auto",
        send_confirmation_email: true,
        send_reminder_email: true,
        external_partner_links: [],
      }),
    getReservation: jest.fn(),
    updateReservation: jest.fn(),
    createReservation: jest.fn(),
    cancelReservation: jest.fn(),
    checkIn: jest.fn(),
    markNoShow: jest.fn(),
    promoteWaitlist: jest.fn(),
    assignTable: jest.fn(),
    claimReservation: jest.fn(() => Promise.resolve({})),
    releaseReservation: jest.fn(() => Promise.resolve({})),
    getTableOptions: jest.fn(() => Promise.resolve({ tables: [] })),
    getStats: jest.fn(() =>
      Promise.resolve({
        total: 0,
        pending: 0,
        confirmed: 0,
        waitlist: 0,
        seated: 1,
        completed: 0,
        cancelled: 0,
        no_show: 0,
        covers: 0,
      }),
    ),
    getUpcomingReservations: (...args: unknown[]) =>
      mockGetUpcomingReservations(
        ...(args as Parameters<typeof mockGetUpcomingReservations>),
      ),
  },
  getAllUpcomingReservations: (...args: unknown[]) =>
    mockGetAllUpcoming(...(args as Parameters<typeof mockGetAllUpcoming>)),
}));

jest.mock("@/api/business", () => ({
  businessApi: {
    getBusinessTables: jest.fn(() => Promise.resolve({ tables: [] })),
  },
  getBusiness: jest.fn(() =>
    Promise.resolve({ default_currency: "USD", timezone: "America/New_York" }),
  ),
  getBusinessOperatingHours: jest.fn(() => Promise.resolve([])),
}));

jest.mock("@/hooks/useSSEEvents", () => ({
  useSSEEvents: () => ({ retriesExhausted: false, reconnect: jest.fn() }),
}));

jest.mock("@/providers/HybridAuthProvider", () => ({
  useAuth: () => ({ staffData: null, isStaffUser: false }),
}));

jest.mock("@/contexts/StaffPermissionsContext", () => ({
  useStaffPermissionsContext: () => ({
    permissions: [],
    rolePermissions: [],
    customGrants: [],
    isLoading: false,
    isError: false,
    refetch: jest.fn(),
  }),
}));

jest.mock("../ReservationToggle", () => ({
  __esModule: true,
  default: () => null,
  useReservationStatus: () => ({
    enabled: true,
    loading: false,
    setEnabled: jest.fn(),
  }),
}));

import ReservationManager from "../ReservationManager";

function nextArrivalCard(): HTMLElement {
  const label = screen.getByText("Próxima llegada");
  const card = label.closest("div");
  if (!card) throw new Error("next-arrival card missing");
  return card;
}

function coversTodayCard(): HTMLElement {
  const label = screen.getByText("Comensales hoy");
  const card = label.closest("div");
  if (!card) throw new Error("covers-today card missing");
  return card;
}

describe("ReservationManager next-arrival KPI", () => {
  beforeEach(() => {
    mockGetUpcomingReservations.mockReset();
    mockGetUpcomingReservations.mockImplementation(() =>
      Promise.resolve({
        reservations: [franky],
        total: 1,
      }),
    );
    mockGetReservations.mockReset();
    mockGetReservations.mockImplementation(
      (_id: number, start?: string, end?: string) => {
        const rows = listForRange(start, end);
        return Promise.resolve({
          reservations: rows,
          total: rows.length,
          page: 1,
          page_size: 25,
          total_pages: 1,
        });
      },
    );
  });

  it("names the upcoming API booking on the default Hoy strip", async () => {
    render(<ReservationManager businessId={1} />);

    await waitFor(() => {
      expect(screen.getByText("Seated Today")).toBeInTheDocument();
    });

    await waitFor(() => {
      const card = nextArrivalCard();
      expect(card.textContent).not.toMatch(/Ninguna/);
      expect(card.textContent).toMatch(/19:00/);
      expect(card.textContent).toMatch(/QA Test Franky/);
    });
  });

  it("shows the confirmed upcoming booking instead of Ninguna", async () => {
    const { container } = render(<ReservationManager businessId={1} />);

    fireEvent.change(getDateFilterSelect(container), {
      target: { value: "upcoming" },
    });

    await waitFor(() => {
      expect(screen.getByText("QA Test Franky")).toBeInTheDocument();
    });

    expect(nextArrivalCard().textContent).not.toMatch(/Ninguna/);
    expect(nextArrivalCard().textContent).toMatch(/QA Test Franky/);
  });

  it("keeps Próxima llegada off Ninguna after a leftover today board hydrate", async () => {
    const user = userEvent.setup();
    const { container } = render(<ReservationManager businessId={1} />);

    await waitFor(() => {
      expect(screen.getByText("Seated Today")).toBeInTheDocument();
    });
    expect(nextArrivalCard().textContent).not.toMatch(/Ninguna/);
    expect(nextArrivalCard().textContent).toMatch(/QA Test Franky/);

    await user.click(screen.getByLabelText("Tablero"));
    await screen.findByTestId("reservations-board");

    await user.click(screen.getByLabelText("Lista"));
    fireEvent.change(getDateFilterSelect(container), {
      target: { value: "upcoming" },
    });

    await waitFor(() => {
      expect(screen.getByText("QA Test Franky")).toBeInTheDocument();
    });

    const card = nextArrivalCard();
    expect(card.textContent).not.toMatch(/Ninguna/);
    expect(card.textContent).toMatch(/19:00/);
    expect(card.textContent).toMatch(/QA Test Franky/);
  });

  it("names a confirmed Próximas row the device clock already expired", async () => {
    const expiredFranky = {
      ...franky,
      reservation_time: "2026-08-20T15:00:00.000Z",
    };
    mockGetUpcomingReservations.mockResolvedValue({
      reservations: [],
      total: 0,
    });
    mockGetReservations.mockImplementation(
      (_id: number, start?: string, end?: string) => {
        const isSingleDay = Boolean(start && end && start === end);
        const rows = isSingleDay ? [seatedToday] : [expiredFranky];
        return Promise.resolve({
          reservations: rows,
          total: rows.length,
          page: 1,
          page_size: 25,
          total_pages: 1,
        });
      },
    );

    const { container } = render(<ReservationManager businessId={1} />);
    fireEvent.change(getDateFilterSelect(container), {
      target: { value: "upcoming" },
    });

    await waitFor(() => {
      expect(screen.getByText("QA Test Franky")).toBeInTheDocument();
    });

    const card = nextArrivalCard();
    expect(card.textContent).not.toMatch(/Ninguna/);
    expect(card.textContent).toMatch(/QA Test Franky/);
  });

  it("counts venue-today confirmed covers when stats return 0", async () => {
    const todayFranky = {
      ...franky,
      reservation_time: wallTimeToInstant(
        `${businessDateKey(new Date(), VENUE_TZ)}T19:00`,
        VENUE_TZ,
      ).toISOString(),
    };
    mockGetUpcomingReservations.mockResolvedValue({
      reservations: [todayFranky],
      total: 1,
    });
    mockGetReservations.mockImplementation(() =>
      Promise.resolve({
        reservations: [todayFranky],
        total: 1,
        page: 1,
        page_size: 25,
        total_pages: 1,
      }),
    );

    render(<ReservationManager businessId={1} />);

    await waitFor(() => {
      expect(screen.getByText("QA Test Franky")).toBeInTheDocument();
    });
    await waitFor(() => {
      expect(coversTodayCard().textContent).toMatch(/2/);
    });
    expect(nextArrivalCard().textContent).not.toMatch(/Ninguna/);
    expect(nextArrivalCard().textContent).toMatch(/QA Test Franky/);
  });
});
