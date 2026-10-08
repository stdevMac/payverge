"use client";

import React, { createContext } from "react";

type Theme = "light";

interface ThemeContextType {
  theme: Theme;
  setTheme: (theme: Theme) => void;
  resolvedTheme: "light";
}

const ThemeContext = createContext<ThemeContextType | undefined>(undefined);

// Light-only for now (see CLAUDE.md design context). When dark mode ships,
// reintroduce state, mount-time class application via useEffect, and a
// MutationObserver in providers.tsx that explicitly ignores its own
// additions — the previous implementation called classList during render
// and combined with the observer to freeze the tab on first nav.
const NOOP_SET_THEME = (_theme: Theme) => {
  // No-op: theme is fixed to "light" until dark mode ships.
};

const STATIC_VALUE: ThemeContextType = {
  theme: "light",
  setTheme: NOOP_SET_THEME,
  resolvedTheme: "light",
};

export function ThemeProvider({ children }: { children: React.ReactNode }) {
  return (
    <ThemeContext.Provider value={STATIC_VALUE}>
      {children}
    </ThemeContext.Provider>
  );
}
