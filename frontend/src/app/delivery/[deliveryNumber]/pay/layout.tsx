import type { Metadata } from "next";
import type { ReactNode } from "react";

// SEO-0.5: guest delivery pages are token URLs — never indexable. The pay page
// is a client component, so the robots directive lives in this server layout.
export const metadata: Metadata = {
  title: "Delivery payment | Payverge",
  robots: { index: false, follow: false },
};

export default function DeliveryPayLayout({ children }: { children: ReactNode }) {
  return children;
}
