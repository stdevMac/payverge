# Sidebar Tab Design Standard

**Status:** proposed, derived from a 2026-05-23 live audit of every dashboard tab.
**Scope:** every tab reachable from `DashboardSidebar` in the business dashboard.
**Non-goals:** the sidebar itself, the top nav (Home/Dashboard/Admin), the AuthModal, the global footer.
**Theme:** light only. Per CLAUDE.md, dark mode is a future initiative — do **not** add new `dark:` variants while applying this standard.

## Design tokens (from `frontend/tailwind.config.ts` — already in the system)

| Purpose | Token | Hex |
|---|---|---|
| Primary action / active rail | `bg-brand` | `#1a6b6a` |
| Primary hover | `bg-brand-dark` | `#145554` |
| Brand tint / icon container fill | `bg-brand-50` | `#f0f7f7` |
| Surface (page) | `bg-warm-50` | `#faf9f6` |
| Surface (card) | `bg-white` | `#ffffff` |
| Surface (icon-in-box) | `bg-gray-50` / `bg-warm-50` | `#faf9f6` |
| Body text | `text-ink-900` | `#1c1917` |
| Muted text | `text-ink-600` / `text-gray-700` | `#544d44` / `#403b34` |
| Border (card / input) | `border-gray-200` | `#e4e0d8` |
| Display heading font | `font-title` | DM Serif Display |
| Body font | default sans / `font-inter` (aliased to DM Sans) | DM Sans |

> Note: the Tailwind palette deliberately collapses `gray`, `warm`, and `ink` to the same warm-neutral scale (see `tailwind.config.ts:11-23`). Use `ink-*` for *text* and `warm-*` / `gray-*` for *surfaces/borders* so a reader scans intent quickly.

## 1. Page header (title + subtitle + primary action)

### Standard

```tsx
<header className="flex items-start justify-between gap-4 mb-6">
  <div>
    <h1 className="font-title text-2xl text-ink-900">{title}</h1>
    <p className="text-ink-600 text-sm mt-1">{subtitle}</p>
  </div>
  <div className="flex items-center gap-3 flex-wrap">
    {secondaryAction /* outline button, gray border, no icon-fill */}
    {primaryAction   /* bg-brand text-white, +icon, rounded-lg, px-4 py-2 */}
  </div>
</header>
```

### Rationale

Every tab needs three things: a one-line title that says where you are, a one-line subtitle that says what you can do here, and at most two header-level actions. Using `font-title` (DM Serif Display) on the `<h1>` matches CLAUDE.md's "Serif headings + clean sans body" — the brand's premium-fintech signal lives in the heading typeface. Two-action max prevents the Bills/Menu/Business-Page three-button crowding noted in the audit. The destructive "Disable X" toggle must NOT live in this slot — it belongs in the relevant settings sub-tab. (Moving it out fixes the Business Page red-button outlier and the Bills "Disable Ordering" wedged-against-Create-Bill problem.)

### Canonical implementation

`frontend/src/components/business/TableManager.tsx:410-426` — the only header in the audit that uses `font-title`, the only header that pairs exactly one outline-secondary with one filled-primary, the only header that puts toolbar concerns out of the header. Use this verbatim.

## 2. Toolbar (search, filters, view toggles)

### Standard

Below the header, in a single horizontal row, in this left→right order, with `gap-3 mb-4`:

```tsx
<div className="flex items-center gap-3 mb-4">
  {/* 1. Primary filter — pill-chip row (if a small fixed set, ≤6 chips) */}
  <FilterChips ... />
  {/* 2. Search — flex-1, with left-aligned Search icon */}
  <SearchInput ... />
  {/* 3. Secondary filter — single status dropdown */}
  <StatusSelect ... />
  {/* 4. Reset — text button, visible only when filters/search are active */}
  {hasActiveFilters && <ResetButton />}
  {/* 5. View toggle — only when the page supports >1 layout */}
  {hasViewToggle && <ViewToggle ... />}
  {/* 6. Refresh — icon button, far right */}
  <RefreshButton />
</div>
```

Rules:

