/** @jest-environment jsdom */
import { act, fireEvent, render, screen } from "@testing-library/react";
import ForgotPasswordClient from "../ForgotPasswordClient";
import { parseInstanceInfo } from "@/lib/instance/instanceInfo";
import {
  resetInstanceCacheForTests,
  setInstanceForTests,
} from "@/hooks/useInstance";

jest.mock("@/api/auth", () => ({
  authAPI: { requestPasswordReset: jest.fn().mockResolvedValue(undefined) },
}));

jest.mock("@/i18n/SimpleTranslationProvider", () => ({
  useSimpleLocale: () => ({ locale: "en" }),
  getTranslation: (key: string) => key,
}));

afterEach(() => resetInstanceCacheForTests());

async function submit(): Promise<void> {
  render(<ForgotPasswordClient />);
  fireEvent.change(screen.getByRole("textbox"), {
    target: { value: "owner@example.com" },
  });
  const form = screen.getByRole("textbox").closest("form") as HTMLFormElement;
  await act(async () => {
    fireEvent.submit(form);
  });
}

describe("ForgotPasswordClient success hint follows features.email", () => {
  it("tells the user to ask their administrator when EMAIL_PROVIDER=log", async () => {
    setInstanceForTests(
      parseInstanceInfo({
        registration_mode: "invite",
        features: { email: false },
      }),
    );
    await submit();
    expect(
      await screen.findByText("forgotPassword.success.noEmailHint"),
    ).toBeInTheDocument();
    expect(screen.queryByText("forgotPassword.success.spamHint")).toBeNull();
  });

  it("keeps the spam-folder hint when email is on", async () => {
    setInstanceForTests(
      parseInstanceInfo({
        registration_mode: "invite",
        features: { email: true },
      }),
    );
    await submit();
    expect(
      await screen.findByText("forgotPassword.success.spamHint"),
    ).toBeInTheDocument();
  });
});
