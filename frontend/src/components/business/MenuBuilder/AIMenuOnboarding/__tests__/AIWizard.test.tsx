/** @jest-environment jsdom */
/**
 * L8 AIWizard findings:
 *  - F10: a Cancel control during generation returns to the chat (retry-able)
 *         and discards the in-flight result.
 *  - F11: a failed generation surfaces retry-specific copy while keeping the
 *         Generate Menu button available.
 *  - F12: the generation step narration actually rotates (state-backed timer).
 */

import React from "react";
import {
  act,
  fireEvent,
  render,
  screen,
  waitFor,
} from "@testing-library/react";

// Resolve the rotating-step keys + cancel/error keys to human strings; return
// the key for everything else.
jest.mock("@/i18n/SimpleTranslationProvider", () => {
  const TABLE: Record<string, string> = {
    "aiMenuOnboarding.wizard.generating.steps.analyzing": "Analyzing",
    "aiMenuOnboarding.wizard.generating.steps.crafting": "Crafting",
    "aiMenuOnboarding.wizard.generating.steps.pricing": "Pricing",
    "aiMenuOnboarding.wizard.generating.steps.finalizing": "Finalizing",
    "aiMenuOnboarding.wizard.generating.cancel": "Cancel generation",
    "aiMenuOnboarding.wizard.generateMenu": "Generate Menu",
    "aiMenuOnboarding.wizard.retryResponse": "Retry this response",
    "aiMenuOnboarding.wizard.errors.structuredOutput":
      "The AI response was incomplete.",
    "aiMenuOnboarding.wizard.errors.generationCancelled":
      "Generation cancelled. Tap Generate Menu to try again.",
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

jest.mock("@/utils/apiError", () => ({
  errMessage: () => "",
  getApiErrorCode: (err: { response?: { data?: { code?: string } } }) =>
    err?.response?.data?.code,
  getApiErrorStatus: (err: { response?: { status?: number }; status?: number }) =>
    err?.response?.status ?? err?.status,
  isApiNetworkError: (err: { code?: string; response?: unknown }) =>
    err != null &&
    typeof err === "object" &&
    ("code" in err || "status" in err || "response" in err) &&
    (err as { response?: unknown }).response == null,
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

// Drive the wizard from idle → chatting → isComplete so the Generate Menu
// button renders. Returns the Generate Menu button + the onMenuGenerated spy.
async function arriveAtComplete() {
  mockStart.mockResolvedValue({
    session_id: 1,
    response: { message: "Hi", suggested_options: [] },
  });
  mockSend.mockResolvedValue({
    response: { message: "All set?", suggested_options: [], is_complete: true },
  });
  const onMenuGenerated = jest.fn();

  render(
    <AIWizard
      businessId={42}
      onMenuGenerated={onMenuGenerated}
    />,
  );

  const startBtn = await screen.findByText(
    "aiMenuOnboarding.wizard.startConversation",
  );
  await act(async () => {
    fireEvent.click(startBtn);
  });

  const input = await screen.findByPlaceholderText(
    "aiMenuOnboarding.wizard.inputPlaceholder",
  );
  await act(async () => {
    fireEvent.change(input, { target: { value: "italian" } });
    fireEvent.keyDown(input, { key: "Enter" });
  });

  // Generate Menu button appears once is_complete=true.
  const genBtn = await screen.findByText("Generate Menu");
  return { genBtn, onMenuGenerated };
}

describe("AIWizard generation (L8)", () => {
  const originalScrollTo = (
    window.HTMLElement.prototype as unknown as { scrollTo?: () => void }
  ).scrollTo;

  beforeEach(() => {
    jest.clearAllMocks();
    jest.spyOn(console, "error").mockImplementation(() => {});
    // jsdom lacks Element.scrollTo, which the wizard's chat auto-scroll uses.
    (
      window.HTMLElement.prototype as unknown as { scrollTo: () => void }
    ).scrollTo = jest.fn();
  });

  afterEach(() => {
    (console.error as jest.Mock).mockRestore?.();
    jest.useRealTimers();
    // Restore so the patched scrollTo doesn't leak across files in this worker.
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

  it("F10: Cancel during generation returns to chat with retry-able copy and discards the stale result", async () => {
    const { genBtn, onMenuGenerated } = await arriveAtComplete();

    // A generation call that never resolves until we let it.
    let resolveGen: (v: unknown) => void = () => {};
    mockGenerate.mockReturnValue(
      new Promise((res) => {
        resolveGen = res;
      }),
    );

    // Re-render not needed; click generate (button is in the tree).
    await act(async () => {
      fireEvent.click(genBtn);
    });
    // Cancel control is visible during generation (same block F12 reads).
    const cancelBtn = screen.getByText("Cancel generation");
    await act(async () => {
      fireEvent.click(cancelBtn);
    });

    // Retry-able copy shows and the chat input is back (step === chatting),
    // so the operator is no longer trapped on the indeterminate bar.
    expect(
      screen.getByText("Generation cancelled. Tap Generate Menu to try again."),
    ).toBeInTheDocument();
    expect(
      screen.getByPlaceholderText("aiMenuOnboarding.wizard.inputPlaceholder"),
    ).toBeInTheDocument();

    // The late resolution must NOT advance to the menu (stale id discarded):
    // onMenuGenerated was never called and the Generate Menu button stays.
    await act(async () => {
      resolveGen({ menu: { categories: [] } });
      await Promise.resolve();
    });
    expect(onMenuGenerated).not.toHaveBeenCalled();
    expect(screen.getByText("Generate Menu")).toBeInTheDocument();
  });

  it("F11: a failed generation shows retry-specific copy and keeps the Generate Menu button", async () => {
    const { genBtn } = await arriveAtComplete();
    mockGenerate.mockRejectedValue(new Error("boom"));

    await act(async () => {
      fireEvent.click(genBtn);
    });

    await waitFor(() =>
      expect(
        screen.getByText("Generation failed. Tap Generate Menu to try again."),
      ).toBeInTheDocument(),
    );
    expect(screen.getByText("Generate Menu")).toBeInTheDocument();
  });

  it("F12: the generation step narration rotates over time", async () => {
    jest.useFakeTimers();
    const { genBtn } = await arriveAtComplete();
    mockGenerate.mockReturnValue(new Promise(() => {})); // never resolves

    await act(async () => {
      fireEvent.click(genBtn);
    });

    // Index starts at 0 → "Analyzing".
    expect(screen.getByText("Analyzing")).toBeInTheDocument();

    // Advance one tick (2s) → index 1 → "Crafting".
    await act(async () => {
      jest.advanceTimersByTime(2000);
    });
    expect(screen.getByText("Crafting")).toBeInTheDocument();

    await act(async () => {
      jest.advanceTimersByTime(2000);
    });
    expect(screen.getByText("Pricing")).toBeInTheDocument();
  });

  it("keeps the failed user turn and retries without duplicating it", async () => {
    mockStart.mockResolvedValue({
      session_id: 1,
      response: { message: "Hi", suggested_options: [] },
    });
    mockSend
      .mockRejectedValueOnce({
        response: { data: { code: "ai_structured_output_invalid" } },
      })
      .mockResolvedValueOnce({
        response: {
          message: "Which price range?",
          is_complete: false,
          suggested_options: ["Budget"],
        },
      });
    render(
      <AIWizard
        businessId={42}
        onMenuGenerated={jest.fn()}
      />,
    );
    fireEvent.click(
      await screen.findByText("aiMenuOnboarding.wizard.startConversation"),
    );
    const input = await screen.findByPlaceholderText(
      "aiMenuOnboarding.wizard.inputPlaceholder",
    );
    fireEvent.change(input, { target: { value: "Six dishes" } });
    fireEvent.keyDown(input, { key: "Enter" });
    expect(
      await screen.findByRole("button", { name: "Retry this response" }),
    ).toBeInTheDocument();
    expect(screen.getAllByText("Six dishes")).toHaveLength(1);
    fireEvent.click(
      screen.getByRole("button", { name: "Retry this response" }),
    );
    expect(await screen.findByText("Which price range?")).toBeInTheDocument();
    expect(mockSend).toHaveBeenLastCalledWith(42, 1, "", "en", { retry: true });
    expect(screen.getAllByText("Six dishes")).toHaveLength(1);
  });

  it("renders suggested options as semantic buttons", async () => {
    mockStart.mockResolvedValue({
      session_id: 1,
      response: { message: "Choose", suggested_options: ["Budget"] },
    });
    render(
      <AIWizard
        businessId={42}
        onMenuGenerated={jest.fn()}
      />,
    );
    fireEvent.click(
      await screen.findByText("aiMenuOnboarding.wizard.startConversation"),
    );
    const option = await screen.findByRole("button", { name: "Budget" });
    expect(option.tagName).toBe("BUTTON");
  });
});
