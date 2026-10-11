import { getTranslation } from "../SimpleTranslationProvider";
import type { OperatorLocale } from "../localeRegistry";
import enDirector from "../messages/en/directorConsole.json";
import {
  mapUndoErrorKey,
  UNDO_WINDOW_HOURS,
} from "../../components/business/DirectorConsole/proposalActions";

// L4-19 — the corrected undo copy advertises the REAL 24h undo window instead
// of a generic "try again". es-AR is a delta layer over es, so a stale override
// silently shadows a corrected es string: the operator's primary market keeps
// reading the old copy while en/es look fixed. These tests pin the merged
// es-AR output, which is what an Argentine operator actually sees.

const NAMESPACE = "directorConsole";

// Every key the undo path can surface, per mapUndoErrorKey + the applied-changes
// drawer error. Derived from the mapper so a new branch cannot escape the gate.
const UNDO_KEYS = [
  ...new Set([
    mapUndoErrorKey({ code: "already_undone" }),
    mapUndoErrorKey({ status: 409 }),
    mapUndoErrorKey({ status: 410 }),
    mapUndoErrorKey({ status: 500 }),
    "errors.undoFailed",
  ]),
];

const placeholders = (s: string) =>
  [...s.matchAll(/\{(\w+)\}/g)].map((m) => m[1]).sort();

const resolve = (key: string, locale: OperatorLocale) =>
  getTranslation(`${NAMESPACE}.${key}`, locale) as string;

const enValue = (key: string) =>
  key
    .split(".")
    .reduce<unknown>(
      (acc, part) => (acc as Record<string, unknown>)?.[part],
      enDirector as unknown,
    ) as string;

describe("L4-19 — es-AR must not shadow the corrected undo copy", () => {
  test("the mapper covers the four undo outcomes plus the drawer error", () => {
    // Sanity: the gate below is only meaningful if it actually enumerates keys.
    expect(UNDO_KEYS).toHaveLength(5);
    expect(UNDO_KEYS).toContain("proposal.errors.undoFailed");
    expect(UNDO_KEYS).toContain("errors.undoFailed");
  });

  test.each(UNDO_KEYS)(
    "%s resolves in es-AR without falling back to a raw key or English",
    (key) => {
      const ar = resolve(key, "es-AR");
      expect(ar).toBeTruthy();
      expect(ar).not.toBe(`${NAMESPACE}.${key}`);
      expect(ar).not.toBe(enValue(key));
    },
  );

  test.each(UNDO_KEYS)(
    "%s keeps every placeholder the corrected en string carries",
    (key) => {
      const expected = placeholders(enValue(key));
      // es must carry them (the L4-19 correction), and the es-AR delta must not
      // drop them by re-overriding with an older generic string.
      expect(placeholders(resolve(key, "es"))).toEqual(expected);
      expect(placeholders(resolve(key, "es-AR"))).toEqual(expected);
    },
  );

  test("both undoFailed strings state the real 24h window in es-AR", () => {
    for (const key of ["errors.undoFailed", "proposal.errors.undoFailed"]) {
      const ar = resolve(key, "es-AR");
      expect(ar).toContain("{hours}");
      // Voseo, not the Peninsular "Puedes" that es uses.
      expect(ar).toContain("Podés");
      expect(ar).not.toMatch(/\bPuedes\b/);
      // The stale override said only "Intentalo de nuevo" with no window.
      expect(ar).not.toBe("No se pudo deshacer el cambio. Intentalo de nuevo.");
    }
  });

  test("UNDO_WINDOW_HOURS is what the copy is interpolated with", () => {
    expect(UNDO_WINDOW_HOURS).toBe(24);
  });

  test("window-free undo errors inherit es (no needless es-AR override)", () => {
    // undoStale / undoExpired / alreadyUndone have no second-person form in es,
    // so es-AR must fall through rather than carry a divergent copy.
    for (const key of [
      "proposal.errors.undoStale",
      "proposal.errors.undoExpired",
      "proposal.errors.alreadyUndone",
    ]) {
      expect(resolve(key, "es-AR")).toBe(resolve(key, "es"));
    }
  });
});
