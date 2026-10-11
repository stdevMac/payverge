"use client";

import { useCallback, useEffect, useRef, useState } from "react";
import {
  spacesApi,
  type LayoutDocument,
  type PutDraftResponse,
} from "@/api/spaces";
import type { AutosaveStatus } from "../types";

const DEBOUNCE_MS = 800;

export interface UseAutosaveOptions {
  businessId: number;
  spaceId: number;
  /** Draft revision expected by the server (optimistic concurrency). */
  draftRevision: number;
  onRevisionChange: (revision: number) => void;
  /** Serialize current editor document to API layout. */
  getLayout: () => LayoutDocument;
  /** Bumps when local document changes and should be saved. */
  dirtyToken: number;
  enabled?: boolean;
}

export function useAutosave({
  businessId,
  spaceId,
  draftRevision,
  onRevisionChange,
  getLayout,
  dirtyToken,
  enabled = true,
}: UseAutosaveOptions) {
  const [status, setStatus] = useState<AutosaveStatus>("idle");
  const [lastError, setLastError] = useState<string | null>(null);
  const [lastSavedAt, setLastSavedAt] = useState<number | null>(null);
  const revisionRef = useRef(draftRevision);
  revisionRef.current = draftRevision;
  const timerRef = useRef<ReturnType<typeof setTimeout> | null>(null);
  const inFlightRef = useRef(false);
  const pendingRef = useRef(false);
  const lastSavedToken = useRef(dirtyToken);
  const mountedRef = useRef(true);

  useEffect(() => {
    mountedRef.current = true;
    return () => {
      mountedRef.current = false;
      if (timerRef.current) clearTimeout(timerRef.current);
    };
  }, []);

  const saveNow = useCallback(async (): Promise<PutDraftResponse | null> => {
    if (!enabled) return null;
    if (typeof navigator !== "undefined" && !navigator.onLine) {
      setStatus("offline");
      return null;
    }
    if (inFlightRef.current) {
      pendingRef.current = true;
      return null;
    }
    inFlightRef.current = true;
    setStatus("saving");
    setLastError(null);
    try {
      const layout = getLayout();
      const res = await spacesApi.putLayoutDraft(businessId, spaceId, {
        expected_revision: revisionRef.current,
        layout,
      });
      if (!mountedRef.current) return res;
      lastSavedToken.current = dirtyToken;
      onRevisionChange(res.draft_revision);
      revisionRef.current = res.draft_revision;
      setLastSavedAt(Date.now());
      setStatus("saved");
      return res;
    } catch (err: unknown) {
      if (!mountedRef.current) return null;
      const ax = err as {
        response?: { status?: number; data?: { code?: string; error?: string } };
        message?: string;
      };
      const code = ax?.response?.data?.code;
      if (code === "revision_conflict" || ax?.response?.status === 409) {
        setStatus("conflict");
        setLastError(code || "revision_conflict");
      } else if (typeof navigator !== "undefined" && !navigator.onLine) {
        setStatus("offline");
      } else {
        setStatus("failed");
        setLastError(
          ax?.response?.data?.error ||
            ax?.message ||
            "save_failed",
        );
      }
      return null;
    } finally {
      inFlightRef.current = false;
      if (pendingRef.current && mountedRef.current) {
        pendingRef.current = false;
        void saveNow();
      }
    }
  }, [
    businessId,
    spaceId,
    enabled,
    getLayout,
    dirtyToken,
    onRevisionChange,
  ]);

  // Debounced save when dirtyToken advances past last saved.
  useEffect(() => {
    if (!enabled) return;
    if (dirtyToken === lastSavedToken.current) return;
    if (dirtyToken === 0) return;
    setStatus((s) => (s === "conflict" ? s : "dirty"));
    if (timerRef.current) clearTimeout(timerRef.current);
    timerRef.current = setTimeout(() => {
      void saveNow();
    }, DEBOUNCE_MS);
    return () => {
      if (timerRef.current) clearTimeout(timerRef.current);
    };
  }, [dirtyToken, enabled, saveNow]);

  useEffect(() => {
    const onOnline = () => {
      if (status === "offline" || status === "dirty") {
        void saveNow();
      }
    };
    const onOffline = () => setStatus("offline");
    window.addEventListener("online", onOnline);
    window.addEventListener("offline", onOffline);
    return () => {
      window.removeEventListener("online", onOnline);
      window.removeEventListener("offline", onOffline);
    };
  }, [saveNow, status]);

  const isDirty =
    dirtyToken !== lastSavedToken.current &&
    status !== "saved" &&
    status !== "idle";

  return {
    status,
    lastError,
    lastSavedAt,
    saveNow,
    isDirty,
    markClean: () => {
      lastSavedToken.current = dirtyToken;
      setStatus("saved");
    },
  };
}
