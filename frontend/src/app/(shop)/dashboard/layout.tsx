import type { Metadata } from "next";
import { absoluteSiteUrl } from "@/config/publicConfig";
import { pageOpenGraphFromCanonical } from "@/lib/seo/openGraphImages";

// Provide a dashboard-specific browser tab title. The /dashboard page
// itself is a client component (Web3 hooks + Zustand) and can't export
// metadata directly; a server layout in the same folder gives Next.js
// somewhere to attach it. `robots: noindex` keeps the authenticated
// surface out of search indexes if the auth wall ever leaks.
const DASHBOARD_TITLE = "Dashboard — Payverge";
const DASHBOARD_URL = absoluteSiteUrl("/dashboard");

export const metadata: Metadata = {
  title: DASHBOARD_TITLE,
  robots: { index: false, follow: false },
  alternates: { canonical: DASHBOARD_URL },
  openGraph: pageOpenGraphFromCanonical({
    title: DASHBOARD_TITLE,
    url: DASHBOARD_URL,
  }),
};

export default function DashboardLayout({
  children,
}: Readonly<{ children: React.ReactNode }>) {
  return children;
}
