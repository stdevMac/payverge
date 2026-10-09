/** @jest-environment jsdom */
import React from "react";
import { render, screen, fireEvent, waitFor } from "@testing-library/react";
import TableHostActions from "../TableHostActions";
import type { TableRowData } from "../TableRow";
import enDash from "@/i18n/messages/en/businessDashboard.json";
import esDash from "@/i18n/messages/es/businessDashboard.json";
import esArDash from "@/i18n/messages/es-ar/businessDashboard.json";

const mockSeatTable = jest.fn();
const mockClearTable = jest.fn();
const mockTransferTable = jest.fn();
const mockMergeTable = jest.fn();

jest.mock("@/api/tableFloor", () => ({
  seatTable: (...args: unknown[]) => mockSeatTable(...args),
  clearTable: (...args: unknown[]) => mockClearTable(...args),
  transferTable: (...args: unknown[]) => mockTransferTable(...args),
  mergeTable: (...args: unknown[]) => mockMergeTable(...args),
}));

jest.mock("react-hot-toast", () => ({
  __esModule: true,
  default: { success: jest.fn(), error: jest.fn() },
}));

// NextUI dropdown/modal portals are heavy in jsdom — stub to plain controls.
jest.mock("@nextui-org/react", () => {
  const React = require("react");
  return {
    Dropdown: ({ children }: { children: React.ReactNode }) => <div>{children}</div>,
    DropdownTrigger: ({ children }: { children: React.ReactNode }) => <>{children}</>,
    DropdownMenu: ({
      children,
      onAction,
    }: {
      children: React.ReactNode;
      onAction?: (key: string) => void;
    }) => (
      <div data-testid="host-menu">
        {React.Children.map(children, (child: any) =>
          child
            ? React.cloneElement(child, {
                onClick: () => onAction?.(child.key || child.props?.key),
              })
            : null,
        )}
      </div>
    ),
    DropdownItem: ({
      children,
      isDisabled,
      onClick,
      textValue,
      href,
      description,
    }: {
      children: React.ReactNode;
      isDisabled?: boolean;
      onClick?: () => void;
      textValue?: string;
      href?: string;
      description?: string;
    }) =>
      href ? (
        <a href={href} data-description={description}>
          {textValue || children}
        </a>
      ) : (
        <button
          type="button"
          disabled={isDisabled}
          onClick={onClick}
          data-description={description}
        >
          {textValue || children}
        </button>
      ),
    Modal: ({ isOpen, children }: { isOpen: boolean; children: React.ReactNode }) =>
      isOpen ? <div data-testid="host-modal">{children}</div> : null,
    ModalContent: ({ children }: { children: React.ReactNode }) => <div>{children}</div>,
    ModalHeader: ({ children }: { children: React.ReactNode }) => <div>{children}</div>,
    ModalBody: ({ children }: { children: React.ReactNode }) => <div>{children}</div>,
    ModalFooter: ({ children }: { children: React.ReactNode }) => <div>{children}</div>,
    Button: ({
      children,
      onPress,
      isDisabled,
    }: {
      children: React.ReactNode;
      onPress?: () => void;
      isDisabled?: boolean;
    }) => (
      <button type="button" disabled={isDisabled} onClick={onPress}>
        {children}
      </button>
    ),
    Input: ({
      value,
      onValueChange,
      label,
    }: {
      value?: string;
      onValueChange?: (v: string) => void;
      label?: string;
    }) => (
      <label>
        {label}
        <input
          aria-label={label}
          value={value || ""}
          onChange={(e) => onValueChange?.(e.target.value)}
        />
      </label>
    ),
    Select: ({ children }: { children: React.ReactNode }) => <div>{children}</div>,
    SelectItem: ({ children }: { children: React.ReactNode }) => <div>{children}</div>,
  };
});

const occupied: TableRowData = {
  id: 1,
  name: "Patio 1",
  table_code: "P1",
  is_active: true,
  status: "occupied",
  capacity: 4,
  active_bill: {
    id: 99,
    total: 12,
    physical_item_quantity: 1,
    created_at: new Date().toISOString(),
  },
  server_name: "Ana",
  next_reservation: null,
  last_seen: null,
};

const occupiedEmptyCheck: TableRowData = {
  ...occupied,
  active_bill: {
    id: 99,
    total: 0,
    physical_item_quantity: 0,
    created_at: new Date().toISOString(),
  },
};

