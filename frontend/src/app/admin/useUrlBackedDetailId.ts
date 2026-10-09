import { useCallback, useEffect, useState } from "react";

export function adminDetailIdFromParam(raw: string | null | undefined): number | null {
  if (!raw) return null;
  const id = Number(raw);
  if (!Number.isInteger(id) || id <= 0) return null;
  return id;
}

function adminDetailDismissKey(key: string): string {
  return `payverge:admin-detail-dismiss:${key}`;
}

function readDismissed(key: string): number | null {
  if (typeof sessionStorage === "undefined") return null;
  try {
    return adminDetailIdFromParam(sessionStorage.getItem(adminDetailDismissKey(key)));
  } catch {
    return null;
  }
}

function writeDismissed(key: string, id: number | null): void {
  if (typeof sessionStorage === "undefined") return;
  try {
    if (id == null) sessionStorage.removeItem(adminDetailDismissKey(key));
    else sessionStorage.setItem(adminDetailDismissKey(key), String(id));
  } catch {
    // private mode / quota — in-memory closedLocally still applies
  }
}

/**
 * URL-backed admin detail dialog id that stays closed after dismiss even when
 * the App Router remounts the page while ?user= / ?business= is still present.
 */
export function useUrlBackedDetailId(
  searchParams: { get: (key: string) => string | null } | null,
  key: string,
  replace: (id: number | null) => void,
) {
  const urlId = adminDetailIdFromParam(searchParams?.get(key) ?? null);
  const [localId, setLocalId] = useState<number | null>(() =>
    readDismissed(key) === urlId ? null : urlId,
  );
  const [closedLocally, setClosedLocally] = useState(
    () => readDismissed(key) === urlId && urlId != null,
  );

  useEffect(() => {
    if (!urlId) {
      writeDismissed(key, null);
      setClosedLocally(false);
      setLocalId(null);
      return;
    }
    if (readDismissed(key) === urlId) {
      setClosedLocally(true);
      setLocalId(null);
      return;
    }
    setLocalId(urlId);
    setClosedLocally(false);
  }, [urlId, key]);

  const open = useCallback(
    (id: number) => {
      writeDismissed(key, null);
      setClosedLocally(false);
      setLocalId(id);
      replace(id);
    },
    [key, replace],
  );

  const close = useCallback(() => {
    writeDismissed(key, localId ?? urlId);
    setClosedLocally(true);
    setLocalId(null);
    replace(null);
  }, [key, localId, replace, urlId]);

  const dismissed = readDismissed(key) === urlId && urlId != null;
  return {
    selectedId: closedLocally || dismissed ? null : (localId ?? urlId),
    open,
    close,
  };
}
