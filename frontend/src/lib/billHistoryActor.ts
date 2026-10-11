/**
 * Bill-history actor as an operator should see it.
 *
 * The backend records the acting staff member's email (or a system tag) as
 * the history actor. The demo generator seeds staff as
 * `demo+admin<N>-business<N>-staff<N>@<domain>` and writes the system actors
 * `demo` / `demo-seed`; printing those verbatim leaks generator plumbing into
 * the bill timeline. They carry no information for the reader, so the
 * timeline omits the "by …" line for them. Real actors pass through unchanged.
 */
const SEEDED_DEMO_EMAIL = /^demo\+admin\d+-[^@\s]*@/i;
const SEEDED_SYSTEM_ACTORS = new Set(["demo", "demo-seed"]);

export function displayBillHistoryActor(
  actor: string | null | undefined,
): string | null {
  const trimmed = actor?.trim();
  if (!trimmed) return null;
  if (SEEDED_SYSTEM_ACTORS.has(trimmed.toLowerCase())) return null;
  if (SEEDED_DEMO_EMAIL.test(trimmed)) return null;
  return trimmed;
}
