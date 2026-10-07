/** @jest-environment jsdom */
import React from "react";
import { render, screen, fireEvent, waitFor } from "@testing-library/react";
import EmailManagementPage from "./page";

const mockGet = jest.fn();
const mockPost = jest.fn();

jest.mock("@/api", () => ({
  axiosInstance: {
    get: (...args: unknown[]) => mockGet(...args),
    post: (...args: unknown[]) => mockPost(...args),
  },
}));

beforeEach(() => {
  jest.clearAllMocks();
  mockGet.mockResolvedValue({
    data: {
      businesses: [
        { id: 1, name: "A", owner_name: "OA", email: "a@x.com" },
        { id: 2, name: "B", owner_name: "OB", email: "b@x.com" },
        { id: 3, name: "C", owner_name: "OC", email: "c@x.com" },
      ],
      total: 3,
      with_email: 3,
      recipient_count: 3,
      sample: ["a@x.com", "b@x.com", "c@x.com"],
      kind: "real",
    },
  });
  mockPost.mockResolvedValue({
    data: { message: "Sent", success_count: 3, failed_count: 0, total: 3 },
  });
});

async function fillForm() {
  render(<EmailManagementPage />);
  // findBy* keeps its own retry budget so the on-mount business-count fetch
  // resolving late under full-suite worker contention doesn't trip the 5s
  // default (this suite passes in isolation; it only flaked in the full run).
  await screen.findByText(/3 deliverable recipients/i, undefined, {
    timeout: 10_000,
  });
  fireEvent.change(screen.getByPlaceholderText(/New Feature/i), {
    target: { value: "My Title" },
  });
  fireEvent.change(screen.getByPlaceholderText(/Brief introduction/i), {
    target: { value: "Intro" },
  });
  fireEvent.change(screen.getByPlaceholderText(/Detailed information/i), {
    target: { value: "Body" },
  });
}

describe("EmailManagementPage — recipient summary", () => {
  it("shows the resolved deliverable count for All Businesses", async () => {
    await fillForm();
    expect(screen.getByTestId("recipient-summary")).toHaveTextContent(
      /Will send to 3 recipients/i,
    );
  });

  it("shows a sample of resolved recipient addresses before send", async () => {
    await fillForm();
    const sample = screen.getByTestId("recipient-sample");
    expect(sample).toHaveTextContent(/a@x\.com/);
    expect(sample).toHaveTextContent(/b@x\.com/);
    expect(sample).toHaveTextContent(/c@x\.com/);
  });
});

describe("EmailManagementPage — confirmation before broadcast", () => {
  it("does not POST on the first click — it opens a confirmation dialog", async () => {
    await fillForm();

    const sendBtn = screen.getByRole("button", {
      name: /Send Email Broadcast/i,
    });
    fireEvent.click(sendBtn);

    // No request fired yet.
    await Promise.resolve();
    expect(mockPost).not.toHaveBeenCalled();

    // A confirmation dialog with a Confirm action appears.
    expect(
      await screen.findByRole("button", { name: /Confirm.*Send|Send to/i }),
    ).toBeInTheDocument();
  });

  it("echoes recipient count and sample in the confirm step", async () => {
    await fillForm();

    fireEvent.click(
      screen.getByRole("button", { name: /Send Email Broadcast/i }),
    );

    expect(
      await screen.findByTestId("confirm-recipient-count"),
    ).toHaveTextContent(/3 recipients/i);
    expect(screen.getByTestId("confirm-recipient-sample")).toHaveTextContent(
      /a@x\.com/,
    );
    expect(
      screen.getByRole("button", { name: /Confirm & Send to 3/i }),
    ).toBeInTheDocument();
  });

  it("POSTs only after confirming in the dialog", async () => {
    await fillForm();

    fireEvent.click(
      screen.getByRole("button", { name: /Send Email Broadcast/i }),
    );

    const confirm = await screen.findByRole("button", {
      name: /Confirm.*Send|Send to/i,
    });
    fireEvent.click(confirm);

    await waitFor(() => expect(mockPost).toHaveBeenCalledTimes(1));
    expect(mockPost).toHaveBeenCalledWith(
      "/admin/emails/operational-update",
      expect.objectContaining({
        update_title: "My Title",
        recipients: ["all_businesses"],
      }),
    );
  });
});
