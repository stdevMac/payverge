/** @jest-environment jsdom */
import { fireEvent, render, screen, waitFor } from "@testing-library/react";
import ForgotPasswordClient from "../ForgotPasswordClient";
import { authAPI } from "@/api/auth";

jest.mock("@/api/auth", () => ({
  authAPI: { requestPasswordReset: jest.fn().mockResolvedValue(undefined) },
}));

jest.mock("@/i18n/SimpleTranslationProvider", () => ({
  useSimpleLocale: () => ({ locale: "en" }),
  getTranslation: (key: string) => key,
}));

const dest = "/business/1/dashboard?tab=accounting&sub=invoices";

describe("ForgotPasswordClient redirect preservation (#403)", () => {
  it("keeps a safe deep-link on Back to login", () => {
    render(<ForgotPasswordClient redirectParam={dest} />);
    const back = screen.getByRole("link", { name: /request\.backToLogin/i });
    expect(back).toHaveAttribute(
      "href",
      `/dashboard?redirect=${encodeURIComponent(dest)}`,
    );
  });

  it("keeps that deep-link after the reset email is sent", async () => {
    render(<ForgotPasswordClient redirectParam={dest} />);
    fireEvent.change(screen.getByRole("textbox"), {
      target: { value: "owner@example.com" },
    });
    fireEvent.submit(screen.getByRole("textbox").closest("form") as HTMLFormElement);
    await waitFor(() => {
      expect(authAPI.requestPasswordReset).toHaveBeenCalled();
    });
    expect(
      screen.getByRole("link", { name: /success\.returnToLogin/i }),
    ).toHaveAttribute("href", `/dashboard?redirect=${encodeURIComponent(dest)}`);
  });

  it("ignores an external redirect and stays on /dashboard", () => {
    render(<ForgotPasswordClient redirectParam="https://evil.example" />);
    expect(screen.getByRole("link", { name: /request\.backToLogin/i })).toHaveAttribute(
      "href",
      "/dashboard",
    );
  });
});
