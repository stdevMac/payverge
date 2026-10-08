import { readFileSync } from "fs";
import { join } from "path";

// Regression-lock: a user who just reset their password must land somewhere
// that accepts a password. /staff/login is passwordless (Google + code only),
// so reset destinations must point at /dashboard (full AuthModal).
describe("ResetPasswordClient login destinations", () => {
  const source = readFileSync(
    join(__dirname, "ResetPasswordClient.tsx"),
    "utf8",
  );

  it("does not send reset users to the passwordless /staff/login", () => {
    expect(source).not.toMatch(/href="\/staff\/login"/);
  });

  it("routes the success + back-to-login links through the locale-aware login helper", () => {
    expect(source).toContain("buildLoginReturnHref");
    expect(source).not.toMatch(/href="\/dashboard"/);
  });
});