const available: TableRowData = {
  ...occupied,
  id: 2,
  name: "Patio 2",
  table_code: "P2",
  status: "available",
  active_bill: null,
  server_name: null,
};

const occupiedOrphan: TableRowData = {
  ...occupied,
  active_bill: null,
  server_name: null,
};

const tString = (key: string) => key;

describe("TableHostActions", () => {
  beforeEach(() => {
    mockSeatTable.mockReset().mockResolvedValue({ action: "seat_walk_in" });
    mockClearTable.mockReset().mockResolvedValue({ action: "clear" });
    mockTransferTable.mockReset().mockResolvedValue({ action: "transfer" });
    mockMergeTable.mockReset().mockResolvedValue({ action: "merge" });
  });

  it("seats a walk-in from an available table", async () => {
    const onChanged = jest.fn();
    render(
      <TableHostActions
        businessId={9}
        table={available}
        targets={[]}
        onChanged={onChanged}
        tString={tString}
      />,
    );

    fireEvent.click(screen.getByText("hostActions.seat"));
    // Modal opens — confirm seat.
    fireEvent.click(screen.getByText("hostActions.seatConfirm"));

    await waitFor(() => {
      expect(mockSeatTable).toHaveBeenCalledWith(9, 2, {
        party_size: undefined,
        reservation_id: undefined,
      });
    });
    expect(onChanged).toHaveBeenCalled();
  });

  it("hits Liberar on an unpaid check so kitchen_tickets_live can surface", async () => {
    const toast = (await import("react-hot-toast")).default;
    (toast.error as jest.Mock).mockClear();
    mockClearTable.mockReset().mockRejectedValue({
      isAxiosError: true,
      response: {
        status: 409,
        data: {
          error: "The kitchen is still working this table — bump or cancel the open tickets first",
          code: "kitchen_tickets_live",
        },
      },
    });
    const onChanged = jest.fn();
    render(
      <TableHostActions
        businessId={9}
        table={occupied}
        targets={[]}
        onChanged={onChanged}
        tString={tString}
      />,
    );

    fireEvent.click(screen.getByText("hostActions.clear"));

    await waitFor(() => {
      expect(mockClearTable).toHaveBeenCalledWith(9, 1);
    });
    await waitFor(() => {
      expect(toast.error).toHaveBeenCalledWith("hostActions.errors.kitchenTicketsLive");
    });
    expect(onChanged).not.toHaveBeenCalled();
    expect(toast.error).not.toHaveBeenCalledWith("hostActions.errors.settleRequired");
  });

  it("localizes settle_required from the API when Liberar is just unpaid", async () => {
    const toast = (await import("react-hot-toast")).default;
    (toast.error as jest.Mock).mockClear();
    mockClearTable.mockReset().mockRejectedValue({
      isAxiosError: true,
      response: {
        status: 409,
        data: {
          error: "Settle or void the open check before clearing this table",
          code: "settle_required",
        },
      },
    });
    const onChanged = jest.fn();
    render(
      <TableHostActions
        businessId={9}
        table={occupied}
        targets={[]}
        onChanged={onChanged}
        tString={tString}
      />,
    );

    fireEvent.click(screen.getByText("hostActions.clear"));

    await waitFor(() => {
      expect(mockClearTable).toHaveBeenCalledWith(9, 1);
    });
    await waitFor(() => {
      expect(toast.error).toHaveBeenCalledWith("hostActions.errors.settleRequired");
    });
    expect(onChanged).not.toHaveBeenCalled();
  });

  it("clears an occupied empty-path table via the clear action", async () => {
    const onChanged = jest.fn();
    render(
      <TableHostActions
        businessId={9}
        table={occupiedEmptyCheck}
        targets={[]}
        onChanged={onChanged}
        tString={tString}
      />,
    );

    fireEvent.click(screen.getByText("hostActions.clear"));
    await waitFor(() => {
      expect(mockClearTable).toHaveBeenCalledWith(9, 1);
    });
    expect(onChanged).toHaveBeenCalled();
  });

  // Issue 704 — the backend rejects Liberar while the kitchen still holds live
  // tickets with code `kitchen_tickets_live`. Before the fix floorErrorMessage
  // had no branch for it, so getApiErrorMessage() leaked the handler's raw
  // English at a Spanish-speaking host. Drive the real component with the real
  // es-AR bundle so a missing key fails here, not on the floor.
  it("localizes the kitchen_tickets_live 409 instead of leaking backend English", async () => {
    const toast = (await import("react-hot-toast")).default;
    const scope = (bundle: Record<string, any>) =>
      (bundle as any).dashboard.tableManager;
    const esArString = (key: string) =>
      key.split(".").reduce<any>((acc, part) => acc?.[part], scope(esArDash)) ?? key;

    const backendEnglish =
      "The kitchen is still working this table — bump or cancel the open tickets first";
    mockClearTable.mockReset().mockRejectedValue({
      isAxiosError: true,
      response: {
        status: 409,
        data: { error: backendEnglish, code: "kitchen_tickets_live" },
      },
    });

    render(
      <TableHostActions
        businessId={9}
        table={occupiedEmptyCheck}
        targets={[]}
        onChanged={jest.fn()}
        tString={esArString}
      />,
    );

    fireEvent.click(screen.getByText(esArString("hostActions.clear")));

    await waitFor(() => {
      expect(mockClearTable).toHaveBeenCalledWith(9, 1);
    });
    await waitFor(() => {
      expect(toast.error).toHaveBeenCalledWith(
        esArString("hostActions.errors.kitchenTicketsLive"),
      );
    });
    // The raw handler string must never reach the host.
    expect(toast.error).not.toHaveBeenCalledWith(backendEnglish);
    // And the key must actually resolve — an unresolved path echoes itself.
    expect(esArString("hostActions.errors.kitchenTicketsLive")).not.toContain(
      "hostActions.",
    );
  });

  // #704 follow-up: a pending guest send is invisible to Live View (never on
  // bill_items) so the host sees an empty $0 walk-in and taps Liberar. The
  // backend refuses with a DISTINCT code — the remediation is the approval
  // queue, not the pass — so it must not reuse the kitchen copy.
  it("localizes the orders_pending_approval 409 with queue copy, not kitchen copy", async () => {
    const toast = (await import("react-hot-toast")).default;
    (toast.error as jest.Mock).mockClear();
    const scope = (bundle: Record<string, any>) =>
      (bundle as any).dashboard.tableManager;
    const esArString = (key: string) =>
      key.split(".").reduce<any>((acc, part) => acc?.[part], scope(esArDash)) ?? key;

    const backendEnglish =
      "A guest order is still waiting for approval — approve or reject it before freeing this table";
    mockClearTable.mockReset().mockRejectedValue({
      isAxiosError: true,
      response: {
        status: 409,
        data: { error: backendEnglish, code: "orders_pending_approval" },
      },
    });

    render(
      <TableHostActions
        businessId={9}
        table={occupiedEmptyCheck}
        targets={[]}
        onChanged={jest.fn()}
        tString={esArString}
      />,
    );

    fireEvent.click(screen.getByText(esArString("hostActions.clear")));

    await waitFor(() => {
      expect(mockClearTable).toHaveBeenCalledWith(9, 1);
    });
    await waitFor(() => {
      expect(toast.error).toHaveBeenCalledWith(
        esArString("hostActions.errors.ordersPendingApproval"),
      );
    });
    expect(toast.error).not.toHaveBeenCalledWith(backendEnglish);
    // Nothing is cooking — sending the host to the pass would be a lie.
    expect(toast.error).not.toHaveBeenCalledWith(
      esArString("hostActions.errors.kitchenTicketsLive"),
    );
    expect(
      esArString("hostActions.errors.ordersPendingApproval"),
    ).not.toContain("hostActions.");
  });

  it("ships ordersPendingApproval in every operator locale, voseo on es-AR", () => {
    const read = (bundle: Record<string, any>) =>
      (bundle as any).dashboard.tableManager.hostActions.errors
        .ordersPendingApproval;
    const en = read(enDash);
    const es = read(esDash);
    const esAr = read(esArDash);
    [en, es, esAr].forEach((v) => {
      expect(typeof v).toBe("string");
      expect(v.length).toBeGreaterThan(0);
    });
    expect(es).not.toBe(en);
    // es-AR is the voseo override tier: tuteo imperatives must not survive.
    expect(esAr).not.toBe(es);
    expect(esAr).toMatch(/aprobalo|rechazalo/);
    expect(esAr).not.toMatch(/apruébalo|recházalo/);
  });

  it("ships kitchenTicketsLive in every operator locale, voseo on es-AR", () => {
    const read = (bundle: Record<string, any>) =>
      (bundle as any).dashboard.tableManager.hostActions.errors.kitchenTicketsLive;
    const en = read(enDash);
    const es = read(esDash);
    const esAr = read(esArDash);
    [en, es, esAr].forEach((v) => {
      expect(typeof v).toBe("string");
      expect(v.length).toBeGreaterThan(0);
    });
    expect(es).not.toBe(en);
    // es-AR is the voseo override tier: tuteo imperatives must not survive.
    expect(esAr).not.toBe(es);
    expect(esAr).toMatch(/cerr\u00e1|cancel\u00e1/);
  });

  it("keeps Liberar tappable on occupied-without-check so pending/kitchen 409s can surface", () => {
    render(
      <TableHostActions
        businessId={9}
        table={occupiedOrphan}
        targets={[]}
        onChanged={jest.fn()}
        tString={tString}
      />,
    );
    expect(screen.getByText("hostActions.clear")).not.toBeDisabled();
    expect(screen.getByText("hostActions.seat")).toBeDisabled();
  });

  // #794 live repro (sha 0024205b6c01, business 86): T2/T3/T5 are occupied
  // 1-3d with bills_count=0 and leftover kitchen tickets 847/848/850 still in
  // the pass. Floor actions opened with Seat disabled, Transfer/Merge disabled
  // and Liberar as the only live control — no destination, and Liberar is
  // explicitly not the fix because it destroys the tickets.
  it("routes a leftover occupied table to its kitchen tickets and says there is no bill", () => {
    render(
      <TableHostActions
        businessId={9}
        table={occupiedOrphan}
        targets={[]}
        onChanged={jest.fn()}
        tString={tString}
      />,
    );

    const openKitchen = screen.getByText("hostActions.openKitchenTickets");
    expect(openKitchen).toHaveAttribute("href", "?tab=kitchen&kitchenStatus=all");
    // The panel states the absence instead of leaving the host guessing.
    expect(openKitchen).toHaveAttribute(
      "data-description",
      "hostActions.noOpenBill",
    );
    expect(screen.queryByText("hostActions.openBill")).not.toBeInTheDocument();
    // Liberar stays reachable, but it is no longer the only thing you can do.
    expect(screen.getByText("hostActions.clear")).toBeEnabled();
  });

  it("routes an occupied table with a check straight to that bill", () => {
    render(
      <TableHostActions
        businessId={9}
        table={occupied}
        targets={[]}
        onChanged={jest.fn()}
        tString={tString}
      />,
    );

    expect(screen.getByText("hostActions.openBill")).toHaveAttribute(
      "href",
      "?tab=bills&billId=99",
    );
    expect(
      screen.queryByText("hostActions.openKitchenTickets"),
    ).not.toBeInTheDocument();
  });

  it("says there is no open bill on a free table instead of a dead entry", () => {
    render(
      <TableHostActions
        businessId={9}
        table={available}
        targets={[]}
        onChanged={jest.fn()}
        tString={tString}
      />,
    );

    const none = screen.getByText("hostActions.noOpenBill");
    expect(none).toBeDisabled();
    expect(none).not.toHaveAttribute("href");
  });

  it("ships the leftover destination copy in every operator locale", () => {
    const effectiveEsAr = {
      ...esDash.dashboard.tableManager.hostActions,
      ...(esArDash as any).dashboard?.tableManager?.hostActions,
    };
    for (const bundle of [
      enDash.dashboard.tableManager.hostActions,
      esDash.dashboard.tableManager.hostActions,
      effectiveEsAr,
    ]) {
      expect((bundle as any).openBill).toBeTruthy();
      expect((bundle as any).openKitchenTickets).toBeTruthy();
      expect((bundle as any).noOpenBill).toBeTruthy();
    }
    expect(esDash.dashboard.tableManager.hostActions.openKitchenTickets).not.toBe(
      enDash.dashboard.tableManager.hostActions.openKitchenTickets,
    );
  });

  it("exposes transfer and merge for occupied tables", () => {
    render(
      <TableHostActions
        businessId={9}
        table={occupied}
        targets={[
          {
            id: 2,
            name: "Patio 2",
            status: "available",
            is_active: true,
            has_open_bill: false,
          },
        ]}
        onChanged={jest.fn()}
        tString={tString}
      />,
    );
    expect(screen.getByText("hostActions.transfer")).not.toBeDisabled();
    expect(screen.getByText("hostActions.merge")).not.toBeDisabled();
    expect(screen.getByText("hostActions.seat")).toBeDisabled();
  });
});
