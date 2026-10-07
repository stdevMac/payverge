export type PendingCartIntent = {
  version: 1;
  tableCode: string;
  sessionId: string;
  nonce: string;
  expiresAt: number;
  menuItemId?: string;
  bundleId?: number;
  quantity: number;
  notes?: string;
};

type PendingCartIntentError =
  | "invalid_intent"
  | "invalid_scope"
  | "not_found"
  | "corrupt"
  | "expired"
  | "table_mismatch"
  | "session_mismatch"
  | "replayed"
  | "receipt_capacity"
  | "storage_unavailable";

export type PendingCartWriteResult =
  | { ok: true }
  | { ok: false; error: PendingCartIntentError };

export type PendingCartConsumeResult =
  | { ok: true; intent: PendingCartIntent }
  | { ok: false; error: PendingCartIntentError };

export interface PendingCartStorage {
  getItem(key: string): string | null;
  setItem(key: string, value: string): void;
  removeItem(key: string): void;
}

export interface PendingCartStorageOptions {
  now?: number;
  storage?: PendingCartStorage;
  randomUUID?: () => string;
}

export type PendingCartSessionBindingResult =
  | { ok: true; sessionId: string }
  | {
      ok: false;
      error: "invalid_scope" | "entropy_unavailable" | "storage_unavailable";
    };

type ConsumedNonceReceipt = {
  version: 1;
  entries: Array<{ sessionId: string; nonce: string; expiresAt: number }>;
};

const MAX_TTL_MS = 120_000;
const MAX_CONSUMED_NONCES = 32;
const RAW_TABLE_CODE_RE = /^[A-Za-z0-9][A-Za-z0-9_-]{0,62}$/;
const UUID_RE =
  /^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$/;

export const isPendingCartUUID = (value: unknown): value is string =>
  typeof value === "string" && UUID_RE.test(value);
const INTENT_FIELDS = new Set([
  "version",
  "tableCode",
  "sessionId",
  "nonce",
  "expiresAt",
  "menuItemId",
  "bundleId",
  "quantity",
  "notes",
]);

const canonicalTableCode = (tableCode: unknown): string | null => {
  if (typeof tableCode !== "string") return null;
  const trimmed = tableCode.trim();
  if (!RAW_TABLE_CODE_RE.test(trimmed)) return null;
  return trimmed.toUpperCase();
};

export const canonicalPendingCartTableCode = (
  tableCode: unknown,
): string | null => canonicalTableCode(tableCode);

export const pendingCartKey = (tableCode: string): string => {
  const canonical = canonicalTableCode(tableCode);
  if (canonical === null) throw new Error("invalid_table_code");
  return `payverge_pending_cart_v1:${canonical}`;
};

const consumedNonceKey = (tableCode: string): string =>
  `${pendingCartKey(tableCode)}:consumed`;

const pendingCartSessionBindingKey = (tableCode: string): string =>
  `payverge_pending_cart_session_v1:${tableCode}`;

const containsUnsafeUnicode = (value: string): boolean =>
  /[\p{Cc}\p{Cf}]/u.test(value);

const safeOpaqueString = (
  value: unknown,
  maxCodePoints: number,
): value is string =>
  typeof value === "string" &&
  value.length > 0 &&
  value === value.trim() &&
  Array.from(value).length <= maxCodePoints &&
  !containsUnsafeUnicode(value);

const safeNotes = (value: unknown): value is string | undefined =>
  value === undefined ||
  (typeof value === "string" &&
    Array.from(value).length <= 200 &&
    !containsUnsafeUnicode(value));

const safeNow = (value: unknown): value is number =>
  typeof value === "number" && Number.isSafeInteger(value) && value >= 0;

const safeBundleID = (value: unknown): value is number =>
  typeof value === "number" && Number.isSafeInteger(value) && value > 0;

const safeQuantity = (value: unknown): value is number =>
  typeof value === "number" &&
  Number.isInteger(value) &&
  value >= 1 &&
  value <= 20;

const isRecord = (value: unknown): value is Record<string, unknown> =>
  typeof value === "object" && value !== null && !Array.isArray(value);

