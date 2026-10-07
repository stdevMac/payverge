/**
 * Shared helpers for ReservationManager DOM tests (NextUI Select has no aria on
 * the native <select> — find it by option values).
 */
export function getDateFilterSelect(
  container: HTMLElement | Document = document,
): HTMLSelectElement {
  const root =
    container instanceof Document ? container : (container as HTMLElement);
  const found = Array.from(root.querySelectorAll("select")).find((s) =>
    Array.from(s.options).some((o) => o.value === "all_time"),
  );
  if (!found) {
    throw new Error("date filter <select> with all_time option not found");
  }
  return found;
}
