/** @jest-environment jsdom */
import React from "react";
import { render, screen, fireEvent } from "@testing-library/react";
import ShiftEditorModal, {
  type ShiftEditorLabels,
  type ShiftConflictLabels,
  type ShiftEditorInitial,
} from "./ShiftEditorModal";
import type { Position } from "@/api/positions";
import type { StaffMember } from "@/api/staff";

const labels: ShiftEditorLabels = {
  createTitle: "New shift",
  editTitle: "Edit shift",
  startTime: "Start",
  endTime: "End",
  position: "Position",
  assignee: "Assignee",
  unassignedOption: "— Open shift —",
  breakMinutes: "Break (min)",
  breakMinutesInvalid: "Break minutes cannot be negative.",
  notes: "Notes",
  save: "Save",
  saving: "Saving…",
  delete: "Delete",
  deleting: "Deleting…",
  cancel: "Cancel",
  positionRequired: "Pick a position first",
  endEqualsStart: "End time must differ from start time.",
  close: "Close",
  deleteConfirmTitle: "Delete this shift?",
  deleteConfirm: "Yes, delete",
  deleteKeep: "Keep shift",
  duration: "Shift length: {duration}",
  repeatLabel: "Also add on",
  viewTeamProfile: "View in Team",
  discardConfirmTitle: "Discard shift changes?",
  discardConfirmDescription:
    "You have unsaved edits on this shift. Closing now will discard them.",
  discardConfirm: "Discard",
  discardKeep: "Keep editing",
};

const conflictLabels: ShiftConflictLabels = {
  heading: "Heads up",
  timeoff: "On approved time off",
  unavailable: "Unavailable {range}",
  unavailableAllDay: "Unavailable all day",
  doublebook: "Already scheduled on an overlapping shift on {day}",
  overrideHint: "You can still save.",
};

const staff: StaffMember[] = [
  {
    id: 7,
    name: "Dana Ruiz",
    email: "dana@x.com",
    role: "server",
    business_id: 1,
    is_active: true,
    created_at: "",
    updated_at: "",
  },
];

const baseInitial: ShiftEditorInitial = {
  startTime: "09:00",
  endTime: "17:00",
  positionId: null,
  staffId: null,
  breakMinutes: 0,
  notes: "",
};

const WEEK_DAYS = [
  "2026-07-06",
  "2026-07-07",
  "2026-07-08",
  "2026-07-09",
  "2026-07-10",
  "2026-07-11",
  "2026-07-12",
];

function renderModal(overrides: {
  positions: Position[];
  onSubmit?: jest.Mock;
  initial?: ShiftEditorInitial;
}) {
  const onSubmit = overrides.onSubmit ?? jest.fn();
  render(
    <ShiftEditorModal
      isEdit={false}
      dayKey="2026-07-06"
      weekDays={WEEK_DAYS}
      initial={overrides.initial ?? baseInitial}
      positions={overrides.positions}
      staff={staff}
      labels={labels}
      team={{}}
      approvedTimeOff={[]}
      conflictLabels={conflictLabels}
      locale="en"
      submitting={false}
      deleting={false}
      onCancel={jest.fn()}
      onSubmit={onSubmit}
    />,
  );
  return { onSubmit };
}

const soloPosition: Position = {
  id: 3,
  business_id: 1,
  name: "Server",
  color_hex: "#1a6b6a",
  department: "FOH",
  is_active: true,
  sort_order: 0,
};

const cook: Position = {
  id: 4,
  business_id: 1,
  name: "Cook",
  color_hex: "#b45309",
  department: "BOH",
  is_active: true,
  sort_order: 1,
};

