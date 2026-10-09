"use client";

import React, {
  createContext,
  useCallback,
  useContext,
  useMemo,
  useRef,
  useState,
} from "react";
import ManagerPinModal from "./ManagerPinModal";
import { isRawValidatorDump } from "@/utils/apiError";

/**
 * ManagerPinProvider / useManagerPinPrompt — IMP-14
 *
 * Pattern: the provider renders a single, shared <ManagerPinModal /> at the
 * root and exposes `promptForPin()` to any descendant. Callers wrap a backend
 * request in `withManagerPin()` which:
 *   1. Issues the request.
 *   2. If the backend returns a 403 with `code: "pin_required"` or
 *      `"pin_invalid"`, opens the modal and awaits a PIN.
 *   3. Retries the request with the supplied `X-Manager-Pin` header.
 *   4. Surfaces `pin_invalid` inline in the modal so the operator can fix
 *      one digit instead of starting over.
 *
 * Anything that needs a PIN should call `withManagerPin()` rather than
 * issuing the request directly. The default copy is English-only because
 * the IMP-14 spec defers i18n — wire translations in a follow-up.
 */

type PromptOptions = {
  title?: string;
  description?: string;
};

type ManagerPinContextValue = {
  promptForPin: (options?: PromptOptions) => Promise<string>;
};

const ManagerPinContext = createContext<ManagerPinContextValue | null>(null);

type PendingResolver = {
  resolve: (pin: string) => void;
  reject: (reason?: unknown) => void;
  /**
   * When set, the modal forwards submitted PINs into this verifier instead
   * of resolving immediately. The verifier returns void on success (closing
   * the modal) or throws `{ message }` to surface an inline error.
   */
  verify?: (pin: string) => Promise<void>;
  options: PromptOptions;
};

export function ManagerPinProvider({
  children,
}: {
  children: React.ReactNode;
}) {
  const [isOpen, setIsOpen] = useState(false);
  const [activeOptions, setActiveOptions] = useState<PromptOptions>({});
  const pendingRef = useRef<PendingResolver | null>(null);

  const closeAndClear = useCallback(() => {
    setIsOpen(false);
    pendingRef.current = null;
  }, []);

  const promptForPin = useCallback((options: PromptOptions = {}): Promise<string> => {
    // If a prompt is already in flight, reject the previous one so we don't
    // silently stack two modals. This is an edge case (rapid double-click)
    // but worth defending against.
    if (pendingRef.current) {
      pendingRef.current.reject(new Error("Replaced by another PIN prompt"));
    }
    setActiveOptions(options);
    return new Promise<string>((resolve, reject) => {
      pendingRef.current = { resolve, reject, options };
      setIsOpen(true);
    });
  }, []);

  const handleSubmit = useCallback(async (pin: string) => {
    const pending = pendingRef.current;
    if (!pending) {
      return;
    }
    if (pending.verify) {
      // Verifier-mode: caller supplied a function that retries the original
      // request inline. We rethrow so ManagerPinModal can show the message.
      await pending.verify(pin);
      // Verifier succeeded — close + resolve with the accepted PIN so the
      // caller's outer code can keep going.
      pending.resolve(pin);
      closeAndClear();
      return;
    }
    pending.resolve(pin);
    closeAndClear();
  }, [closeAndClear]);

  const handleCancel = useCallback(() => {
    const pending = pendingRef.current;
    if (pending) {
      pending.reject(new Error("PIN entry cancelled"));
    }
    closeAndClear();
  }, [closeAndClear]);

  // Expose the verifier-mode setter via context so withManagerPin can swap
  // it in before opening the modal. We don't export the raw setter — callers
  // go through `withManagerPin` (below) which is the documented API.
  const promptWithVerifier = useCallback(
    (verify: (pin: string) => Promise<void>, options: PromptOptions = {}): Promise<string> => {
      if (pendingRef.current) {
        pendingRef.current.reject(new Error("Replaced by another PIN prompt"));
      }
      setActiveOptions(options);
      return new Promise<string>((resolve, reject) => {
        pendingRef.current = { resolve, reject, verify, options };
        setIsOpen(true);
      });
    },
    [],
  );

  const contextValue = useMemo<ManagerPinContextValue>(
    () => ({ promptForPin }),
    [promptForPin],
  );

  // Stash promptWithVerifier on the context object too — typed as part of
  // the internal hook below so it isn't part of the public API.
  (contextValue as { __promptWithVerifier?: typeof promptWithVerifier }).__promptWithVerifier =
    promptWithVerifier;

  return (
    <ManagerPinContext.Provider value={contextValue}>
      {children}
      <ManagerPinModal
        isOpen={isOpen}
        title={activeOptions.title}
        description={activeOptions.description}
        onSubmit={handleSubmit}
        onCancel={handleCancel}
      />
    </ManagerPinContext.Provider>
  );
}

