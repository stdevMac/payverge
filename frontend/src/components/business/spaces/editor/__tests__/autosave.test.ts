/**
 * @jest-environment jsdom
 */
import { act, renderHook, waitFor } from "@testing-library/react";
import { useAutosave } from "../hooks/useAutosave";

const mockPut = jest.fn();

jest.mock("@/api/spaces", () => ({
  spacesApi: {
    putLayoutDraft: (...args: unknown[]) => mockPut(...args),
  },
}));

describe("useAutosave", () => {
  beforeEach(() => {
    jest.clearAllMocks();
    jest.useFakeTimers();
    mockPut.mockResolvedValue({
      space: { id: 1 },
      draft_revision: 2,
      has_unpublished_changes: true,
      validation: { valid: true },
    });
  });

  afterEach(() => {
    jest.useRealTimers();
  });

  it("debounces putLayoutDraft by 800ms and reports saved", async () => {
    const onRevisionChange = jest.fn();
    const getLayout = jest.fn(() => ({
      schema_version: 1,
      width_mm: 10000,
      height_mm: 8000,
      tables: [],
    }));

    const { result, rerender } = renderHook(
      ({ token }: { token: number }) =>
        useAutosave({
          businessId: 1,
          spaceId: 7,
          draftRevision: 1,
          onRevisionChange,
          getLayout,
          dirtyToken: token,
          enabled: true,
        }),
      { initialProps: { token: 0 } },
    );

    expect(result.current.status).toBe("idle");

    rerender({ token: 1 });

    await act(async () => {
      jest.advanceTimersByTime(799);
    });
    expect(mockPut).not.toHaveBeenCalled();

    await act(async () => {
      jest.advanceTimersByTime(2);
    });

    await waitFor(() => {
      expect(mockPut).toHaveBeenCalledWith(1, 7, {
        expected_revision: 1,
        layout: expect.objectContaining({ width_mm: 10000 }),
      });
    });

    await waitFor(() => {
      expect(result.current.status).toBe("saved");
    });
    expect(onRevisionChange).toHaveBeenCalledWith(2);
  });

  it("sets conflict status on revision_conflict", async () => {
    mockPut.mockRejectedValue({
      response: { status: 409, data: { code: "revision_conflict" } },
    });
    const getLayout = jest.fn(() => ({ schema_version: 1 }));

    const { result, rerender } = renderHook(
      ({ token }: { token: number }) =>
        useAutosave({
          businessId: 1,
          spaceId: 7,
          draftRevision: 3,
          onRevisionChange: jest.fn(),
          getLayout,
          dirtyToken: token,
          enabled: true,
        }),
      { initialProps: { token: 0 } },
    );

    rerender({ token: 5 });
    await act(async () => {
      jest.advanceTimersByTime(900);
    });

    await waitFor(() => {
      expect(result.current.status).toBe("conflict");
    });
  });
});
