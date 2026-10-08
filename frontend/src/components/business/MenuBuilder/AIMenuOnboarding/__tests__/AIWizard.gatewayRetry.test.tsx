/** @jest-environment jsdom */
/**
 * #598 — wizard replies routinely take ~150s and can outlive the proxy
 * (Caddy/Cloudflare 502s without CORS headers surface in the browser as
 * network errors) or the request timeout. The backend persists the user
 * message either way and `retry: true` recovers the pending reply.
 *
 * These tests pin the recovery UX:
 *  - a gateway 5xx / network-shaped failure on send shows the "reply is still
 *    being prepared" copy WITH the retry affordance (not a dead-end error);
 *  - clicking retry sends `{ retry: true }` with an empty message and appends
 *    the recovered assistant reply;
 *  - a retry that itself hits the gateway keeps the delayed-reply copy and
 *    the retry button;
 *  - structured-output failures keep their existing copy;
 *  - a failed generation keeps the existing generateRetry copy + button.
 *
 * Deliberately does NOT mock @/utils/apiError: the classification of
 * sanitized axios errors (status / code / response shape) is the behavior
 * under test.
 */

import React from "react";
import { act, fireEvent, render, screen, waitFor } from "@testing-library/react";

jest.mock("@/i18n/SimpleTranslationProvider", () => {
  const TABLE: Record<string, string> = {
    "aiMenuOnboarding.wizard.retryResponse": "Retry this response",
    "aiMenuOnboarding.wizard.generateMenu": "Generate Menu",
    "aiMenuOnboarding.wizard.errors.structuredOutput":
      "The AI response was incomplete.",
    "aiMenuOnboarding.wizard.errors.replyDelayed":
      "This reply is taking longer than usual. Retry to fetch it.",
    "aiMenuOnboarding.wizard.errors.sendMessage": "Failed to send message",
    "aiMenuOnboarding.wizard.errors.generateRetry":
      "Generation failed. Tap Generate Menu to try again.",
  };
  return {
    useSimpleLocale: () => ({ locale: "en", setLocale: jest.fn() }),
    getTranslation: (key: string) => TABLE[key] ?? key,
  };
});

jest.mock("@/api/business", () => ({
  startWizardSession: jest.fn(),
  sendWizardMessage: jest.fn(),
  generateMenuFromWizard: jest.fn(),
}));

import AIWizard from "../AIWizard";
import {
  startWizardSession,
  sendWizardMessage,
  generateMenuFromWizard,
} from "@/api/business";

const mockStart = startWizardSession as jest.Mock;
const mockSend = sendWizardMessage as jest.Mock;
const mockGenerate = generateMenuFromWizard as jest.Mock;

// Sanitized-error shapes as produced by api/tools/instance.ts toSanitizedError
// (sanitizeError swaps the raw axios message for generic product copy and
// keeps status / code / response.{status,data}).
function gateway502Error(): Error {
  return Object.assign(
    new Error("Something went wrong on our end. Please try again later."),
    {
      status: 502,
      response: { status: 502, data: "<html>502 Bad Gateway</html>" },
    },
  );
}

// Timeout / CORS-masked proxy error: axios got no response at all; only a
// transport code survives sanitization.
function networkError(): Error {
  return Object.assign(
    new Error(
      "Unable to connect to the server. Please check your internet connection and try again.",
    ),
    { code: "ECONNABORTED" },
  );
}

function structuredOutputError(): Error {
  return Object.assign(new Error("Request failed with status code 502"), {
    status: 502,
    code: "ai_structured_output_invalid",
    response: {
      status: 502,
      data: { code: "ai_structured_output_invalid", error: "bad JSON" },
    },
  });
}

async function arriveAtChatting() {
  mockStart.mockResolvedValue({
    session_id: 7,
    response: { message: "Hi! What cuisine?", suggested_options: [] },
  });
  const onMenuGenerated = jest.fn();
  render(
    <AIWizard
      businessId={42}
      onMenuGenerated={onMenuGenerated}
    />,
  );
  fireEvent.click(
    await screen.findByText("aiMenuOnboarding.wizard.startConversation"),
  );
  const input = await screen.findByPlaceholderText(
    "aiMenuOnboarding.wizard.inputPlaceholder",
  );
  return { input, onMenuGenerated };
}

async function sendMessage(input: HTMLElement, text: string) {
  await act(async () => {
    fireEvent.change(input, { target: { value: text } });
    fireEvent.keyDown(input, { key: "Enter" });
  });
}

