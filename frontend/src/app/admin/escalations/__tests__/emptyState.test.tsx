/** @jest-environment jsdom */
import React from "react";
import { render, screen, waitFor } from "@testing-library/react";
import AdminEscalationsPage from "../page";
import { getAdminEscalations } from "@/api/adminEscalations";

jest.mock("@/api/adminEscalations", () => ({
  getAdminEscalations: jest.fn(),
  patchEscalation: jest.fn(),
  parseEscalationTranscriptTarget: jest.fn(),
  ESCALATION_STATUSES: ["open", "in_progress", "resolved", "closed"],
}));

jest.mock("@/components/admin/primitives", () => ({
  AdminPageFrame: ({ children }: { children: React.ReactNode }) => (
    <div>{children}</div>
  ),
  OpsTranscriptModal: () => null,
  formatAdminDate: (d: string) => d,
}));

jest.mock("@/utils/apiError", () => ({
  apiErrorDetail: () => "error",
}));

describe("AdminEscalationsPage empty state (Task 33)", () => {
  it("renders EmptyState for zero escalations", async () => {
    (getAdminEscalations as jest.Mock).mockResolvedValue({
      escalations: [],
      total: 0,
    });
    render(<AdminEscalationsPage />);
    await waitFor(() => {
      expect(screen.getByTestId("admin-escalations-empty")).toBeInTheDocument();
    });
    expect(
      screen.getByRole("heading", { name: /no escalations raised yet/i }),
    ).toBeInTheDocument();
    expect(screen.queryByRole("table")).not.toBeInTheDocument();
  });
});
