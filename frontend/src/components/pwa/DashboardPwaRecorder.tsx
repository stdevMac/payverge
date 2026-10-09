"use client";

import { useEffect, useRef } from "react";
import { usePathname } from "next/navigation";
import { usePwaInstall } from "@/providers/PwaInstallProvider";

const BUSINESS_DASHBOARD_PATH = /^\/business\/[^/?#]+\/dashboard$/;

function isRecordableDashboardPath(pathname: string): boolean {
  return pathname === "/staff/home" || BUSINESS_DASHBOARD_PATH.test(pathname);
}

export default function DashboardPwaRecorder({ ready }: { ready: boolean }) {
  const pathname = usePathname();
  const { recordDashboardVisit } = usePwaInstall();
  const lastRecordedVisitRef = useRef<{
    pathname: string;
    recorder: typeof recordDashboardVisit;
  } | null>(null);

  useEffect(() => {
    const lastVisit = lastRecordedVisitRef.current;
    if (
      !ready ||
      !pathname ||
      !isRecordableDashboardPath(pathname) ||
      (lastVisit?.pathname === pathname &&
        lastVisit.recorder === recordDashboardVisit)
    ) {
      return;
    }

    lastRecordedVisitRef.current = {
      pathname,
      recorder: recordDashboardVisit,
    };
    recordDashboardVisit(pathname);
  }, [pathname, ready, recordDashboardVisit]);

  return null;
}
