/** @jest-environment jsdom */
import React from "react";
import {
  act,
  render,
  screen,
  waitFor,
  fireEvent,
} from "@testing-library/react";

import BrowserPrintAgent from "./BrowserPrintAgent";

const mockClaimPrintJob = jest.fn();
const mockMarkPresented = jest.fn();
const mockConfirmPrinted = jest.fn();
const mockRetryPrintJob = jest.fn();
const mockAgentCancelPrintJob = jest.fn();
const mockRenewPrintLease = jest.fn();
const mockPrintFn = jest.fn().mockResolvedValue(undefined);
let mockPermissions = ["print:bill", "printers:read"];
let mockSSEOptions: {
  onEvent: (event: { type: string; data: Record<string, unknown> }) => void;
  onReconnect?: () => void;
} | null = null;

jest.mock("@/api/print", () => ({
  claimPrintJob: (...args: unknown[]) => mockClaimPrintJob(...args),
  markPresented: (...args: unknown[]) => mockMarkPresented(...args),
  confirmPrinted: (...args: unknown[]) => mockConfirmPrinted(...args),
  retryPrintJob: (...args: unknown[]) => mockRetryPrintJob(...args),
  agentCancelPrintJob: (...args: unknown[]) => mockAgentCancelPrintJob(...args),
  renewPrintLease: (...args: unknown[]) => mockRenewPrintLease(...args),
  formatPrintJobStatus: (s: string) => s || "unknown",
}));

jest.mock("@/hooks/useSSEEvents", () => ({
  useSSEEvents: (options: typeof mockSSEOptions) => {
    mockSSEOptions = options;
    return { degraded: false, blocked: false, reconnect: jest.fn() };
  },
}));

jest.mock("./useIframePrint", () => ({
  useIframePrint: () => ({ print: mockPrintFn }),
}));

jest.mock("./usePrintLeader", () => ({
  usePrintLeader: () => ({ current: true }),
  getOrCreatePrintClientId: () => "11111111-1111-4111-8111-111111111111",
}));

jest.mock("@/contexts/StaffPermissionsContext", () => ({
  useStaffPermissionsContext: () => ({
    permissions: mockPermissions,
    rolePermissions: [],
    customGrants: [],
    isLoading: false,
    isError: false,
    refetch: () => {},
  }),
}));

jest.mock("@/i18n/SimpleTranslationProvider", () => ({
  useSimpleLocale: () => ({ locale: "en" }),
  getTranslation: (key: string) => {
    const map: Record<string, string> = {
      "printers.agent.confirmTitle": "Confirm print",
      "printers.agent.confirmBody": "Did it print?",
      "printers.agent.printed": "Printed",
      "printers.agent.retry": "Retry",
      "printers.agent.cancel": "Cancel",
      "printers.agent.stationOnline": "Print station online",
      "printers.agent.stationOffline": "Print station offline",
      "printers.agent.leader": "Leader",
      "printers.agent.follower": "Standby",
      "printers.agent.pendingCount": "0 pending",
      "printers.agent.setupHint": "Keep this tab open.",
      "printers.agent.idle": "Idle",
      "printers.agent.jobMeta": "Job meta",
      "printers.agent.currentJob": "Job",
      "printers.agent.lastError": "Error",
      "printers.agent.dismissError": "Dismiss error",
      "printers.agent.reconnect": "Reconnect now",
      "printers.stopStation": "Stop station",
      "printers.agent.chooseStationTitle": "Choose printer",
      "printers.agent.chooseStationBody": "Choose the physical printer.",
      "printers.agent.stationLoadError": "Could not load printers",
      "printers.agent.noBrowserStations": "No printers",
      "printers.loading": "Loading",
    };
    return map[key] ?? key;
  },
}));

