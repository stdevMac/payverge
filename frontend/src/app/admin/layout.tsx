import type { Metadata } from "next";
import AdminLayoutClient from "./AdminLayoutClient";

// Override the marketing default <title> so admin pages don't reuse the
// landing-page metadata. `robots: noindex` also keeps the admin chrome
// out of search results regardless of what the auth gate does.
export const metadata: Metadata = {
  title: "Admin — Payverge",
  robots: { index: false, follow: false },
};

// Admin chrome is gated on the client by AdminLayoutClient (auth + role
// check via HybridAuthProvider session-info). Do NOT add an SSR
// `cookies()` / edge cookie-presence gate here: when the API runs on its own
// host (e.g. api.example.com) auth cookies are host-only there (SEC-5 / #302 /
// #314 rejects a parent COOKIE_DOMAIN like `.example.com`), so the frontend
// origin never sees `session_token` and a cookie-presence bounce would 307
// every visitor — including real admins. AuthenticationAdminMiddleware on the
// API remains the security boundary; the client check only avoids flashing
// admin chrome to non-admins.
export default function AdminLayout({
  children,
}: Readonly<{
  children: React.ReactNode;
}>) {
  return <AdminLayoutClient>{children}</AdminLayoutClient>;
}
