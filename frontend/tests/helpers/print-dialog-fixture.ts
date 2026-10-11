/**
 * Playwright helpers for deterministic browser print dialogs.
 * Stubs window.print on future iframes so useIframePrint resolves without a
 * real OS print dialog.
 */
import type { Page } from "@playwright/test";

/**
 * Installs an iframe print stub that resolves as soon as print() is called.
 * Returns a promise that becomes true when print is invoked.
 */
export async function installPrintDialogStub(page: Page): Promise<() => Promise<boolean>> {
  await page.addInitScript(() => {
    (window as unknown as { __payvergePrintCalls?: number }).__payvergePrintCalls = 0;
    const origCreate = document.createElement.bind(document);
    document.createElement = ((tag: string, opts?: ElementCreationOptions) => {
      const el = origCreate(tag, opts);
      if (tag.toLowerCase() === "iframe") {
        el.addEventListener("load", () => {
          try {
            const cw = (el as HTMLIFrameElement).contentWindow;
            if (cw) {
              cw.print = () => {
                const w = window as unknown as { __payvergePrintCalls?: number };
                w.__payvergePrintCalls = (w.__payvergePrintCalls ?? 0) + 1;
              };
            }
          } catch {
            /* ignore */
          }
        });
      }
      return el;
    }) as typeof document.createElement;
  });

  return async () => {
    return page.evaluate(() => {
      const w = window as unknown as { __payvergePrintCalls?: number };
      return (w.__payvergePrintCalls ?? 0) > 0;
    });
  };
}

export type MenuPrintWindowCompletion = "cancel" | "complete" | "none";

/**
 * Installs a stub for future same-origin menu print popups. The cancel mode
 * intentionally dispatches afterprint on the print() call stack, matching the
 * browser lifecycle that previously left the operator app unresponsive.
 * Returns a getter for the number of popup print calls.
 */
export async function installMenuPrintWindowStub(
  page: Page,
  completion: MenuPrintWindowCompletion,
): Promise<() => Promise<number>> {
  await page.addInitScript((requestedCompletion) => {
    const parent = window as unknown as {
      __payvergeMenuPrintCalls?: number;
    };
    parent.__payvergeMenuPrintCalls = 0;
    const originalOpen = window.open.bind(window);

    window.open = ((url?: string | URL, target?: string, features?: string) => {
      const child = originalOpen(url, target, features);
      if (!child) return child;

      let isSameOrigin = false;
      try {
        const requestedUrl = new URL(String(url ?? ""), window.location.href);
        isSameOrigin = requestedUrl.origin === window.location.origin;
        void child.document;
      } catch {
        isSameOrigin = false;
      }
      if (!isSameOrigin) return child;

      child.print = () => {
        parent.__payvergeMenuPrintCalls =
          (parent.__payvergeMenuPrintCalls ?? 0) + 1;
        if (requestedCompletion === "cancel") {
          child.dispatchEvent(new Event("afterprint"));
        } else if (requestedCompletion === "complete") {
          queueMicrotask(() => child.dispatchEvent(new Event("afterprint")));
        }
      };
      return child;
    }) as typeof window.open;
  }, completion);

  return async () =>
    page.evaluate(() => {
      const current = window as unknown as {
        __payvergeMenuPrintCalls?: number;
      };
      return current.__payvergeMenuPrintCalls ?? 0;
    });
}