describe("AIWizard gateway-failure retry (#598)", () => {
  const originalScrollTo = (
    window.HTMLElement.prototype as unknown as { scrollTo?: () => void }
  ).scrollTo;

  beforeEach(() => {
    jest.clearAllMocks();
    jest.spyOn(console, "error").mockImplementation(() => {});
    (
      window.HTMLElement.prototype as unknown as { scrollTo: () => void }
    ).scrollTo = jest.fn();
  });

  afterEach(() => {
    (console.error as jest.Mock).mockRestore?.();
    if (originalScrollTo === undefined) {
      delete (
        window.HTMLElement.prototype as unknown as { scrollTo?: () => void }
      ).scrollTo;
    } else {
      (
        window.HTMLElement.prototype as unknown as { scrollTo?: () => void }
      ).scrollTo = originalScrollTo;
    }
  });

  it("shows the delayed-reply copy and a retry affordance on a proxy 502 (no structured code)", async () => {
    const { input } = await arriveAtChatting();
    mockSend.mockRejectedValueOnce(gateway502Error());

    await sendMessage(input, "Six pasta dishes");

    // Recovery copy — NOT the generic send failure and NOT the raw axios text.
    expect(
      await screen.findByText(
        "This reply is taking longer than usual. Retry to fetch it.",
      ),
    ).toBeInTheDocument();
    expect(
      screen.queryByText("Failed to send message"),
    ).not.toBeInTheDocument();
    // Retry affordance is present (previously reserved for structured-output).
    expect(
      screen.getByRole("button", { name: "Retry this response" }),
    ).toBeInTheDocument();
    // The user's turn is kept — no re-typing needed.
    expect(screen.getAllByText("Six pasta dishes")).toHaveLength(1);
  });

  it("shows the delayed-reply retry affordance on a timeout/CORS-masked network error", async () => {
    const { input } = await arriveAtChatting();
    mockSend.mockRejectedValueOnce(networkError());

    await sendMessage(input, "Vegan brunch menu");

    expect(
      await screen.findByText(
        "This reply is taking longer than usual. Retry to fetch it.",
      ),
    ).toBeInTheDocument();
    expect(
      screen.getByRole("button", { name: "Retry this response" }),
    ).toBeInTheDocument();
  });

  it("retry after a gateway failure sends {retry: true} and appends the recovered reply", async () => {
    const { input } = await arriveAtChatting();
    mockSend
      .mockRejectedValueOnce(gateway502Error())
      .mockResolvedValueOnce({
        response: {
          message: "Great — here is your pasta lineup.",
          is_complete: false,
          suggested_options: [],
        },
      });

    await sendMessage(input, "Six pasta dishes");
    fireEvent.click(
      await screen.findByRole("button", { name: "Retry this response" }),
    );

    expect(
      await screen.findByText("Great — here is your pasta lineup."),
    ).toBeInTheDocument();
    // Empty message + retry flag: fetches the pending reply, does not re-send.
    expect(mockSend).toHaveBeenLastCalledWith(42, 7, "", "en", { retry: true });
    // No duplicated user turn, and the error bubble is gone.
    expect(screen.getAllByText("Six pasta dishes")).toHaveLength(1);
    expect(
      screen.queryByRole("button", { name: "Retry this response" }),
    ).not.toBeInTheDocument();
  });

  it("keeps the delayed-reply copy and retry button when the retry itself hits the gateway", async () => {
    const { input } = await arriveAtChatting();
    mockSend
      .mockRejectedValueOnce(gateway502Error())
      .mockRejectedValueOnce(networkError());

    await sendMessage(input, "Six pasta dishes");
    fireEvent.click(
      await screen.findByRole("button", { name: "Retry this response" }),
    );

    await waitFor(() =>
      expect(
        screen.getByText(
          "This reply is taking longer than usual. Retry to fetch it.",
        ),
      ).toBeInTheDocument(),
    );
    // Still retryable — not converted into the structured-output copy.
    expect(
      screen.getByRole("button", { name: "Retry this response" }),
    ).toBeInTheDocument();
    expect(
      screen.queryByText("The AI response was incomplete."),
    ).not.toBeInTheDocument();
  });

  it("structured-output failures keep their existing copy (not the gateway copy)", async () => {
    const { input } = await arriveAtChatting();
    mockSend.mockRejectedValueOnce(structuredOutputError());

    await sendMessage(input, "Six pasta dishes");

    expect(
      await screen.findByText("The AI response was incomplete."),
    ).toBeInTheDocument();
    expect(
      screen.queryByText(
        "This reply is taking longer than usual. Retry to fetch it.",
      ),
    ).not.toBeInTheDocument();
    expect(
      screen.getByRole("button", { name: "Retry this response" }),
    ).toBeInTheDocument();
  });

  it("a gateway failure during generation keeps the existing retry-copy behavior (Generate Menu stays; no message-level retry)", async () => {
    const { input } = await arriveAtChatting();
    mockSend.mockResolvedValueOnce({
      response: { message: "All set?", is_complete: true, suggested_options: [] },
    });
    await sendMessage(input, "That is everything");

    const genBtn = await screen.findByText("Generate Menu");
    mockGenerate.mockRejectedValueOnce(gateway502Error());
    await act(async () => {
      fireEvent.click(genBtn);
    });

    // Existing behavior: errMessage-first copy (sanitized server text), with
    // errors.generateRetry as the fallback when the message is empty.
    await waitFor(() =>
      expect(
        screen.getByText(
          "Something went wrong on our end. Please try again later.",
        ),
      ).toBeInTheDocument(),
    );
    // Generate Menu stays available; no message-level retry button appears
    // and the delayed-reply copy is reserved for the message path.
    expect(screen.getByText("Generate Menu")).toBeInTheDocument();
    expect(
      screen.queryByRole("button", { name: "Retry this response" }),
    ).not.toBeInTheDocument();
    expect(
      screen.queryByText(
        "This reply is taking longer than usual. Retry to fetch it.",
      ),
    ).not.toBeInTheDocument();

    // And the empty-message fallback still lands on the generateRetry copy.
    mockGenerate.mockRejectedValueOnce(
      Object.assign(new Error(""), {
        status: 502,
        response: { status: 502, data: "<html>502</html>" },
      }),
    );
    await act(async () => {
      fireEvent.click(screen.getByText("Generate Menu"));
    });
    await waitFor(() =>
      expect(
        screen.getByText("Generation failed. Tap Generate Menu to try again."),
      ).toBeInTheDocument(),
    );
    expect(screen.getByText("Generate Menu")).toBeInTheDocument();
  });
});
