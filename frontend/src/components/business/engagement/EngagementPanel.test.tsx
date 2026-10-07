/** @jest-environment jsdom */
import React from "react";
import { render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import EngagementPanel from "./EngagementPanel";
import { checklistsApi, documentsApi, pollsApi } from "@/api/engagement";
import { positionsApi } from "@/api/positions";

jest.mock("@/i18n/SimpleTranslationProvider", () => ({
  useSimpleLocale: () => ({ locale: "en", setLocale: jest.fn() }),
  getTranslation: (key: string) => key,
}));
jest.mock("@/contexts/ToastContext", () => ({
  useToast: () => ({
    showSuccess: jest.fn(),
    showError: jest.fn(),
    showInfo: jest.fn(),
    showWarning: jest.fn(),
    showToast: jest.fn(),
  }),
}));
jest.mock("@/api/engagement", () => ({
  checklistsApi: { listTemplates: jest.fn(), createTemplate: jest.fn() },
  documentsApi: { list: jest.fn(), create: jest.fn(), update: jest.fn() },
  pollsApi: { list: jest.fn(), create: jest.fn(), close: jest.fn() },
}));
jest.mock("@/api/positions", () => ({
  positionsApi: { list: jest.fn() },
}));

const mockedChecklists = checklistsApi as unknown as { listTemplates: jest.Mock };
const mockedDocuments = documentsApi as unknown as { list: jest.Mock };
const mockedPolls = pollsApi as unknown as { list: jest.Mock; create: jest.Mock };
const mockedPositions = positionsApi as unknown as { list: jest.Mock };

function renderPanel() {
  const qc = new QueryClient({ defaultOptions: { queries: { retry: false } } });
  return render(
    <QueryClientProvider client={qc}>
      <EngagementPanel businessId="42" />
    </QueryClientProvider>,
  );
}

beforeEach(() => {
  jest.clearAllMocks();
  mockedChecklists.listTemplates.mockResolvedValue([]);
  mockedDocuments.list.mockResolvedValue({ documents: [], acked: {} });
  mockedPolls.list.mockResolvedValue([]);
  mockedPolls.create.mockResolvedValue({ id: 1 });
  mockedPositions.list.mockResolvedValue([]);
});

describe("EngagementPanel", () => {
  it("renders the three tabs, mounts the checklists composer first, and shows no dollars", async () => {
    renderPanel();
    expect(screen.getByRole("tab", { name: "dashboard.engagement.tabs.checklists" })).toBeInTheDocument();
    expect(screen.getByRole("tab", { name: "dashboard.engagement.tabs.documents" })).toBeInTheDocument();
    expect(screen.getByRole("tab", { name: "dashboard.engagement.tabs.polls" })).toBeInTheDocument();
    await waitFor(() => expect(mockedChecklists.listTemplates).toHaveBeenCalledWith("42"));
    // Cold start (no templates yet): the creation form stays open by default.
    await waitFor(() =>
      expect(
        screen.getByRole("heading", { name: "dashboard.engagement.checklists.new" }),
      ).toBeInTheDocument(),
    );
    expect(document.body.textContent).not.toMatch(/\$\d/);
  });

  it("lists existing checklists first — the creation form hides behind New checklist", async () => {
    mockedChecklists.listTemplates.mockResolvedValue([
      { id: 3, business_id: 42, name: "Opening duties", kind: "opening", items: [] },
    ]);
    renderPanel();
    await waitFor(() =>
      expect(screen.getByText("Opening duties")).toBeInTheDocument(),
    );
    // The always-open form no longer greets the tab…
    expect(
      screen.queryByPlaceholderText("dashboard.engagement.checklists.namePlaceholder"),
    ).not.toBeInTheDocument();
    // …a New-checklist button opens it on demand.
    await userEvent.click(
      screen.getByRole("button", { name: /dashboard\.engagement\.checklists\.new/ }),
    );
    expect(
      screen.getByPlaceholderText("dashboard.engagement.checklists.namePlaceholder"),
    ).toBeInTheDocument();
  });

  it("switches to the polls composer when the polls tab is selected", async () => {
    renderPanel();
    await userEvent.click(screen.getByRole("tab", { name: "dashboard.engagement.tabs.polls" }));
    await waitFor(() => expect(mockedPolls.list).toHaveBeenCalledWith("42"));
    expect(
      screen.getByRole("heading", { name: "dashboard.engagement.polls.new" }),
    ).toBeInTheDocument();
    expect(document.body.textContent).not.toMatch(/\$\d/);
  });

  it("opens a poll with a prefixed audience token (never a bare value)", async () => {
    renderPanel();
    await userEvent.click(screen.getByRole("tab", { name: "dashboard.engagement.tabs.polls" }));
    await waitFor(() => expect(mockedPolls.list).toHaveBeenCalledWith("42"));

    await userEvent.type(
      screen.getByLabelText("dashboard.engagement.polls.question"),
      "Pizza night?",
    );
    await userEvent.type(screen.getByLabelText("dashboard.engagement.polls.option 1"), "Yes");
    await userEvent.type(screen.getByLabelText("dashboard.engagement.polls.option 2"), "No");
    // AudienceSelect is a NextUI Select: the label matches both the visible
    // trigger and NextUI's hidden native <select>; drive the native one.
    const audienceNative = screen
      .getAllByLabelText("dashboard.engagement.audience.label")
      .find((el) => el.tagName === "SELECT") as HTMLSelectElement;
    await userEvent.selectOptions(audienceNative, "role:server");
    await userEvent.click(screen.getByRole("button", { name: "dashboard.engagement.polls.save" }));

    await waitFor(() => expect(mockedPolls.create).toHaveBeenCalled());
    expect(mockedPolls.create.mock.calls[0][1]).toMatchObject({ audience_filter: "role:server" });
  });
});