/**
 * RequestFn — a function that performs the actual HTTP call. It MUST accept
 * an optional `pin` argument and forward it as the `X-Manager-Pin` header.
 * Returning the parsed response shape is up to the caller; we just thread
 * the value through.
 */
type RequestFn<T> = (pin?: string) => Promise<T>;

type PinErrorShape = {
  code?: string;
  message?: string;
};

/**
 * Extract the `{ code, error }` shape returned by RequireManagerPIN. We
 * accept both axios-style errors (`error.response.data`) and bare Fetch
 * Responses; payloads outside that shape fall through unchanged.
 * Gin dumps never surface as the modal message (FIND-034 residual).
 */
function extractPinError(error: unknown): PinErrorShape | null {
  if (!error || typeof error !== "object") return null;
  const maybeAxios = error as {
    response?: { status?: number; data?: { code?: string; error?: string } };
  };
  if (maybeAxios.response && maybeAxios.response.status === 403 && maybeAxios.response.data) {
    const code = maybeAxios.response.data.code;
    if (code === "pin_required" || code === "pin_invalid") {
      const raw = maybeAxios.response.data.error;
      const safe =
        typeof raw === "string" &&
        raw.trim().length > 0 &&
        raw.length <= 200 &&
        !isRawValidatorDump(raw)
          ? raw
          : code === "pin_invalid"
            ? "Invalid PIN"
            : undefined;
      return { code, message: safe };
    }
  }
  return null;
}

/**
 * useWithManagerPin — wraps a request that may require a manager PIN.
 *
 * Usage:
 *   const { withManagerPin } = useWithManagerPin();
 *   await withManagerPin(
 *     (pin) => adjustBillItem(billId, itemId, body, { pin }),
 *     { title: "Void item", description: `Voiding ${item.name}` },
 *   );
 *
 * On the first attempt no PIN is sent (`pin === undefined`). If the backend
 * returns 403 pin_required, we open the modal and retry with the entered
 * PIN. If the backend then returns pin_invalid, the modal surfaces an
 * inline error and the operator can correct without restarting the flow.
 */
export function useWithManagerPin() {
  const ctx = useContext(ManagerPinContext) as
    | (ManagerPinContextValue & {
        __promptWithVerifier?: (
          verify: (pin: string) => Promise<void>,
          options?: PromptOptions,
        ) => Promise<string>;
      })
    | null;

  if (!ctx) {
    throw new Error(
      "useWithManagerPin must be used inside <ManagerPinProvider>",
    );
  }

  const withManagerPin = useCallback(
    async <T,>(
      request: RequestFn<T>,
      options: PromptOptions = {},
    ): Promise<T> => {
      try {
        return await request();
      } catch (error) {
        const pinErr = extractPinError(error);
        if (!pinErr) throw error;
        // pin_required (first attempt) and pin_invalid (re-prompt with the
        // last error message inline) both funnel through the same verifier.
        const initialMessage =
          pinErr.code === "pin_invalid" ? pinErr.message ?? "Invalid PIN" : null;
        let result: T | undefined;
        // Wrap the retry in a verifier so the modal stays open on invalid
        // PINs instead of bouncing the operator back to the click that
        // started it.
        const promptWithVerifier = ctx.__promptWithVerifier;
        if (!promptWithVerifier) {
          // Fallback: single attempt via promptForPin.
          const enteredPin = await ctx.promptForPin(options);
          return await request(enteredPin);
        }
        const verify = async (pin: string) => {
          try {
            result = await request(pin);
          } catch (innerErr) {
            const innerPin = extractPinError(innerErr);
            if (innerPin && innerPin.code === "pin_invalid") {
              throw new Error(innerPin.message ?? "Invalid PIN");
            }
            // Non-PIN error: bail out of the modal verifier and let the
            // outer try/catch resurface it.
            throw innerErr;
          }
        };
        // Surface the prior pin_invalid message in the modal description if
        // we have one — purely cosmetic, the modal's own inline error will
        // overwrite on the next bad attempt.
        const promptOpts: PromptOptions = initialMessage
          ? { ...options, description: initialMessage }
          : options;
        await promptWithVerifier(verify, promptOpts);
        if (result === undefined) {
          // Should be unreachable — promptWithVerifier only resolves after
          // the verifier returned a value.
          throw new Error("PIN verification completed without a result");
        }
        return result;
      }
    },
    [ctx],
  );

  return { withManagerPin };
}
