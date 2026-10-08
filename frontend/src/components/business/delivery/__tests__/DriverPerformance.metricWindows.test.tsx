/** @jest-environment jsdom */
import React from "react";
import { render, screen, within } from "@testing-library/react";
import DriverPerformance from "../DriverPerformance";
import * as deliveryApiModule from "@/api/delivery";

jest.mock("@/api/delivery", () => ({
  deliveryApi: {
    listDriverPerformance: jest.fn(),
    getDriverPerformance: jest.fn(),
  },
}));

jest.mock("../useDeliveryQueries", () => ({
  useDeliveryBusiness: () => ({ data: { default_currency: "USD" }, isLoading: false }),
}));

// The toast fns must keep a stable identity across renders: `load` is a
// useCallback keyed on showError, and a fresh function every render re-fires the
// load effect forever, leaving the component pinned to its skeleton.
jest.mock("@/contexts/ToastContext", () => {
  const showSuccess = jest.fn();
  const showError = jest.fn();
  return { useToast: () => ({ showSuccess, showError }) };
});

jest.mock("@/i18n/SimpleTranslationProvider", () => {
  const messages: Record<string, string> = {
    "deliverySettings.performance.title": "Driver Performance",
    "deliverySettings.performance.subtitle": "Throughput",
    "deliverySettings.performance.actions.refresh": "Refresh",
    "deliverySettings.performance.actions.view": "View {name}",
    "deliverySettings.performance.actions.viewLabel": "View",
    "deliverySettings.performance.placeholders.noData": "—",
    "deliverySettings.performance.units.min": "min",
    "deliverySettings.performance.columns.driver": "Driver",
    "deliverySettings.performance.columns.status": "Status",
    "deliverySettings.performance.columns.inProgress": "In progress",
    "deliverySettings.performance.columns.completedToday": "Today",
    "deliverySettings.performance.columns.completedWeek": "Last 7 days",
    "deliverySettings.performance.columns.avgDelivery": "Avg delivery",
    "deliverySettings.performance.columns.onTime": "On time",
    "deliverySettings.performance.columns.rating": "Rating",
    "deliverySettings.performance.columns.issues": "Issues",
    "deliverySettings.performance.columns.actions": "Actions",
    "deliverySettings.performance.windows.allTime": "All time",
    "deliverySettings.dispatch.drivers.status.online": "Online",
  };
  return {
    useSimpleLocale: () => ({ locale: "en" }),
    getTranslation: (key: string) => messages[key] ?? key,
  };
});

jest.mock("@nextui-org/react", () => ({
  Button: ({ children, onPress, "aria-label": ariaLabel, startContent: _sc }: any) => (
    <button type="button" onClick={onPress} aria-label={ariaLabel}>
      {children}
    </button>
  ),
  Chip: ({ children }: any) => <span>{children}</span>,
  Skeleton: () => <div data-testid="skeleton" />,
  Card: ({ children }: any) => <div>{children}</div>,
  CardBody: ({ children }: any) => <div>{children}</div>,
  Spinner: () => <div data-testid="spinner" />,
  Modal: ({ isOpen, children }: any) => (isOpen ? <div>{children}</div> : null),
  ModalContent: ({ children }: any) => (
    <div>{typeof children === "function" ? children(() => {}) : children}</div>
  ),
  ModalHeader: ({ children }: any) => <div>{children}</div>,
  ModalBody: ({ children }: any) => <div>{children}</div>,
  ModalFooter: ({ children }: any) => <div>{children}</div>,
  Table: ({ children, "aria-label": ariaLabel, removeWrapper: _rw, ...rest }: any) => (
    <table aria-label={ariaLabel} {...rest}>
      {children}
    </table>
  ),
  TableHeader: ({ children }: any) => (
    <thead>
      <tr>{children}</tr>
    </thead>
  ),
  TableColumn: ({ children }: any) => <th>{children}</th>,
  TableBody: ({ children }: any) => <tbody>{children}</tbody>,
  TableRow: ({ children, ...rest }: any) => <tr {...rest}>{children}</tr>,
  TableCell: ({ children }: any) => <td>{children}</td>,
}));

const mockList = (deliveryApiModule as any).deliveryApi.listDriverPerformance as jest.Mock;

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
  active_queue: [],
};

// L3-44 / audit decision #7. The Rendimiento table mixes two time windows in one
// row: `completed_today` and `completed_week` are date-bounded in
// loadDriverPerformanceAggregates, while avg delivery, on-time rate, rating,
// cancellations and failures are unbounded lifetime aggregates. Only the two
// windowed columns said so, which invited the reading that "28 min" and "92%"
// described today's shift.
describe("DriverPerformance — metric windows", () => {
  beforeEach(() => {
    jest.clearAllMocks();
    mockList.mockResolvedValue({ drivers: [sampleDriver] });
  });

  const header = (label: string) => {
    const th = screen.getByText(label).closest("th");
    if (!th) throw new Error(`no column header for "${label}"`);
    return th;
  };

  const renderTable = async () => {
    render(<DriverPerformance businessId={42} />);
    await screen.findByTestId("performance-table");
  };

  it("marks the lifetime columns as all-time", async () => {
    await renderTable();

    for (const label of ["Avg delivery", "On time", "Rating", "Issues"]) {
      expect(within(header(label)).getByText("All time")).toBeInTheDocument();
    }
  });

  it("leaves the already-windowed columns alone", async () => {
    await renderTable();

    // "Today" and "Last 7 days" carry their window in the label itself; adding
    // "All time" underneath would contradict it.
    for (const label of ["Driver", "Status", "In progress", "Today", "Last 7 days"]) {
      expect(within(header(label)).queryByText("All time")).not.toBeInTheDocument();
    }
  });
});
