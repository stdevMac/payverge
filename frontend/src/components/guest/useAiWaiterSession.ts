import {
  useCallback,
  useEffect,
  useLayoutEffect,
  useRef,
  useState,
} from "react";
import { createAiWaiterSession } from "@/api/aiWaiter";

export interface UseAiWaiterSessionOptions {
  businessId: number;
  tableCode: string;
  mode: "ordering" | "concierge";
  language: string;
}

export interface UseAiWaiterSessionResult {
  sessionToken: string | null;
  ensureSession: () => Promise<string>;
  recreateSession: () => Promise<string>;
}

const staleSessionError = (): Error => new Error("ai_waiter_session_stale");

const validSessionToken = (value: unknown): value is string =>
  typeof value === "string" &&
  value.length > 0 &&
  value.length <= 4096 &&
  value.trim() === value;

// Session creation mutates one HttpOnly cookie shared by every hook instance.
// Serialize requests so a stale response can never land after the current
// scope and overwrite its cookie. The module tail resolves to void and retains
// no token or response data.
let sessionCreateTail: Promise<void> | null = null;

const enqueueSessionCreate = <T>(start: () => Promise<T>): Promise<T> => {
  const predecessor = sessionCreateTail;
  const run = predecessor ? predecessor.then(start, start) : start();
  const settled = run.then(
    () => undefined,
    () => undefined,
  );
  sessionCreateTail = settled;
  void settled.then(() => {
    if (sessionCreateTail === settled) sessionCreateTail = null;
  });
  return run;
};

const isRateLimitedCreate = (error: unknown): boolean => {
  const status = (error as { response?: { status?: number } })?.response
    ?.status;
  return status === 429;
};

const rateLimitedCreateError = (): {
  response: { status: number; data: { code: string } };
} => ({
  response: { status: 429, data: { code: "RATE_LIMITED" } },
});

/**
 * Owns the guest waiter's in-memory bearer token. The server cookie remains the
 * durable credential; this hook never writes the replayable token to browser
 * storage. Session identity is business + table + mode + locale. A locale
 * switch mints a replacement conversation and archives the predecessor.
 */
export function useAiWaiterSession({
  businessId,
  tableCode,
  mode,
  language,
}: UseAiWaiterSessionOptions): UseAiWaiterSessionResult {
  const scopeKey = `${businessId}\u0000${tableCode}\u0000${mode}\u0000${language || "en"}`;
  const [sessionState, setSessionState] = useState<{
    scopeKey: string;
    token: string | null;
  }>(() => ({ scopeKey, token: null }));
  const tokenRef = useRef<{ scopeKey: string; token: string | null }>({
    scopeKey,
    token: null,
  });
  const activeScopeRef = useRef(scopeKey);
  const generationRef = useRef(0);
  const mountedRef = useRef(false);
  const inFlightRef = useRef<{
    promise: Promise<string>;
    kind: "ensure" | "recreate";
  } | null>(null);
  const predecessorTokenRef = useRef<string | null>(null);
  const createBlockedRef = useRef(false);

  useLayoutEffect(() => {
    mountedRef.current = true;
    return () => {
      mountedRef.current = false;
      generationRef.current += 1;
      inFlightRef.current = null;
    };
  }, []);

  useEffect(() => {
    if (activeScopeRef.current === scopeKey) return;
    predecessorTokenRef.current = tokenRef.current.token;
    activeScopeRef.current = scopeKey;
    generationRef.current += 1;
    tokenRef.current = { scopeKey, token: null };
    inFlightRef.current = null;
    createBlockedRef.current = false;
    setSessionState({ scopeKey, token: null });
  }, [scopeKey]);

  const createForCurrentScope = useCallback(async (): Promise<string> => {
    const startedScope = scopeKey;
    const startedGeneration = generationRef.current;
    const created = await enqueueSessionCreate(async () => {
      if (
        !mountedRef.current ||
        activeScopeRef.current !== startedScope ||
        generationRef.current !== startedGeneration
      ) {
        throw staleSessionError();
      }
      const predecessor = predecessorTokenRef.current;
      return createAiWaiterSession(businessId, {
        table_code: tableCode || "",
        mode,
        language: language || "en",
        ...(predecessor ? { replace_session_token: predecessor } : {}),
      });
    });
    if (
      !mountedRef.current ||
      activeScopeRef.current !== startedScope ||
      generationRef.current !== startedGeneration
    ) {
      throw staleSessionError();
    }
    if (!validSessionToken(created?.session_token)) {
      throw new Error("invalid_ai_waiter_session");
    }
    predecessorTokenRef.current = null;
    createBlockedRef.current = false;
    tokenRef.current = { scopeKey: startedScope, token: created.session_token };
    setSessionState({ scopeKey: startedScope, token: created.session_token });
    return created.session_token;
  }, [businessId, language, mode, scopeKey, tableCode]);

  const ensureSession = useCallback(async (): Promise<string> => {
    if (
      tokenRef.current.scopeKey === scopeKey &&
      tokenRef.current.token !== null
    ) {
      return tokenRef.current.token;
    }
    if (inFlightRef.current) return inFlightRef.current.promise;
    if (createBlockedRef.current) {
      throw rateLimitedCreateError();
    }

    const pending = createForCurrentScope();
    inFlightRef.current = { promise: pending, kind: "ensure" };
    try {
      return await pending;
    } catch (error) {
      if (isRateLimitedCreate(error)) {
        createBlockedRef.current = true;
      }
      throw error;
    } finally {
      if (inFlightRef.current?.promise === pending) inFlightRef.current = null;
    }
  }, [createForCurrentScope, scopeKey]);

  const recreateSession = useCallback(async (): Promise<string> => {
    if (inFlightRef.current?.kind === "recreate") {
      return inFlightRef.current.promise;
    }
    generationRef.current += 1;
    predecessorTokenRef.current = tokenRef.current.token;
    tokenRef.current = { scopeKey, token: null };
    setSessionState({ scopeKey, token: null });
    inFlightRef.current = null;
    createBlockedRef.current = false;
    const pending = createForCurrentScope();
    inFlightRef.current = { promise: pending, kind: "recreate" };
    try {
      return await pending;
    } catch (error) {
      if (isRateLimitedCreate(error)) {
        createBlockedRef.current = true;
      }
      throw error;
    } finally {
      if (inFlightRef.current?.promise === pending) inFlightRef.current = null;
    }
  }, [createForCurrentScope, scopeKey]);

  return {
    sessionToken:
      sessionState.scopeKey === scopeKey ? sessionState.token : null,
    ensureSession,
    recreateSession,
  };
}
