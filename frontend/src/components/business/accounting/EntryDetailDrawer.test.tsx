/** @jest-environment jsdom */
import React from "react";
import { fireEvent, render, screen } from "@testing-library/react";
import type { ManualLedgerEntry } from "@/api/accounting";
import { useVoidEntry } from "@/hooks/accounting/useAccountingQueries";
import EntryDetailDrawer from "./EntryDetailDrawer";

const mockMutate = jest.fn();

jest.mock("@/hooks/accounting/useAccountingQueries", () => ({
  useVoidEntry: jest.fn(() => ({ mutate: mockMutate, isPending: false })),
}));

jest.mock("../shared/DetailDrawer", () => ({
  __esModule: true,
  default: ({
    open,
    title,
    subtitle,
    footer,
    children,
  }: {
    open: boolean;
    title: string;
    subtitle?: string;
    footer?: React.ReactNode;
    children: React.ReactNode;
  }) =>
    open ? (
      <div data-testid="entry-detail-drawer">
        <h2>{title}</h2>
        {subtitle ? <p>{subtitle}</p> : null}
        <div data-testid="entry-detail-body">{children}</div>
        {footer ? <div data-testid="entry-detail-footer">{footer}</div> : null}
      </div>
    ) : null,
}));

jest.mock("../modals/ConfirmationModal", () => ({
  __esModule: true,
  default: ({
    isOpen,
    title,
    description,
    confirmLabel,
    onConfirm,
  }: {
    isOpen: boolean;
    title: string;
    description: string;
    confirmLabel?: string;
    onConfirm: () => void;
  }) =>
    isOpen ? (
      <div data-testid="confirm-void-modal" role="dialog">
        <h3>{title}</h3>
        <p>{description}</p>
        <button type="button" onClick={onConfirm}>
          {confirmLabel ?? "Confirm"}
        </button>
      </div>
    ) : null,
}));

jest.mock("../premium", () => ({
  AnimatedNumberText: ({
    value,
    format,
    className,
  }: {
    value: number;
    format?: (v: number) => string;
    className?: string;
  }) => (
    <span data-testid="animated-amount" className={className}>
      {format ? format(value) : String(value)}
    </span>
  ),
}));

const useVoidEntryMock = useVoidEntry as jest.Mock;

const t = (key: string) => key;
const tWith = (key: string, replacements: Record<string, string | number>) => {
  let value = key;
  Object.entries(replacements).forEach(([name, replacement]) => {
    value = value.replace(new RegExp(`\\{${name}\\}`, "g"), String(replacement));
  });
  // Tests still assert on the key prefix when the template is the key itself.
  if (value === key) {
    return `${key}:${JSON.stringify(replacements)}`;
  }
  return value;
};

function entry(
  overrides: Partial<ManualLedgerEntry> & { id: number } = { id: 1 },
): ManualLedgerEntry {
  return {
    business_id: 42,
    entry_type: "expense",
    category: "inventory",
    amount: 40 as ManualLedgerEntry["amount"],
    currency: "USD",
    occurred_at: "2026-01-15T12:00:00Z",
    description: "Inventory restock",
    notes: "Weekly flour order",
    reference: "PO-1001",
    voided_at: null,
    created_at: "2026-01-15T13:00:00Z",
    created_by_staff: { id: 7, name: "Ana Manager" },
    ...overrides,
  };
}

function renderDrawer(
  props: Partial<React.ComponentProps<typeof EntryDetailDrawer>> = {},
) {
  const defaults: React.ComponentProps<typeof EntryDetailDrawer> = {
    open: true,
    entry: entry(),
    businessId: "42",
    locale: "en",
    currency: "USD",
    canWrite: true,
    onClose: jest.fn(),
    onDuplicate: jest.fn(),
    t,
    tWith,
  };
  return render(<EntryDetailDrawer {...defaults} {...props} />);
}

