/** @jest-environment jsdom */
import { fireEvent, render, screen } from "@testing-library/react";
import ForgotPasswordClient from "../ForgotPasswordClient";
import { authAPI } from "@/api/auth";

jest.mock("@/api/auth", () => ({
  authAPI: { requestPasswordReset: jest.fn().mockResolvedValue(undefined) },
}));

jest.mock("@/i18n/SimpleTranslationProvider", () => ({
  useSimpleLocale: () => ({ locale: "es" }),
  getTranslation: (key: string) => key,
}));

describe("ForgotPasswordClient empty submit (#882)", () => {
  beforeEach(() => {
    (authAPI.requestPasswordReset as jest.Mock).mockClear();
  });

  it("does not hit the rate-limited reset endpoint when email is empty", () => {
    render(<ForgotPasswordClient />);
    const form = screen.getByRole("textbox").closest("form") as HTMLFormElement;
    fireEvent.submit(form);
    expect(authAPI.requestPasswordReset).not.toHaveBeenCalled();
    expect(screen.getByRole("alert")).toBeInTheDocument();
  });

  it("sends ES back-to-login to the sign-in dialog with the Spanish locale", () => {
    render(<ForgotPasswordClient />);
    expect(screen.getByRole("link", { name: /request\.backToLogin/i })).toHaveAttribute(
      "href",
      "/dashboard?auth=signin&lang=es",
    );
  });
});
