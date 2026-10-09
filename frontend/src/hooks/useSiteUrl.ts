"use client";

import { useSyncExternalStore } from "react";

import { getRenderedSiteUrl, getSiteUrl } from "@/config/publicConfig";

// The value never changes after the config script ran; nothing to subscribe to.
const subscribe = () => () => {};

/**
 * The canonical site URL for a client component's render output.
 *
 * When PUBLIC_URL is unset the server renders the dev default and the browser
 * resolves its own origin. Calling getSiteUrl() during render would then
 * mismatch on hydration. This hook hydrates with the server's value and
 * re-renders with the browser's. Event handlers and effects can keep calling
 * getSiteUrl() directly.
 */
export function useSiteUrl(): string {
  return useSyncExternalStore(subscribe, getSiteUrl, getRenderedSiteUrl);
}
