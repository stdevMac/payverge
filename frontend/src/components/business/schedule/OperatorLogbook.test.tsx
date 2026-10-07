/** @jest-environment jsdom */
import React from "react";
import { render, screen, waitFor } from "@testing-library/react";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import OperatorLogbook from "./OperatorLogbook";
import { logbookApi, type ShiftNote } from "@/api/logbook";

const COPY: Record<string, string> = {
  "dashboardLogbook.title": "Shift log",
  "dashboardLogbook.loading": "Loading shift log…",
  "dashboardLogbook.authorFallback": "Team member",
  "dashboardLogbook.countOne": "1 note",
  "dashboardLogbook.countOther": "{count} notes",
  "dashboardLogbook.categories.sales": "Sales",
  "dashboardLogbook.categories.maintenance": "Maintenance",
  "dashboardLogbook.categories.other": "Other",
};

jest.mock("@/i18n/SimpleTranslationProvider", () => ({
  useSimpleLocale: () => ({ locale: "en", setLocale: jest.fn() }),
  getTranslation: (key: string) => COPY[key] ?? key,
}));

jest.mock("@/api/logbook", () => ({
  logbookApi: { list: jest.fn() },
}));

const mockedList = logbookApi.list as jest.Mock;

function renderLogbook(businessTimezone: string | null = null) {
  const qc = new QueryClient({ defaultOptions: { queries: { retry: false } } });
  return render(
    <QueryClientProvider client={qc}>
      <OperatorLogbook
        businessId="42"
        businessTimezone={businessTimezone}
      />
    </QueryClientProvider>,
  );
}

beforeEach(() => jest.clearAllMocks());

const note = (over: Partial<ShiftNote>): ShiftNote => ({
  id: 1,
  business_id: 42,
  shift_id: null,
  for_date: "2026-07-01T00:00:00Z",
  author_staff_id: 7,
  author_name: "Ana",
  category: "sales",
  content: "Strong brunch",
  created_at: "2026-07-01T14:05:00Z",
  ...over,
});

test("stamps note time in the venue zone, not 4:00 AM UTC (#247)", async () => {
  mockedList.mockResolvedValue([
    note({ created_at: "2026-08-11T20:00:00.000Z" }),
  ]);
  renderLogbook("America/New_York");
  expect(await screen.findByText("Ana")).toBeInTheDocument();
  const body = document.body.textContent || "";
  expect(body).toMatch(/4:00\s*PM/);
  expect(body).not.toMatch(/4:00\s*AM/);
});

test("shows today's handover notes with author + category and no dollars", async () => {
  mockedList.mockResolvedValue([
    note({ id: 1, author_name: "Ana", category: "sales", content: "Strong brunch" }),
    note({ id: 2, author_staff_id: 99, author_name: "", category: "maintenance", content: "Fan rattling" }),
  ]);
  renderLogbook();

  expect(await screen.findByText("Ana")).toBeInTheDocument();
  expect(screen.getByText("Strong brunch")).toBeInTheDocument();
  // An author with no resolved name degrades to the team-member fallback.
  expect(screen.getByText("Team member")).toBeInTheDocument();
  expect(screen.getByText("Fan rattling")).toBeInTheDocument();
  // Count summary + money-free.
  expect(screen.getByText("2 notes")).toBeInTheDocument();
  expect(document.body.textContent).not.toMatch(/\$\d/);
});

test("self-hides when there are no notes today", async () => {
  mockedList.mockResolvedValue([]);
  const { container } = renderLogbook();
  await waitFor(() => expect(mockedList).toHaveBeenCalled());
  expect(container).toBeEmptyDOMElement();
});

test("self-hides when the read fails (non-manager 403)", async () => {
  mockedList.mockRejectedValue(new Error("403"));
  const { container } = renderLogbook();
  await waitFor(() => expect(mockedList).toHaveBeenCalled());
  expect(container).toBeEmptyDOMElement();
});