describe("ShiftEditorModal", () => {
  it("bounds the mobile sheet to the viewport with a sticky header and scroll body (#464)", () => {
    renderModal({ positions: [soloPosition] });
    const sheet = screen.getByTestId("shift-editor-sheet");
    expect(sheet.className).toMatch(/max-h-\[100dvh\]/);
    expect(sheet.className).toMatch(/\bflex-col\b/);
    expect(sheet.className).toMatch(/\boverflow-hidden\b/);
    const body = screen.getByTestId("shift-editor-body");
    expect(body.className).toMatch(/overflow-y-auto/);
    expect(body.className).toMatch(/min-h-0/);
    expect(
      screen.getByRole("button", { name: "Close" }),
    ).toBeInTheDocument();
    expect(screen.getByRole("button", { name: "Save" })).toBeInTheDocument();
  });

  it("pre-selects the sole position so a new shift saves in one tap", () => {
    const { onSubmit } = renderModal({ positions: [soloPosition] });
    fireEvent.click(screen.getByRole("button", { name: "Save" }));
    expect(onSubmit).toHaveBeenCalledTimes(1);
    expect(onSubmit.mock.calls[0][0]).toMatchObject({ positionId: 3, staffId: null });
  });

  it("blocks save and surfaces the required error when no position is chosen", () => {
    const { onSubmit } = renderModal({ positions: [soloPosition, cook] });
    fireEvent.click(screen.getByRole("button", { name: "Save" }));
    expect(onSubmit).not.toHaveBeenCalled();
    expect(screen.getByText("Pick a position first")).toBeInTheDocument();
  });

  it("defaults the position to the assignee's matching role until touched", () => {
    // Dana is a server; two positions exist — the Server one should pre-select.
    const { onSubmit } = renderModal({
      positions: [soloPosition, cook],
      initial: { ...baseInitial, staffId: 7 },
    });
    fireEvent.click(screen.getByRole("button", { name: "Save" }));
    expect(onSubmit).toHaveBeenCalledTimes(1);
    expect(onSubmit.mock.calls[0][0]).toMatchObject({ positionId: 3, staffId: 7 });
  });

  it("shows a live shift-length readout that subtracts the break", () => {
    renderModal({ positions: [soloPosition] });
    // 09:00–17:00, no break.
    expect(screen.getByText("Shift length: 8h")).toBeInTheDocument();
    fireEvent.change(screen.getByLabelText("Break (min)"), {
      target: { value: "30" },
    });
    expect(screen.getByText("Shift length: 7h 30m")).toBeInTheDocument();
  });

  it("repeats a new shift across the picked weekdays", () => {
    const { onSubmit } = renderModal({ positions: [soloPosition] });
    // The clicked day is locked on; pick Wednesday too.
    const wed = screen.getByRole("checkbox", { name: /2026-07-08/ });
    fireEvent.click(wed);
    fireEvent.click(screen.getByRole("button", { name: "Save" }));
    expect(onSubmit).toHaveBeenCalledTimes(1);
    expect(onSubmit.mock.calls[0][0]).toMatchObject({
      dayKey: "2026-07-06",
      days: ["2026-07-06", "2026-07-08"],
    });
  });

  it("requires an explicit confirmation before deleting a shift", () => {
    const onDelete = jest.fn();
    render(
      <ShiftEditorModal
        isEdit
        dayKey="2026-07-06"
        initial={{ ...baseInitial, positionId: 3 }}
        positions={[soloPosition]}
        staff={staff}
        labels={labels}
        team={{}}
        approvedTimeOff={[]}
        conflictLabels={conflictLabels}
        locale="en"
        submitting={false}
        deleting={false}
        onCancel={jest.fn()}
        onSubmit={jest.fn()}
        onDelete={onDelete}
      />,
    );
    // First press arms the confirmation — nothing is deleted yet.
    fireEvent.click(screen.getByRole("button", { name: "Delete" }));
    expect(onDelete).not.toHaveBeenCalled();
    expect(screen.getByText("Delete this shift?")).toBeInTheDocument();
    // "Keep shift" disarms.
    fireEvent.click(screen.getByRole("button", { name: "Keep shift" }));
    expect(screen.queryByText("Delete this shift?")).not.toBeInTheDocument();
    // Arm again and confirm.
    fireEvent.click(screen.getByRole("button", { name: "Delete" }));
    fireEvent.click(screen.getByRole("button", { name: "Yes, delete" }));
    expect(onDelete).toHaveBeenCalledTimes(1);
  });

  it("renders the published-week note when provided", () => {
    render(
      <ShiftEditorModal
        isEdit={false}
        dayKey="2026-07-06"
        initial={baseInitial}
        positions={[soloPosition]}
        staff={staff}
        labels={labels}
        team={{}}
        approvedTimeOff={[]}
        conflictLabels={conflictLabels}
        locale="en"
        submitting={false}
        deleting={false}
        onCancel={jest.fn()}
        onSubmit={jest.fn()}
        publishedNote="Live week — changes are visible immediately"
      />,
    );
    expect(
      screen.getByText("Live week — changes are visible immediately"),
    ).toBeInTheDocument();
  });

  it("offers View in Team for an assigned shift in edit mode (wave 4)", () => {
    const onViewStaff = jest.fn();
    render(
      <ShiftEditorModal
        isEdit
        dayKey="2026-07-06"
        initial={{ ...baseInitial, positionId: 3, staffId: 7 }}
        positions={[soloPosition]}
        staff={staff}
        labels={labels}
        team={{}}
        approvedTimeOff={[]}
        conflictLabels={conflictLabels}
        locale="en"
        submitting={false}
        deleting={false}
        onCancel={jest.fn()}
        onSubmit={jest.fn()}
        onViewStaff={onViewStaff}
      />,
    );
    fireEvent.click(screen.getByRole("button", { name: "View in Team" }));
    expect(onViewStaff).toHaveBeenCalledWith("Dana Ruiz");
  });
});

