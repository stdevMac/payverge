/** @jest-environment jsdom */
import React from "react";
import { render, screen, waitFor, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import ChecklistTemplateComposer from "./ChecklistTemplateComposer";
import { checklistsApi } from "@/api/engagement";
import { getBusinessStaff } from "@/api/staff";

jest.mock("@/i18n/SimpleTranslationProvider", () => ({
  useSimpleLocale: () => ({ locale: "en", setLocale: jest.fn() }),
  getTranslation: (key: string) => key,
}));
const mockShowSuccess = jest.fn();
const mockShowError = jest.fn();
jest.mock("@/contexts/ToastContext", () => ({
  useToast: () => ({
    showSuccess: mockShowSuccess,
    showError: mockShowError,
    showInfo: jest.fn(),
    showWarning: jest.fn(),
    showToast: jest.fn(),
  }),
}));
jest.mock("@/api/engagement", () => ({
  checklistsApi: {
    listTemplates: jest.fn(),
    createTemplate: jest.fn(),
    createRun: jest.fn(),
    listBusinessRuns: jest.fn(),
  },
}));
jest.mock("@/api/staff", () => ({ getBusinessStaff: jest.fn() }));

const mockedChecklists = checklistsApi as unknown as {
  listTemplates: jest.Mock;
  createTemplate: jest.Mock;
  createRun: jest.Mock;
  listBusinessRuns: jest.Mock;
};
const mockedStaff = getBusinessStaff as unknown as jest.Mock;

function renderComposer() {
  const qc = new QueryClient({ defaultOptions: { queries: { retry: false } } });
  return render(
    <QueryClientProvider client={qc}>
      <ChecklistTemplateComposer businessId="42" />
    </QueryClientProvider>,
  );
}

beforeEach(() => {
  jest.clearAllMocks();
  mockedChecklists.listTemplates.mockResolvedValue([
    { id: 3, business_id: 42, name: "Opening", kind: "opening", position_id: null, is_active: true },
  ]);
  mockedStaff.mockResolvedValue({
    staff: [
      { id: 7, name: "Dana", email: "d@x.co", role: "server", business_id: 42, created_at: "", updated_at: "" },
    ],
    pending_invitations: [],
  });
});

describe("ChecklistTemplateComposer", () => {
  it("assigns a run to a staffer from a template row (createRun)", async () => {
    mockedChecklists.listBusinessRuns.mockResolvedValue([]);
    mockedChecklists.createRun.mockResolvedValue({ id: 99, status: "pending" });

    renderComposer();
    // The template row's Assign button opens the assign modal.
    await userEvent.click(
      await screen.findByRole("button", {
        name: "dashboard.engagement.checklists.assign",
      }),
    );
    const dialog = await screen.findByRole("dialog");

    // Pick Dana in the StaffSelect (NextUI Autocomplete): type to filter then
    // select the option from the listbox.
    const staffInput = within(dialog).getByLabelText(
      "dashboard.engagement.checklists.assignStaff",
    );
    await userEvent.click(staffInput);
    await userEvent.type(staffInput, "Dana");
    const option = await screen.findByRole("option", { name: /Dana/ });
    await userEvent.click(option);

    await userEvent.click(
      within(dialog).getByRole("button", {
        name: "dashboard.engagement.checklists.assignStart",
      }),
    );
    await waitFor(() =>
      expect(mockedChecklists.createRun).toHaveBeenCalledWith("42", {
        template_id: 3,
        assigned_staff_id: 7,
      }),
    );
    await waitFor(() => expect(mockShowSuccess).toHaveBeenCalled());
  });

  it("renders the operator run-status list with template + assignee names", async () => {
    mockedChecklists.listBusinessRuns.mockResolvedValue([
      {
        id: 1,
        business_id: 42,
        template_id: 3,
        assigned_staff_id: 7,
        shift_id: null,
        for_date: "2026-07-20T00:00:00Z",
        status: "in_progress",
        completed_at: null,
        template_name: "Opening",
        assigned_staff_name: "Dana",
      },
    ]);

    renderComposer();
    // The run's template name + assignee + status chip all render.
    expect(await screen.findAllByText("Opening")).not.toHaveLength(0);
    expect(screen.getByText("Dana")).toBeInTheDocument();
    expect(
      screen.getByText("dashboard.engagement.checklists.runStatus.in_progress"),
    ).toBeInTheDocument();
  });
});
