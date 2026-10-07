import type { Metadata } from "next";
import { ToastProvider } from "@/contexts/ToastContext";

// Provide an account-specific browser tab title. The /account page is a
// client component (auth hooks + search params) and can't export metadata
// directly; this server layout is where Next.js attaches it. `robots: noindex`
// keeps the authenticated surface out of search indexes if the auth wall
// ever leaks.
export const metadata: Metadata = {
  title: "Account — Payverge",
  robots: { index: false, follow: false },
};

// The (shop) route-group layout intentionally owns chrome only and has no
// ToastProvider — the dashboard mounts its own inside its page tree. /account
// renders AccountEmailPreferences, which calls useToast(), so scope a
// ToastProvider to this route only. This avoids double-wrapping the dashboard
// subtree that already provides its own.
export default function AccountLayout({
  children,
}: Readonly<{
  children: React.ReactNode;
}>) {
  return <ToastProvider>{children}</ToastProvider>;
}
