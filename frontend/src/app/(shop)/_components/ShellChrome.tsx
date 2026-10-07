"use client";

import { usePathname } from "next/navigation";
import TopMenuLazy from "@/components/ui/top-menu/TopMenuLazy";
import { Footer } from "@/components/Footer";

// /admin keeps a chrome-less canvas — the global admin console runs
// modal-dense workflows that benefit from the full viewport.
function isChromelessRoute(pathname: string | null): boolean {
  if (!pathname) return false;
  return pathname.startsWith("/admin");
}

// Signed-in dashboard and account routes keep the top nav, but suppress the
// public footer — its links have no place on a portfolio, operator, or
// account surface. Locale switching
// remains available in the relevant dashboard chrome.
function isAuthenticatedChromeRoute(pathname: string | null): boolean {
  if (!pathname) return false;
  return (
    pathname === "/dashboard" ||
    pathname === "/account" ||
    pathname.startsWith("/account/") ||
    /^\/business\/[^/]+\/dashboard(\/|$)/.test(pathname)
  );
}

export default function ShellChrome({
  children,
}: {
  children: React.ReactNode;
}) {
  const pathname = usePathname();

  if (isChromelessRoute(pathname)) {
    return <div className="flex-1">{children}</div>;
  }

  const showFooter = !isAuthenticatedChromeRoute(pathname);

  return (
    <>
      <TopMenuLazy />
      <div className="flex-1">{children}</div>
      {showFooter && <Footer />}
    </>
  );
}
