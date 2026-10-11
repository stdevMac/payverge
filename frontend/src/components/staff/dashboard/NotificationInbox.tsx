"use client";

import React, { useEffect, useRef } from "react";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { notificationsApi, type StaffNotification } from "@/api/notifications";
import { SkeletonList } from "@/components/ui/skeletons";
import type { NotificationBellLabels } from "./NotificationBell";

export interface NotificationInboxProps {
  businessId: string;
  labels: NotificationBellLabels;
  locale: string;
  onClose: () => void;
  containerRef?: React.RefObject<HTMLElement | null>;
}

const listKey = (b: string) => ["staff", "notifications", "list", b] as const;
const unreadKey = (b: string) => ["staff", "notifications", "unread", b] as const;

export function NotificationInbox({ businessId, labels, locale, onClose, containerRef }: NotificationInboxProps) {
  const qc = useQueryClient();
  const markedUnreadForBusinessRef = useRef<string | null>(null);
  const { data: items = [], isLoading } = useQuery({
    queryKey: listKey(businessId),
    queryFn: () => notificationsApi.list(businessId, { limit: 50 }),
  });

  const markAll = useMutation({
    mutationFn: () => notificationsApi.markRead(businessId, { all: true }),
    onSuccess: () => {
      qc.invalidateQueries({ queryKey: unreadKey(businessId) });
      qc.invalidateQueries({ queryKey: listKey(businessId) });
    },
    onError: () => {
      if (markedUnreadForBusinessRef.current === businessId) {
        markedUnreadForBusinessRef.current = null;
      }
    },
  });

  // Opening the inbox marks everything read (one shot).
  useEffect(() => {
    if (
      !isLoading &&
      markedUnreadForBusinessRef.current !== businessId &&
      items.some((n) => n.read_at === null)
    ) {
      markedUnreadForBusinessRef.current = businessId;
      markAll.mutate();
    }
  }, [businessId, isLoading, items, markAll]);

  const dialogRef = useRef<HTMLDivElement>(null);

  // Close on Escape key.
  useEffect(() => {
    const handler = (e: KeyboardEvent) => {
      if (e.key === "Escape") onClose();
    };
    document.addEventListener("keydown", handler);
    return () => document.removeEventListener("keydown", handler);
  }, [onClose]);

  // Close on outside click. Uses the container boundary (includes the trigger)
  // when provided, else falls back to the dialog itself.
  useEffect(() => {
    const handler = (e: PointerEvent) => {
      const boundary = containerRef?.current ?? dialogRef.current;
      if (boundary && !boundary.contains(e.target as Node)) {
        onClose();
      }
    };
    document.addEventListener("pointerdown", handler);
    return () => document.removeEventListener("pointerdown", handler);
  }, [onClose, containerRef]);

  return (
    <div
      ref={dialogRef}
      role="dialog"
      aria-label={labels.title}
      className="absolute right-0 z-50 mt-2 w-80 rounded-2xl border border-gray-200 bg-white p-2 shadow-lg"
    >
      <div className="flex items-center justify-between px-2 py-1">
        <span className="text-sm font-semibold text-gray-900">{labels.title}</span>
        <button type="button" onClick={() => markAll.mutate()} className="text-xs text-brand hover:underline">
          {labels.markAll}
        </button>
      </div>
      {isLoading ? (
        <SkeletonList rows={3} bordered={false} className="px-1" />
      ) : items.length === 0 ? (
        <p className="px-2 py-6 text-center text-sm text-gray-500">{labels.empty}</p>
      ) : (
        <ul className="max-h-96 overflow-y-auto">
          {items.map((n: StaffNotification) => (
            <li key={n.id}>
              <a
                href={n.url || "#"}
                onClick={onClose}
                className={
                  "block rounded-xl px-3 py-2 hover:bg-gray-50 " + (n.read_at === null ? "bg-brand-50/60" : "")
                }
              >
                <span className="block text-sm font-medium text-gray-900">{n.title}</span>
                {n.body ? <span className="block text-xs text-gray-600">{n.body}</span> : null}
                <span className="mt-0.5 block text-[11px] text-gray-400">
                  {new Date(n.created_at).toLocaleString(locale)}
                </span>
              </a>
            </li>
          ))}
        </ul>
      )}
    </div>
  );
}
