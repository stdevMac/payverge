// app/providers.tsx
"use client";

import { NextUIProvider } from "@nextui-org/react";
import { ThemeProvider } from "@/context/ThemeContext";

export function Providers({ children }: { children: React.ReactNode }) {
  return (
    <ThemeProvider>
      <NextUIProvider>
        {children}
        {/* <PortfolioCheck /> */}
      </NextUIProvider>
    </ThemeProvider>
  );
}
