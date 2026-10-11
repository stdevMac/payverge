/** @jest-environment jsdom */
/**
 * L5-27: negative break minutes must show errorMessage, not clamp silently to 0.
 */
import React from "react";
import { render, screen, fireEvent } from "@testing-library/react";
import ShiftEditorModal from "../ShiftEditorModal";

const labels = {
  createTitle: "Create",
  editTitle: "Edit",
  startTime: "Start",
  endTime: "End",
  position: "Position",
  assignee: "Assignee",
  unassignedOption: "Open",
  breakMinutes: "Break",
  breakMinutesInvalid: "Break cannot be negative",
  endEqualsStart: "End equals start",
  notes: "Notes",
  save: "Save",
  saving: "Saving",
  delete: "Delete",
  deleting: "Deleting",
  cancel: "Cancel",
  positionRequired: "Position required",
  close: "Close",
  deleteConfirmTitle: "Confirm delete",
  deleteConfirm: "Yes delete",
  deleteKeep: "Keep",
  duration: "Duration {duration}",
  repeatLabel: "Repeat",
  viewTeamProfile: "View",
  discardConfirmTitle: "Discard?",
  discardConfirm: "Discard",
  discardKeep: "Keep editing",
};

const conflictLabels = {
  heading: "Conflicts",
  timeoff: "Time off",
  doublebook: "Double {day}",
  unavailable: "Unavailable {range}",
  unavailableAllDay: "Unavailable all day",
  overrideHint: "Override",
};

describe("L5-27 break minutes negative error", () => {
  it("shows breakMinutesInvalid when operator types a negative value", () => {
    render(
      <ShiftEditorModal
        isEdit={false}
        dayKey="2026-06-01"
        weekDays={["2026-06-01"]}
        initial={{
          startTime: "09:00",
          endTime: "17:00",
          positionId: 1,
          staffId: null,
          breakMinutes: 0,
          notes: "",
        }}
        positions={[
          {
            id: 1,
            business_id: 1,
            name: "Server",
            color_hex: "#1a6b6a",
            department: "FOH",
            is_active: true,
            sort_order: 0,
          },
        ]}
        staff={[]}
        labels={labels as never}
        team={{}}
        approvedTimeOff={[]}
        conflictLabels={conflictLabels as never}
        locale="en"
        submitting={false}
        deleting={false}
        onCancel={jest.fn()}
        onSubmit={jest.fn()}
      />,
    );
    const input = screen.getByTestId("shift-break-minutes");
    fireEvent.change(input, { target: { value: "-5" } });
    expect(screen.getByText("Break cannot be negative")).toBeInTheDocument();
    // Must NOT have silently clamped to 0.
    expect((input as HTMLInputElement).value).toBe("-5");
  });
});