const canonicalizeIntent = (
  value: unknown,
  now: number,
): PendingCartIntent | null => {
  if (
    !isRecord(value) ||
    Object.keys(value).some((key) => !INTENT_FIELDS.has(key))
  ) {
    return null;
  }
  if (value.version !== 1 || typeof value.tableCode !== "string") return null;

  const tableCode = canonicalTableCode(value.tableCode);
  if (tableCode === null) return null;
  if (
    !safeOpaqueString(value.sessionId, 512) ||
    !safeOpaqueString(value.nonce, 200) ||
    !safeNow(value.expiresAt) ||
    value.expiresAt <= now ||
    value.expiresAt > now + MAX_TTL_MS ||
    !safeQuantity(value.quantity) ||
    !safeNotes(value.notes)
  ) {
    return null;
  }

  const hasMenuItem = value.menuItemId !== undefined;
  const hasBundle = value.bundleId !== undefined;
  if (hasMenuItem === hasBundle) return null;
  if (
    hasMenuItem &&
    (typeof value.menuItemId !== "string" ||
      value.menuItemId !== value.menuItemId.trim() ||
      !UUID_RE.test(value.menuItemId))
  ) {
    return null;
  }
  if (hasBundle && !safeBundleID(value.bundleId)) return null;

  return {
    version: 1,
    tableCode,
    sessionId: value.sessionId,
    nonce: value.nonce,
    expiresAt: value.expiresAt,
    ...(hasMenuItem ? { menuItemId: value.menuItemId as string } : {}),
    ...(hasBundle ? { bundleId: value.bundleId as number } : {}),
    quantity: value.quantity,
    ...(value.notes === undefined ? {} : { notes: value.notes }),
  };
};

const resolveStorage = (
  provided?: PendingCartStorage,
): PendingCartStorage | null => {
  if (provided !== undefined) return provided;
  try {
    return typeof window === "undefined" ? null : window.sessionStorage;
  } catch {
    return null;
  }
};

/**
 * Returns a non-secret, per-tab continuity binding for one table route.
 * This value is deliberately separate from the memory-only AI Waiter bearer.
 */
export function getOrCreatePendingCartSessionBinding(
  tableCodeInput: string,
  options: PendingCartStorageOptions = {},
): PendingCartSessionBindingResult {
  const tableCode = canonicalTableCode(tableCodeInput);
  if (tableCode === null) return { ok: false, error: "invalid_scope" };
  const storage = resolveStorage(options.storage);
  if (storage === null) return { ok: false, error: "storage_unavailable" };
  const key = pendingCartSessionBindingKey(tableCode);

  let existing: string | null;
  try {
    existing = storage.getItem(key);
  } catch {
    return { ok: false, error: "storage_unavailable" };
  }
  if (existing !== null) {
    if (isPendingCartUUID(existing)) return { ok: true, sessionId: existing };
    try {
      storage.removeItem(key);
    } catch {
      return { ok: false, error: "storage_unavailable" };
    }
  }

  const randomUUID =
    options.randomUUID ??
    (typeof globalThis.crypto?.randomUUID === "function"
      ? globalThis.crypto.randomUUID.bind(globalThis.crypto)
      : null);
  if (randomUUID === null) {
    return { ok: false, error: "entropy_unavailable" };
  }

  let sessionId: string;
  try {
    sessionId = randomUUID();
  } catch {
    return { ok: false, error: "entropy_unavailable" };
  }
  if (!isPendingCartUUID(sessionId)) {
    return { ok: false, error: "entropy_unavailable" };
  }
  try {
    storage.setItem(key, sessionId);
  } catch {
    return { ok: false, error: "storage_unavailable" };
  }
  return { ok: true, sessionId };
}

const parseReceipt = (
  raw: string | null,
  now: number,
): ConsumedNonceReceipt | null => {
  if (raw === null) return { version: 1, entries: [] };
  let value: unknown;
  try {
    value = JSON.parse(raw);
  } catch {
    return null;
  }
  if (
    !isRecord(value) ||
    Object.keys(value).some((key) => key !== "version" && key !== "entries") ||
    value.version !== 1 ||
    !Array.isArray(value.entries) ||
    value.entries.length > MAX_CONSUMED_NONCES
  ) {
    return null;
  }

  const entries: ConsumedNonceReceipt["entries"] = [];
  const seen = new Set<string>();
  for (const entry of value.entries) {
    if (
      !isRecord(entry) ||
      Object.keys(entry).some(
        (key) => key !== "sessionId" && key !== "nonce" && key !== "expiresAt",
      ) ||
      !safeOpaqueString(entry.sessionId, 512) ||
      !safeOpaqueString(entry.nonce, 200) ||
      !safeNow(entry.expiresAt) ||
      entry.expiresAt > now + MAX_TTL_MS
    ) {
      return null;
    }
    if (entry.expiresAt <= now) continue;
    const identity = `${entry.sessionId}\u0000${entry.nonce}`;
    if (seen.has(identity)) return null;
    seen.add(identity);
    entries.push({
      sessionId: entry.sessionId,
      nonce: entry.nonce,
      expiresAt: entry.expiresAt,
    });
  }
  return { version: 1, entries };
};

