"use client";

import { useInstance } from "@/hooks/useInstance";
import { isPublicDemo } from "@/lib/instance/instanceInfo";
import { useGuestTranslation } from "@/i18n/GuestTranslationProvider";

/** Guest-tier key: guest-messages common.*. */
const DEMO_PERSONAL_DATA_KEY = "common.demoPersonalDataNotice";

function NoticeView({ message, className }: { message: string; className?: string }) {
  return (
    <p
      role="note"
      data-testid="demo-personal-data-notice"
      className={`rounded-lg border border-amber-200 bg-amber-50 px-3 py-2 text-sm text-ink-900 ${className ?? ""}`}
    >
      {message}
    </p>
  );
}

/**
 * Public-demo warning for guest forms that collect personal data
 * (reservations, delivery, customer accounts). Renders nothing unless
 * /api/v1/instance reports DEMO_MODE, the same signal as the demo banner.
 */
export function GuestDemoPersonalDataNotice({ className }: { className?: string }) {
  const { instance } = useInstance();
  const { t } = useGuestTranslation();
  if (!isPublicDemo(instance)) return null;
  return <NoticeView message={t(DEMO_PERSONAL_DATA_KEY)} className={className} />;
}
