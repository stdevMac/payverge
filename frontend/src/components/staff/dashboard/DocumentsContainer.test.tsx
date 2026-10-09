/** @jest-environment jsdom */
import React from "react";
import { render, screen, waitFor } from "@testing-library/react";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import DocumentsContainer from "./DocumentsContainer";
import { documentsApi } from "@/api/engagement";

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
  documentsApi: { list: jest.fn(), ack: jest.fn() },
}));

const mockedDocuments = documentsApi as unknown as { list: jest.Mock; ack: jest.Mock };

const docs = [
  {
    id: 11, business_id: 42, title: "Employee handbook", url: "", content: "Be on time.",
    version: 2, require_ack: true, audience_filter: "all", created_at: "2026-06-30T00:00:00Z",
  },
  {
    id: 12, business_id: 42, title: "Allergen policy", url: "", content: "Wash hands.",
    version: 1, require_ack: true, audience_filter: "all", created_at: "2026-06-29T00:00:00Z",
  },
];

function renderContainer() {
  const qc = new QueryClient({ defaultOptions: { queries: { retry: false } } });
  return render(
    <QueryClientProvider client={qc}>
      <DocumentsContainer businessId="42" />
    </QueryClientProvider>,
  );
}

beforeEach(() => {
  jest.clearAllMocks();
});

describe("DocumentsContainer", () => {
  it("consumes the { documents, acked } shape, gating the Ack button on the server acked map across reload", async () => {
    // doc 11 already acknowledged server-side (current version); doc 12 not yet.
    mockedDocuments.list.mockResolvedValue({ documents: docs, acked: { "11": true } });
    renderContainer();

    await waitFor(() => expect(screen.getByText("Employee handbook")).toBeInTheDocument());
    expect(screen.getByText("Allergen policy")).toBeInTheDocument();

    // Exactly one Acknowledge button remains — doc 12; doc 11 is hidden by the
    // server acked map even though no client-session ack happened this load.
    const ackButtons = screen.getAllByRole("button", {
      name: "staffEngagement.documents.acknowledge",
    });
    expect(ackButtons).toHaveLength(1);
    // doc 11 shows the acknowledged chip.
    expect(screen.getAllByText("staffEngagement.documents.acknowledged").length).toBeGreaterThan(0);

    expect(document.body.textContent).not.toMatch(/\$\d/);
  });

  it("shows the Ack button for every require_ack doc when the server acked map is empty", async () => {
    mockedDocuments.list.mockResolvedValue({ documents: docs, acked: {} });
    renderContainer();

    await waitFor(() => expect(screen.getByText("Employee handbook")).toBeInTheDocument());
    const ackButtons = screen.getAllByRole("button", {
      name: "staffEngagement.documents.acknowledge",
    });
    expect(ackButtons).toHaveLength(2);
    expect(document.body.textContent).not.toMatch(/\$\d/);
  });
});
