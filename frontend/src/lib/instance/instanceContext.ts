"use client";

import { createContext } from "react";
import type { InstanceInfo } from "@/lib/instance/instanceInfo";

/**
 * The root layout's server-fetched GET /api/v1/instance payload. The window
 * seed only exists in the browser, so without this the server render saw
 * "unknown" while the first client render saw the real answer: gated UI and
 * branding then differed between SSR HTML and hydration (a flash plus a
 * hydration mismatch). Reading the same payload from context on both sides
 * keeps them identical.
 */
export const InstanceContext = createContext<InstanceInfo | null>(null);
