import { isSafeRedirectUrl } from "@/utils/safeRedirect";

const PREFIX = "payverge:pwa";
const SEVEN_DAYS_MS = 7 * 24 * 60 * 60 * 1000;
const DASHBOARD_PATH = /^\/business\/[^/?#]+\/dashboard$/;

type PwaRoleType = "owner" | "staff";

export interface PwaIdentity {
  key: string;
  roleType: PwaRoleType;
}

export interface PwaAuthIdentityInput {
  isStaffUser: boolean;
  staffId: number | null;
  staffBusinessId: number | null;
  isOAuthUser: boolean;
  oauthUserId: number | null;
  isWeb3User: boolean;
  walletAddress: string | null;
}

export function derivePwaIdentity(
  input: PwaAuthIdentityInput,
): PwaIdentity | null {
  if (input.isStaffUser && input.staffId && input.staffBusinessId) {
    return {
      key: `staff:${input.staffId}:${input.staffBusinessId}`,
      roleType: "staff",
    };
  }

  if (input.isOAuthUser && input.oauthUserId) {
    return { key: `user:${input.oauthUserId}`, roleType: "owner" };
  }

  if (input.isWeb3User && input.walletAddress) {
    return {
      key: `web3:${input.walletAddress.toLowerCase()}`,
      roleType: "owner",
    };
  }

  return null;
}

function key(identity: string, suffix: string): string {
  return `${PREFIX}:${identity}:${suffix}`;
}

function read(
  storage: Storage,
  storageKey: string,
): { value: string | null; ok: boolean } {
  try {
    return { value: storage.getItem(storageKey), ok: true };
  } catch {
    return { value: null, ok: false };
  }
}

function write(storage: Storage, storageKey: string, value: string): boolean {
  try {
    storage.setItem(storageKey, value);
    return true;
  } catch {
    return false;
  }
}

function remove(storage: Storage, storageKey: string): void {
  try {
    storage.removeItem(storageKey);
  } catch {
    return;
  }
}

export function noteDashboardVisit(
  local: Storage,
  session: Storage,
  identity: string,
): boolean {
  const eligibleKey = key(identity, "eligible");
  const sessionKey = key(identity, "dashboard-session");
  const eligibility = read(local, eligibleKey);
  const sessionVisit = read(session, sessionKey);

  if (!eligibility.ok || !sessionVisit.ok) return false;
  if (!write(session, sessionKey, "1") || !write(local, eligibleKey, "1")) {
    return false;
  }

  return eligibility.value === "1" && sessionVisit.value !== "1";
}

/**
 * Returns false when callers must suppress the card because session state cannot persist.
 */
export function markCardShown(session: Storage, identity: string): boolean {
  return write(session, key(identity, "card-shown"), "1");
}

export function wasCardShown(session: Storage, identity: string): boolean {
  const result = read(session, key(identity, "card-shown"));
  return !result.ok || result.value === "1";
}

export function snoozeForSevenDays(
  local: Storage,
  identity: string,
  now = Date.now(),
): void {
  write(local, key(identity, "snoozed-until"), String(now + SEVEN_DAYS_MS));
}

export function isSnoozed(
  local: Storage,
  identity: string,
  now = Date.now(),
): boolean {
  const value = Number(read(local, key(identity, "snoozed-until")).value ?? 0);
  return Number.isFinite(value) && value > now;
}

export function markInstallComplete(local: Storage, identity: string): void {
  write(local, key(identity, "complete"), "1");
}

export function isInstallComplete(local: Storage, identity: string): boolean {
  return read(local, key(identity, "complete")).value === "1";
}

export function saveLastDashboardPath(
  local: Storage,
  identity: string,
  path: string,
): boolean {
  if (!isSafeRedirectUrl(path) || !DASHBOARD_PATH.test(path)) return false;
  return write(local, key(identity, "last-dashboard"), path);
}

export function getLastDashboardPath(
  local: Storage,
  identity: string,
): string | null {
  const path = read(local, key(identity, "last-dashboard")).value;
  return path && isSafeRedirectUrl(path) && DASHBOARD_PATH.test(path)
    ? path
    : null;
}

export function clearLastDashboardPath(local: Storage, identity: string): void {
  remove(local, key(identity, "last-dashboard"));
}

export function storageAvailable(storage: Storage): boolean {
  const probe = `${PREFIX}:storage-probe`;
  try {
    storage.setItem(probe, "1");
    const available = storage.getItem(probe) === "1";
    storage.removeItem(probe);
    return available;
  } catch {
    return false;
  }
}
