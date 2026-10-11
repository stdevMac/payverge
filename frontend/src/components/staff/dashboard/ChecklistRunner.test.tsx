/** @jest-environment jsdom */
import React from "react";
import { render, screen, fireEvent } from "@testing-library/react";
import ChecklistRunner, { type ChecklistRunnerLabels } from "@/components/staff/dashboard/ChecklistRunner";
import type { ChecklistRun, ChecklistRunDetail } from "@/api/engagement";

const labels: ChecklistRunnerLabels = {
  title: "Checklists",
  empty: "No checklists assigned",
  emptyHint: "Tasks show up here.",
  loading: "Loading checklists",
  statusPending: "Not started",
  statusInProgress: "In progress",
  statusComplete: "Complete",
  back: "Back",
  required: "Required",
  itemsEmpty: "No tasks on this checklist",
  openRun: "Open checklist",
  tickError: "Couldn't update the task",
};

const runs: ChecklistRun[] = [
  {
    id: 1, business_id: 42, template_id: 3, assigned_staff_id: 7, shift_id: null,
    for_date: "2026-06-30T00:00:00Z", status: "in_progress", completed_at: null,
  },
  {
    id: 2, business_id: 42, template_id: 3, assigned_staff_id: 7, shift_id: null,
    for_date: "2026-06-29T00:00:00Z", status: "complete", completed_at: "2026-06-29T20:00:00Z",
  },
];

const detail: ChecklistRunDetail = {
  run: runs[0],
  items: [
    { item_id: 91, label: "Unlock doors", is_required: true, sort_order: 0, done: false, note: "", completed_at: null },
    { item_id: 92, label: "Turn on lights", is_required: false, sort_order: 1, done: true, note: "", completed_at: "2026-06-30T08:00:00Z" },
  ],
};

const baseProps = {
  labels,
  formatDate: (iso: string) => iso,
  selectedRunId: null as number | null,
  onSelectRun: jest.fn(),
  onBack: jest.fn(),
  detail: null as ChecklistRunDetail | null,
  togglingItemId: null as number | null,
  onToggleItem: jest.fn(),
};

beforeEach(() => jest.clearAllMocks());

describe("ChecklistRunner — runs overview", () => {
  it("renders the runs with status and never shows a dollar amount", () => {
    render(<ChecklistRunner runs={runs} {...baseProps} />);
    expect(screen.getByText("In progress")).toBeInTheDocument();
    expect(screen.getByText("Complete")).toBeInTheDocument();
    expect(document.body.textContent).not.toMatch(/\$\d/);
  });

  it("selects a run when its row is pressed", () => {
    const onSelectRun = jest.fn();
    render(<ChecklistRunner runs={runs} {...baseProps} onSelectRun={onSelectRun} />);
    fireEvent.click(screen.getAllByRole("button", { name: "Open checklist" })[0]);
    expect(onSelectRun).toHaveBeenCalledWith(1);
  });

  it("renders an empty state when there are no runs", () => {
    render(<ChecklistRunner runs={[]} {...baseProps} />);
    expect(screen.getByText("No checklists assigned")).toBeInTheDocument();
  });

  it("renders the loading state", () => {
    render(<ChecklistRunner runs={[]} {...baseProps} loading />);
    expect(screen.getByRole("status", { name: "Loading checklists" })).toBeInTheDocument();
  });
});

describe("ChecklistRunner — run detail ticker", () => {
  it("renders the run's items, marks required ones, and never shows a dollar amount", () => {
    render(<ChecklistRunner runs={runs} {...baseProps} selectedRunId={1} detail={detail} />);
    expect(screen.getByText("Unlock doors")).toBeInTheDocument();
    expect(screen.getByText("Turn on lights")).toBeInTheDocument();
    expect(screen.getByText("Required")).toBeInTheDocument();
    expect(document.body.textContent).not.toMatch(/\$\d/);
  });

  it("toggles an item, calling the tick handler with the next state", () => {
    const onToggleItem = jest.fn();
    render(
      <ChecklistRunner
        runs={runs}
        {...baseProps}
        selectedRunId={1}
        detail={detail}
        onToggleItem={onToggleItem}
      />,
    );
    // "Unlock doors" starts undone → toggling it on calls tick(91, true).
    fireEvent.click(screen.getByRole("checkbox", { name: "Unlock doors" }));
    expect(onToggleItem).toHaveBeenCalledWith(91, true);
  });

  it("goes back to the overview when Back is pressed", () => {
    const onBack = jest.fn();
    render(
      <ChecklistRunner runs={runs} {...baseProps} selectedRunId={1} detail={detail} onBack={onBack} />,
    );
    fireEvent.click(screen.getByRole("button", { name: "Back" }));
    expect(onBack).toHaveBeenCalled();
  });

  it("shows an empty items state when the run has no items", () => {
    render(
      <ChecklistRunner
        runs={runs}
        {...baseProps}
        selectedRunId={1}
        detail={{ run: runs[0], items: [] }}
      />,
    );
    expect(screen.getByText("No tasks on this checklist")).toBeInTheDocument();
  });
});
