import { getLaunchInviteCode } from "@/utils/launchInvite";

describe("getLaunchInviteCode", () => {
  it("reads and trims the canonical launch invite query parameter", () => {
    expect(getLaunchInviteCode("?invite_code=%20cohort-ALPHA%20")).toBe(
      "cohort-ALPHA",
    );
  });

  it("ignores the retired ?invite= alias and omits empty values", () => {
    expect(getLaunchInviteCode("?invite=cohort-BETA")).toBeUndefined();
    expect(getLaunchInviteCode("?invite_code=%20%20")).toBeUndefined();
  });
});
