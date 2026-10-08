import {
  TABLERO_CHROME_CLASS,
  TABLERO_CHROME_MAX_HEIGHT_CLASS,
  TABLERO_CHROME_MAX_HEIGHT_PX,
  TABLERO_VIEWPORT_1024,
  TABLERO_VIEWPORT_CLIPPED,
  fillShellClipsBoard,
  reservationBoardChromeClipsPane,
  tableroBoardMinHeightPx,
  tableroHostStandBudget,
} from "./reservationBoardLayout";

describe("tablero host-stand layout budget", () => {
  it("fits Waitlist / Seated / Completed on a 1024×768 overflow-hidden shell", () => {
    const budget = tableroHostStandBudget(TABLERO_VIEWPORT_1024);
    expect(budget.boardMinPx).toBeGreaterThanOrEqual(256);
    expect(budget.fits).toBe(true);
  });

  it("still fits the original y≈701 / 696px clip viewport", () => {
    const budget = tableroHostStandBudget(TABLERO_VIEWPORT_CLIPPED);
    expect(tableroBoardMinHeightPx(696)).toBe(256);
    expect(budget.fits).toBe(true);
  });

  it("fails if board chrome loses its max-height cap (approval + filters)", () => {
    expect(TABLERO_CHROME_MAX_HEIGHT_PX).toBe(192);
    expect(TABLERO_CHROME_CLASS).toContain(TABLERO_CHROME_MAX_HEIGHT_CLASS);
    expect(TABLERO_CHROME_CLASS).toMatch(/overflow-y-auto/);
    expect(reservationBoardChromeClipsPane(TABLERO_CHROME_CLASS)).toBe(false);
    expect(
      reservationBoardChromeClipsPane("flex shrink-0 flex-col gap-3"),
    ).toBe(true);
    expect(
      reservationBoardChromeClipsPane("max-h-[44rem] overflow-y-auto shrink-0 min-h-0"),
    ).toBe(true);
  });

  it("fails if the fill shell still uses space-y (clips flex-1 board)", () => {
    expect(fillShellClipsBoard("flex h-full min-h-0 flex-col gap-3 p-4")).toBe(
      false,
    );
    expect(
      fillShellClipsBoard("flex h-full min-h-0 flex-col space-y-5 p-4 sm:p-6"),
    ).toBe(true);
  });
});
