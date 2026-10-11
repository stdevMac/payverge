/**
 * Counter naming helpers shared by CounterManager UI.
 * Keep in sync with backend/internal/database/business.go:
 *   isGeneratedCounterName / isSeedDefaultCounterName / shouldRewriteCounterName
 */

import { COUNTER_PREFIX_MAX_LENGTH } from "./counters";

export function desiredCounterName(prefix: string, counterNumber: number): string {
  const trimmed = (prefix || "C").trim() || "C";
  return `${trimmed}${counterNumber}`;
}

/** Compact auto-name: C1, D2, BAR3 — no whitespace. */
export function isGeneratedCounterName(
  name: string,
  counterNumber: number,
): boolean {
  const suffix = String(counterNumber);
  if (!name.endsWith(suffix)) return false;
  const prefix = name.slice(0, name.length - suffix.length);
  if (!prefix || prefix.length > COUNTER_PREFIX_MAX_LENGTH) return false;
  if (/\s/.test(prefix)) return false;
  const last = prefix[prefix.length - 1];
  if (last >= "0" && last <= "9") return false;
  return true;
}

/** Legacy/demo seed labels: "Counter 1", "Mostrador 2", "Pickup 3". */
export function isSeedDefaultCounterName(
  name: string,
  counterNumber: number,
): boolean {
  const suffix = ` ${counterNumber}`;
  if (!name.endsWith(suffix)) return false;
  const word = name.slice(0, name.length - suffix.length).trim().toLowerCase();
  return word === "counter" || word === "mostrador" || word === "pickup";
}

/** Names the backend will rewrite when settings are saved / Apply is clicked. */
export function isApplyableDefaultCounterName(
  name: string,
  counterNumber: number,
): boolean {
  return (
    isGeneratedCounterName(name, counterNumber) ||
    isSeedDefaultCounterName(name, counterNumber)
  );
}

export type CounterNamingMismatch = {
  id: number;
  counterNumber: number;
  current: string;
  desired: string;
  applyable: boolean;
};

export function listCounterNamingMismatches(
  counters: Array<{ id: number; counter_number: number; name: string }>,
  prefix: string,
): CounterNamingMismatch[] {
  return counters
    .map((c) => {
      const desired = desiredCounterName(prefix, c.counter_number);
      if (c.name === desired) return null;
      return {
        id: c.id,
        counterNumber: c.counter_number,
        current: c.name,
        desired,
        applyable: isApplyableDefaultCounterName(c.name, c.counter_number),
      };
    })
    .filter((m): m is CounterNamingMismatch => m !== null);
}
