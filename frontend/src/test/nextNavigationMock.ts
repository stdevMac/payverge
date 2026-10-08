/**
 * Stateful `next/navigation` test double.
 *
 * jest.setup.js installs a *static* mock: `useSearchParams()` always returns an
 * empty URLSearchParams and `router.push/replace` are no-op jest.fn()s. That is
 * fine for components that only read the URL, but Session P moved real view
 * state (delivery sub-tab, settings section, bills filters, …) into the query
 * string via `useUrlState`. Under the static mock a click writes to the router,
 * the router drops it, the component reads back the same empty params — and the
 * view never changes, so behavioural tests fail for a purely mechanical reason.
 *
 * This double keeps one module-level URL, re-renders subscribers through
 * `useSyncExternalStore` when it changes, and wires push/replace to update it —
 * i.e. it behaves like the App Router does at runtime.
 *
 * Usage (the file-level jest.mock fully replaces jest.setup.js's global mock):
 *
 *   jest.mock("next/navigation", () =>
 *     require("@/test/nextNavigationMock").createStatefulNavigationMock(),
 *   );
 *   import { resetTestUrl } from "@/test/nextNavigationMock";
 *   beforeEach(() => resetTestUrl("/business/1/dashboard?tab=delivery"));
 */
import { useSyncExternalStore } from "react";

let currentUrl = "/";
const listeners = new Set<() => void>();

const emit = () => {
  for (const listener of listeners) listener();
};
const subscribe = (listener: () => void) => {
  listeners.add(listener);
  return () => {
    listeners.delete(listener);
  };
};
// Must be referentially stable per URL — useSyncExternalStore re-reads it on
// every render and loops forever if the snapshot identity keeps changing.
const getSnapshot = () => currentUrl;

/** Current test URL (path + optional search). */
export function getTestUrl(): string {
  return currentUrl;
}

/** Navigate the double; notifies mounted components. */
export function setTestUrl(next: string): void {
  currentUrl = next;
  emit();
}

/** Reset between tests. Call in beforeEach so suites do not leak URLs. */
export function resetTestUrl(next = "/"): void {
  setTestUrl(next);
}

/**
 * Loaded through `require()` inside jest.mock factories, which static
 * analysis cannot see.
 * @public
 */
export function createStatefulNavigationMock() {
  const navigate = (href: string) => setTestUrl(href);
  const useUrl = () => useSyncExternalStore(subscribe, getSnapshot, getSnapshot);
  return {
    useRouter: () => ({
      push: navigate,
      replace: navigate,
      prefetch: () => {},
      back: () => {},
      forward: () => {},
      refresh: () => {},
    }),
    usePathname: () => useUrl().split("?")[0] || "/",
    useSearchParams: () => new URLSearchParams(useUrl().split("?")[1] ?? ""),
    useParams: () => ({}),
  };
}
