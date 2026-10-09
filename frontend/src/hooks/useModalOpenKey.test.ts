/** @jest-environment jsdom */
import { renderHook, act } from "@testing-library/react";
import { useModalOpenKey } from "./useModalOpenKey";

describe("useModalOpenKey (L3-13)", () => {
  it("starts at 0 and bumps each time isOpen becomes true", () => {
    const { result, rerender } = renderHook(
      ({ open }: { open: boolean }) => useModalOpenKey(open),
      { initialProps: { open: false } },
    );
    expect(result.current).toBe(0);

    act(() => {
      rerender({ open: true });
    });
    expect(result.current).toBe(1);

    act(() => {
      rerender({ open: false });
    });
    expect(result.current).toBe(1);

    act(() => {
      rerender({ open: true });
    });
    expect(result.current).toBe(2);
  });
});
