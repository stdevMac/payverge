/** @jest-environment jsdom */
import React from "react";
import { render, screen, waitFor, fireEvent } from "@testing-library/react";
import DriverPerformance from "../DriverPerformance";
import * as deliveryApiModule from "@/api/delivery";

jest.mock("@/api/delivery", () => ({
  deliveryApi: {
    listDriverPerformance: jest.fn(),
    getDriverPerformance: jest.fn(),
  },
}));

jest.mock("../useDeliveryQueries", () => ({
  useDeliveryBusiness: () => ({
    data: { default_currency: "USD" },
    isLoading: false,
  }),
}));

jest.mock("@/contexts/ToastContext", () => {
  const showSuccess = jest.fn();
  const showError = jest.fn();
  return {
    useToast: () => ({ showSuccess, showError }),
    __toastFns: { showSuccess, showError },
  };
});

jest.mock("@/i18n/SimpleTranslationProvider", () => {
  // Minimal in-test EN bundle — only the keys this suite asserts on need to be real.
  const messages: Record<string, string> = {
    "deliverySettings.performance.title": "Driver Performance",
    "deliverySettings.performance.subtitle": "Throughput",
    "deliverySettings.performance.actions.refresh": "Refresh",
    "deliverySettings.performance.actions.view": "View {name}",
    "deliverySettings.performance.actions.viewLabel": "View",
    "deliverySettings.performance.actions.close": "Close",
    "deliverySettings.performance.empty.title": "No drivers yet",
    "deliverySettings.performance.empty.subtitle": "Add a driver",
    "deliverySettings.performance.placeholders.noData": "—",
    "deliverySettings.performance.units.min": "min",
    "deliverySettings.performance.columns.driver": "Driver",
    "deliverySettings.performance.columns.status": "Status",
    "deliverySettings.performance.columns.inProgress": "In progress",
    "deliverySettings.performance.columns.completedToday": "Today",
    "deliverySettings.performance.columns.completedWeek": "Week",
    "deliverySettings.performance.columns.avgDelivery": "Avg",
    "deliverySettings.performance.columns.onTime": "On time",
    "deliverySettings.performance.columns.rating": "Rating",
    "deliverySettings.performance.columns.actions": "Actions",
    "deliverySettings.dispatch.drivers.status.online": "Online",
  };
  return {
    useSimpleLocale: () => ({ locale: "en" }),
    getTranslation: (key: string) => messages[key] ?? key,
  };
});

jest.mock("@nextui-org/react", () => ({
  Button: ({ children, onPress, isLoading, isDisabled, "aria-label": ariaLabel, startContent: _sc }: any) => (
    <button type="button" onClick={onPress} disabled={isDisabled || isLoading} aria-label={ariaLabel}>
      {isLoading ? "loading" : children}
    </button>
  ),
  Chip: ({ children }: any) => <span>{children}</span>,
  Skeleton: () => <div data-testid="skeleton" />,
  Card: ({ children }: any) => <div>{children}</div>,
  CardBody: ({ children }: any) => <div>{children}</div>,
  Spinner: () => <div data-testid="spinner" />,
  Modal: ({ isOpen, children }: any) => (isOpen ? <div data-testid="modal">{children}</div> : null),
  ModalContent: ({ children }: any) => (
    <div>{typeof children === "function" ? children(() => {}) : children}</div>
  ),
  ModalHeader: ({ children }: any) => <div>{children}</div>,
  ModalBody: ({ children }: any) => <div>{children}</div>,
  ModalFooter: ({ children }: any) => <div>{children}</div>,
  Table: ({ children, "aria-label": ariaLabel, removeWrapper: _rw, ...rest }: any) => (
    <table aria-label={ariaLabel} {...rest}>{children}</table>
  ),
  TableHeader: ({ children }: any) => <thead><tr>{children}</tr></thead>,
  TableColumn: ({ children }: any) => <th>{children}</th>,
  TableBody: ({ children }: any) => <tbody>{children}</tbody>,
  TableRow: ({ children, ...rest }: any) => <tr {...rest}>{children}</tr>,
  TableCell: ({ children }: any) => <td>{children}</td>,
}));

const mockList = (deliveryApiModule as any).deliveryApi
  .listDriverPerformance as jest.Mock;
const mockDetail = (deliveryApiModule as any).deliveryApi
  .getDriverPerformance as jest.Mock;

