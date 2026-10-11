/** @jest-environment jsdom */
import React from "react";
import {
  act,
  fireEvent,
  render,
  screen,
  waitFor,
} from "@testing-library/react";
import type { ManualLedgerEntry } from "@/api/accounting";
import { useCreateEntry } from "@/hooks/accounting/useAccountingQueries";
import { expenseCategories, incomeCategories } from "./categories";
import EntryFormDrawer from "./EntryFormDrawer";

const mockMutate = jest.fn();

jest.mock("@/hooks/accounting/useAccountingQueries", () => ({
  useCreateEntry: jest.fn(() => ({
    mutate: mockMutate,
    isPending: false,
  })),
}));

jest.mock("@/api/accounting", () => ({
  accountingApi: {
    listAccountingCategories: jest.fn().mockResolvedValue({
      defaults: [
        { key: "rent", label: "rent", entry_type: "expense", source: "default" },
        { key: "other", label: "other", entry_type: "expense", source: "default" },
      ],
      custom: [],
    }),
    createRecurringTemplate: jest.fn().mockResolvedValue({}),
  },
}));


const detailDrawerProps: Array<Record<string, unknown>> = [];

jest.mock("../shared/DetailDrawer", () => ({
  __esModule: true,
  default: (props: {
    open: boolean;
    title: string;
    footer?: React.ReactNode;
    children: React.ReactNode;
    [key: string]: unknown;
  }) => {
    detailDrawerProps.push(props);
    const { open, title, footer, children } = props;
    return open ? (
      <div data-testid="entry-form-drawer">
        <h2>{title}</h2>
        <div data-testid="entry-form-body">{children}</div>
        {footer ? <div data-testid="entry-form-footer">{footer}</div> : null}
      </div>
    ) : null;
  },
}));

jest.mock("../shared/SegmentedTabs", () => ({
  __esModule: true,
  default: ({
    tabs,
    activeKey,
    onChange,
    ariaLabel,
  }: {
    tabs: { key: string; label: string }[];
    activeKey: string;
    onChange: (key: string) => void;
    ariaLabel?: string;
  }) => (
    <div role="tablist" aria-label={ariaLabel}>
      {tabs.map((tab) => (
        <button
          key={tab.key}
          type="button"
          role="tab"
          aria-selected={activeKey === tab.key}
          onClick={() => onChange(tab.key)}
        >
          {tab.label}
        </button>
      ))}
    </div>
  ),
}));

const useCreateEntryMock = useCreateEntry as jest.Mock;

const t = (key: string) => key;

function entry(
  overrides: Partial<ManualLedgerEntry> & { id?: number } = {},
): ManualLedgerEntry {
  return {
    id: 1,
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
    ...overrides,
  };
}

function renderDrawer(
  props: Partial<React.ComponentProps<typeof EntryFormDrawer>> = {},
) {
  const defaults: React.ComponentProps<typeof EntryFormDrawer> = {
    open: true,
    onClose: jest.fn(),
    businessId: "42",
    currency: "USD",
    initial: null,
    t,
  };
  return render(<EntryFormDrawer {...defaults} {...props} />);
}

/** NextUI Select renders a hidden native <select> + custom trigger; both share the label. */
function getCategorySelect(): HTMLSelectElement {
  const container = screen.getByTestId("hidden-select-container");
  const select = container.querySelector("select");
  if (!select) throw new Error("hidden category select not found");
  return select;
}

function getFieldInput(label: string): HTMLInputElement | HTMLTextAreaElement {
  // NextUI may attach the label to wrapper + control; pick the actual control.
  const nodes = screen.getAllByLabelText(label);
  const control = nodes.find(
    (n) =>
      n instanceof HTMLInputElement || n instanceof HTMLTextAreaElement,
  );
  if (!control) {
    throw new Error(`input/textarea for ${label} not found`);
  }
  return control;
}

function fillValidForm() {
  fireEvent.change(getCategorySelect(), { target: { value: "inventory" } });

  fireEvent.change(getFieldInput("entryForm.amount"), {
    target: { value: "25.50" },
  });
  fireEvent.change(getFieldInput("entryForm.description"), {
    target: { value: "Produce restock" },
  });
  fireEvent.change(getFieldInput("entryForm.notes"), {
    target: { value: "From market" },
  });
  fireEvent.change(getFieldInput("entryForm.reference"), {
    target: { value: "INV-9" },
  });
}

