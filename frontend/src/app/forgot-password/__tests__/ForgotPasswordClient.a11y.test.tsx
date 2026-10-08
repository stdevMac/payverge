/** @jest-environment jsdom */
import fs from "fs";
import path from "path";
import { render } from "@testing-library/react";
import ForgotPasswordClient from "../ForgotPasswordClient";

// A-5 (resolved-by-removal): the error path in ForgotPasswordClient was fully
// dead — setError was only ever called with "" and the catch swallows errors
// (no-enumeration design). There is therefore no live error to announce. WS3
// removed the dead `{error && …}` block + the `error` state; this lock keeps it
// gone so the finding cannot silently reopen as un-announced error UI.
jest.mock("@/api/auth", () => ({
  authAPI: { requestPasswordReset: jest.fn().mockResolvedValue(undefined) },
}));
jest.mock("@/i18n/SimpleTranslationProvider", () => ({
  useSimpleLocale: () => ({ locale: "en" }),
  getTranslation: (key: string) => key,
}));

describe("ForgotPasswordClient — A-5 dead error path removed", () => {
  const src = fs.readFileSync(
    path.resolve(__dirname, "../ForgotPasswordClient.tsx"),
    "utf8",
  );

  it("declares no unused `error` useState", () => {
    expect(src).not.toMatch(/const \[error, setError\] = useState/);
    expect(src).not.toMatch(/setError\(/);
  });

  it("renders no dead `{error && …}` block", () => {
    expect(src).not.toMatch(/\{error &&/);
  });

  it("still mounts the request form", () => {
    const { container } = render(<ForgotPasswordClient />);
    expect(container.querySelector('input[type="email"]')).toBeTruthy();
  });
});
