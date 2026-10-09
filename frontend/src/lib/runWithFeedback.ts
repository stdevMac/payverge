import toast from "react-hot-toast";
import { isOfflineQueuedError } from "@/api/tools/instance";

/**
 * Wraps a user-triggered async action with the feedback every mutation owes
 * the user: success/error toast + busy state. Fixes the audit's
 * silent-failure class (catch blocks that only console.error).
 *
 * Defaults to react-hot-toast. Components on the custom ToastContext pass
 * `notifier: { success: showSuccess, error: showError }`.
 *
 * Offline queue: when the axios interceptor rejects with OfflineQueuedError
 * (and already toasts "saved, will sync"), we do NOT also fire the hard-error
 * toast — that double-message was the audit offline double-toast failure.
 *
 * Returns `undefined` on failure so callers can keep modals open / skip
 * state updates; pass `rethrow: true` to also propagate.
 */
interface FeedbackNotifier {
  success: (message: string) => void;
  error: (message: string) => void;
}

export interface RunWithFeedbackOptions {
  /** Pre-translated success message. Omit for silent success. */
  success?: string;
  /** Pre-translated error message. Required: failures must be visible. */
  error: string;
  /** Busy/submitting setter — drives Button isLoading / isDisabled. */
  setBusy?: (busy: boolean) => void;
  notifier?: FeedbackNotifier;
  rethrow?: boolean;
}

const defaultNotifier: FeedbackNotifier = {
  success: (m) => toast.success(m),
  error: (m) => toast.error(m),
};

export async function runWithFeedback<T>(
  action: () => Promise<T>,
  opts: RunWithFeedbackOptions,
): Promise<T | undefined> {
  const notifier = opts.notifier ?? defaultNotifier;
  opts.setBusy?.(true);
  try {
    const result = await action();
    if (opts.success) notifier.success(opts.success);
    return result;
  } catch (err) {
    if (isOfflineQueuedError(err)) {
      // Interceptor already informed the operator; treat as pending, not failed.
      if (opts.rethrow) throw err;
      return undefined;
    }
    console.error(opts.error, err);
    notifier.error(opts.error);
    if (opts.rethrow) throw err;
    return undefined;
  } finally {
    opts.setBusy?.(false);
  }
}