const sampleDriver = {
  driver_id: 1,
  driver_name: "Alice",
  status: "online",
  is_active: true,
  completed_today: 3,
  completed_week: 12,
  completed_all_time: 200,
  cancelled_count: 2,
  failed_count: 0,
  in_progress_count: 1,
  avg_pickup_minutes: 4.2,
  avg_delivery_minutes: 28.5,
  on_time_rate: 0.92,
  average_rating: 4.6,
  gross_fees_collected: 145.5,
  gross_tips_collected: 32.25,
  active_queue: [
    {
      delivery_id: 99,
      delivery_number: "DEL-XYZ",
      status: "picked_up",
      customer_name: "Bob",
      address_short: "1 Main, City",
      assigned_at: null,
      estimated_delivery_time: "2026-05-03T12:30:00Z",
      delivery_fee: 5,
      driver_tip: 1.5,
    },
  ],
};

describe("DriverPerformance", () => {
  beforeEach(() => {
    jest.clearAllMocks();
  });

  it("renders the leaderboard with KPI columns", async () => {
    mockList.mockResolvedValueOnce({ drivers: [sampleDriver] });

    render(<DriverPerformance businessId={1} />);

    await waitFor(() => {
      expect(screen.getByTestId("performance-table")).toBeTruthy();
    });

    expect(screen.getByText("Alice")).toBeTruthy();
    expect(screen.getByText("3")).toBeTruthy(); // today
    expect(screen.getByText("12")).toBeTruthy(); // week
    expect(screen.getByText("29 min")).toBeTruthy(); // avg delivery rounded
    expect(screen.getByText("92%")).toBeTruthy(); // on-time
    expect(screen.getByText("4.6")).toBeTruthy(); // rating
  });

  it("renders the empty state when no drivers exist", async () => {
    mockList.mockResolvedValueOnce({ drivers: [] });

    render(<DriverPerformance businessId={1} />);

    await waitFor(() => {
      expect(screen.queryByTestId("performance-table")).toBeNull();
    });
    expect(screen.getByText("No drivers yet")).toBeTruthy();
  });

  it("opens the scorecard modal when the View action is pressed", async () => {
    mockList.mockResolvedValueOnce({ drivers: [sampleDriver] });
    mockDetail.mockResolvedValueOnce(sampleDriver);

    render(<DriverPerformance businessId={1} />);

    await waitFor(() => {
      expect(screen.getByTestId("performance-table")).toBeTruthy();
    });

    const viewButton = screen.getByRole("button", { name: /Alice/ });
    fireEvent.click(viewButton);

    await waitFor(() => {
      expect(screen.getByTestId("modal")).toBeTruthy();
    });
    await waitFor(() => {
      expect(screen.getByTestId("scorecard-body")).toBeTruthy();
    });

    expect(mockDetail).toHaveBeenCalledWith(1, 1);
  });

  it("displays the dash placeholder when KPI values are null", async () => {
    const empty = { ...sampleDriver, avg_delivery_minutes: null, on_time_rate: null, average_rating: null };
    mockList.mockResolvedValueOnce({ drivers: [empty] });

    render(<DriverPerformance businessId={1} />);

    await waitFor(() => {
      expect(screen.getByTestId("performance-table")).toBeTruthy();
    });

    // Three null KPIs → three dashes in the row
    const dashes = screen.getAllByText("—");
    expect(dashes.length).toBeGreaterThanOrEqual(3);
  });
});

// ─── DEL-OP-7: palette conformance ────────────────────────────────────────────
// The dashboard's cohesion campaign normalized operator surfaces onto the warm
// ink/warm token scales. Tailwind's default zinc-* grays are off-palette here
// (visibly cooler than sibling delivery tabs) and must not be used.
describe("DriverPerformance palette (DEL-OP-7)", () => {
  beforeEach(() => {
    jest.clearAllMocks();
  });

  it("uses no off-palette zinc-* text tokens in the populated table", async () => {
    mockList.mockResolvedValue({
      drivers: [{ ...sampleDriver, in_progress_count: 0, failed_count: 0, cancelled_count: 0 }],
    });
    const { container } = render(<DriverPerformance businessId={1} />);
    await waitFor(() => expect(screen.getByTestId("performance-table")).toBeInTheDocument());
    expect(container.innerHTML).not.toMatch(/zinc-/);
  });

  it("uses no off-palette zinc-* text tokens in the empty state", async () => {
    mockList.mockResolvedValue({ drivers: [] });
    const { container } = render(<DriverPerformance businessId={1} />);
    await waitFor(() => expect(screen.getByText("No drivers yet")).toBeInTheDocument());
    expect(container.innerHTML).not.toMatch(/zinc-/);
  });
});
