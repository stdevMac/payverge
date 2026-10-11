import {
  clearLastDashboardPath,
  derivePwaIdentity,
  getLastDashboardPath,
  isInstallComplete,
  isSnoozed,
  markCardShown,
  markInstallComplete,
  noteDashboardVisit,
  saveLastDashboardPath,
  snoozeForSevenDays,
  storageAvailable,
  wasCardShown,
} from "./installState";

class MemoryStorage implements Storage {
  private values = new Map<string, string>();

  get length() {
    return this.values.size;
  }

  clear() {
    this.values.clear();
  }

  getItem(key: string) {
    return this.values.get(key) ?? null;
  }

  key(index: number) {
    return [...this.values.keys()][index] ?? null;
  }

  removeItem(key: string) {
    this.values.delete(key);
  }

  setItem(key: string, value: string) {
    this.values.set(key, value);
  }
}

class ThrowingStorage extends MemoryStorage {
  constructor(private readonly operation: "read" | "write" | "remove") {
    super();
  }

  getItem(key: string) {
    if (this.operation === "read") throw new Error("storage read failed");
    return super.getItem(key);
  }

  setItem(key: string, value: string) {
    if (this.operation === "write") throw new Error("storage write failed");
    super.setItem(key, value);
  }

  removeItem(key: string) {
    if (this.operation === "remove") throw new Error("storage remove failed");
    super.removeItem(key);
  }
}

class FirstWriteFailsStorage extends MemoryStorage {
  private writes = 0;

  setItem(key: string, value: string) {
    this.writes += 1;
    if (this.writes === 1) throw new Error("first storage write failed");
    super.setItem(key, value);
  }
}

describe("derivePwaIdentity", () => {
  it("uses staff id and business id for staff", () => {
    expect(
      derivePwaIdentity({
        isStaffUser: true,
        staffId: 7,
        staffBusinessId: 42,
        isOAuthUser: false,
        oauthUserId: null,
        isWeb3User: false,
        walletAddress: null,
      }),
    ).toEqual({ key: "staff:7:42", roleType: "staff" });
  });

  it("uses backend user id for password or OAuth owners", () => {
    expect(
      derivePwaIdentity({
        isStaffUser: false,
        staffId: null,
        staffBusinessId: null,
        isOAuthUser: true,
        oauthUserId: 19,
        isWeb3User: false,
        walletAddress: null,
      }),
    ).toEqual({ key: "user:19", roleType: "owner" });
  });

  it("normalizes a Web3 address and rejects incomplete principals", () => {
    expect(
      derivePwaIdentity({
        isStaffUser: false,
        staffId: null,
        staffBusinessId: null,
        isOAuthUser: false,
        oauthUserId: null,
        isWeb3User: true,
        walletAddress: "0xAbC",
      }),
    ).toEqual({ key: "web3:0xabc", roleType: "owner" });

    expect(
      derivePwaIdentity({
        isStaffUser: true,
        staffId: null,
        staffBusinessId: 42,
        isOAuthUser: false,
        oauthUserId: null,
        isWeb3User: false,
        walletAddress: null,
      }),
    ).toBeNull();
  });
});

