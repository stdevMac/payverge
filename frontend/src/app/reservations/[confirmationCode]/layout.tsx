import type { Metadata } from "next";

interface ReservationLayoutProps {
  params: Promise<{ confirmationCode: string }>;
}

/**
 * PG-10: honest per-page <title> for guest reservation micro-pages.
 * Do not inherit the marketing root title, and never put the confirmation
 * code (or other PII) into the document title.
 */
export async function generateMetadata(
  _props: ReservationLayoutProps,
): Promise<Metadata> {
  return {
    title: "Reservation | Payverge",
    description: "View or manage your restaurant reservation.",
    robots: { index: false, follow: false },
  };
}

export default function ReservationLayout({
  children,
}: Readonly<{ children: React.ReactNode }>) {
  return <>{children}</>;
}
