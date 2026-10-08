import { startTokenRefreshTimer } from "../tokenRefresh";

describe("startTokenRefreshTimer", () => {
  beforeEach(() => {
    jest.useFakeTimers();
  });

  afterEach(() => {
    jest.useRealTimers();
  });

  it("calls refreshFn at 12 minutes (no proactive warning fires)", () => {
    const refreshFn = jest.fn().mockResolvedValue(true);

    startTokenRefreshTimer({
      refreshFn,
      onRefreshSuccess: jest.fn(),
      onSessionExpired: jest.fn(),
    });

    // No callbacks should fire before the interval lands.
    jest.advanceTimersByTime(10 * 60 * 1000);
    expect(refreshFn).not.toHaveBeenCalled();

    jest.advanceTimersByTime(2 * 60 * 1000);
    expect(refreshFn).toHaveBeenCalledTimes(1);
  });

  it("calls onRefreshSuccess after successful refresh and reschedules", async () => {
    const onRefreshSuccess = jest.fn();
    const refreshFn = jest.fn().mockResolvedValue(true);

    startTokenRefreshTimer({
      refreshFn,
      onRefreshSuccess,
      onSessionExpired: jest.fn(),
    });

    jest.advanceTimersByTime(12 * 60 * 1000);
    await Promise.resolve();
    await Promise.resolve();

    expect(onRefreshSuccess).toHaveBeenCalledTimes(1);

    // The next cycle should be queued — advance again and verify refreshFn fires twice total.
    jest.advanceTimersByTime(12 * 60 * 1000);
    await Promise.resolve();
    await Promise.resolve();

    expect(refreshFn).toHaveBeenCalledTimes(2);
  });

  // A single failed refresh used to call onSessionExpired AND stop rescheduling
  // — so one transient 429/5xx/network blip at the 12-min tick permanently
  // killed proactive refresh, after which the 15-min cookie silently expired.
  // A transient failure must instead retry on a shorter cadence, not give up.
  it("retries on a shorter cadence after a transient failure instead of giving up", async () => {
    const onSessionExpired = jest.fn();
    const refreshFn = jest.fn().mockResolvedValue(false);

    startTokenRefreshTimer({
      refreshFn,
      onRefreshSuccess: jest.fn(),
      onSessionExpired,
    });

    // First scheduled refresh fails transiently.
    await jest.advanceTimersByTimeAsync(12 * 60 * 1000);
    expect(refreshFn).toHaveBeenCalledTimes(1);
    expect(onSessionExpired).not.toHaveBeenCalled();

    // It must reschedule (sooner than a full interval) and try again rather
    // than latching dead.
    await jest.advanceTimersByTimeAsync(60 * 1000);
    expect(refreshFn).toHaveBeenCalledTimes(2);
    expect(onSessionExpired).not.toHaveBeenCalled();
  });

  it("retries unrecovered rotation after 60 seconds without expiring", async () => {
    const onSessionExpired = jest.fn();
    const onRefreshSuccess = jest.fn();
    const rotatedResult = {
      ok: false as const,
      alreadyRotated: true,
      sessionDead: false,
    };
    const refreshFn = jest
      .fn()
      .mockResolvedValueOnce(rotatedResult)
      .mockResolvedValueOnce(true);

    startTokenRefreshTimer({
      refreshFn,
      onRefreshSuccess,
      onSessionExpired,
    });

    await jest.advanceTimersByTimeAsync(12 * 60 * 1000);
    expect(refreshFn).toHaveBeenCalledTimes(1);
    expect(onSessionExpired).not.toHaveBeenCalled();

    await jest.advanceTimersByTimeAsync(60 * 1000);
    expect(refreshFn).toHaveBeenCalledTimes(2);
    expect(onRefreshSuccess).toHaveBeenCalledTimes(1);
    expect(onSessionExpired).not.toHaveBeenCalled();
  });

  it("never expires repeated failures explicitly classified as non-dead", async () => {
    const onSessionExpired = jest.fn();
    const onRefreshSuccess = jest.fn();
    const refreshFn = jest.fn().mockResolvedValue({
      ok: false,
      sessionDead: false,
    });

    startTokenRefreshTimer({
      refreshFn,
      onRefreshSuccess,
      onSessionExpired,
    });

    await jest.advanceTimersByTimeAsync(12 * 60 * 1000);
    await jest.advanceTimersByTimeAsync(3 * 60 * 1000);

    expect(refreshFn).toHaveBeenCalledTimes(4);
    expect(onRefreshSuccess).not.toHaveBeenCalled();
    expect(onSessionExpired).not.toHaveBeenCalled();
  });

  it("recovers and resumes the normal cadence when a retried refresh succeeds", async () => {
    const onRefreshSuccess = jest.fn();
    const onSessionExpired = jest.fn();
    const refreshFn = jest
      .fn()
      .mockResolvedValueOnce(false)
      .mockResolvedValue(true);

    startTokenRefreshTimer({
      refreshFn,
      onRefreshSuccess,
      onSessionExpired,
    });

    // First attempt fails transiently...
    await jest.advanceTimersByTimeAsync(12 * 60 * 1000);
    // ...the retry succeeds, clearing the failure streak.
    await jest.advanceTimersByTimeAsync(60 * 1000);

    expect(onRefreshSuccess).toHaveBeenCalledTimes(1);
    expect(onSessionExpired).not.toHaveBeenCalled();

    // After recovery the normal 12-min cadence resumes.
    await jest.advanceTimersByTimeAsync(12 * 60 * 1000);
    expect(refreshFn).toHaveBeenCalledTimes(3);
  });

  it("reports expiry only after repeated consecutive failures", async () => {
    const onSessionExpired = jest.fn();
    const refreshFn = jest.fn().mockResolvedValue(false);

    startTokenRefreshTimer({
      refreshFn,
      onRefreshSuccess: jest.fn(),
      onSessionExpired,
    });

    // First scheduled attempt fails — must NOT expire yet.
    await jest.advanceTimersByTimeAsync(12 * 60 * 1000);
    expect(onSessionExpired).not.toHaveBeenCalled();

    // Second consecutive failure — still tolerated.
    await jest.advanceTimersByTimeAsync(60 * 1000);
    expect(onSessionExpired).not.toHaveBeenCalled();

    // Third consecutive failure is a genuinely dead session → surface expiry
    // exactly once and stop retrying.
    await jest.advanceTimersByTimeAsync(60 * 1000);
    expect(onSessionExpired).toHaveBeenCalledTimes(1);

    const callsAtExpiry = refreshFn.mock.calls.length;
    await jest.advanceTimersByTimeAsync(12 * 60 * 1000);
    expect(refreshFn).toHaveBeenCalledTimes(callsAtExpiry);
  });

  it("cleanup function clears the pending refresh", () => {
    const refreshFn = jest.fn();

    const cleanup = startTokenRefreshTimer({
      refreshFn,
      onRefreshSuccess: jest.fn(),
      onSessionExpired: jest.fn(),
    });

    cleanup();

    jest.advanceTimersByTime(20 * 60 * 1000);

    expect(refreshFn).not.toHaveBeenCalled();
  });

  it("retries on the 60s cadence after a transient failure — inside the 90s server replay grace", async () => {
    jest.useFakeTimers();
    try {
      const refreshFn = jest
        .fn<Promise<boolean>, []>()
        .mockResolvedValueOnce(false) // transient failure at the 12-min tick
        .mockResolvedValueOnce(true); // 60s retry succeeds
      const onRefreshSuccess = jest.fn();
      const onSessionExpired = jest.fn();
      const stop = startTokenRefreshTimer({
        refreshFn,
        onRefreshSuccess,
        onSessionExpired,
      });

      await jest.advanceTimersByTimeAsync(12 * 60 * 1000);
      expect(refreshFn).toHaveBeenCalledTimes(1);

      // 59s: retry has NOT fired yet; 60s: it has. 60s < the server's 90s
      // RefreshReuseGracePeriod, so a lost-response retry can never trigger
      // family revocation (P1-9 root cause).
      await jest.advanceTimersByTimeAsync(59 * 1000);
      expect(refreshFn).toHaveBeenCalledTimes(1);
      await jest.advanceTimersByTimeAsync(1000);
      expect(refreshFn).toHaveBeenCalledTimes(2);
      expect(onRefreshSuccess).toHaveBeenCalledTimes(1);
      expect(onSessionExpired).not.toHaveBeenCalled();
      stop();
    } finally {
      jest.useRealTimers();
    }
  });

});