describe("BrowserPrintAgent", () => {
  beforeEach(() => {
    jest.clearAllMocks();
    mockSSEOptions = null;
    mockPermissions = ["print:bill", "printers:read"];
    mockClaimPrintJob.mockResolvedValue({
      job: {
        id: 9,
        business_id: 1,
        printer_id: 1,
        kind: "receipt",
        source_type: "bill",
        source_id: 1,
        status: "printing",
        payload_html: "<html>hi</html>",
        language: "en",
        created_at: new Date().toISOString(),
        printed_at: null,
      },
      pending_count: 0,
    });
    mockMarkPresented.mockResolvedValue(undefined);
    mockConfirmPrinted.mockResolvedValue(undefined);
  });

  it("claims, presents, and does not auto-confirm printed", async () => {
    render(<BrowserPrintAgent businessId={1} printerId={7} enabled isOwner />);

    await waitFor(() => expect(mockClaimPrintJob).toHaveBeenCalled());
    expect(mockClaimPrintJob).toHaveBeenCalledWith(
      1,
      "11111111-1111-4111-8111-111111111111",
      7,
      expect.any(Object),
    );
    await waitFor(() =>
      expect(mockPrintFn).toHaveBeenCalledWith("<html>hi</html>"),
    );
    await waitFor(() => expect(mockMarkPresented).toHaveBeenCalled());
    expect(mockConfirmPrinted).not.toHaveBeenCalled();

    expect(await screen.findByRole("dialog")).toBeInTheDocument();
    expect(
      screen.getByRole("button", { name: /printed/i }),
    ).toBeInTheDocument();
  });

  it("keeps a healthy idle print station out of sight", async () => {
    mockClaimPrintJob.mockResolvedValue({ job: null, pending_count: 0 });

    render(<BrowserPrintAgent businessId={1} printerId={7} enabled isOwner />);

    await waitFor(() => expect(mockClaimPrintJob).toHaveBeenCalled());
    expect(screen.queryByTestId("print-station-tray")).not.toBeInTheDocument();
  });

  it("confirms only when operator clicks Printed", async () => {
    render(<BrowserPrintAgent businessId={1} printerId={7} enabled isOwner />);
    await waitFor(() => expect(mockMarkPresented).toHaveBeenCalled());
    fireEvent.click(await screen.findByRole("button", { name: /printed/i }));
    await waitFor(() =>
      expect(mockConfirmPrinted).toHaveBeenCalledWith(
        1,
        9,
        "11111111-1111-4111-8111-111111111111",
        expect.any(Object),
      ),
    );
  });

  it("does not mount poll UI when disabled", () => {
    render(
      <BrowserPrintAgent
        businessId={1}
        printerId={7}
        enabled={false}
        isOwner
      />,
    );
    expect(screen.queryByTestId("print-station-tray")).not.toBeInTheDocument();
    expect(mockClaimPrintJob).not.toHaveBeenCalled();
  });

  it("does not claim jobs with read-only printer access", () => {
    mockPermissions = ["printers:read"];

    render(<BrowserPrintAgent businessId={1} printerId={7} enabled />);

    expect(screen.queryByTestId("print-station-tray")).not.toBeInTheDocument();
    expect(mockClaimPrintJob).not.toHaveBeenCalled();
  });

  it("does not claim anything until this browser is assigned to a printer", () => {
    render(
      <BrowserPrintAgent businessId={1} printerId={null} enabled isOwner />,
    );

    expect(mockClaimPrintJob).not.toHaveBeenCalled();
    expect(mockPrintFn).not.toHaveBeenCalled();
  });

  it("does not show a global chooser before settings assign a station", () => {
    mockPermissions = ["print:receipt"];

    render(
      <BrowserPrintAgent
        businessId={1}
        printerId={null}
        enabled
        onStopStation={jest.fn()}
      />,
    );

    expect(screen.queryByTestId("print-station-setup")).not.toBeInTheDocument();
    expect(mockClaimPrintJob).not.toHaveBeenCalled();
  });

  it("clears a transient claim error after reconnect succeeds", async () => {
    mockClaimPrintJob
      .mockRejectedValueOnce(new Error("Failed to fetch"))
      .mockResolvedValueOnce({ job: null, pending_count: 0 });
    render(
      <BrowserPrintAgent
        businessId={1}
        printerId={7}
        enabled
        isOwner
        onStopStation={jest.fn()}
      />,
    );
    expect(await screen.findByRole("alert")).toHaveTextContent(/error/i);
    fireEvent.click(screen.getByRole("button", { name: /reconnect/i }));
    await waitFor(() => expect(mockClaimPrintJob).toHaveBeenCalledTimes(2));
    await waitFor(() =>
      expect(screen.queryByRole("alert")).not.toBeInTheDocument(),
    );
  });

  it("claims immediately only for a matching printer wake", async () => {
    mockClaimPrintJob.mockResolvedValue({ job: null, pending_count: 0 });
    render(
      <BrowserPrintAgent
        businessId={1}
        printerId={7}
        enabled
        isOwner
        onStopStation={jest.fn()}
      />,
    );
    await waitFor(() => expect(mockClaimPrintJob).toHaveBeenCalledTimes(1));
    expect(mockSSEOptions).not.toBeNull();
    act(() =>
      mockSSEOptions!.onEvent({
        type: "print.bill_available",
        data: { printer_id: 8 },
      }),
    );
    expect(mockClaimPrintJob).toHaveBeenCalledTimes(1);
    act(() =>
      mockSSEOptions!.onEvent({
        type: "print.bill_available",
        data: { printer_id: 7 },
      }),
    );
    await waitFor(() => expect(mockClaimPrintJob).toHaveBeenCalledTimes(2));
  });
});