describe("EntryFormDrawer", () => {
  beforeEach(() => {
    jest.clearAllMocks();
    jest.useFakeTimers();
    jest.setSystemTime(new Date("2026-03-10T15:00:00Z"));
    detailDrawerProps.length = 0;
    useCreateEntryMock.mockReturnValue({
      mutate: mockMutate,
      isPending: false,
    });
  });

  afterEach(() => {
    jest.useRealTimers();
  });

  // R2-3b: the drawer sets `dirty` but used to pass no `discardConfirm`, so
  // DetailDrawer fell back to hardcoded English copy for every locale.
  it("passes localized discard copy to DetailDrawer", () => {
    renderDrawer();
    const props = detailDrawerProps[detailDrawerProps.length - 1];
    expect(props.discardConfirm).toEqual({
      title: "discardConfirm.entry.title",
      description: "discardConfirm.entry.description",
      confirmLabel: "discardConfirm.confirm",
      cancelLabel: "discardConfirm.cancel",
    });
  });

  it("renders new title when initial is null", () => {
    renderDrawer();
    expect(screen.getByText("entryForm.titleNew")).toBeInTheDocument();
  });

  it("shows inline validation errors and does not mutate on empty submit", () => {
    renderDrawer();

    fireEvent.click(
      screen.getByRole("button", { name: "entryForm.submit" }),
    );

    expect(screen.getByText("entryForm.errors.amountRequired")).toBeInTheDocument();
    expect(
      screen.getByText("entryForm.errors.descriptionRequired"),
    ).toBeInTheDocument();
    expect(
      screen.getByText("entryForm.errors.categoryRequired"),
    ).toBeInTheDocument();
    expect(mockMutate).not.toHaveBeenCalled();
  });

  it("shows amountPositive when amount is zero or negative", () => {
    renderDrawer();

    fireEvent.change(getCategorySelect(), { target: { value: "rent" } });
    fireEvent.change(getFieldInput("entryForm.amount"), {
      target: { value: "0" },
    });
    fireEvent.change(getFieldInput("entryForm.description"), {
      target: { value: "Rent" },
    });

    fireEvent.click(
      screen.getByRole("button", { name: "entryForm.submit" }),
    );

    expect(
      screen.getByText("entryForm.errors.amountPositive"),
    ).toBeInTheDocument();
    expect(mockMutate).not.toHaveBeenCalled();
  });

  it("swaps category options when type toggles income/expense", async () => {
    renderDrawer();

    // Default type is expense — rent is expense-only.
    let categorySelect = getCategorySelect();
    const expenseOptions = Array.from(categorySelect.querySelectorAll("option")).map(
      (o) => o.getAttribute("value") || o.value,
    );
    expect(expenseOptions).toEqual(expect.arrayContaining([...expenseCategories]));
    expect(expenseOptions).not.toEqual(
      expect.arrayContaining(["catering", "off_platform_sale"]),
    );

    fireEvent.click(
      screen.getByRole("tab", { name: "entryForm.typeIncome" }),
    );

    await waitFor(() => {
      categorySelect = getCategorySelect();
      const incomeOptions = Array.from(
        categorySelect.querySelectorAll("option"),
      ).map((o) => o.getAttribute("value") || o.value);
      expect(incomeOptions).toEqual(
        expect.arrayContaining([...incomeCategories]),
      );
      expect(incomeOptions).not.toEqual(
        expect.arrayContaining(["rent", "inventory"]),
      );
    });
  });

  it("submits create payload via useCreateEntry", () => {
    renderDrawer();

    fillValidForm();

    // Pick a specific date.
    fireEvent.change(getFieldInput("entryForm.occurredAt"), {
      target: { value: "2026-03-05" },
    });

    fireEvent.click(
      screen.getByRole("button", { name: "entryForm.submit" }),
    );

    expect(mockMutate).toHaveBeenCalledTimes(1);
    const [payload] = mockMutate.mock.calls[0];
    expect(payload).toEqual(
      expect.objectContaining({
        entry_type: "expense",
        category: "inventory",
        amount: 25.5,
        currency: "USD",
        occurred_at: "2026-03-05T12:00:00Z",
        description: "Produce restock",
        notes: "From market",
        reference: "INV-9",
      }),
    );
  });

  it("prefills via initial (duplicate) but uses today's date", () => {
    renderDrawer({
      initial: entry({
        entry_type: "income",
        category: "catering",
        amount: 120 as ManualLedgerEntry["amount"],
        description: "Private event",
        notes: "Deposit",
        reference: "EVT-3",
        occurred_at: "2025-12-01T12:00:00Z",
      }),
    });

    expect(screen.getByText("entryForm.titleDuplicate")).toBeInTheDocument();
    expect(screen.getByRole("tab", { name: "entryForm.typeIncome" })).toHaveAttribute(
      "aria-selected",
      "true",
    );

    expect(getFieldInput("entryForm.amount")).toHaveValue(120);
    expect(getFieldInput("entryForm.description")).toHaveValue("Private event");
    expect(getFieldInput("entryForm.notes")).toHaveValue("Deposit");
    expect(getFieldInput("entryForm.reference")).toHaveValue("EVT-3");
    // Today from fake timers, not original occurred_at.
    expect(getFieldInput("entryForm.occurredAt")).toHaveValue("2026-03-10");

    expect(getCategorySelect().value).toBe("catering");
  });

  it("closes drawer and calls onSuccess on mutate onSuccess", () => {
    const onClose = jest.fn();
    const onSuccess = jest.fn();
    renderDrawer({ onClose, onSuccess });

    fillValidForm();
    fireEvent.click(
      screen.getByRole("button", { name: "entryForm.submit" }),
    );

    expect(mockMutate).toHaveBeenCalled();
    const opts = mockMutate.mock.calls[0][1] as { onSuccess: () => void };
    act(() => {
      opts.onSuccess();
    });

    expect(onSuccess).toHaveBeenCalled();
    expect(onClose).toHaveBeenCalled();
  });

  it("surfaces server error in role=alert without closing", () => {
    const onClose = jest.fn();
    renderDrawer({ onClose });

    fillValidForm();
    fireEvent.click(
      screen.getByRole("button", { name: "entryForm.submit" }),
    );

    const opts = mockMutate.mock.calls[0][1] as {
      onError: (err: unknown) => void;
    };
    act(() => {
      opts.onError(new Error("backend says no"));
    });

    expect(screen.getByRole("alert")).toHaveTextContent("backend says no");
    expect(onClose).not.toHaveBeenCalled();
  });

  it("does not render when closed", () => {
    renderDrawer({ open: false });
    expect(screen.queryByTestId("entry-form-drawer")).not.toBeInTheDocument();
  });
});
