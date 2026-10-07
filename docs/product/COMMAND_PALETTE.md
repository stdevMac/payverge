# ⌘K Command Palette — "Quick Jump"

Built overnight (2026-05-30) as an autonomous surprise feature on the
`feature/command-palette` worktree.

## Why this feature

Payverge's brand DNA is explicitly "Stripe's clarity, Linear's refinement,
Vercel's sharpness, Raycast's premium feel." A command palette is *the*
signature dev-tools-polish move. The operator dashboard has **20 sections**
(`sidebarConfig.ts`); today you hunt-and-click the sidebar. Now: press **⌘K**
(Ctrl+K on Windows/Linux) anywhere in the dashboard → fuzzy-search every section
and quick action → jump instantly.

## The "bug-free, polished" guarantee

The #1 way a feature like this feels *buggy* is offering navigation that goes
nowhere. So the palette is **tier- and permission-aware by construction**: it
reuses the exact same access logic the sidebar uses (extracted into a shared
`tabAccess.ts` so the two can never drift). Staff see only their allowed tabs;
locked tabs show a lock chip and route to the upgrade view, identical to the
sidebar.

## Architecture (all under `src/components/business/commandPalette/`)

| File | Responsibility |
|------|----------------|
| `tabAccess.ts` | Shared, pure tier/role lock logic — extracted from `DashboardSidebar`, now imported by both. Single source of truth. |
| `commandRegistry.ts` | Pure: build the command list (nav + actions) from inputs; fuzzy-score + filter. Fully unit-tested, no React. |
| `CommandPalette.tsx` | The dialog: keyboard nav, focus trap, ARIA combobox/listbox, grouped + ranked results, brand styling, motion-reduce aware. |
| `CommandPaletteProvider.tsx` | Owns open state, the global ⌘K listener, and recent-section persistence. Exposes `useCommandPalette()`. |
| `CommandPaletteTrigger.tsx` | The discoverable "Search… ⌘K" pill rendered in the dashboard header. |

Integration: mounted in `DashboardLayout.tsx`, fed the tier flags / allowedTabs /
business it already has. i18n strings in `businessDashboard.commandPalette.*`
(en, es, es-AR). Scoped keyframes in `globals.css`, gated behind `motion-reduce`.

## Testing

- `tabAccess.test.ts` — lock/hidden matrix parity with sidebar behavior.
- `commandRegistry.test.ts` — access filtering, fuzzy ranking, recents.
- `CommandPalette.test.tsx` — open/close, keyboard nav, selection, a11y roles.
