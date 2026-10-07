/** @jest-environment jsdom */

import {
  consumePendingCartIntent,
  getOrCreatePendingCartSessionBinding,
  pendingCartKey,
  writePendingCartIntent,
  type PendingCartIntent,
} from "../pendingCartIntent";

const NOW = 1_800_000_000_000;
const ITEM_ID = "018f99f8-4f0d-74ac-bbee-91b2f7b57e04";
const BINDING_ID = "728a70ef-0e4f-49b7-9978-229b7bc2be59";

const itemIntent = (
  overrides: Partial<PendingCartIntent> = {},
): PendingCartIntent => ({
  version: 1,
  tableCode: "T1",
  sessionId: "session-abc",
  nonce: "nonce-018f99f8",
  expiresAt: NOW + 120_000,
  menuItemId: ITEM_ID,
  quantity: 2,
  notes: "sin cebolla",
  ...overrides,
});

describe("pending cart intent storage", () => {
  beforeEach(() => sessionStorage.clear());

  it("uses a canonical table-scoped key without putting payload data in the URL", () => {
    const before = window.location.href;

    expect(pendingCartKey(" t-1 ")).toBe("payverge_pending_cart_v1:T-1");
    expect(pendingCartKey("T-2")).not.toBe(pendingCartKey("T-1"));
    expect(pendingCartKey("T-1")).not.toContain(ITEM_ID);
    expect(window.location.href).toBe(before);
  });

  it("supports a legitimate one-character ASCII table code", () => {
    const intent = itemIntent({ tableCode: "1" });
    expect(pendingCartKey(" 1 ")).toBe("payverge_pending_cart_v1:1");
    expect(writePendingCartIntent(intent, { now: NOW })).toEqual({ ok: true });
    expect(
      consumePendingCartIntent("1", "session-abc", { now: NOW + 1 }),
    ).toEqual({ ok: true, intent });
  });

  it.each([
    ["ß1", "SS1"],
    ["ſ1", "S1"],
  ])(
    "rejects Unicode table scope %p before it can collide with %p",
    (unicode, ascii) => {
      expect(() => pendingCartKey(unicode)).toThrow("invalid_table_code");
      expect(pendingCartKey(ascii)).toContain(`:${ascii}`);

      const asciiIntent = itemIntent({ tableCode: ascii });
      expect(writePendingCartIntent(asciiIntent, { now: NOW })).toEqual({
        ok: true,
      });
      expect(
        writePendingCartIntent(itemIntent({ tableCode: unicode }), {
          now: NOW,
        }),
      ).toEqual({ ok: false, error: "invalid_intent" });
      expect(
        consumePendingCartIntent(unicode, "session-abc", { now: NOW + 1 }),
      ).toEqual({ ok: false, error: "invalid_scope" });
      expect(sessionStorage.getItem(pendingCartKey(ascii))).not.toBeNull();
    },
  );

  it("fails with a controlled error for a runtime non-string key input", () => {
    expect(() => pendingCartKey(null as never)).toThrow("invalid_table_code");
    expect(() => pendingCartKey([] as never)).toThrow("invalid_table_code");
    expect(() => pendingCartKey(7 as never)).toThrow("invalid_table_code");
  });

  it("writes and consumes one canonical menu-item intent", () => {
    expect(writePendingCartIntent(itemIntent(), { now: NOW })).toEqual({
      ok: true,
    });

    const result = consumePendingCartIntent("t1", "session-abc", {
      now: NOW + 1,
    });
    expect(result).toEqual({ ok: true, intent: itemIntent() });
    expect(sessionStorage.getItem(pendingCartKey("T1"))).toBeNull();
  });

  it("preserves a safe positive numeric bundle ID", () => {
    const intent = itemIntent({ menuItemId: undefined, bundleId: 42 });

    expect(writePendingCartIntent(intent, { now: NOW })).toEqual({ ok: true });
    expect(
      consumePendingCartIntent("T1", "session-abc", { now: NOW + 1 }),
    ).toEqual({ ok: true, intent });
  });

  it("consumes once and rejects a manually replayed nonce", () => {
    const intent = itemIntent();
    writePendingCartIntent(intent, { now: NOW });
    expect(
      consumePendingCartIntent("T1", "session-abc", { now: NOW + 1 }).ok,
    ).toBe(true);

    sessionStorage.setItem(pendingCartKey("T1"), JSON.stringify(intent));
    expect(
      consumePendingCartIntent("T1", "session-abc", { now: NOW + 2 }),
    ).toEqual({ ok: false, error: "replayed" });
    expect(sessionStorage.getItem(pendingCartKey("T1"))).toBeNull();
  });

  it("rejects a replay at write time while its bounded receipt is live", () => {
    const intent = itemIntent();
    writePendingCartIntent(intent, { now: NOW });
    consumePendingCartIntent("T1", "session-abc", { now: NOW + 1 });

    expect(writePendingCartIntent(intent, { now: NOW + 2 })).toEqual({
      ok: false,
      error: "replayed",
    });
  });

  it("scopes consumed nonces by table and session", () => {
    const first = itemIntent({ nonce: "shared-nonce" });
    writePendingCartIntent(first, { now: NOW });
    consumePendingCartIntent("T1", "session-abc", { now: NOW + 1 });

    const anotherSession = itemIntent({
      sessionId: "session-other",
      nonce: "shared-nonce",
    });
    expect(writePendingCartIntent(anotherSession, { now: NOW + 2 })).toEqual({
      ok: true,
    });
    expect(
      consumePendingCartIntent("T1", "session-other", { now: NOW + 3 }),
    ).toEqual({ ok: true, intent: anotherSession });

    const anotherTable = itemIntent({
      tableCode: "T2",
      nonce: "shared-nonce",
    });
    expect(writePendingCartIntent(anotherTable, { now: NOW + 2 })).toEqual({
      ok: true,
    });
    expect(
      consumePendingCartIntent("T2", "session-abc", { now: NOW + 3 }),
    ).toEqual({ ok: true, intent: anotherTable });
  });

  it("prunes expired replay receipts", () => {
    const first = itemIntent({
      nonce: "reusable-after-expiry",
      expiresAt: NOW + 10,
    });
    writePendingCartIntent(first, { now: NOW });
    consumePendingCartIntent("T1", "session-abc", { now: NOW + 1 });

    const replacement = itemIntent({
      nonce: "reusable-after-expiry",
      expiresAt: NOW + 20_000,
    });
    expect(writePendingCartIntent(replacement, { now: NOW + 11 })).toEqual({
      ok: true,
    });
  });

  it("rejects a replay receipt with an unbounded future expiry", () => {
    const intent = itemIntent();
    sessionStorage.setItem(pendingCartKey("T1"), JSON.stringify(intent));
    sessionStorage.setItem(
      `${pendingCartKey("T1")}:consumed`,
      JSON.stringify({
        version: 1,
        entries: [
          {
            sessionId: "session-abc",
            nonce: "other-nonce",
            expiresAt: NOW + 120_001,
          },
        ],
      }),
    );

    expect(consumePendingCartIntent("T1", "session-abc", { now: NOW })).toEqual(
      { ok: false, error: "corrupt" },
    );
    expect(sessionStorage.getItem(pendingCartKey("T1"))).toBeNull();
    expect(
      sessionStorage.getItem(`${pendingCartKey("T1")}:consumed`),
    ).toBeNull();
  });

  it("cleans replay receipts with unknown or wrong-typed entry fields", () => {
    for (const entry of [
      {
        sessionId: 7,
        nonce: "other-nonce",
        expiresAt: NOW + 10,
      },
      {
        sessionId: "session-abc",
        nonce: "other-nonce",
        expiresAt: NOW + 10,
        raw: true,
      },
    ]) {
      sessionStorage.setItem(
        pendingCartKey("T1"),
        JSON.stringify(itemIntent()),
      );
      sessionStorage.setItem(
        `${pendingCartKey("T1")}:consumed`,
        JSON.stringify({ version: 1, entries: [entry] }),
      );

      expect(
        consumePendingCartIntent("T1", "session-abc", { now: NOW }),
      ).toEqual({ ok: false, error: "corrupt" });
      expect(sessionStorage.getItem(pendingCartKey("T1"))).toBeNull();
      expect(
        sessionStorage.getItem(`${pendingCartKey("T1")}:consumed`),
      ).toBeNull();
    }
  });

  it("bounds live replay receipts without evicting a still-live nonce", () => {
    for (let index = 0; index < 32; index += 1) {
      const intent = itemIntent({ nonce: `nonce-${index}` });
      expect(writePendingCartIntent(intent, { now: NOW })).toEqual({
        ok: true,
      });
      expect(
        consumePendingCartIntent("T1", "session-abc", { now: NOW + 1 }).ok,
      ).toBe(true);
    }

    const overflow = itemIntent({ nonce: "nonce-overflow" });
    expect(writePendingCartIntent(overflow, { now: NOW })).toEqual({
      ok: true,
    });
    expect(
      consumePendingCartIntent("T1", "session-abc", { now: NOW + 1 }),
    ).toEqual({ ok: false, error: "receipt_capacity" });
    expect(sessionStorage.getItem(pendingCartKey("T1"))).toBeNull();

    expect(
      writePendingCartIntent(itemIntent({ nonce: "nonce-0" }), {
        now: NOW + 2,
      }),
    ).toEqual({
      ok: false,
      error: "replayed",
    });
  });

  it.each([
    ["wrong table", "T2", "session-abc", "table_mismatch"],
    ["wrong session", "T1", "session-other", "session_mismatch"],
  ])("terminally removes a %s intent", (_case, table, session, error) => {
    const intent = itemIntent();
    sessionStorage.setItem(pendingCartKey(table), JSON.stringify(intent));

    expect(consumePendingCartIntent(table, session, { now: NOW + 1 })).toEqual({
      ok: false,
      error,
    });
    expect(sessionStorage.getItem(pendingCartKey(table))).toBeNull();
  });

  it("removes expired intents", () => {
    const intent = itemIntent({ expiresAt: NOW + 10 });
    sessionStorage.setItem(pendingCartKey("T1"), JSON.stringify(intent));

    expect(
      consumePendingCartIntent("T1", "session-abc", { now: NOW + 10 }),
    ).toEqual({ ok: false, error: "expired" });
    expect(sessionStorage.getItem(pendingCartKey("T1"))).toBeNull();
  });

  it.each([
    ["already expired", NOW],
    ["more than two minutes", NOW + 120_001],
    ["fractional timestamp", NOW + 0.5],
    ["unsafe timestamp", Number.MAX_SAFE_INTEGER + 1],
  ])("rejects an invalid expiry: %s", (_case, expiresAt) => {
    expect(
      writePendingCartIntent(itemIntent({ expiresAt }), { now: NOW }),
    ).toEqual({ ok: false, error: "invalid_intent" });
    expect(sessionStorage.getItem(pendingCartKey("T1"))).toBeNull();
  });

  it("accepts the exact two-minute boundary and fails closed on clock rollback", () => {
    const intent = itemIntent({ expiresAt: NOW + 120_000 });
    expect(writePendingCartIntent(intent, { now: NOW })).toEqual({ ok: true });
    expect(
      consumePendingCartIntent("T1", "session-abc", { now: NOW - 1 }),
    ).toEqual({ ok: false, error: "corrupt" });
    expect(sessionStorage.getItem(pendingCartKey("T1"))).toBeNull();
  });

  it.each([Number.NaN, Infinity, 1.5, Number.MAX_SAFE_INTEGER + 1])(
    "fails safely for invalid clock %p",
    (now) => {
      expect(writePendingCartIntent(itemIntent(), { now })).toEqual({
        ok: false,
        error: "storage_unavailable",
      });
      expect(consumePendingCartIntent("T1", "session-abc", { now })).toEqual({
        ok: false,
        error: "invalid_scope",
      });
    },
  );

  it.each([
    ["neither ID", { menuItemId: undefined }],
    ["both IDs", { bundleId: 42 }],
    ["non-UUID item ID", { menuItemId: "Harvest Bowl" }],
    ["padded item ID", { menuItemId: ` ${ITEM_ID}` }],
    ["zero bundle", { menuItemId: undefined, bundleId: 0 }],
    ["negative bundle", { menuItemId: undefined, bundleId: -1 }],
    [
      "unsafe bundle",
      { menuItemId: undefined, bundleId: Number.MAX_SAFE_INTEGER + 1 },
    ],
    ["fractional bundle", { menuItemId: undefined, bundleId: 4.2 }],
  ])("rejects %s", (_case, overrides) => {
    expect(writePendingCartIntent(itemIntent(overrides), { now: NOW })).toEqual(
      { ok: false, error: "invalid_intent" },
    );
  });

  it.each([0, 21, 1.5, Number.NaN])("rejects quantity %p", (quantity) => {
    expect(
      writePendingCartIntent(itemIntent({ quantity }), { now: NOW }),
    ).toEqual({ ok: false, error: "invalid_intent" });
  });

  it.each(["x".repeat(201), "bad\u0000note", "bad\u202enote", null])(
    "rejects malformed notes %p",
    (notes) => {
      expect(
        writePendingCartIntent(itemIntent({ notes: notes as never }), {
          now: NOW,
        }),
      ).toEqual({ ok: false, error: "invalid_intent" });
    },
  );

  it("counts notes by Unicode code points", () => {
    expect(
      writePendingCartIntent(itemIntent({ notes: "🍽️".repeat(100) }), {
        now: NOW,
      }),
    ).toEqual({ ok: true });
    expect(
      writePendingCartIntent(itemIntent({ notes: "😀".repeat(201) }), {
        now: NOW,
      }),
    ).toEqual({ ok: false, error: "invalid_intent" });
  });

  it.each([
    ["version", { version: 2 }],
    ["table", { tableCode: "../T1" }],
    ["session", { sessionId: " session-abc " }],
    ["nonce", { nonce: "bad\u202enonce" }],
  ])("rejects malformed %s scope", (_case, overrides) => {
    expect(
      writePendingCartIntent(itemIntent(overrides as never), { now: NOW }),
    ).toEqual({ ok: false, error: "invalid_intent" });
  });

  it("rejects unknown fields rather than persisting them", () => {
    expect(
      writePendingCartIntent(
        {
          ...itemIntent(),
          rawModelPayload: { href: "https://evil.test" },
        } as never,
        { now: NOW },
      ),
    ).toEqual({ ok: false, error: "invalid_intent" });
  });

  it.each(["{", "null", "[]", '{"version":1}'])(
    "cleans corrupt stored JSON %p",
    (raw) => {
      sessionStorage.setItem(pendingCartKey("T1"), raw);

      expect(
        consumePendingCartIntent("T1", "session-abc", { now: NOW }),
      ).toEqual({ ok: false, error: "corrupt" });
      expect(sessionStorage.getItem(pendingCartKey("T1"))).toBeNull();
    },
  );

  it("strictly removes stored records with unknown fields or wrong runtime types", () => {
    for (const stored of [
      { ...itemIntent(), raw: true },
      { ...itemIntent(), quantity: "2" },
      { ...itemIntent(), menuItemId: null },
    ]) {
      sessionStorage.setItem(pendingCartKey("T1"), JSON.stringify(stored));
      expect(
        consumePendingCartIntent("T1", "session-abc", { now: NOW }),
      ).toEqual({ ok: false, error: "corrupt" });
      expect(sessionStorage.getItem(pendingCartKey("T1"))).toBeNull();
    }
  });

  it.each([
    ["bad table", "X".repeat(64), "session-abc"],
    ["bad session", "T1", "x".repeat(513)],
  ])("rejects malformed consume scope: %s", (_case, table, session) => {
    expect(consumePendingCartIntent(table, session, { now: NOW })).toEqual({
      ok: false,
      error: "invalid_scope",
    });
  });

  it("never writes pending intent data to localStorage", () => {
    localStorage.setItem("sentinel", "keep");
    writePendingCartIntent(itemIntent(), { now: NOW });

    expect(localStorage.length).toBe(1);
    expect(localStorage.getItem("sentinel")).toBe("keep");
  });

  it("does not return a consumed intent if removing the pending value fails", () => {
    const values = new Map<string, string>();
    const storage = {
      getItem: (key: string) => values.get(key) ?? null,
      setItem: (key: string, value: string) => values.set(key, value),
      removeItem: () => {
        throw new DOMException("blocked", "SecurityError");
      },
    };
    expect(writePendingCartIntent(itemIntent(), { now: NOW, storage }).ok).toBe(
      true,
    );

    expect(
      consumePendingCartIntent("T1", "session-abc", {
        now: NOW + 1,
        storage,
      }),
    ).toEqual({ ok: false, error: "storage_unavailable" });
  });

  it("fails safely when sessionStorage throws", () => {
    const throwingStorage = {
      getItem: () => {
        throw new DOMException("blocked", "SecurityError");
      },
      setItem: () => {
        throw new DOMException("full", "QuotaExceededError");
      },
      removeItem: () => {
        throw new DOMException("blocked", "SecurityError");
      },
    };

    expect(
      writePendingCartIntent(itemIntent(), {
        now: NOW,
        storage: throwingStorage,
      }),
    ).toEqual({ ok: false, error: "storage_unavailable" });
    expect(
      consumePendingCartIntent("T1", "session-abc", {
        now: NOW,
        storage: throwingStorage,
      }),
    ).toEqual({ ok: false, error: "storage_unavailable" });
  });
});

