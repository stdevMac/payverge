import {
  loadGuestMenuDependencies,
  resolveGuestTableSettlement,
  shouldShowMenuLoadingGate,
  withTimeout,
} from "./guestMenuLoad";

describe("shouldShowMenuLoadingGate (#420)", () => {
  it("holds the skeleton only while loading and no table payload exists", () => {
    expect(shouldShowMenuLoadingGate(true, null)).toBe(true);
    expect(shouldShowMenuLoadingGate(false, null)).toBe(false);
  });

  it("clears the gate once table data is present, even if loading is still true", () => {
    expect(
      shouldShowMenuLoadingGate(true, { table: { table_code: "F4SHYQ2KLL" } }, "F4SHYQ2KLL"),
    ).toBe(false);
  });

  it("keeps the skeleton when cached data belongs to a different table", () => {
    expect(
      shouldShowMenuLoadingGate(true, { table: { table_code: "OTHER" } }, "F4SHYQ2KLL"),
    ).toBe(true);
  });
});

describe("withTimeout", () => {
  it("rejects a hanging promise as a network timeout", async () => {
    await expect(withTimeout(new Promise(() => {}), 20, "hang")).rejects.toMatchObject({
      message: "hang",
      code: "ECONNABORTED",
    });
  });

  it("resolves a successful promise before the deadline", async () => {
    await expect(withTimeout(Promise.resolve("ok"), 50)).resolves.toBe("ok");
  });
});

describe("loadGuestMenuDependencies (#420)", () => {
  it("returns the table payload without waiting for a hanging bill", async () => {
    const started = Date.now();
    const result = await loadGuestMenuDependencies({
      table: () => Promise.resolve({ table: { table_code: "T1" } }),
      bill: () => new Promise(() => {}),
      business: () => Promise.resolve({ business: { name: "Demo" } }),
      timeoutMs: 80,
    });
    expect(Date.now() - started).toBeLessThan(70);
    expect(result.table).toEqual({
      status: "fulfilled",
      value: { table: { table_code: "T1" } },
    });

    const rest = await result.rest;
    expect(rest.bill.status).toBe("rejected");
    expect(rest.business.status).toBe("fulfilled");
  });

  it("surfaces a timed-out table load as a rejected settlement", async () => {
    const result = await loadGuestMenuDependencies({
      table: () => new Promise(() => {}),
      bill: () => Promise.resolve({ bill: null }),
      business: () => Promise.resolve({}),
      timeoutMs: 15,
    });
    expect(result.table.status).toBe("rejected");
    if (result.table.status === "rejected") {
      expect((result.table.reason as { code?: string }).code).toBe("ECONNABORTED");
    }
  });

  it("keeps a fulfilled table when business 404s", async () => {
    const result = await loadGuestMenuDependencies({
      table: () => Promise.resolve({ table: { table_code: "T1" } }),
      bill: () => Promise.resolve({ bill: null }),
      business: () => Promise.reject({ response: { status: 404 } }),
      timeoutMs: 40,
    });
    expect(result.table.status).toBe("fulfilled");
    const rest = await result.rest;
    expect(rest.business.status).toBe("rejected");
  });
});

describe("guest table settlement on the menu surface (#682)", () => {
  const quiet = {
    bill: () => Promise.resolve({ bill: null }),
    business: () => Promise.resolve({}),
    timeoutMs: 200,
  };

  it("retries a 200 with an empty body before the page can claim the code is missing", async () => {
    let calls = 0;
    const result = await loadGuestMenuDependencies({
      ...quiet,
      table: () => {
        calls += 1;
        return Promise.resolve(
          calls === 1 ? undefined : { table: { table_code: "T1" } },
        );
      },
    });
    expect(calls).toBe(2);
    expect(result.table).toEqual({
      status: "fulfilled",
      value: { table: { table_code: "T1" } },
    });
  });

  it("does not spend a second hop when the first payload already carries a table", async () => {
    let calls = 0;
    const result = await loadGuestMenuDependencies({
      ...quiet,
      table: () => {
        calls += 1;
        return Promise.resolve({ table: { table_code: "T1" } });
      },
    });
    expect(calls).toBe(1);
    expect(result.table.status).toBe("fulfilled");
  });

  it("retries a 404 once and keeps the 404 when it persists", async () => {
    let calls = 0;
    const result = await loadGuestMenuDependencies({
      ...quiet,
      table: () => {
        calls += 1;
        return Promise.reject({ response: { status: 404 } });
      },
    });
    expect(calls).toBe(2);
    expect(resolveGuestTableSettlement(result.table)).toEqual({
      kind: "error",
      error: "not_found",
    });
  });

  it("recovers a 404 that resolves on the retry hop", async () => {
    let calls = 0;
    const result = await loadGuestMenuDependencies({
      ...quiet,
      table: () => {
        calls += 1;
        return calls === 1
          ? Promise.reject({ response: { status: 404 } })
          : Promise.resolve({ table: { table_code: "T1" } });
      },
    });
    expect(calls).toBe(2);
    expect(resolveGuestTableSettlement(result.table)).toEqual({
      kind: "ready",
      value: { table: { table_code: "T1" } },
    });
  });

  it("never classifies a persisting empty payload as not_found", async () => {
    // The transport lost the body twice. The code is NOT proven missing, so the
    // menu must render retryable network copy — never the 404 / QR-is-wrong page.
    let calls = 0;
    const result = await loadGuestMenuDependencies({
      ...quiet,
      table: () => {
        calls += 1;
        return Promise.resolve(undefined);
      },
    });
    expect(calls).toBe(2);
    expect(resolveGuestTableSettlement(result.table)).toEqual({
      kind: "error",
      error: "network",
    });
  });

  it("does not burn a retry hop on a timed-out table load", async () => {
    let calls = 0;
    const result = await loadGuestMenuDependencies({
      bill: () => Promise.resolve({ bill: null }),
      business: () => Promise.resolve({}),
      timeoutMs: 15,
      table: () => {
        calls += 1;
        return new Promise(() => {});
      },
    });
    expect(calls).toBe(1);
    expect(resolveGuestTableSettlement(result.table)).toEqual({
      kind: "error",
      error: "network",
    });
  });
});

describe("resolveGuestTableSettlement (#682)", () => {
  it("hands a real payload straight through", () => {
    const value = { table: { table_code: "T1" } };
    expect(resolveGuestTableSettlement({ status: "fulfilled", value })).toEqual({
      kind: "ready",
      value,
    });
  });

  it("maps an empty fulfilled payload to network, not not_found", () => {
    expect(
      resolveGuestTableSettlement({ status: "fulfilled", value: {} }),
    ).toEqual({ kind: "error", error: "network" });
  });

  it("maps a real 404 rejection to not_found", () => {
    expect(
      resolveGuestTableSettlement({
        status: "rejected",
        reason: { response: { status: 404 } },
      }),
    ).toEqual({ kind: "error", error: "not_found" });
  });
});