describe("EntryDetailDrawer", () => {
  beforeEach(() => {
    jest.clearAllMocks();
    useVoidEntryMock.mockReturnValue({ mutate: mockMutate, isPending: false });
  });

  it("renders amount hero, definition rows, audit, and immutable hint", () => {
    renderDrawer();

    expect(screen.getByText("entryDetail.title")).toBeInTheDocument();
    expect(screen.getByText("Inventory restock")).toBeInTheDocument();
    expect(screen.getByTestId("animated-amount")).toBeInTheDocument();
    expect(screen.getByText(/\$40\.00/)).toBeInTheDocument();

    // Definition-list labels + values
    expect(screen.getByText("entryDetail.category")).toBeInTheDocument();
    expect(screen.getByText("entryDetail.type")).toBeInTheDocument();
    expect(screen.getByText("entryDetail.occurredAt")).toBeInTheDocument();
    expect(screen.getByText("entryDetail.notes")).toBeInTheDocument();
    expect(screen.getByText("entryDetail.reference")).toBeInTheDocument();
    expect(screen.getByText("Weekly flour order")).toBeInTheDocument();
    expect(screen.getByText("PO-1001")).toBeInTheDocument();

    // Audit + immutable
    expect(screen.getByText(/entryDetail\.audit\.createdBy/)).toBeInTheDocument();
    expect(screen.getByText(/Ana Manager/)).toBeInTheDocument();
    expect(screen.getByText("entryDetail.immutableHint")).toBeInTheDocument();
  });

  it("omits empty notes and reference rows", () => {
    renderDrawer({
      entry: entry({ id: 2, notes: "", reference: undefined }),
    });

    expect(screen.queryByText("entryDetail.notes")).not.toBeInTheDocument();
    expect(screen.queryByText("entryDetail.reference")).not.toBeInTheDocument();
  });

  it("shows Void and Void & duplicate for active entries when canWrite", () => {
    renderDrawer();

    expect(
      screen.getByRole("button", { name: "entryDetail.actions.duplicate" }),
    ).toBeInTheDocument();
    expect(
      screen.getByRole("button", { name: "entryDetail.actions.void" }),
    ).toBeInTheDocument();
    expect(
      screen.getByRole("button", {
        name: "entryDetail.actions.voidAndDuplicate",
      }),
    ).toBeInTheDocument();
  });

  it("hides Void actions for voided entries and shows voided audit", () => {
    renderDrawer({
      entry: entry({
        id: 3,
        voided_at: "2026-01-20T12:00:00Z",
        voided_by_staff: { id: 9, name: "Owner Kim" },
      }),
    });

    expect(
      screen.getByRole("button", { name: "entryDetail.actions.duplicate" }),
    ).toBeInTheDocument();
    expect(
      screen.queryByRole("button", { name: "entryDetail.actions.void" }),
    ).not.toBeInTheDocument();
    expect(
      screen.queryByRole("button", {
        name: "entryDetail.actions.voidAndDuplicate",
      }),
    ).not.toBeInTheDocument();
    expect(screen.getByText(/entryDetail\.audit\.voidedBy/)).toBeInTheDocument();
    expect(screen.getByText(/Owner Kim/)).toBeInTheDocument();
  });

  it("hides void actions when canWrite is false", () => {
    renderDrawer({ canWrite: false });

    expect(
      screen.getByRole("button", { name: "entryDetail.actions.duplicate" }),
    ).toBeInTheDocument();
    expect(
      screen.queryByRole("button", { name: "entryDetail.actions.void" }),
    ).not.toBeInTheDocument();
    expect(
      screen.queryByRole("button", {
        name: "entryDetail.actions.voidAndDuplicate",
      }),
    ).not.toBeInTheDocument();
  });

  it("opens ConfirmationModal on Void and calls useVoidEntry mutate on confirm", () => {
    const onClose = jest.fn();
    renderDrawer({ onClose });

    fireEvent.click(
      screen.getByRole("button", { name: "entryDetail.actions.void" }),
    );

    expect(screen.getByTestId("confirm-void-modal")).toBeInTheDocument();
    expect(screen.getByText("entryDetail.confirmVoidTitle")).toBeInTheDocument();

    fireEvent.click(screen.getByRole("button", { name: "entries.void" }));

    expect(mockMutate).toHaveBeenCalledWith(
      1,
      expect.objectContaining({ onSuccess: expect.any(Function) }),
    );

    // Simulate mutation success → drawer closes
    const opts = mockMutate.mock.calls[0][1] as { onSuccess: () => void };
    opts.onSuccess();
    expect(onClose).toHaveBeenCalled();
  });

  it("void & duplicate voids then calls onDuplicate", () => {
    const onDuplicate = jest.fn();
    const onClose = jest.fn();
    const e = entry({ id: 5 });
    renderDrawer({ entry: e, onDuplicate, onClose });

    fireEvent.click(
      screen.getByRole("button", {
        name: "entryDetail.actions.voidAndDuplicate",
      }),
    );
    fireEvent.click(screen.getByRole("button", { name: "entries.void" }));

    expect(mockMutate).toHaveBeenCalledWith(
      5,
      expect.objectContaining({ onSuccess: expect.any(Function) }),
    );

    const opts = mockMutate.mock.calls[0][1] as { onSuccess: () => void };
    opts.onSuccess();
    expect(onDuplicate).toHaveBeenCalledWith(e);
    expect(onClose).toHaveBeenCalled();
  });

  it("Duplicate calls onDuplicate without voiding", () => {
    const onDuplicate = jest.fn();
    const e = entry({ id: 8 });
    renderDrawer({ entry: e, onDuplicate });

    fireEvent.click(
      screen.getByRole("button", { name: "entryDetail.actions.duplicate" }),
    );

    expect(onDuplicate).toHaveBeenCalledWith(e);
    expect(mockMutate).not.toHaveBeenCalled();
  });
});
