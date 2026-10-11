/**
 * Host-stand Tablero layout budget.
 *
 * The operator dashboard is `h-[100dvh] overflow-hidden`; the window never
 * scrolls. Approval queue + filters used to paint at y≈701 and starve the
 * board. These constants cap that chrome and guarantee a remaining-viewport
 * scrollport so Waitlist / Seated / Completed stay reachable at 1024-class
 * heights (including the original 696px clip).
 */

const TABLERO_CHROME_MAX_HEIGHT_REM = 12;
export const TABLERO_CHROME_MAX_HEIGHT_PX = TABLERO_CHROME_MAX_HEIGHT_REM * 16;
export const TABLERO_CHROME_MAX_HEIGHT_CLASS = "max-h-[12rem]";

/** Approval + insights + filters: scroll inside the cap, never grow to y≈701. */
export const TABLERO_CHROME_CLASS = [
  "flex min-h-0 shrink-0 flex-col gap-3 overflow-y-auto",
  TABLERO_CHROME_MAX_HEIGHT_CLASS,
].join(" ");

export const TABLERO_BOARD_MIN_HEIGHT_CLASS =
  "min-h-[min(16rem,calc(100dvh-22rem))]";

export const TABLERO_VIEWPORT_1024 = { width: 1024, height: 768 } as const;
export const TABLERO_VIEWPORT_CLIPPED = { width: 1024, height: 696 } as const;

export function tableroBoardMinHeightPx(viewportHeightPx: number): number {
  return Math.min(16 * 16, viewportHeightPx - 22 * 16);
}

/**
 * Conservative paint of the overflow-hidden shell when Tablero is filling.
 * Must stay in lockstep with `DashboardTabShell` fill classes (`gap-3 p-4`,
 * no `space-y-*`) and a dense page header.
 */
export function tableroHostStandBudget(viewport: {
  width: number;
  height: number;
}): {
  consumedPx: number;
  boardMinPx: number;
  leftoverPx: number;
  fits: boolean;
} {
  const topMenuPx = 64;
  const fillShellPadYPx = 32;
  const denseHeaderPx = 72;
  const subTabsPx = 44;
  const fillGapsPx = 24;
  const boardMinPx = tableroBoardMinHeightPx(viewport.height);
  const consumedPx =
    topMenuPx +
    fillShellPadYPx +
    denseHeaderPx +
    subTabsPx +
    fillGapsPx +
    TABLERO_CHROME_MAX_HEIGHT_PX +
    boardMinPx;
  return {
    consumedPx,
    boardMinPx,
    leftoverPx: viewport.height - consumedPx,
    fits: consumedPx <= viewport.height,
  };
}

export function reservationBoardChromeClipsPane(className: string): boolean {
  const hasCap = className.includes(TABLERO_CHROME_MAX_HEIGHT_CLASS);
  const scrolls = className.includes("overflow-y-auto");
  const canShrink = className.includes("shrink-0") && className.includes("min-h-0");
  return !(hasCap && scrolls && canShrink);
}

export function fillShellClipsBoard(className: string): boolean {
  return /\bspace-y-/.test(className);
}
