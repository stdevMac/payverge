/** @jest-environment jsdom */
import { fireEvent, render, screen, waitFor } from "@testing-library/react";
import ResetPasswordClient from "./ResetPasswordClient";
import { authAPI } from "@/api/auth";

jest.mock("next/navigation", () => ({
  useSearchParams: () => new URLSearchParams("token=reset-token"),
}));
jest.mock("@/api/auth", () => ({
  authAPI: { resetPassword: jest.fn() },
}));
jest.mock("@/i18n/SimpleTranslationProvider", () => ({
  useSimpleLocale: () => ({ locale: "en" }),
  getTranslation: (key: string) => key.replace(/^resetPassword\./, ""),
}));
jest.mock("@/utils/apiError", () => ({
  getLocalizedApiError: () => "Localized reset error",
}));

describe("ResetPasswordClient password UX", () => {
  beforeEach(() => jest.clearAllMocks());

  it("focuses the first invalid password and keeps paste/reveal controls available", async () => {
    render(<ResetPasswordClient />);
    const password = screen.getByLabelText("request.newPasswordLabel");
    fireEvent.change(password, { target: { value: "short" } });
    fireEvent.click(screen.getByRole("button", { name: "request.submit" }));

    await waitFor(() => expect(password).toHaveFocus());
    expect(screen.getByRole("alert")).toHaveTextContent(
      "request.passwordTooShort",
    );
    expect(password).toHaveAttribute("autocomplete", "new-password");
    expect(
      screen.getAllByRole("button", { name: "request.showPassword" }),
    ).toHaveLength(1);
    expect(
      screen.queryByLabelText("request.confirmPasswordLabel"),
    ).not.toBeInTheDocument();
  });

  it("submits one strong password and maps server errors consistently", async () => {
    (authAPI.resetPassword as jest.Mock).mockRejectedValueOnce({
      response: { data: { code: "AUTH_TOKEN_INVALID" } },
    });
    render(<ResetPasswordClient />);
    const password = screen.getByLabelText("request.newPasswordLabel");
    fireEvent.change(password, { target: { value: "long-enough" } });
    fireEvent.click(screen.getByRole("button", { name: "request.submit" }));
    expect(await screen.findByRole("alert")).toHaveTextContent(
      "Localized reset error",
    );
    expect(authAPI.resetPassword).toHaveBeenCalledWith(
      "reset-token",
      "long-enough",
    );
  });
});
