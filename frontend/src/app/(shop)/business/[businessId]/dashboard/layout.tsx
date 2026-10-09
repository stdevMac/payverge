import type { Metadata } from "next";

// Per-business dashboards live behind auth; we expose a generic title
// rather than embedding the slug (which would be the URL-encoded id,
// not the friendly business name) and keep it out of search indexes.
export const metadata: Metadata = {
  title: "Business dashboard — Payverge",
  robots: { index: false, follow: false },
};

export default function BusinessDashboardLayout({
  children,
}: Readonly<{ children: React.ReactNode }>) {
  return children;
}
