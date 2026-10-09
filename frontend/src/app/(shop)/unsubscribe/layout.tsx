import type { Metadata } from "next";
import { absoluteSiteUrl } from "@/config/publicConfig";
import { pageOpenGraphFromCanonical } from "@/lib/seo/openGraphImages";

const TITLE = "Unsubscribe — Payverge";
const DESCRIPTION =
  "Stop Payverge marketing emails. Billing and account-critical messages still send.";
// Module scope is evaluated in the server process at runtime (the root layout
// is dynamic), so this follows the deployment's PUBLIC_URL.
const CANONICAL = absoluteSiteUrl("/unsubscribe");

export const metadata: Metadata = {
  title: TITLE,
  description: DESCRIPTION,
  robots: { index: false, follow: false },
  alternates: { canonical: CANONICAL },
  openGraph: pageOpenGraphFromCanonical({
    title: TITLE,
    description: DESCRIPTION,
    url: CANONICAL,
  }),
};

export default function UnsubscribeLayout({
  children,
}: {
  children: React.ReactNode;
}) {
  return children;
}
