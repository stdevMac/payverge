"use client";

import { useEffect, useState } from "react";
import {
  resolveErrorBoundaryCopy,
  resolveErrorBoundaryCopyForLocale,
  type BoundaryCopy,
} from "./errorBoundaryCopy";

/**
 * Boundary copy that hydrates cleanly: the first render (server and client)
 * is English, matching what the server can know, and the effect then switches
 * to the locale resolved from ?lang= / <html lang> / navigator. Reading the
 * browser locale during render made the client's first pass differ from the
 * server HTML and tripped a hydration mismatch.
 */
export function useErrorBoundaryCopy(): BoundaryCopy {
  const [copy, setCopy] = useState<BoundaryCopy>(() =>
    resolveErrorBoundaryCopyForLocale("en"),
  );
  useEffect(() => {
    setCopy(resolveErrorBoundaryCopy());
  }, []);
  return copy;
}
