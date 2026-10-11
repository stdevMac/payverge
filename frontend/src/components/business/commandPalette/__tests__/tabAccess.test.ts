import {
  getTabLockMeta,
  LOCK_EXEMPT_TABS,
  type AccessState,
} from "../tabAccess";

const ACTIVE: AccessState = { loading: false, isSuspended: false };
const LOCKED: AccessState = { loading: false, isSuspended: true };

describe("getTabLockMeta", () => {
  it("never gates while the lock state is loading", () => {
    const loading: AccessState = { loading: true, isSuspended: true };
    expect(getTabLockMeta("bills", loading, false)).toEqual({
      locked: false,
      hidden: false,
    });
  });

  it("leaves every tab open for an operational business", () => {
    for (const tabKey of ["bills", "menu", "director-console", "marketing", "settings"]) {
      expect(getTabLockMeta(tabKey, ACTIVE, false)).toEqual({
        locked: false,
        hidden: false,
      });
    }
  });

  it("locks everything but settings when an administrator locked the business", () => {
    for (const tabKey of ["bills", "menu", "tables", "ai-waiter", "analytics"]) {
      expect(getTabLockMeta(tabKey, LOCKED, false)).toEqual({
        locked: true,
        hidden: false,
      });
    }
    for (const tabKey of LOCK_EXEMPT_TABS) {
      expect(getTabLockMeta(tabKey, LOCKED, false).locked).toBe(false);
    }
  });

  it("hides locked tabs from staff instead of showing the lock notice", () => {
    expect(getTabLockMeta("bills", LOCKED, true)).toEqual({
      locked: true,
      hidden: true,
    });
    expect(getTabLockMeta("settings", LOCKED, true).hidden).toBe(false);
  });
});
