/**
 * The one dashboard table look. Cash Register (`CashRegisterDashboard.tsx`) is
 * the source of truth and consumes these constants directly — its movements and
 * history tables ARE this recipe, so drift is impossible. Every list-management
 * tab (Team's StaffManagement, AI Waiter's conversations) adopts the same look.
 *
 * The rule the cohesion audit is enforcing: NO grey header band. Headers are
 * quiet uppercase labels on the panel background. Hairlines are fine: the
 * canonical table draws a `warm-200/80` rule under the header row (via
 * `divide-y` on the <table>) and `warm-100` rules between body rows — the ban
 * is on filled `thead` backgrounds, not on dividers.
 *
 * Plain <table>: `tableRoot` on the <table>, `tableHeaderCell` on each <th>,
 * `tableBodyCell` + a per-column ink tone on each <td>, `tableBody` on the
 * <tbody>. `tableBodyCell` is structure-only (no text color) because the
 * source varies tone per column (`text-ink-600` secondary, `text-ink-950`
 * money, `text-ink-500` timestamps) and this repo has no tailwind-merge —
 * baking a default color into the constant would fight per-cell overrides by
 * stylesheet order.
 * NextUI <Table>: spread `tableClassNames` into `classNames={...}` and pass
 * `removeWrapper`.
 */

/**
 * <table> element. The `divide-y` puts a 1px `warm-200/80` border-top on the
 * <tbody> (its only later sibling) — that's the canonical hairline under the
 * header row. Renders because Tailwind preflight keeps tables
 * `border-collapse: collapse`.
 */
export const tableRoot = "w-full divide-y divide-warm-200/80 text-left text-sm";

/** <th> — uppercase label typography + padding. No grey band, ever. */
export const tableHeaderCell =
  "px-3 py-2 text-xs font-semibold uppercase tracking-[0.14em] text-ink-600";

/** <td> — body cell structure. Add the column's ink tone inline (see above). */
export const tableBodyCell = "px-3 py-3 text-sm";

/** <tbody> — hairline separators between body rows, none after the last. */
export const tableBody = "divide-y divide-warm-100";

/**
 * NextUI `<Table classNames={tableClassNames} removeWrapper>` preset built from
 * the constants above so it renders like the plain Cash Register table.
 *
 * How it maps onto NextUI's DOM (<thead> holds the header <tr> plus a 1px
 * spacer <tr>; <tbody> is a direct sibling; the theme never sets
 * `border-separate`, so preflight's `border-collapse` keeps row/row-group
 * borders rendering):
 * - `table`: `tableRoot`'s divide draws the warm-200/80 hairline as border-top
 *   on <tbody> — under the header row, exactly like the plain table.
 * - `tbody`: `tableBody`'s divide draws warm-100 hairlines between body rows
 *   only; the last row gets no bottom border. No `tr` border classes — a
 *   `border-b`+`last:border-0` on the tr slot would also hit the header row
 *   (and its `last:` would key off the thead's spacer row, not the body).
 * - `th`: `bg-transparent` kills NextUI's default `bg-default-100` grey band;
 *   `h-auto` beats the default `h-10` so `py-2` sets the header height.
 * - `td`: default readable tone `text-ink-800` is safe to override per cell
 *   here because NextUI merges slot + cell classes through tailwind-merge.
 */
export const tableClassNames = {
  base: "overflow-x-auto",
  table: tableRoot,
  th: `h-auto bg-transparent ${tableHeaderCell}`,
  td: `${tableBodyCell} text-ink-800`,
  tbody: tableBody,
  // Defensive: if a consumer forgets `removeWrapper`, keep the wrapper invisible.
  wrapper: "rounded-none border-0 bg-transparent p-0 shadow-none",
} as const;