describe("pending cart non-secret session binding", () => {
  beforeEach(() => sessionStorage.clear());

  it("creates and reuses one table-scoped UUID without persisting a bearer", () => {
    const randomUUID = jest.fn(() => BINDING_ID);

    expect(
      getOrCreatePendingCartSessionBinding(" t1 ", { randomUUID }),
    ).toEqual({ ok: true, sessionId: BINDING_ID });
    expect(
      getOrCreatePendingCartSessionBinding("T1", {
        randomUUID: () => "018f99f8-4f0d-74ac-bbee-91b2f7b57e05",
      }),
    ).toEqual({ ok: true, sessionId: BINDING_ID });
    expect(randomUUID).toHaveBeenCalledTimes(1);
    expect(sessionStorage.getItem("payverge_pending_cart_session_v1:T1")).toBe(
      BINDING_ID,
    );
    expect(JSON.stringify(sessionStorage)).not.toContain("bearer-token");
  });

  it("isolates bindings by table", () => {
    const ids = [BINDING_ID, "018f99f8-4f0d-74ac-bbee-91b2f7b57e05"];
    const randomUUID = () => ids.shift() ?? "";

    expect(getOrCreatePendingCartSessionBinding("T1", { randomUUID })).toEqual({
      ok: true,
      sessionId: BINDING_ID,
    });
    expect(getOrCreatePendingCartSessionBinding("T2", { randomUUID })).toEqual({
      ok: true,
      sessionId: "018f99f8-4f0d-74ac-bbee-91b2f7b57e05",
    });
  });

  it.each(["corrupt", " 728a70ef-0e4f-49b7-9978-229b7bc2be59 ", "ß1"])(
    "replaces an invalid stored binding %p without using it",
    (stored) => {
      sessionStorage.setItem("payverge_pending_cart_session_v1:T1", stored);
      expect(
        getOrCreatePendingCartSessionBinding("T1", {
          randomUUID: () => BINDING_ID,
        }),
      ).toEqual({ ok: true, sessionId: BINDING_ID });
    },
  );

  it.each(["ß1", "ſ1", null, [], 7])(
    "fails safely for invalid runtime table scope %p",
    (tableCode) => {
      expect(() =>
        getOrCreatePendingCartSessionBinding(tableCode as never, {
          randomUUID: () => BINDING_ID,
        }),
      ).not.toThrow();
      expect(
        getOrCreatePendingCartSessionBinding(tableCode as never, {
          randomUUID: () => BINDING_ID,
        }),
      ).toEqual({ ok: false, error: "invalid_scope" });
    },
  );

  it.each([
    [
      "throws",
      () => {
        throw new Error("entropy unavailable");
      },
    ],
    ["returns malformed", () => "not-a-uuid"],
    ["returns uppercase", () => BINDING_ID.toUpperCase()],
  ])("fails safely when randomUUID %s", (_case, randomUUID) => {
    expect(getOrCreatePendingCartSessionBinding("T1", { randomUUID })).toEqual({
      ok: false,
      error: "entropy_unavailable",
    });
    expect(
      sessionStorage.getItem("payverge_pending_cart_session_v1:T1"),
    ).toBeNull();
  });

  it.each(["get", "remove", "set"])(
    "fails safely when binding storage %s throws",
    (failure) => {
      const values = new Map<string, string>();
      if (failure === "remove") {
        values.set("payverge_pending_cart_session_v1:T1", "corrupt");
      }
      const storage = {
        getItem: (key: string) => {
          if (failure === "get") throw new Error("blocked");
          return values.get(key) ?? null;
        },
        setItem: (key: string, value: string) => {
          if (failure === "set") throw new Error("blocked");
          values.set(key, value);
        },
        removeItem: (key: string) => {
          if (failure === "remove") throw new Error("blocked");
          values.delete(key);
        },
      };

      expect(
        getOrCreatePendingCartSessionBinding("T1", {
          storage,
          randomUUID: () => BINDING_ID,
        }),
      ).toEqual({ ok: false, error: "storage_unavailable" });
    },
  );
});