- **Chips** for ≤6 mutually-exclusive filters (Reservations' Today/Next 2h/Needs table/Waitlist/No-show risk/All — perfect). Drop chips and use a `<select>` once filter count ≥7.
- **Dates** use the same chip row (Today / This week / This month / Last 30d / YTD / Custom) plus inline date inputs only when "Custom" is chosen. Accounting already does this — adopt it everywhere a date range is needed.
- **View toggle** uses the same two-icon segmented control pattern Menu and Plugins already use (grid/list); Reservations' List/Board should remap to the same icon-only control with `aria-label`s.
- **Search** is never wrapped in its own labeled section card. Staff's "Search & Filter" card is the outlier — remove the wrapper.

### Rationale

A consistent left-to-right reading order (filter → search → narrow → reset → view → refresh) lets operators build the same muscle memory across tabs. The audit catalogued four different toolbar shapes; this standard is a superset that subsumes all of them. Inline filters keep the header free for actions (the Stripe / Linear pattern).

### Canonical implementation

Search + dropdown + clear + refresh: `frontend/src/components/business/TableManager.tsx:428-463`.
Chip-row filters: ReservationManager's Tabs config (`frontend/src/components/business/ReservationManager.tsx:1044-1063`) — use NextUI `Tabs` with the same `classNames` overrides so chips render with the brand-tinted active cursor.

## 3. Content surface (table vs. card grid)

### Standard

The page body is a stack of **section cards**:

```tsx
<section className="bg-white border border-gray-200 rounded-lg shadow-sm mb-6">
  <header className="border-b border-gray-200 p-6">
    <div className="flex items-center gap-3">
      <div className="w-12 h-12 bg-gray-50 rounded-xl flex items-center justify-center border border-gray-100">
        <Icon className="w-6 h-6 text-ink-700" />
      </div>
      <div>
        <h2 className="font-title text-xl text-ink-900">{title}</h2>
        <p className="text-ink-600 text-sm">{subtitle}</p>
      </div>
    </div>
  </header>
  <div className="p-6">{children}</div>
</section>
```

Inside a section card, content is one of:

- **Table** for record collections that are read mostly by row (Bills, Tables, Staff, CRM, Inventory Items, Accounting Manual Entries). Columns left-align text, right-align money, status pill in its own column.
- **Card grid** (2/3/4-col responsive) for items that need a thumbnail or visual identity (Menu, Plugins). 4-col on `xl`, 3-col on `lg`, 2-col on `md`, 1-col on `sm`.
- **Form** for settings (Counter, Delivery, Business Page, Settings, Business profile sections). Two-column on `md+`, full-width inputs on `sm`.
- **Stat row** (4-col stats) only ABOVE the first section card, never embedded in one. Use neutral surface (`bg-white border border-gray-200`); only use amber/red tinted stat cards when the stat is itself an alert count (Inventory's Low Stock / Out of Stock).

The Accounting six-mini-card sparkline strip is allowed where genuine time-series-per-metric is needed, but it should not be the default — that's a domain pattern, not a generalizable layout.

### Rationale

The "section card" pattern is the most consistently rendered piece of the dashboard — Counter, Staff, Plugins, Business Page, Settings, and Accounting all already use it. Codifying it removes the Subscriptions outlier (cards with no page header) and the Analytics outlier (entire page wrapped in one card). Reserving stat rows above the first card prevents Inventory's stat-strip-inside-content drift.

### Canonical implementation

Section card markup: `frontend/src/components/business/Kitchen.tsx:413-428`.
Stats row pattern: `frontend/src/components/business/StaffManagement.tsx` (4-col grid w/ icon + value + label — `mb-6`).

## 4. Empty state (icon + headline + body + CTA)

### Standard

```tsx
<div className="text-center py-12 px-6">
  <div className="w-16 h-16 bg-gray-50 rounded-2xl flex items-center justify-center mx-auto mb-6 border border-gray-100">
    <Icon className="w-8 h-8 text-ink-400" />
  </div>
  <h3 className="font-title text-lg text-ink-900 mb-2">{title}</h3>
  <p className="text-ink-600 text-sm max-w-md mx-auto mb-6">{description}</p>
  {primaryCta /* same filled-teal button as the header primary */}
</div>
```

Rules:

- The icon is the same lucide-react icon as the sidebar item for that tab. Identity is preserved across nav → empty state.
- One headline (≤6 words), one body line (≤120 chars), one CTA.
- No illustrations. No mascots. No background gradients. (Anti-references in CLAUDE.md.)
- The CTA is the **same** filled-teal button as the page header — same shape, same icon, same label — so operators don't have to learn two creation paths.

### Rationale

Kitchen's empty state is already this shape minus the CTA. Reservations is this shape plus a CTA. Standardizing on "always include the CTA" turns the empty state into a productive on-ramp instead of a dead end — the audit found Kitchen's empty state had no way to drive an order. The icon parity rule keeps the user oriented; the body-length rule prevents the marketing-style empty states that creep in over time.

### Canonical implementation

Icon container + headline + body: `frontend/src/components/business/Kitchen.tsx:450-460`.
The CTA wiring follows the same pattern as Reservations' "Create Reservation" button (`ReservationManager.tsx:1002-1013`) — `<Button>` with `bg-brand text-white`, `startContent={<Plus className="w-4 h-4" />}`.

## 5. Loading state

### Standard

```tsx
<div className="bg-white border border-gray-200 rounded-lg shadow-sm">
  <div className="flex justify-center py-12">
    <Spinner aria-label={t("loading")} />
  </div>
</div>
```

For pages that have already rendered structural chrome (header, tabs, toolbar), keep those visible and show the spinner only inside the section card. Skeleton placeholders (Overview's gray block bars) are acceptable on the first paint when the layout itself is data-driven, but every skeleton must mirror the *real* layout — never use a generic 4-block placeholder where a table will render.

### Rationale

The page header and toolbar are layout, not data. Skinning the whole page when only the data is loading is disorienting (Overview's first paint, before stats arrive, currently does this). Confining the spinner to the data region matches Reservations and Kitchen.

### Canonical implementation

`frontend/src/components/business/ReservationManager.tsx:1017-1022`.

## 6. Error state

### Standard

```tsx
<div className="bg-white border border-red-200 rounded-lg shadow-sm">
  <div className="text-center py-12 px-6">
    <div className="w-16 h-16 bg-red-50 rounded-2xl flex items-center justify-center mx-auto mb-6 border border-red-100">
      <AlertCircle className="w-8 h-8 text-red-600" />
    </div>
    <h3 className="font-title text-lg text-ink-900 mb-2">{title}</h3>
    <p className="text-ink-600 text-sm max-w-md mx-auto mb-6">{message}</p>
    <button className="bg-brand text-white px-4 py-2 rounded-lg font-medium hover:bg-brand-dark">{retryLabel}</button>
  </div>
</div>
```

Structurally identical to empty state — same icon-in-box + headline + body + CTA — but with red tints on the container and icon, and a "Retry" instead of a creation CTA. This keeps cognitive load low: the page shape doesn't change when something fails, only the message and color do. Error message text never leaks raw API errors (per CLAUDE.md's "boundary validation only" principle).

### Canonical implementation

Existing pattern: `frontend/src/components/business/DashboardLayout.tsx` uses `AlertCircle` from lucide for the global error shell — reuse the same import and tinting in tab-level error states.

## 7. Destructive / "Disable" actions

### Standard

There are exactly three shapes:

| Action | Style | Where it appears |
|---|---|---|
| Toggle a feature off (Reservations, Counter, Inventory, CRM, Bills' "Disable Ordering") | Outline gray button: `border border-gray-200 text-ink-900 bg-white` — no red | Inside the relevant feature's **Settings sub-tab**, NOT the page header |
| Permanently remove an item (Plugin "Disable", Settings "Remove" logo, table row "Delete") | Red text-only link: `text-red-600 hover:text-red-700`, no border | In the card / row of the thing being removed |
| Cancel a billing-cycle subscription | Red text-only link, inside the Subscription card body | Subscription tab only |

There is no "filled red destructive button" anywhere in the system. The Business Page "Disable Page" filled-red button is the outlier and must be reverted.

### Rationale

Three styles is two more than needed, but every existing usage maps to one of these three intents, so the standard mostly relabels what's already there. Pulling "Disable X" off the page header solves the audit's most consistent complaint (header bar carrying secondary toggles) and also fixes the visual weight problem where header CTAs end up four wide.

## 8. Sub-tab navigation

### Standard

Two shapes only, never mixed on the same page:

| Use case | Component | When |
|---|---|---|
| Switch between sibling views of the same dataset (Bills "Active / History", CRM "Customers / Segments / Loyalty", Delivery sections) | NextUI `<Tabs>` with underline cursor: `tabList: ""`, `cursor: "bg-brand"`, full-width container | Same data domain, no settings split |
| Switch between sub-pages with distinct concerns (Settings "Business Profile / Design & Branding / Payment / Currency"; Business Page "Essentials / Visuals / Operations / Reviews") | NextUI `<Tabs>` with icon underline, larger padding (`p-4`), `tabList: "gap-2"` | Sub-page-as-form clusters |

Pill-chip tabs (Kitchen, Inventory, Accounting) collapse into the first variant — they should look like underline tabs, not pills. NextUI's `Tabs` lets us control this via `classNames`; consolidating the visual is a single PR.

### Rationale

Audit found three sub-tab shapes; this collapses them to two, both already implemented in NextUI. The icon vs. plain split is meaningful — icons cue "this is a sub-area with its own surface area", plain underline cues "same content filtered differently".

## 9. Status / hero banners

### Standard

When the page needs to communicate the *current operational status* of the feature itself (Delivery's "Delivery is on • 3 active zones"), use a single brand-tinted info banner immediately below the header:

```tsx
<div className="bg-brand-50 border border-brand-100 rounded-lg p-4 mb-6 flex items-center justify-between">
  <div className="flex items-center gap-3">
    <div className="w-10 h-10 bg-white rounded-lg flex items-center justify-center border border-brand-100">
      <Icon className="w-5 h-5 text-brand-700" />
    </div>
    <div>
      <p className="font-medium text-ink-900">{title}</p>
      <p className="text-ink-600 text-sm">{subtitle}</p>
    </div>
  </div>
  {actionButton /* the feature's outline disable/enable toggle */}
</div>
```

Use this exclusively for status, never for marketing — the audit found two "About this feature" cream-tinted cards (Counter, Settings) that read as marketing copy. Those should be deleted; if the operator needs an explainer, link out to docs from the page header subtitle.

## 10. Right-rail pattern

### Standard

A right rail (Accounting "Recent Activity", Subscriptions "Recent Activity") is allowed when:

- The main content is a single dense card (not a stack of cards), AND
- The rail is genuinely contextual to the main card (activity, audit log, AI suggestions).

Otherwise, the rail is forbidden. Outside those two tabs today, the dashboard is full-width and should stay that way.

Markup:

```tsx
<div className="grid grid-cols-1 lg:grid-cols-[1fr_320px] gap-6">
  <main>{/* primary section cards */}</main>
  <aside className="space-y-4">{/* rail cards */}</aside>
</div>
```

Below `lg`, the rail collapses to a section card stacked under the main content.

## Cross-section: typography scale

| Element | Class | Notes |
|---|---|---|
| `<h1>` page title | `font-title text-2xl text-ink-900` | Always DM Serif. Always 24px. |
| `<h2>` section card heading | `font-title text-xl text-ink-900` | DM Serif, 20px |
| `<h3>` empty / error headline | `font-title text-lg text-ink-900 mb-2` | DM Serif, 18px |
| Eyebrow (above hero / section) | `text-xs font-semibold tracking-widest uppercase text-ink-600` | Use sparingly — Overview only today |
| Body | `text-sm text-ink-600` or `text-ink-700` for higher weight | DM Sans |
| Table cell | `text-sm text-ink-900` | DM Sans |
| Stat value | `text-2xl font-semibold text-ink-900` (or `font-title` for Overview hero only) | |
| Pill / chip / status badge | `text-xs font-medium px-2.5 py-0.5 rounded-full` w/ brand-50 / amber-50 / red-50 backgrounds | Never use full-saturation hex |

## How to apply this standard

When a tab deviates from this spec, apply the fixes in this order:

1. Fix headers: universal `font-title` + standard subtitle + ≤2 actions.
2. Move "Disable X" out of headers into settings sub-tabs.
3. Reformat sub-tabs to underline.
4. Standardize empty states to the canonical icon-in-box + CTA pattern.
5. Consolidate toolbar order.

Every rule above points at existing canonical code; none needs a new component.
