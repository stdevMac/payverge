/** @jest-environment jsdom */
import { render, screen } from "@testing-library/react";

jest.mock("next/navigation", () => ({
  useSearchParams: () => new URLSearchParams("token=invalid-qa"),
}));

jest.mock("@/i18n/SimpleTranslationProvider", () => ({
  useSimpleLocale: () => ({ locale: "es" }),
  getTranslation: (key: string) => {
    const map: Record<string, string> = {
      "verifyEmail.messages.failed": "No se pudo verificar el correo.",
      "verifyEmail.resend.emailLabel": "Correo electrónico",
      "verifyEmail.resend.emailPlaceholder": "tu@ejemplo.com",
      "verifyEmail.resend.submit": "Reenviar correo de verificación",
      "verifyEmail.resend.prompt": "¿No recibiste el correo?",
      "verifyEmail.headings.error": "No se pudo verificar",
      "verifyEmail.labels.error": "Error",
    };
    return map[key] ?? key;
  },
}));

jest.mock("@/api/auth", () => ({
  authAPI: {
    verifyEmail: jest.fn(),
    resendVerification: jest.fn(),
  },
}));

jest.mock("@/providers/HybridAuthProvider", () => ({
  useAuth: () => ({ refreshSession: jest.fn().mockResolvedValue(undefined) }),
}));

import { authAPI } from "@/api/auth";
import VerifyEmailClient from "./VerifyEmailClient";

describe("VerifyEmailClient invalid token (#457)", () => {
  it("shows localized failure copy and names the email field by purpose", async () => {
    (authAPI.verifyEmail as jest.Mock).mockRejectedValue({
      status: 400,
      message: "Please check the form and try again.",
      response: {
        status: 400,
        data: {
          error: "Please check the form and try again.",
          code: "invalid_token",
        },
      },
    });

    render(<VerifyEmailClient />);

    expect(
      await screen.findByText("No se pudo verificar el correo."),
    ).toBeInTheDocument();
    expect(
      screen.queryByText("Please check the form and try again."),
    ).not.toBeInTheDocument();
    expect(
      screen.getByRole("textbox", { name: /correo electrónico/i }),
    ).toBeInTheDocument();
  });
});
