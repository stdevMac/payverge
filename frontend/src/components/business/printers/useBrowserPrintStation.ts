"use client";

import { useCallback, useEffect, useState } from "react";

const STATION_EVENT = "payverge:print-station-changed";
const stationKey = (businessId: number) =>
  `payverge_print_station:${businessId}`;

const readStation = (businessId: number): number | null => {
  if (businessId <= 0 || typeof window === "undefined") return null;
  try {
    // Privacy modes / blocked site data make the localStorage accessor itself
    // throw SecurityError; treat that as "no station armed" instead of
    // crashing every consumer (RecentPrintJobs renders through this hook).
    const parsed = Number(window.localStorage.getItem(stationKey(businessId)));
    return Number.isInteger(parsed) && parsed > 0 ? parsed : null;
  } catch {
    return null;
  }
};

export function useBrowserPrintStation(businessId: number | null) {
  const id = businessId ?? 0;
  const [printerId, setPrinterId] = useState<number | null>(null);

  useEffect(() => {
    setPrinterId(readStation(id));
    if (id <= 0) return;

    const sync = () => setPrinterId(readStation(id));
    const onStorage = (event: StorageEvent) => {
      if (event.key === stationKey(id)) sync();
    };
    window.addEventListener("storage", onStorage);
    window.addEventListener(STATION_EVENT, sync);
    return () => {
      window.removeEventListener("storage", onStorage);
      window.removeEventListener(STATION_EVENT, sync);
    };
  }, [id]);

  const selectPrinter = useCallback(
    (nextPrinterId: number) => {
      if (id <= 0 || !Number.isInteger(nextPrinterId) || nextPrinterId <= 0) {
        return;
      }
      try {
        window.localStorage.setItem(stationKey(id), String(nextPrinterId));
      } catch {
        return; // Storage blocked: nothing persisted, so nothing to announce.
      }
      window.dispatchEvent(new CustomEvent(STATION_EVENT));
    },
    [id],
  );

  const stopStation = useCallback(() => {
    if (id <= 0) return;
    try {
      window.localStorage.removeItem(stationKey(id));
    } catch {
      return; // Storage blocked: nothing was persisted to remove.
    }
    window.dispatchEvent(new CustomEvent(STATION_EVENT));
  }, [id]);

  return { printerId, selectPrinter, stopStation };
}
