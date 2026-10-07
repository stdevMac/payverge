import axios from "axios";

/**
 * True when an error is the result of an aborted request (an `AbortController`
 * firing on `useEffect` cleanup / a superseded poll) rather than a real
 * failure.
 *
 * axios raises a `CanceledError` (code `ERR_CANCELED`) when its request is
 * aborted via `{ signal }`; the native fetch path raises a DOMException named
 * `AbortError`. Call-site catch handlers should early-return on this so an
 * abort never surfaces as a toast or console error.
 */
export function isAbortError(error: unknown): boolean {
  if (axios.isCancel(error)) return true;
  if (typeof error !== "object" || error === null) return false;
  const err = error as { name?: unknown; code?: unknown; message?: unknown };
  if (err.name === "AbortError" || err.name === "CanceledError") return true;
  if (err.code === "ERR_CANCELED") return true;
  return false;
}
