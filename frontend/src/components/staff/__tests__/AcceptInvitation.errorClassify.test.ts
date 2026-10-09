import { readFileSync } from "fs";
import { join } from "path";

describe("AcceptInvitation classifies invite errors", () => {
  const source = readFileSync(
    join(__dirname, "..", "AcceptInvitation.tsx"),
    "utf8",
  );

  it("uses classifyInvitationError to distinguish permanent vs transient", () => {
    // P3: code-first classification (stable backend codes), message is fallback.
    expect(source).toContain(
      "classifyInvitationError(message, getApiErrorCode(error))",
    );
    expect(source).toContain('setErrorKind("permanent")');
  });

  it("renders distinct expired vs transient copy + a retry CTA", () => {
    expect(source).toContain('tString("messages.expiredTitle")');
    expect(source).toContain('tString("messages.transientTitle")');
    expect(source).toContain('tString("actions.tryAgain")');
  });
});
