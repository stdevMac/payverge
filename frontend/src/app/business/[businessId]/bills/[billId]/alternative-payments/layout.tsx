import type { Metadata } from "next";

// This leaf is outside (shop)/.../dashboard, so it would otherwise inherit
// the marketing root title. Keep it an authenticated operator surface.
export const metadata: Metadata = {
  title: "Alternative payments — Payverge",
  robots: { index: false, follow: false },
};

export default function AlternativePaymentsLayout({
  children,
}: Readonly<{ children: React.ReactNode }>) {
  return children;
}
