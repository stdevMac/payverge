"use client";

import { useCallback } from "react";

export function useIframePrint() {
  const print = useCallback(async (html: string) => {
    return new Promise<void>((resolve, reject) => {
      const iframe = document.createElement("iframe");
      let settled = false;
      let fallbackTimer: ReturnType<typeof setTimeout> | null = null;
      iframe.setAttribute("data-print-hidden", "true");
      iframe.style.position = "fixed";
      iframe.style.right = "0";
      iframe.style.bottom = "0";
      iframe.style.width = "0";
      iframe.style.height = "0";
      iframe.style.border = "0";
      document.body.appendChild(iframe);

      const clearFallback = () => {
        if (fallbackTimer) {
          clearTimeout(fallbackTimer);
          fallbackTimer = null;
        }
      };

      // Immediate removal — only safe on error/reject paths that never opened a
      // print dialog (no afterprint dispatch is in flight to destroy).
      const cleanup = () => {
        clearFallback();
        if (iframe.parentElement) iframe.parentElement.removeChild(iframe);
      };

      const finish = () => {
        if (settled) return;
        settled = true;
        clearFallback();
        // Restore focus to the parent: on cancel, focus is stranded inside the
        // (about-to-be-removed) print frame, leaving the page unresponsive.
        window.focus();
        // Resolve promptly so callers' "printing" spinners clear.
        resolve();
        // Defer frame removal: Chrome fires afterprint while print() is still on
        // the call stack, so removing the iframe now would destroy the browsing
        // context mid-dispatch and hang the renderer. Guarded for double-removal.
        setTimeout(() => {
          if (iframe.parentElement) iframe.parentElement.removeChild(iframe);
        }, 500);
      };

      iframe.addEventListener("load", () => {
        try {
          const printWindow = iframe.contentWindow;
          if (!printWindow) {
            cleanup();
            reject(new Error("iframe contentWindow unavailable"));
            return;
          }
          printWindow.addEventListener("afterprint", finish, { once: true });
          printWindow.focus();
          printWindow.print();
          fallbackTimer = setTimeout(finish, 5000);
        } catch (e) {
          cleanup();
          reject(e);
        }
      });

      const doc = iframe.contentWindow?.document;
      if (!doc) {
        cleanup();
        reject(new Error("iframe contentWindow unavailable"));
        return;
      }
      doc.open();
      doc.write(html);
      doc.close();
    });
  }, []);

  return { print };
}