describe("L5-27 Descanso / break minutes no native English bubble", () => {
  it("uses type=text on break field and form has noValidate", () => {
    const { container } = render(
      <ShiftEditorModal
        isEdit={false}
        dayKey="2026-07-06"
        weekDays={WEEK_DAYS}
        initial={baseInitial}
        positions={[soloPosition]}
        staff={staff}
        labels={labels}
        team={{}}
        approvedTimeOff={[]}
        conflictLabels={conflictLabels}
        locale="en"
        submitting={false}
        deleting={false}
        onCancel={jest.fn()}
        onSubmit={jest.fn()}
      />,
    );
    const form = container.querySelector("form");
    expect(form).toHaveAttribute("novalidate");
    const breakInput = screen.getByTestId("shift-break-minutes");
    expect(breakInput).toHaveAttribute("type", "text");
    expect(breakInput).toHaveAttribute("inputmode", "numeric");
    // No native min attribute that would surface Chrome English bubbles.
    expect(breakInput).not.toHaveAttribute("min");
  });

  it("shows localized invalid error for negative break without silent clamp (L5-27)", () => {
    const { onSubmit } = renderModal({ positions: [soloPosition] });
    const breakInput = screen.getByTestId("shift-break-minutes");
    // Keep raw string so breakMinutesInvalid is reachable (not dead copy).
    fireEvent.change(breakInput, { target: { value: "-15" } });
    expect(breakInput).toHaveValue("-15");
    expect(screen.getByText("Break minutes cannot be negative.")).toBeInTheDocument();
    fireEvent.click(screen.getByRole("button", { name: "Save" }));
    expect(onSubmit).not.toHaveBeenCalled();
  });

  // L5-28: Fin === Inicio is S-5 on the End field, not under Position.
  it("rejects Fin equal to Inicio with S-5 on the end field (L5-28)", () => {
    const { onSubmit } = renderModal({
      positions: [soloPosition],
      initial: { ...baseInitial, startTime: "09:00", endTime: "09:00", positionId: 3 },
    });
    fireEvent.click(screen.getByRole("button", { name: "Save" }));
    expect(onSubmit).not.toHaveBeenCalled();
    expect(
      screen.getByText("End time must differ from start time."),
    ).toBeInTheDocument();
    // Error must be field-scoped to Fin (TimeField isInvalid), not the position row.
    const endInput = screen.getByTestId("shift-end-time");
    expect(endInput).toHaveAttribute("aria-invalid", "true");
    expect(screen.queryByText("Pick a position first")).not.toBeInTheDocument();
  });

  // L5-26: Enter/re-submit while submitting must not call onSubmit again.
  it("ignores submit while submitting is true (L5-26 double-submit guard)", () => {
    const onSubmit = jest.fn();
    const { container } = render(
      <ShiftEditorModal
        isEdit={false}
        dayKey="2026-07-06"
        weekDays={WEEK_DAYS}
        initial={{ ...baseInitial, positionId: 3 }}
        positions={[soloPosition]}
        staff={staff}
        labels={labels}
        team={{}}
        approvedTimeOff={[]}
        conflictLabels={conflictLabels}
        locale="en"
        submitting
        deleting={false}
        onCancel={jest.fn()}
        onSubmit={onSubmit}
      />,
    );
    // Enter re-entry path: form submit while isLoading is already true.
    const form = container.querySelector("form");
    expect(form).toBeTruthy();
    fireEvent.submit(form!);
    fireEvent.submit(form!);
    expect(onSubmit).not.toHaveBeenCalled();
  });
});
