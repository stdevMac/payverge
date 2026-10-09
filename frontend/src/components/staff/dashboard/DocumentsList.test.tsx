/** @jest-environment jsdom */
import React from "react";
import { render, screen, fireEvent } from "@testing-library/react";
import DocumentsList, { type DocumentsListLabels } from "@/components/staff/dashboard/DocumentsList";
import type { DocumentRow } from "@/api/engagement";

const labels: DocumentsListLabels = {
  title: "Documents",
  empty: "No documents yet",
  emptyHint: "Policies appear here.",
  loading: "Loading documents",
  open: "Open",
  version: "Version {n}",
  ackRequired: "Acknowledgement required",
  acknowledge: "I have read this",
  acknowledged: "Acknowledged",
};

const docs: DocumentRow[] = [
  {
    id: 11, business_id: 42, title: "Employee handbook", url: "", content: "Be on time.",
    version: 2, require_ack: true, audience_filter: "all", created_at: "2026-06-30T00:00:00Z",
  },
  {
    id: 12, business_id: 42, title: "Allergen policy", url: "https://example.com/policy.pdf",
    content: "", version: 1, require_ack: false, audience_filter: "all", created_at: "2026-06-29T00:00:00Z",
  },
];

describe("DocumentsList", () => {
  it("renders documents, version + open link, and never shows a dollar amount", () => {
    render(
      <DocumentsList
        docs={docs}
        labels={labels}
        ackedIds={new Set()}
        ackingId={null}
        onAck={jest.fn()}
      />,
    );
    expect(screen.getByText("Employee handbook")).toBeInTheDocument();
    expect(screen.getByText("Version 2")).toBeInTheDocument();
    expect(screen.getByRole("link", { name: "Open" })).toHaveAttribute(
      "href",
      "https://example.com/policy.pdf",
    );
    expect(document.body.textContent).not.toMatch(/\$\d/);
  });

  it("acknowledges a document that requires it", () => {
    const onAck = jest.fn();
    render(
      <DocumentsList
        docs={docs}
        labels={labels}
        ackedIds={new Set()}
        ackingId={null}
        onAck={onAck}
      />,
    );
    fireEvent.click(screen.getByRole("button", { name: "I have read this" }));
    expect(onAck).toHaveBeenCalledWith(11);
  });

  it("shows the acknowledged chip once acked and hides the button", () => {
    render(
      <DocumentsList
        docs={docs}
        labels={labels}
        ackedIds={new Set([11])}
        ackingId={null}
        onAck={jest.fn()}
      />,
    );
    expect(screen.getByText("Acknowledged")).toBeInTheDocument();
    expect(screen.queryByRole("button", { name: "I have read this" })).not.toBeInTheDocument();
  });

  it("renders an empty state when there are no documents", () => {
    render(
      <DocumentsList docs={[]} labels={labels} ackedIds={new Set()} ackingId={null} onAck={jest.fn()} />,
    );
    expect(screen.getByText("No documents yet")).toBeInTheDocument();
  });
});
