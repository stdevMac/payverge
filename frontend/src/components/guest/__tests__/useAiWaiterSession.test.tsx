/** @jest-environment jsdom */
import { act, renderHook } from "@testing-library/react";
import { createAiWaiterSession } from "@/api/aiWaiter";
import {
  useAiWaiterSession,
  type UseAiWaiterSessionOptions,
} from "@/components/guest/useAiWaiterSession";

jest.mock("@/api/aiWaiter", () => ({
  createAiWaiterSession: jest.fn(),
}));

const mockedCreate = createAiWaiterSession as jest.MockedFunction<
  typeof createAiWaiterSession
>;

const session = (token: string) => ({
  session_token: token,
  greeting: "Hello",
  expires_at: "2999-01-01T00:00:00Z",
});

function deferred<T>() {
  let resolve!: (value: T) => void;
  let reject!: (reason?: unknown) => void;
  const promise = new Promise<T>((res, rej) => {
    resolve = res;
    reject = rej;
  });
  return { promise, resolve, reject };
}

describe("useAiWaiterSession", () => {
  beforeEach(() => {
    jest.clearAllMocks();
    mockedCreate.mockReset();
  });

  it("reuses one in-memory session and dedupes concurrent creation", async () => {
    const pending = deferred<ReturnType<typeof session>>();
    mockedCreate.mockReturnValueOnce(pending.promise);
    const { result } = renderHook(() =>
      useAiWaiterSession({
        businessId: 7,
        tableCode: "T1",
        mode: "ordering",
        language: "es",
      }),
    );

    let first!: Promise<string>;
    let second!: Promise<string>;
    act(() => {
      first = result.current.ensureSession();
      second = result.current.ensureSession();
    });
    expect(mockedCreate).toHaveBeenCalledTimes(1);
    expect(mockedCreate).toHaveBeenCalledWith(7, {
      table_code: "T1",
      mode: "ordering",
      language: "es",
    });

    await act(async () => pending.resolve(session("token-1")));
    await expect(first).resolves.toBe("token-1");
    await expect(second).resolves.toBe("token-1");
    expect(result.current.sessionToken).toBe("token-1");

    await expect(result.current.ensureSession()).resolves.toBe("token-1");
    expect(mockedCreate).toHaveBeenCalledTimes(1);
  });

  it("mints a locale-specific session and replaces the previous conversation on language change", async () => {
    mockedCreate
      .mockResolvedValueOnce(session("token-en"))
      .mockResolvedValueOnce(session("token-es-ar"))
      .mockResolvedValueOnce(session("token-table"))
      .mockResolvedValueOnce(session("token-mode"))
      .mockResolvedValueOnce(session("token-business"));
    const { result, rerender } = renderHook(
      (props: {
        businessId: number;
        tableCode: string;
        mode: "ordering" | "concierge";
        language: string;
      }) => useAiWaiterSession(props),
      {
        initialProps: {
          businessId: 7,
          tableCode: "T1",
          mode: "ordering",
          language: "en",
        } as UseAiWaiterSessionOptions,
      },
    );

    await act(async () => void (await result.current.ensureSession()));
    expect(mockedCreate).toHaveBeenNthCalledWith(1, 7, {
      table_code: "T1",
      mode: "ordering",
      language: "en",
    });

    rerender({
      businessId: 7,
      tableCode: "T1",
      mode: "ordering",
      language: "es-AR",
    });
    await act(async () => void (await result.current.ensureSession()));
    expect(result.current.sessionToken).toBe("token-es-ar");
    expect(mockedCreate).toHaveBeenNthCalledWith(2, 7, {
      table_code: "T1",
      mode: "ordering",
      language: "es-AR",
      replace_session_token: "token-en",
    });

    rerender({
      businessId: 7,
      tableCode: "T2",
      mode: "ordering",
      language: "es-AR",
    });
    await act(async () => void (await result.current.ensureSession()));
    expect(result.current.sessionToken).toBe("token-table");

    rerender({
      businessId: 7,
      tableCode: "T2",
      mode: "concierge",
      language: "es-AR",
    });
    await act(async () => void (await result.current.ensureSession()));
    expect(result.current.sessionToken).toBe("token-mode");

    rerender({
      businessId: 8,
      tableCode: "T2",
      mode: "concierge",
      language: "es-AR",
    });
    await act(async () => void (await result.current.ensureSession()));
    expect(result.current.sessionToken).toBe("token-business");
    expect(mockedCreate).toHaveBeenCalledTimes(5);
  });

  it("treats a blank language as en so hydrating the default locale does not replace the session", async () => {
    mockedCreate.mockResolvedValueOnce(session("token-en"));
    const { result, rerender } = renderHook(
      ({ language }: { language: string }) =>
        useAiWaiterSession({
          businessId: 7,
          tableCode: "T1",
          mode: "ordering",
          language,
        }),
      { initialProps: { language: "" } },
    );

    await act(async () => void (await result.current.ensureSession()));
    rerender({ language: "en" });
    await expect(result.current.ensureSession()).resolves.toBe("token-en");
    expect(mockedCreate).toHaveBeenCalledTimes(1);
  });

  it("recreateSession always mints a fresh token for the same scope", async () => {
    mockedCreate
      .mockResolvedValueOnce(session("token-1"))
      .mockResolvedValueOnce(session("token-2"));
    const { result } = renderHook(() =>
      useAiWaiterSession({
        businessId: 7,
        tableCode: "T1",
        mode: "ordering",
        language: "en",
      }),
    );

    await act(async () => void (await result.current.ensureSession()));
    await act(async () => void (await result.current.recreateSession()));

    expect(result.current.sessionToken).toBe("token-2");
    expect(mockedCreate).toHaveBeenCalledTimes(2);
    expect(mockedCreate).toHaveBeenNthCalledWith(2, 7, {
      table_code: "T1",
      mode: "ordering",
      language: "en",
      replace_session_token: "token-1",
    });
  });

  it("does not auto-retry ensureSession after a rate-limited create", async () => {
    mockedCreate.mockRejectedValue({
      response: { status: 429, data: { code: "RATE_LIMITED" } },
    });
    const { result } = renderHook(() =>
      useAiWaiterSession({
        businessId: 7,
        tableCode: "T1",
        mode: "ordering",
        language: "en",
      }),
    );

    await act(async () => {
      await expect(result.current.ensureSession()).rejects.toMatchObject({
        response: { status: 429 },
      });
    });
    await act(async () => {
      await expect(result.current.ensureSession()).rejects.toMatchObject({
        response: { status: 429 },
      });
    });
    expect(mockedCreate).toHaveBeenCalledTimes(1);
  });

  it("lets an explicit recreate retry after a rate-limited create", async () => {
    mockedCreate
      .mockRejectedValueOnce({
        response: { status: 429, data: { code: "RATE_LIMITED" } },
      })
      .mockResolvedValueOnce(session("token-after-limit"));
    const { result } = renderHook(() =>
      useAiWaiterSession({
        businessId: 7,
        tableCode: "T1",
        mode: "ordering",
        language: "en",
      }),
    );

    await act(async () => {
      await expect(result.current.ensureSession()).rejects.toMatchObject({
        response: { status: 429 },
      });
    });
    await act(async () => void (await result.current.recreateSession()));
    expect(result.current.sessionToken).toBe("token-after-limit");
    expect(mockedCreate).toHaveBeenCalledTimes(2);
  });

  it("clears a failed create so ensureSession can retry", async () => {
    mockedCreate
      .mockRejectedValueOnce(new Error("offline"))
      .mockResolvedValueOnce(session("token-after-retry"));
    const { result } = renderHook(() =>
      useAiWaiterSession({
        businessId: 7,
        tableCode: "T1",
        mode: "ordering",
        language: "en",
      }),
    );

    await expect(result.current.ensureSession()).rejects.toThrow("offline");
    await act(async () => void (await result.current.ensureSession()));
    expect(result.current.sessionToken).toBe("token-after-retry");
    expect(mockedCreate).toHaveBeenCalledTimes(2);
  });

  it("makes recreation authoritative and shares its fresh in-flight request", async () => {
    const oldPending = deferred<ReturnType<typeof session>>();
    const freshPending = deferred<ReturnType<typeof session>>();
    mockedCreate
      .mockReturnValueOnce(oldPending.promise)
      .mockReturnValueOnce(freshPending.promise);
    const { result } = renderHook(() =>
      useAiWaiterSession({
        businessId: 7,
        tableCode: "T1",
        mode: "ordering",
        language: "en",
      }),
    );

    let old!: Promise<string>;
    let recreated!: Promise<string>;
    let concurrentEnsure!: Promise<string>;
    let concurrentRecreate!: Promise<string>;
    act(() => {
      old = result.current.ensureSession();
      recreated = result.current.recreateSession();
      concurrentEnsure = result.current.ensureSession();
      concurrentRecreate = result.current.recreateSession();
    });
    const oldRejected = expect(old).rejects.toThrow("ai_waiter_session_stale");
    expect(mockedCreate).toHaveBeenCalledTimes(1);

    await act(async () => oldPending.resolve(session("old-token")));
    await oldRejected;
    await act(async () => {
      await Promise.resolve();
      await Promise.resolve();
    });
    expect(mockedCreate).toHaveBeenCalledTimes(2);
    await act(async () => freshPending.resolve(session("fresh-token")));
    await expect(recreated).resolves.toBe("fresh-token");
    await expect(concurrentEnsure).resolves.toBe("fresh-token");
    await expect(concurrentRecreate).resolves.toBe("fresh-token");
    expect(result.current.sessionToken).toBe("fresh-token");
  });

  it("serializes session creation across unmount/remount so the current cookie response lands last", async () => {
    const oldPending = deferred<ReturnType<typeof session>>();
    const freshPending = deferred<ReturnType<typeof session>>();
    mockedCreate
      .mockReturnValueOnce(oldPending.promise)
      .mockReturnValueOnce(freshPending.promise);
    const oldHook = renderHook(() =>
      useAiWaiterSession({
        businessId: 7,
        tableCode: "T1",
        mode: "ordering",
        language: "en",
      }),
    );
    const old = oldHook.result.current.ensureSession();
    const oldRejected = expect(old).rejects.toThrow("ai_waiter_session_stale");
    oldHook.unmount();

    const newHook = renderHook(() =>
      useAiWaiterSession({
        businessId: 7,
        tableCode: "T2",
        mode: "ordering",
        language: "en",
      }),
    );
    const fresh = newHook.result.current.ensureSession();
    expect(mockedCreate).toHaveBeenCalledTimes(1);

    await act(async () => oldPending.resolve(session("old-cookie")));
    await oldRejected;
    await act(async () => {
      await Promise.resolve();
      await Promise.resolve();
    });
    expect(mockedCreate).toHaveBeenCalledTimes(2);
    await act(async () => freshPending.resolve(session("fresh-cookie")));
    await expect(fresh).resolves.toBe("fresh-cookie");
    expect(newHook.result.current.sessionToken).toBe("fresh-cookie");
  });

  it("never exposes the previous scope token during the next scope render", async () => {
    mockedCreate.mockResolvedValueOnce(session("scope-one"));
    const observed: Array<string | null> = [];
    const { result, rerender } = renderHook(
      ({ tableCode }: { tableCode: string }) => {
        const value = useAiWaiterSession({
          businessId: 7,
          tableCode,
          mode: "ordering",
          language: "en",
        });
        observed.push(value.sessionToken);
        return value;
      },
      { initialProps: { tableCode: "T1" } },
    );
    await act(async () => void (await result.current.ensureSession()));
    expect(result.current.sessionToken).toBe("scope-one");

    rerender({ tableCode: "T2" });
    expect(observed.at(-1)).toBeNull();
    expect(result.current.sessionToken).toBeNull();
  });

  it("rejects and ignores a create result after scope change or unmount", async () => {
    const scopePending = deferred<ReturnType<typeof session>>();
    const unmountPending = deferred<ReturnType<typeof session>>();
    mockedCreate
      .mockReturnValueOnce(scopePending.promise)
      .mockReturnValueOnce(unmountPending.promise);
    const { result, rerender, unmount } = renderHook(
      ({ tableCode }: { tableCode: string }) =>
        useAiWaiterSession({
          businessId: 7,
          tableCode,
          mode: "ordering",
          language: "en",
        }),
      { initialProps: { tableCode: "T1" } },
    );

    let staleScope!: Promise<string>;
    act(() => {
      staleScope = result.current.ensureSession();
    });
    rerender({ tableCode: "T2" });
    const staleScopeRejected = expect(staleScope).rejects.toThrow(
      "ai_waiter_session_stale",
    );
    await act(async () => scopePending.resolve(session("wrong-scope")));
    await staleScopeRejected;
    expect(result.current.sessionToken).toBeNull();

    let staleUnmount!: Promise<string>;
    act(() => {
      staleUnmount = result.current.ensureSession();
    });
    const staleUnmountRejected = expect(staleUnmount).rejects.toThrow(
      "ai_waiter_session_stale",
    );
    unmount();
    await act(async () => unmountPending.resolve(session("after-unmount")));
    await staleUnmountRejected;
  });
});
