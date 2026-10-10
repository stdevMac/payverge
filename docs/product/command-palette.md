# Command palette (⌘K)

The operator dashboard has a command palette. Press **⌘K** (Ctrl+K on Windows
and Linux) anywhere in the dashboard, or click the "Search… ⌘K" pill in the
header, to fuzzy-search every dashboard section, quick actions and record
jumps (for example a bill or a menu item), and open the result directly.

## Access rules

The palette never offers navigation the sidebar would not. Both read the same
lock logic in `tabAccess.ts`:

- While the lock state is still loading, nothing is gated.
- When a server administrator suspends or closes a business, every tab except
  settings is locked. Owners see locked tabs with a lock notice; staff do not
  see them at all.

`tabRegistryParity.test.ts` pins the palette's tab list to the sidebar's
`PRIMARY_TABS` and `SECONDARY_TABS` (`sidebar/sidebarConfig.ts`), so a tab
cannot be added to one without the other.

## Code

All files are under `frontend/src/components/business/commandPalette/`.

| File | Responsibility |
|------|----------------|
| `tabAccess.ts` | Pure lock/hidden logic shared with `DashboardSidebar`. |
| `commandRegistry.ts` | Pure: builds the command list (navigate, actions, records, recent) and does fuzzy scoring and filtering. No React, no i18n. |
| `CommandPalette.tsx` | The dialog: keyboard navigation, focus trap, ARIA combobox/listbox, grouped and ranked results. Animations use `motion-safe:`. |
| `CommandPaletteProvider.tsx` | Open state, the global ⌘K listener and recently used sections (stored per business in local storage). Exposes `useCommandPalette()`. |
| `CommandPaletteTrigger.tsx` | The "Search… ⌘K" pill in the dashboard header. |

The provider is mounted in `DashboardLayout.tsx`. Strings are under
`businessDashboard.commandPalette.*` in the en, es and es-ar operator bundles.
The `cmdk-fade` and `cmdk-pop` keyframes are in `globals.css`.

## Tests

In `commandPalette/__tests__/`:

- `tabAccess.test.ts`: the lock/hidden matrix.
- `tabRegistryParity.test.ts`: palette and sidebar list the same tabs.
- `commandRegistry.test.ts`: access filtering, fuzzy ranking, recents.
- `CommandPalette.test.tsx`, `CommandPaletteProvider.test.tsx`,
  `CommandPaletteTrigger.test.tsx`: open and close, keyboard navigation,
  selection, ARIA roles.
