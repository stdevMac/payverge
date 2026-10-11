/** @jest-environment jsdom */
import { renderHook, act } from "@testing-library/react";
import { useDirtyForm } from "@/hooks/useDirtyForm";

describe("useDirtyForm", () => {
  it("is not dirty until a baseline is captured (not-loaded guard)", () => {
    const { result } = renderHook(
      ({ value }) => useDirtyForm(value),
      { initialProps: { value: { a: 1 } } },
    );
    expect(result.current.dirty).toBe(false);
  });

  it("is not dirty after markClean when value matches the baseline", () => {
    const { result } = renderHook(
      ({ value }) => useDirtyForm(value),
      { initialProps: { value: { a: 1, b: "x" } } },
    );
    act(() => {
      result.current.markClean();
    });
    expect(result.current.dirty).toBe(false);
  });

  it("becomes dirty when current deep-differs from the baseline", () => {
    const { result, rerender } = renderHook(
      ({ value }) => useDirtyForm(value),
      { initialProps: { value: { a: 1 } } },
    );
    act(() => {
      result.current.markClean({ a: 1 });
    });
    rerender({ value: { a: 2 } });
    expect(result.current.dirty).toBe(true);
  });

  it("deep-compares nested objects (not reference identity)", () => {
    const baseline = { nested: { n: 1 }, list: [1, 2] };
    const { result, rerender } = renderHook(
      ({ value }) => useDirtyForm(value),
      { initialProps: { value: baseline } },
    );
    act(() => {
      result.current.markClean(baseline);
    });
    // New object, same contents — still clean.
    rerender({ value: { nested: { n: 1 }, list: [1, 2] } });
    expect(result.current.dirty).toBe(false);
    // Nested change — dirty.
    rerender({ value: { nested: { n: 2 }, list: [1, 2] } });
    expect(result.current.dirty).toBe(true);
  });

  it("markClean(snapshot) re-baselines to the provided value", () => {
    const { result, rerender } = renderHook(
      ({ value }) => useDirtyForm(value),
      { initialProps: { value: { a: 1 } } },
    );
    act(() => {
      result.current.markClean({ a: 1 });
    });
    rerender({ value: { a: 9 } });
    expect(result.current.dirty).toBe(true);
    act(() => {
      result.current.markClean({ a: 9 });
    });
    expect(result.current.dirty).toBe(false);
  });

  it("clearBaseline returns to the not-loaded (not dirty) state", () => {
    const { result, rerender } = renderHook(
      ({ value }) => useDirtyForm(value),
      { initialProps: { value: { a: 1 } } },
    );
    act(() => {
      result.current.markClean({ a: 1 });
    });
    rerender({ value: { a: 2 } });
    expect(result.current.dirty).toBe(true);
    act(() => {
      result.current.clearBaseline();
    });
    expect(result.current.dirty).toBe(false);
  });
});
