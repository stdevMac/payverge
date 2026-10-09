/** @jest-environment jsdom */
import React from "react";
import { render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";

const mockCreateAiWaiterSession = jest.fn();
const mockCreateAiWaiterTestSession = jest.fn();
const mockSendAiWaiterTestChat = jest.fn();
const mockParseAiWaiterChatResponse = jest.fn();

jest.mock("@/api/aiWaiter", () => ({
  createAiWaiterSession: (...args: unknown[]) =>
    mockCreateAiWaiterSession(...args),
  createAiWaiterTestSession: (...args: unknown[]) =>
    mockCreateAiWaiterTestSession(...args),
  sendAiWaiterTestChat: (...args: unknown[]) =>
    mockSendAiWaiterTestChat(...args),
  parseAiWaiterChatResponse: (...args: unknown[]) =>
    mockParseAiWaiterChatResponse(...args),
}));

import AiWaiterTestChat from "../AiWaiterTestChat";

describe("AiWaiterTestChat isolation", () => {
  beforeEach(() => {
    jest.clearAllMocks();
    mockCreateAiWaiterTestSession.mockResolvedValue({
      session_token: "optest-token",
      greeting: "Sandbox greeting",
      expires_at: "2026-08-12T00:00:00Z",
    });
    mockSendAiWaiterTestChat.mockResolvedValue({
      role: "model",
      parts: [{ text: "No peanut dishes tonight." }],
    });
    mockParseAiWaiterChatResponse.mockReturnValue({
      role: "model",
      parts: [{ text: "No peanut dishes tonight." }],
      response_v2: {
        answer: { content: "No peanut dishes tonight." },
      },
    });
  });

  it("keeps the first typed message after creating the sandbox session", async () => {
    const user = userEvent.setup();
    const t = (key: string) => key;
    render(<AiWaiterTestChat businessId={42} aiEnabled language="es-AR" t={t} />);

    const input = screen.getByRole("textbox");
    await user.type(input, "86 the ribs");
    await user.click(screen.getByRole("button", { name: "settings.testChat.send" }));

    await waitFor(() => {
      expect(mockSendAiWaiterTestChat).toHaveBeenCalled();
    });
    expect(screen.getByText(/86 the ribs/)).toBeInTheDocument();
    expect(screen.getByText(/Sandbox greeting/)).toBeInTheDocument();
    expect(screen.getByText(/No peanut dishes tonight/)).toBeInTheDocument();
  });

  it("uses operator sandbox APIs and never calls guest createAiWaiterSession", async () => {
    const user = userEvent.setup();
    const t = (key: string) => key;
    render(<AiWaiterTestChat businessId={42} aiEnabled language="en" t={t} />);

    await user.click(
      screen.getByRole("button", {
        name: "settings.testChat.probeAllergen",
      }),
    );

    await waitFor(() => {
      expect(mockCreateAiWaiterTestSession).toHaveBeenCalledWith(42, {
        language: "en",
      });
    });
    expect(mockCreateAiWaiterSession).not.toHaveBeenCalled();
    await waitFor(() => {
      expect(mockSendAiWaiterTestChat).toHaveBeenCalled();
    });
    const chatArgs = mockSendAiWaiterTestChat.mock.calls[0];
    expect(chatArgs[0]).toBe(42);
    expect(chatArgs[1].session_token).toBe("optest-token");
    expect(chatArgs[1].language).toBe("en");
  });

  it.each([
    ["en", "I have a nut allergy — what can I safely order?"],
    [
      "es",
      "Tengo alergia a los frutos secos — ¿qué puedo pedir con seguridad?",
    ],
    [
      "es-AR",
      "Tengo alergia a los frutos secos — ¿qué puedo pedir con seguridad?",
    ],
  ] as const)(
    "sends the %s allergen preset through the same explicit locale as guest chat",
    async (language, probeText) => {
      const user = userEvent.setup();
      const t = (key: string) =>
        key === "settings.testChat.probeAllergenText" ? probeText : key;
      render(
        <AiWaiterTestChat
          businessId={42}
          aiEnabled
          language={language}
          t={t}
        />,
      );

      await user.click(
        screen.getByRole("button", {
          name: "settings.testChat.probeAllergen",
        }),
      );

      await waitFor(() => {
        expect(mockCreateAiWaiterTestSession).toHaveBeenCalledWith(42, {
          language,
        });
      });
      await waitFor(() => {
        expect(mockSendAiWaiterTestChat).toHaveBeenCalled();
      });
      expect(mockSendAiWaiterTestChat.mock.calls[0][1]).toEqual(
        expect.objectContaining({
          language,
          session_token: "optest-token",
          history: expect.arrayContaining([
            expect.objectContaining({ role: "user", content: probeText }),
          ]),
        }),
      );
    },
  );

  it.each([
    ["en", "What do you recommend tonight?"],
    ["es", "¿Qué me recomiendas esta noche?"],
    ["es-AR", "¿Qué me recomendás esta noche?"],
  ] as const)(
    "sends the %s recommendation preset through the same explicit locale as guest chat",
    async (language, probeText) => {
      const user = userEvent.setup();
      const t = (key: string) =>
        key === "settings.testChat.probeRecommendText" ? probeText : key;
      render(
        <AiWaiterTestChat
          businessId={42}
          aiEnabled
          language={language}
          t={t}
        />,
      );

      await user.click(
        screen.getByRole("button", {
          name: "settings.testChat.probeRecommend",
        }),
      );

      await waitFor(() => {
        expect(mockCreateAiWaiterTestSession).toHaveBeenCalledWith(42, {
          language,
        });
      });
      expect(mockSendAiWaiterTestChat.mock.calls[0][1]).toEqual(
        expect.objectContaining({
          language,
          history: expect.arrayContaining([
            expect.objectContaining({ role: "user", content: probeText }),
          ]),
        }),
      );
    },
  );

  it("recreates the sandbox session when the owner locale changes", async () => {
    const user = userEvent.setup();
    const t = (key: string) => key;
    const { rerender } = render(
      <AiWaiterTestChat businessId={42} aiEnabled language="en" t={t} />,
    );
    await user.click(
      screen.getByRole("button", { name: "settings.testChat.probeAllergen" }),
    );
    await waitFor(() =>
      expect(mockCreateAiWaiterTestSession).toHaveBeenCalledWith(42, {
        language: "en",
      }),
    );

    mockCreateAiWaiterTestSession.mockResolvedValueOnce({
      session_token: "optest-es-ar",
      greeting: "Hola",
      expires_at: "2026-08-12T00:00:00Z",
    });
    rerender(
      <AiWaiterTestChat businessId={42} aiEnabled language="es-AR" t={t} />,
    );
    await user.click(
      screen.getByRole("button", { name: "settings.testChat.probeRecommend" }),
    );
    await waitFor(() =>
      expect(mockCreateAiWaiterTestSession).toHaveBeenCalledWith(42, {
        language: "es-AR",
      }),
    );
    expect(mockSendAiWaiterTestChat.mock.calls.at(-1)?.[1].session_token).toBe(
      "optest-es-ar",
    );
    expect(mockSendAiWaiterTestChat.mock.calls.at(-1)?.[1].language).toBe(
      "es-AR",
    );
  });
});
