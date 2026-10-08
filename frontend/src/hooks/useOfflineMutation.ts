"use client";

import { useCallback, useEffect, useRef, useState } from "react";
import { useConnectivity } from "@/contexts/ConnectivityContext";
import {
  getQueue,
  enqueue,
  drainQueue,
  type QueuedMutation,
} from "@/lib/mutationQueue";
import { axiosInstance } from "@/api/tools/instance";
import { useToast } from "@/contexts/ToastContext";

// Backoff for re-draining items that failed transiently but still have retries
// left. Without this, a queued mutation that 500s on first replay sat in
// localStorage with no further attempt until the device physically toggled
// offline -> online again.
const REDRAIN_DELAYS_MS = [3000, 8000, 20000];

export function useOfflineMutation(userId: string) {
  const { isOnline } = useConnectivity();
  const [pendingCount, setPendingCount] = useState(0);
  const { showSuccess, showError } = useToast();
  const toastRef = useRef({ showSuccess, showError });
  toastRef.current = { showSuccess, showError };
  const redrainRef = useRef<ReturnType<typeof setTimeout> | null>(null);

  useEffect(() => {
    if (!userId) return;
    setPendingCount(getQueue(userId).length);
  }, [userId, isOnline]);

  useEffect(() => {
    if (!isOnline || !userId) return;

    const queue = getQueue(userId);
    if (queue.length === 0) return;

    let cancelled = false;

    const runDrain = (attempt: number) => {
      drainQueue(userId, async (mutation: QueuedMutation) => {
        try {
          await axiosInstance({
            method: mutation.method,
            url: mutation.url,
            data: mutation.body,
          });
          return true;
        } catch {
          // Any rejection (real error, or an OfflineQueuedError if the device
          // dropped offline again mid-drain and the interceptor re-queued the
          // mutation) means the item was NOT delivered — leave it queued for a
          // later attempt rather than removing it.
          return false;
        }
      })
        .then(({ succeeded, failed }) => {
          if (cancelled) return;
          const remaining = getQueue(userId).length;
          setPendingCount(remaining);
          if (succeeded > 0) {
            toastRef.current.showSuccess(
              "Offline actions synced",
              `${succeeded} pending ${succeeded === 1 ? "action" : "actions"} sent successfully.`,
            );
          }
          if (failed > 0) {
            toastRef.current.showError(
              "Some actions failed",
              `${failed} offline ${failed === 1 ? "action" : "actions"} could not be sent.`,
            );
          }
          // Items that failed transiently but still have retries left are
          // neither removed nor counted; re-drain on a bounded backoff so they
          // are not stranded until the next offline->online toggle.
          if (remaining > 0 && attempt < REDRAIN_DELAYS_MS.length) {
            redrainRef.current = setTimeout(
              () => runDrain(attempt + 1),
              REDRAIN_DELAYS_MS[attempt],
            );
          }
        })
        .catch(() => {});
    };

    runDrain(0);

    return () => {
      cancelled = true;
      if (redrainRef.current) clearTimeout(redrainRef.current);
    };
  }, [isOnline, userId]);

  const queueMutation = useCallback(
    (method: string, url: string, body: unknown) => {
      const entry = enqueue(userId, { method, url, body });
      setPendingCount(getQueue(userId).length);
      return entry;
    },
    [userId],
  );

  return { pendingCount, queueMutation };
}
