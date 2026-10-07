"use client";

/**
 * True-viewport host for the Business Page live preview (#600, follow-up to
 * #591).
 *
 * The #591 preview mounts the REAL storefront tree, but it used to render
 * inline in the dashboard document — so Tailwind's `md:`/`lg:` (and any raw
 * `@media`) resolved against the operator's desktop browser viewport, and the
 * 380px phone frame showed desktop layout.
 *
 * This component renders a same-origin `about:blank` <iframe> sized to a real
 * phone viewport (380px wide) and PORTALS its children into the iframe's
 * document. Because the children keep running in the parent window's React
 * tree:
 *
 *   - props flow directly, so the preview still updates live with every
 *     keystroke of the operator's appearance edits (no postMessage protocol,
 *     no refetch-on-save channel),
 *   - all network reads still execute in the dashboard's JS context with the
 *     operator's existing session — there is no new URL, so there is no
 *     publicly reachable unauthenticated preview route to guard,
 *   - the isolated preview QueryClient and every #591 concession (no URL
 *     writes, no AI waiter, no fulfillment side effects, aria-hidden host,
 *     `data-storefront-preview="true"`) are untouched,
 *
 * while the DOM lives in the iframe document, so every CSS media query — and
 * vh units, and `position: sticky/fixed` — resolves against the 380px frame.
 *
 * Styles: Next.js injects Tailwind/global CSS and styled-jsx theme styles
 * into the PARENT document head. `syncHeadStyles` clones every head
 * stylesheet into the frame and a MutationObserver keeps them in sync (the
 * storefront's `StorefrontThemeStyle` rewrites its styled-jsx tag on each
 * brand-color edit). The parent <html>/<body> classes are mirrored too so the
 * next/font CSS variables (--font-dm-sans / --font-dm-serif-display) resolve
 * inside the frame.
 *
 * Known limit (documented, accepted): JS that reads `window.innerWidth` /
 * `window.matchMedia` (e.g. GoogleReviewsSlider's visible-card count) still
 * sees the parent window. CSS-driven breakpoints — the #600 gap — are fully
 * faithful.
 */

import React, { useCallback, useEffect, useRef, useState } from "react";
import { createPortal } from "react-dom";

const STYLE_SYNC_ATTR = "data-storefront-preview-style";
const BASE_STYLE_ATTR = "data-storefront-preview-base";
const MOUNT_ATTR = "data-storefront-preview-mount";

export type StorefrontPreviewFrameProps = {
  /** Accessible name for the iframe element (required on iframes). */
  title: string;
  /** Frame viewport width in CSS px — the breakpoint the preview renders at. */
  width?: number;
  /** Frame viewport height in CSS px. */
  height?: number;
  className?: string;
  children: React.ReactNode;
};

/**
 * Copy parent `head` style tags plus `link[rel=stylesheet]` into the iframe
 * head, replacing the previous copies, and mirror the root/body classes
 * (next/font variables, Tailwind base hooks). Idempotent and cheap enough to
 * re-run on head mutations.
 */
function syncHeadStyles(sourceDoc: Document, targetDoc: Document): void {
  const targetHead = targetDoc.head;
  if (!targetHead) return;

  const fresh: HTMLElement[] = [];
  sourceDoc
    .querySelectorAll('head style, head link[rel="stylesheet"]')
    .forEach((node) => {
      const clone = targetDoc.importNode(node, true) as HTMLElement;
      clone.setAttribute(STYLE_SYNC_ATTR, "true");
      fresh.push(clone);
    });

  // Append the new copies before removing the old ones so the frame never
  // paints a fully unstyled flash mid-sync.
  const stale = Array.from(
    targetHead.querySelectorAll(`[${STYLE_SYNC_ATTR}]`),
  );
  fresh.forEach((node) => targetHead.appendChild(node));
  stale.forEach((node) => node.remove());

  targetDoc.documentElement.className = sourceDoc.documentElement.className;
  if (targetDoc.body && sourceDoc.body) {
    targetDoc.body.className = sourceDoc.body.className;
  }
}

export default function StorefrontPreviewFrame({
  title,
  width = 380,
  height = 700,
  className,
  children,
}: StorefrontPreviewFrameProps) {
  const iframeRef = useRef<HTMLIFrameElement | null>(null);
  const [mountNode, setMountNode] = useState<HTMLElement | null>(null);

  /**
   * (Re)initialize the frame document: base reset styles, a stable portal
   * mount node, and an initial style sync. Idempotent — Firefox and jsdom can
   * replace the initial about:blank document on a late "load" event, in which
   * case a new mount node is created in the final document and the portal
   * remounts there.
   */
  const initFrame = useCallback(() => {
    const doc = iframeRef.current?.contentDocument;
    if (!doc || !doc.body) return;

    if (!doc.head.querySelector(`[${BASE_STYLE_ATTR}]`)) {
      const base = doc.createElement("style");
      base.setAttribute(BASE_STYLE_ATTR, "true");
      // Margin reset + hidden scrollbars: the frame body scrolls like the
      // phone screen did, without a desktop scrollbar eating viewport width.
      base.textContent =
        "html,body{margin:0;padding:0;}" +
        "html{scrollbar-width:none;-ms-overflow-style:none;}" +
        "::-webkit-scrollbar{width:0;height:0;}";
      doc.head.appendChild(base);
    }

    let mount = doc.body.querySelector<HTMLElement>(`[${MOUNT_ATTR}]`);
    if (!mount) {
      mount = doc.createElement("div");
      mount.setAttribute(MOUNT_ATTR, "true");
      doc.body.appendChild(mount);
    }

    syncHeadStyles(document, doc);
    setMountNode((prev) => (prev === mount ? prev : mount));
  }, []);

  useEffect(() => {
    const iframe = iframeRef.current;
    if (!iframe) return;
    initFrame();
    iframe.addEventListener("load", initFrame);
    return () => iframe.removeEventListener("load", initFrame);
  }, [initFrame]);

  // Keep the frame's stylesheet copies in sync with the parent head:
  // styled-jsx rewrites the storefront theme tag on every brand edit, and
  // lazily-loaded chunks append their CSS after mount. rAF-coalesced.
  useEffect(() => {
    if (!mountNode) return;
    const targetDoc = mountNode.ownerDocument;
    let scheduled = false;
    const schedule = () => {
      if (scheduled) return;
      scheduled = true;
      const run = () => {
        scheduled = false;
        syncHeadStyles(document, targetDoc);
      };
      if (typeof requestAnimationFrame === "function") {
        requestAnimationFrame(run);
      } else {
        setTimeout(run, 0);
      }
    };
    const observer = new MutationObserver(schedule);
    observer.observe(document.head, {
      childList: true,
      subtree: true,
      characterData: true,
    });
    return () => observer.disconnect();
  }, [mountNode]);

  return (
    <>
      {/* Decorative preview: aria-hidden + unfocusable, but still titled so
          the element is self-describing in devtools/tests. No `src` — the
          same-origin about:blank document is populated via portal, so no
          preview URL exists to reach unauthenticated. */}
      <iframe
        ref={iframeRef}
        title={title}
        aria-hidden="true"
        tabIndex={-1}
        data-testid="storefront-preview-frame"
        className={className}
        style={{ width: `${width}px`, height: `${height}px`, border: "0", display: "block" }}
      />
      {mountNode ? createPortal(children, mountNode) : null}
    </>
  );
}
