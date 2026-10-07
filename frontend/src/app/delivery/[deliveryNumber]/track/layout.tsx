import type { Metadata } from "next";
import type { ReactNode } from "react";

// Lock the document-level referrer policy for the public tracking route.
// The tracking URL embeds the public delivery_number token; without an explicit
// no-referrer directive, any future site-wide <meta name="referrer"> or external
// sub-resource could leak that token to a third party via the Referer header.
// SEO-0.5: guest delivery pages are token URLs — never indexable.
export const metadata: Metadata = {
  title: "Track your delivery | Payverge",
  referrer: "no-referrer",
  robots: { index: false, follow: false },
};

export default function DeliveryTrackingLayout({ children }: { children: ReactNode }) {
  return children;
}
