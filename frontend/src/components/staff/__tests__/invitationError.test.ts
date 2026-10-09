import { classifyInvitationError } from "../invitationError";

describe("classifyInvitationError", () => {
  it("treats expired / used / revoked invitations as permanent", () => {
    expect(classifyInvitationError("This invitation has expired")).toBe("permanent");
    expect(classifyInvitationError("Invitation already accepted")).toBe("permanent");
    expect(classifyInvitationError("This invitation has been used")).toBe("permanent");
    expect(classifyInvitationError("This invitation was revoked")).toBe("permanent");
    expect(classifyInvitationError("Invitation is no longer valid")).toBe("permanent");
    expect(classifyInvitationError("Invitation not found")).toBe("permanent");
  });

  it("treats network / server blips as transient", () => {
    expect(classifyInvitationError("Failed to validate invitation")).toBe("transient");
    expect(classifyInvitationError("Network Error")).toBe("transient");
    expect(classifyInvitationError("Something went wrong on our end")).toBe("transient");
    expect(classifyInvitationError("")).toBe("transient");
  });

  it("does not misclassify a transient message that merely contains 'used'", () => {
    // A bare "used" pattern would wrongly flag this as permanent and hide retry.
    expect(classifyInvitationError("The service could not be used right now")).toBe("transient");
  });

  it("is case-insensitive", () => {
    expect(classifyInvitationError("INVITATION HAS EXPIRED")).toBe("permanent");
  });
});

describe("classifyInvitationError — code-first (P3)", () => {
  it.each([
    "INVITE_EXPIRED",
    "INVITE_INVALID",
    "INVITE_CANNOT_RESEND",
    "STAFF_ACCOUNT_EXISTS",
    "STAFF_EMAIL_LINKED",
    "STAFF_EMAIL_EXISTS",
    "INVITE_ALREADY_PENDING",
  ])("classifies %s as permanent regardless of message language", (code) => {
    expect(classifyInvitationError("La invitación venció", code)).toBe(
      "permanent",
    );
  });

  it.each(["INVITE_VALIDATION_FAILED", "RATE_LIMITED"])(
    "classifies %s as transient",
    (code) => {
      expect(classifyInvitationError("whatever", code)).toBe("transient");
    },
  );

  it("falls back to substring classification when no code is present", () => {
    expect(classifyInvitationError("Invitation has expired")).toBe("permanent");
    expect(classifyInvitationError("network hiccup")).toBe("transient");
  });

  it("falls back to substrings for unknown codes", () => {
    expect(classifyInvitationError("Invitation has expired", "SOMETHING_NEW")).toBe(
      "permanent",
    );
  });
});
