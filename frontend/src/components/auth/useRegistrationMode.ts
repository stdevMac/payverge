"use client";

import { useCallback, useEffect, useState } from "react";
import {
  fetchRegistrationMode,
  peekRegistrationMode,
  type RegistrationMode,
} from "@/api/registrationMode";

/**
 * Reads the instance's REGISTRATION_MODE while `enabled` (the auth modal is
 * open). `mode` is null until known; callers treat null permissively and let
 * the backend enforce. `markClosed` applies a closed-signup refusal the
 * backend returned after the page loaded (e.g. the operator just flipped
 * the mode).
 */
export function useRegistrationMode(enabled: boolean): {
  mode: RegistrationMode | null;
  markClosed: () => void;
} {
  const [mode, setMode] = useState<RegistrationMode | null>(() =>
    peekRegistrationMode(),
  );

  useEffect(() => {
    if (!enabled) return;
    let cancelled = false;
    void fetchRegistrationMode().then((next) => {
      if (!cancelled && next) {
        setMode((current) => (current === "closed" ? current : next));
      }
    });
    return () => {
      cancelled = true;
    };
  }, [enabled]);

  const markClosed = useCallback(() => setMode("closed"), []);
  return { mode, markClosed };
}
