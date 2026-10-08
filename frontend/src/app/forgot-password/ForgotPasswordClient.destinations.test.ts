import { readFileSync } from "fs";
import { join } from "path";

// Regression-lock for WS3: forgot-password destinations must not dead-end at
// passwordless /staff/login, and the unreachable {error && ...} block (error
// is only ever set to "") must stay removed.
describe("ForgotPasswordClient navigation + dead-code", () => {
  const source = readFileSync(
    join(__dirname, "ForgotPasswordClient.tsx"),
    "utf8",
  );

  it("does not send users to the passwordless /staff/login", () => {
    expect(source).not.toMatch(/href="\/staff\/login"/);
  });

  it("routes the success + footer links through the login-return helper", () => {
    expect(source).toContain("buildLoginReturnHref");
    expect(source).not.toMatch(/href="\/dashboard"/);
  });

  it("removes the unreachable error state and its render block", () => {
    expect(source).not.toContain("const [error, setError]");
    expect(source).not.toContain("{error && (");
  });
});