const removeOrStorageError = (
  storage: PendingCartStorage,
  key: string,
  error: PendingCartIntentError,
): PendingCartConsumeResult => {
  try {
    storage.removeItem(key);
    return { ok: false, error };
  } catch {
    return { ok: false, error: "storage_unavailable" };
  }
};

export function writePendingCartIntent(
  value: PendingCartIntent,
  options: PendingCartStorageOptions = {},
): PendingCartWriteResult {
  const now = options.now ?? Date.now();
  const storage = resolveStorage(options.storage);
  if (!safeNow(now) || storage === null) {
    return { ok: false, error: "storage_unavailable" };
  }
  const intent = canonicalizeIntent(value, now);
  if (intent === null) return { ok: false, error: "invalid_intent" };

  const receiptKey = consumedNonceKey(intent.tableCode);
  let receipt: ConsumedNonceReceipt | null;
  try {
    receipt = parseReceipt(storage.getItem(receiptKey), now);
  } catch {
    return { ok: false, error: "storage_unavailable" };
  }
  if (receipt === null) {
    try {
      storage.removeItem(receiptKey);
    } catch {
      return { ok: false, error: "storage_unavailable" };
    }
    return { ok: false, error: "corrupt" };
  }
  if (
    receipt.entries.some(
      ({ sessionId, nonce }) =>
        sessionId === intent.sessionId && nonce === intent.nonce,
    )
  ) {
    return { ok: false, error: "replayed" };
  }

  try {
    storage.setItem(pendingCartKey(intent.tableCode), JSON.stringify(intent));
    return { ok: true };
  } catch {
    return { ok: false, error: "storage_unavailable" };
  }
}

export function consumePendingCartIntent(
  tableCodeInput: string,
  sessionId: string,
  options: PendingCartStorageOptions = {},
): PendingCartConsumeResult {
  const now = options.now ?? Date.now();
  const storage = resolveStorage(options.storage);
  if (
    !safeNow(now) ||
    storage === null ||
    typeof tableCodeInput !== "string" ||
    !safeOpaqueString(sessionId, 512)
  ) {
    return { ok: false, error: "invalid_scope" };
  }
  const tableCode = canonicalTableCode(tableCodeInput);
  if (tableCode === null) {
    return { ok: false, error: "invalid_scope" };
  }
  const key = pendingCartKey(tableCode);

  let raw: string | null;
  try {
    raw = storage.getItem(key);
  } catch {
    return { ok: false, error: "storage_unavailable" };
  }
  if (raw === null) return { ok: false, error: "not_found" };

  let decoded: unknown;
  try {
    decoded = JSON.parse(raw);
  } catch {
    return removeOrStorageError(storage, key, "corrupt");
  }
  if (!isRecord(decoded)) {
    return removeOrStorageError(storage, key, "corrupt");
  }
  if (
    decoded.version !== 1 ||
    typeof decoded.expiresAt !== "number" ||
    !Number.isSafeInteger(decoded.expiresAt)
  ) {
    return removeOrStorageError(storage, key, "corrupt");
  }
  if (decoded.expiresAt <= now) {
    return removeOrStorageError(storage, key, "expired");
  }
  const intent = canonicalizeIntent(decoded, now);
  if (intent === null) {
    return removeOrStorageError(storage, key, "corrupt");
  }
  if (intent.tableCode !== tableCode) {
    return removeOrStorageError(storage, key, "table_mismatch");
  }
  if (intent.sessionId !== sessionId) {
    return removeOrStorageError(storage, key, "session_mismatch");
  }

  const receiptKey = consumedNonceKey(tableCode);
  let receipt: ConsumedNonceReceipt | null;
  try {
    receipt = parseReceipt(storage.getItem(receiptKey), now);
  } catch {
    return { ok: false, error: "storage_unavailable" };
  }
  if (receipt === null) {
    try {
      storage.removeItem(receiptKey);
    } catch {
      return { ok: false, error: "storage_unavailable" };
    }
    return removeOrStorageError(storage, key, "corrupt");
  }
  if (
    receipt.entries.some(
      ({ sessionId: consumedSession, nonce }) =>
        consumedSession === sessionId && nonce === intent.nonce,
    )
  ) {
    return removeOrStorageError(storage, key, "replayed");
  }

  if (receipt.entries.length >= MAX_CONSUMED_NONCES) {
    return removeOrStorageError(storage, key, "receipt_capacity");
  }

  const entries = [
    ...receipt.entries,
    {
      sessionId: intent.sessionId,
      nonce: intent.nonce,
      expiresAt: intent.expiresAt,
    },
  ];
  try {
    storage.setItem(receiptKey, JSON.stringify({ version: 1, entries }));
    storage.removeItem(key);
  } catch {
    return { ok: false, error: "storage_unavailable" };
  }
  return { ok: true, intent };
}
