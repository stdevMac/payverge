/** @jest-environment jsdom */
import React from "react";
import { render, screen, fireEvent } from "@testing-library/react";
import LogbookView, { type LogbookLabels } from "@/components/staff/dashboard/LogbookView";
import type { ShiftNote } from "@/api/logbook";

const labels: LogbookLabels = {
  title: "Shift logbook",
  subtitle: "Notes for handover",
  composeLabel: "Add a note",
  categoryLabel: "Category",
  contentLabel: "Note",
  contentPlaceholder: "What should the next shift know?",
  submit: "Log note",
  emptyTitle: "No notes yet",
  emptySubtitle: "Log the first note.",
  loading: "Loading logbook",
  categories: { sales: "Sales", guests: "Guests", staffing: "Staffing", maintenance: "Maintenance", other: "Other" },
};

const notes: ShiftNote[] = [
  {
    id: 1, business_id: 42, shift_id: null, for_date: "2026-06-30T00:00:00Z",
    author_staff_id: 7, category: "maintenance", content: "Walk-in fan rattling",
    created_at: "2026-06-30T18:00:00Z",
  },
];

describe("LogbookView", () => {
  it("renders the feed and never shows a dollar amount", () => {
    render(<LogbookView notes={notes} labels={labels} onSubmit={jest.fn()} />);
    expect(screen.getByText("Walk-in fan rattling")).toBeInTheDocument();
    expect(document.body.textContent).not.toMatch(/\$\d/);
  });

  it("renders an empty state when there are no notes", () => {
    render(<LogbookView notes={[]} labels={labels} onSubmit={jest.fn()} />);
    expect(screen.getByText("No notes yet")).toBeInTheDocument();
  });

  it("submits the composed note with category + content", () => {
    const onSubmit = jest.fn();
    render(<LogbookView notes={notes} labels={labels} onSubmit={onSubmit} />);
    fireEvent.change(screen.getByLabelText("Note"), { target: { value: "Closed early" } });
    fireEvent.click(screen.getByRole("button", { name: "Log note" }));
    expect(onSubmit).toHaveBeenCalledWith({ category: "sales", content: "Closed early" });
  });
});
