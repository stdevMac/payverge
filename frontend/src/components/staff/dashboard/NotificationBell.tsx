"use client";

import React, { useCallback, useRef, useState } from "react";
import { useQuery, useQueryClient } from "@tanstack/react-query";
import { Bell } from "lucide-react";
import { notificationsApi } from "@/api/notifications";
import { useSSEEvents } from "@/hooks/useSSEEvents";
import { NotificationInbox } from "./NotificationInbox";

export interface NotificationBellLabels {
  bellAria: string;
  title: string;
  empty: string;
  markAll: string;
}

interface NotificationBellProps {
  businessId: string;
  staffId: number;
  labels: NotificationBellLabels;
  locale: string;
}

const unreadKey = (b: string) => ["staff", "notifications", "unread", b] as const;

function NotificationBell({ businessId, staffId, labels, locale }: NotificationBellProps) {
  const qc = useQueryClient();
  const [open, setOpen] = useState(false);

  const { data: unread = 0 } = useQuery({
    queryKey: unreadKey(businessId),
    queryFn: () => notificationsApi.unreadCount(businessId),
  });

  // Live nudge: refetch the count + open feed only when this staffer is targeted.
  useSSEEvents({
    businessId: Number(businessId),
    enabled: true,
    onEvent: (event: { type: string; data?: { staff_ids?: number[] } }) => {
      if (event.type !== "notification.new") return;
      const ids = event.data?.staff_ids || [];
      if (ids.includes(staffId)) {
        qc.invalidateQueries({ queryKey: unreadKey(businessId) });
        qc.invalidateQueries({ queryKey: ["staff", "notifications", "list", businessId] });
      }
    },
    onReconnect: () => qc.invalidateQueries({ queryKey: unreadKey(businessId) }),
  });

  const containerRef = useRef<HTMLDivElement>(null);
  const toggle = useCallback(() => setOpen((v) => !v), []);

  return (
    <div ref={containerRef} className="relative"> 
      <button
        type="button"
        onClick={toggle}
        aria-label={labels.bellAria}
        className="relative flex h-10 w-10 items-center justify-center rounded-full text-gray-700 hover:bg-gray-100"
      >
        <Bell className="h-5 w-5" aria-hidden="true" />
        {unread > 0 && (
          <span
            className="absolute -right-0.5 -top-0.5 flex min-w-[18px] items-center justify-center rounded-full bg-brand px-1 text-[11px] font-semibold text-white"
            aria-live="polite"
          >
            {unread > 99 ? "99+" : unread}
          </span>
        )}
      </button>
      {open && (
        <NotificationInbox businessId={businessId} labels={labels} locale={locale} onClose={() => setOpen(false)} containerRef={containerRef} />
      )}
    </div>
  );
}

export default NotificationBell;
