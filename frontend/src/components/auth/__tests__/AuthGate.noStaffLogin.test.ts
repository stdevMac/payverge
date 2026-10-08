import { readFileSync } from "fs";
import { join } from "path";

describe("AuthGate redirect destination", () => {
  const source = readFileSync(join(__dirname, "..", "AuthGate.tsx"), "utf8");
  it("does not redirect owners to passwordless /staff/login", () => {
    expect(source).not.toContain("/staff/login");
  });
  it("uses the shared buildAuthRedirectUrl helper", () => {
    expect(source).toContain("buildAuthRedirectUrl(redirectTarget)");
  });
});
