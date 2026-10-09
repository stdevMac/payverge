import { readFileSync } from "fs";
import { join } from "path";

// WS3: a freshly-verified user should be able to go straight to the dashboard
// (highest-intent moment), not only "Return home".
describe("VerifyEmailClient success CTA", () => {
  const source = readFileSync(
    join(__dirname, "VerifyEmailClient.tsx"),
    "utf8",
  );

  it("renders a Continue-to-dashboard CTA on success", () => {
    expect(source).toContain('href="/dashboard"');
    expect(source).toContain('t("actions.continueToDashboard")');
  });

  it("gates the dashboard CTA on the success status", () => {
    expect(source).toContain('status === "success"');
  });
});
