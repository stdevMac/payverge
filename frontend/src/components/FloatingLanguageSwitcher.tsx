"use client";

import { usePathname } from "next/navigation";
import { Z_FLOATING_PILL, zStyle } from "./business-page/designLayers";
import SimpleLanguageSwitcher from "./SimpleLanguageSwitcher";

const STANDALONE_LANGUAGE_ROUTES = [
  "/forgot-password",
  "/reset-password",
  "/verify-email",
] as const;

export default function FloatingLanguageSwitcher() {
  const pathname = usePathname() ?? "/";
  const ownsNoChrome = STANDALONE_LANGUAGE_ROUTES.some(
    (route) => pathname === route || pathname.startsWith(`${route}/`),
  );
  if (!ownsNoChrome) return null;

  return (
    <div className="fixed right-4 top-4" style={zStyle(Z_FLOATING_PILL)}>
      <SimpleLanguageSwitcher />
    </div>
  );
}
