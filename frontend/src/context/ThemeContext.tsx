"use client";

import React, { createContext, useContext, useState } from 'react';

type Theme = 'light';

interface ThemeContextType {
  theme: Theme;
  setTheme: (theme: Theme) => void;
  resolvedTheme: 'light';
}

const ThemeContext = createContext<ThemeContextType | undefined>(undefined);

export function ThemeProvider({ children }: { children: React.ReactNode }) {
  const [theme] = useState<Theme>('light');
  const resolvedTheme = 'light';

  // Always ensure light theme
  if (typeof document !== 'undefined') {
    document.documentElement.classList.remove('dark');
  }

  const handleSetTheme = (newTheme: Theme) => {
    // Only allow light theme
    if (newTheme === 'light') {
      // Theme is already light, no action needed
    }
  };

  return (
    <ThemeContext.Provider
      value={{
        theme,
        setTheme: handleSetTheme,
        resolvedTheme,
      }}
    >
      {children}
    </ThemeContext.Provider>
  );
}

export function useTheme() {
  const context = useContext(ThemeContext);
  if (context === undefined) {
    throw new Error('useTheme must be used within a ThemeProvider');
  }
  return context;
}