describe("PWA identity-scoped state", () => {
  const identity = "user:19";
  let local: Storage;
  let session: Storage;

  beforeEach(() => {
    local = new MemoryStorage();
    session = new MemoryStorage();
  });

  it("becomes eligible only on a later browser session", () => {
    expect(noteDashboardVisit(local, session, identity)).toBe(false);
    expect(noteDashboardVisit(local, session, identity)).toBe(false);

    session = new MemoryStorage();

    expect(noteDashboardVisit(local, session, identity)).toBe(true);
  });

  it("shows at most once per session", () => {
    expect(wasCardShown(session, identity)).toBe(false);

    markCardShown(session, identity);

    expect(wasCardShown(session, identity)).toBe(true);
  });

  it("fails closed across repeated visits when session reads fail", () => {
    local.setItem("payverge:pwa:user:19:eligible", "1");
    const unavailableSession = new ThrowingStorage("read");

    expect(noteDashboardVisit(local, unavailableSession, identity)).toBe(false);
    expect(noteDashboardVisit(local, unavailableSession, identity)).toBe(false);
  });

  it("fails closed across repeated visits when session writes fail", () => {
    local.setItem("payverge:pwa:user:19:eligible", "1");
    const unavailableSession = new ThrowingStorage("write");

    expect(noteDashboardVisit(local, unavailableSession, identity)).toBe(false);
    expect(noteDashboardVisit(local, unavailableSession, identity)).toBe(false);
  });

  it("does not make a recovering session eligible after its first marker write fails", () => {
    const recoveringSession = new FirstWriteFailsStorage();

    expect(noteDashboardVisit(local, recoveringSession, identity)).toBe(false);
    expect(noteDashboardVisit(local, recoveringSession, identity)).toBe(false);
    expect(noteDashboardVisit(local, new MemoryStorage(), identity)).toBe(true);
  });

  it("treats unreadable card state as already shown", () => {
    expect(wasCardShown(new ThrowingStorage("read"), identity)).toBe(true);
  });

  it("reports when marking the card shown cannot persist", () => {
    expect(markCardShown(new ThrowingStorage("write"), identity)).toBe(false);
  });

  it("snoozes for exactly seven days", () => {
    const now = Date.UTC(2026, 7, 4);

    snoozeForSevenDays(local, identity, now);

    expect(
      isSnoozed(local, identity, now + 7 * 24 * 60 * 60 * 1000 - 1),
    ).toBe(true);
    expect(
      isSnoozed(local, identity, now + 7 * 24 * 60 * 60 * 1000),
    ).toBe(false);
  });

  it("keeps completion and launch paths isolated by identity", () => {
    markInstallComplete(local, identity);
    expect(isInstallComplete(local, identity)).toBe(true);
    expect(isInstallComplete(local, "user:20")).toBe(false);

    expect(
      saveLastDashboardPath(local, identity, "/business/casa/dashboard"),
    ).toBe(true);
    expect(getLastDashboardPath(local, identity)).toBe(
      "/business/casa/dashboard",
    );
    expect(getLastDashboardPath(local, "user:20")).toBeNull();
  });

  it("rejects external, malformed, and query-bearing launch paths", () => {
    expect(saveLastDashboardPath(local, identity, "https://evil.test/x")).toBe(
      false,
    );
    expect(saveLastDashboardPath(local, identity, "//evil.test/x")).toBe(false);
    expect(
      saveLastDashboardPath(local, identity, "/business/x/dashboard?tab=bills"),
    ).toBe(false);
    expect(saveLastDashboardPath(local, identity, "/admin")).toBe(false);
  });

  it("clears only the remembered launch path on logout", () => {
    saveLastDashboardPath(local, identity, "/business/casa/dashboard");
    snoozeForSevenDays(local, identity, Date.UTC(2026, 7, 4));

    clearLastDashboardPath(local, identity);

    expect(getLastDashboardPath(local, identity)).toBeNull();
    expect(isSnoozed(local, identity, Date.UTC(2026, 7, 5))).toBe(true);
  });

  it("prefixes persisted state keys with payverge:pwa", () => {
    noteDashboardVisit(local, session, identity);
    markInstallComplete(local, identity);
    saveLastDashboardPath(local, identity, "/business/casa/dashboard");

    expect(local.getItem("payverge:pwa:user:19:eligible")).toBe("1");
    expect(session.getItem("payverge:pwa:user:19:dashboard-session")).toBe(
      "1",
    );
    expect(local.getItem("payverge:pwa:user:19:complete")).toBe("1");
    expect(local.getItem("payverge:pwa:user:19:last-dashboard")).toBe(
      "/business/casa/dashboard",
    );
  });

  it("fails closed when storage reads throw", () => {
    expect(isInstallComplete(new ThrowingStorage("read"), identity)).toBe(false);
  });

  it("reports failed dashboard-path writes without throwing", () => {
    expect(
      saveLastDashboardPath(
        new ThrowingStorage("write"),
        identity,
        "/business/casa/dashboard",
      ),
    ).toBe(false);
  });

  it("swallows storage removal failures", () => {
    expect(() =>
      clearLastDashboardPath(new ThrowingStorage("remove"), identity),
    ).not.toThrow();
  });

  it("fails closed when persistent storage throws during the availability probe", () => {
    expect(storageAvailable(new ThrowingStorage("write"))).toBe(false);
  });
});
