/**
 * English-clone guard for Nordic/PL/VI guest locales.
 *
 * Every string leaf under `aiWaiter` must differ from English so those
 * locales do not ship chat chrome as a byte-identical English copy.
 * Two keys stay identical on purpose: `charCount` is the locale-invariant
 * template "{count}/{max}", and `modeConcierge` is the loanword "Concierge".
 */
import en from "../guest-messages/en.json";
import da from "../guest-messages/da.json";
import no from "../guest-messages/no.json";
import pl from "../guest-messages/pl.json";
import sv from "../guest-messages/sv.json";
import vi from "../guest-messages/vi.json";

// charCount: locale-invariant "{count}/{max}".
// modeConcierge: loanword "Concierge".
const ALLOWED_IDENTICAL = new Set([
  "aiWaiter.charCount",
  "aiWaiter.modeConcierge",
]);

function get(obj: Record<string, unknown>, path: string): unknown {
  return path
    .split(".")
    .reduce<unknown>(
      (acc, k) =>
        acc && typeof acc === "object"
          ? (acc as Record<string, unknown>)[k]
          : undefined,
      obj,
    );
}

/** Dotted paths of every string leaf under `node`, rooted at `prefix`. */
function stringLeaves(node: unknown, prefix: string): string[] {
  if (typeof node === "string") return [prefix];
  if (Array.isArray(node)) {
    return node.flatMap((item, index) =>
      stringLeaves(item, `${prefix}.${index}`),
    );
  }
  if (node && typeof node === "object") {
    return Object.entries(node as Record<string, unknown>).flatMap(([key, value]) =>
      stringLeaves(value, prefix ? `${prefix}.${key}` : key),
    );
  }
  return [];
}

const enRoot = en as Record<string, unknown>;
const enAiWaiterLeaves = stringLeaves(
  (enRoot as { aiWaiter?: unknown }).aiWaiter,
  "aiWaiter",
);

describe("guest locale clone guard", () => {
  it("allowlist entries exist in en", () => {
    const missing = [...ALLOWED_IDENTICAL].filter(
      (key) => typeof get(enRoot, key) !== "string",
    );
    expect(missing).toEqual([]);
  });

  it.each([
    ["da", da],
    ["no", no],
    ["pl", pl],
    ["sv", sv],
    ["vi", vi],
  ] as const)("%s aiWaiter strings are not English clones", (locale, bundle) => {
    const locRoot = bundle as Record<string, unknown>;
    const offenders: string[] = [];
    for (const key of enAiWaiterLeaves) {
      if (ALLOWED_IDENTICAL.has(key)) continue;
      const enVal = get(enRoot, key);
      const locVal = get(locRoot, key);
      if (typeof locVal !== "string" || locVal === enVal) {
        offenders.push(`${locale}:${key}`);
      }
    }
    expect(offenders).toEqual([]);
  });
});
