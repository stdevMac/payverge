import { formatAdminActionType } from "../adminActionLabel";

describe("formatAdminActionType", () => {
  it("maps known action types to canonical labels", () => {
    expect(formatAdminActionType("suspend_business")).toBe("Suspended business");
    expect(formatAdminActionType("reactivate_business")).toBe("Reactivated business");
    expect(formatAdminActionType("close_account")).toBe("Closed account");
    expect(formatAdminActionType("reset_password")).toBe("Reset password");
  });

  it("falls back to a Title-Cased variant for unknown actions", () => {
    // A new backend action that hasn't been added to the map shouldn't
    // render as raw snake_case in the UI.
    expect(formatAdminActionType("do_some_new_thing")).toBe("Do some new thing");
  });

  it("handles empty / nullish input without crashing", () => {
    expect(formatAdminActionType(undefined)).toBe("Action");
    expect(formatAdminActionType(null)).toBe("Action");
    expect(formatAdminActionType("")).toBe("Action");
  });
});
