/** @jest-environment jsdom */
import React from "react";
import { render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { axiosInstance } from "@/api/tools/instance";
import AiWaiterTestChat from "../AiWaiterTestChat";

jest.mock("@/api/tools/instance", () => ({
  axiosInstance: { post: jest.fn(), get: jest.fn() },
}));

const mockedPost = axiosInstance.post as jest.Mock;

const t = (key: string) => {
  if (key === "settings.testChat.send") return "Enviar";
  if (key === "settings.testChat.you") return "Tú";
  if (key === "settings.testChat.assistant") return "IA";
  if (key === "settings.testChat.inputLabel") return "Mensaje de prueba";
  return key;
};

describe("AiWaiterTestChat first Enviar", () => {
  beforeEach(() => {
    jest.clearAllMocks();
    mockedPost.mockImplementation(async (url: string) => {
      if (String(url).endsWith("/ai/test-chat/session")) {
        return {
          data: {
            session_token: "optest-live",
            greeting: "Hola, soy Sage.",
            expires_at: "2026-08-20T00:00:00Z",
          },
        };
      }
      if (String(url).endsWith("/ai/test-chat")) {
        return {
          data: {
            role: "model",
            parts: [{ text: "Ahora mismo no puedo listar mesas libres." }],
          },
        };
      }
      throw new Error(`unexpected url ${url}`);
    });
  });

  it("posts the typed Chat de prueba turn on the first Enviar and renders a bubble", async () => {
    const user = userEvent.setup();
    render(
      <AiWaiterTestChat
        businessId={42}
        aiEnabled
        language="es-AR"
        t={t}
      />,
    );

    const input = screen.getByRole("textbox", { name: "Mensaje de prueba" });
    await user.type(input, "qué mesas están libres?");
    await user.click(screen.getByRole("button", { name: "Enviar" }));

    await waitFor(() => {
      expect(mockedPost).toHaveBeenCalledWith(
        "/inside/businesses/42/ai/test-chat",
        expect.objectContaining({
          language: "es-AR",
          session_token: "optest-live",
          mode: "concierge",
          history: expect.arrayContaining([
            expect.objectContaining({
              role: "user",
              content: "qué mesas están libres?",
            }),
          ]),
        }),
        expect.anything(),
      );
    });
    expect(screen.getByText(/qué mesas están libres\?/)).toBeInTheDocument();
    expect(
      screen.getByText(/Ahora mismo no puedo listar mesas libres/),
    ).toBeInTheDocument();
    expect(
      (screen.getByRole("textbox", { name: "Mensaje de prueba" }) as HTMLInputElement)
        .value,
    ).toBe("");
  });

  it("sends a visible native-input turn even when React draft is still empty (#676)", async () => {
    const user = userEvent.setup();
    render(
      <AiWaiterTestChat
        businessId={42}
        aiEnabled
        language="es-AR"
        t={t}
      />,
    );

    const input = screen.getByRole("textbox", {
      name: "Mensaje de prueba",
    }) as HTMLInputElement;
    const nativeSetter = Object.getOwnPropertyDescriptor(
      window.HTMLInputElement.prototype,
      "value",
    )?.set;
    nativeSetter?.call(input, "qué mesas están libres?");
    expect(input.value).toBe("qué mesas están libres?");

    const send = screen.getByRole("button", { name: "Enviar" });
    expect(send).not.toBeDisabled();
    await user.click(send);

    await waitFor(() => {
      expect(mockedPost).toHaveBeenCalledWith(
        "/inside/businesses/42/ai/test-chat",
        expect.objectContaining({
          language: "es-AR",
          session_token: "optest-live",
          history: expect.arrayContaining([
            expect.objectContaining({
              role: "user",
              content: "qué mesas están libres?",
            }),
          ]),
        }),
        expect.anything(),
      );
    });
    expect(screen.getByText(/qué mesas están libres\?/)).toBeInTheDocument();
  });
});
