/** @jest-environment jsdom */
import React from "react";
import { render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import PollComposer from "./PollComposer";
import { pollsApi } from "@/api/engagement";
import { positionsApi } from "@/api/positions";

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
  pollsApi: { list: jest.fn(), create: jest.fn(), results: jest.fn(), close: jest.fn() },
}));
jest.mock("@/api/positions", () => ({ positionsApi: { list: jest.fn() } }));

const mockedPolls = pollsApi as unknown as {
  list: jest.Mock;
  create: jest.Mock;
  results: jest.Mock;
  close: jest.Mock;
};
const mockedPositions = positionsApi as unknown as { list: jest.Mock };

const openPoll = {
  id: 5,
  business_id: 42,
  author_staff_id: 1,
  question: "Best shift meal?",
  is_anonymous: false,
  audience_filter: "all",
  status: "open" as const,
  closes_at: null,
  options: [
    { id: 10, poll_id: 5, label: "Tacos", sort_order: 0 },
    { id: 11, poll_id: 5, label: "Ramen", sort_order: 1 },
  ],
  my_vote: null,
};

function renderComposer() {
  const qc = new QueryClient({ defaultOptions: { queries: { retry: false } } });
  return render(
    <QueryClientProvider client={qc}>
      <PollComposer businessId="42" />
    </QueryClientProvider>,
  );
}

beforeEach(() => {
  jest.clearAllMocks();
  mockedPositions.list.mockResolvedValue([]);
  mockedPolls.list.mockResolvedValue([openPoll]);
  mockedPolls.results.mockResolvedValue({
    poll_id: 5,
    question: "Best shift meal?",
    is_anonymous: false,
    status: "open",
    options: [
      { option_id: 10, label: "Tacos", votes: 3 },
      { option_id: 11, label: "Ramen", votes: 1 },
    ],
  });
});

describe("PollComposer", () => {
  it("renders live results (bars + counts) on each operator poll card", async () => {
    renderComposer();
    // The operator sees the tally that was previously staff-only.
    await waitFor(() => expect(mockedPolls.results).toHaveBeenCalledWith("42", 5));
    // Both option labels from the results render (the bars/counts surface).
    expect(await screen.findByText("Tacos")).toBeInTheDocument();
    expect(screen.getByText("Ramen")).toBeInTheDocument();
    // The total-votes footer renders (translation-mock returns the key).
    expect(
      screen.getByText("dashboard.engagement.polls.totalVotes"),
    ).toBeInTheDocument();
    // 3+1 votes → Tacos bar is 75% wide.
    const bars = document.querySelectorAll('[style*="width: 75%"]');
    expect(bars.length).toBeGreaterThan(0);
  });

  it("confirms before closing an open poll", async () => {
    mockedPolls.close.mockResolvedValue(undefined);
    renderComposer();

    await userEvent.click(
      await screen.findByRole("button", { name: "dashboard.engagement.polls.close" }),
    );
    // Confirm modal open; close only fires on confirm.
    expect(mockedPolls.close).not.toHaveBeenCalled();
    // The modal's confirm button carries the same "close" label.
    const closeButtons = screen.getAllByRole("button", {
      name: "dashboard.engagement.polls.close",
    });
    await userEvent.click(closeButtons[closeButtons.length - 1]);
    await waitFor(() => expect(mockedPolls.close).toHaveBeenCalledWith("42", 5));
  });
});
