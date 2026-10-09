/** @jest-environment jsdom */
import { act, renderHook } from "@testing-library/react";
import { useOptionalUrlState } from "../useUrlState";
import { normalizeOptionalUrlParam, needsOptionalUrlNormalization } from "../urlState";

const mockReplace = jest.fn();
const mockPush = jest.fn();
let mockSearch = new URLSearchParams();
let mockPathname = "/business/1/dashboard";

jest.mock("next/navigation", () => ({
  useSearchParams: () => mockSearch,
  usePathname: () => mockPathname,
  useRouter: () => ({ replace: mockReplace, push: mockPush }),
}));

const FOCUS = ["lapsed", "vip", "new", "at-risk"] as const;

beforeEach(() => {
  mockReplace.mockClear();
  mockPush.mockClear();
  mockSearch = new URLSearchParams();
  mockPathname = "/business/1/dashboard";
});

describe("normalizeOptionalUrlParam", () => {
  it("returns null when missing", () => {
    expect(normalizeOptionalUrlParam(null, FOCUS)).toBeNull();
    expect(normalizeOptionalUrlParam("", FOCUS)).toBeNull();
  });
  it("returns valid values", () => {
    expect(normalizeOptionalUrlParam("vip", FOCUS)).toBe("vip");
  });
  it("returns null for garbage", () => {
    expect(normalizeOptionalUrlParam("nope", FOCUS)).toBeNull();
  });
});

describe("needsOptionalUrlNormalization", () => {
  it("is true only when raw is present but invalid", () => {
    expect(needsOptionalUrlNormalization("garbage", null)).toBe(true);
    expect(needsOptionalUrlNormalization("vip", "vip")).toBe(false);
    expect(needsOptionalUrlNormalization(null, null)).toBe(false);
  });
});

describe("useOptionalUrlState (L5-11 CRM focus)", () => {
  it("reads a valid focus value", () => {
    mockSearch = new URLSearchParams("tab=crm&focus=vip");
    const { result } = renderHook(() =>
      useOptionalUrlState({ key: "focus", valid: FOCUS }),
    );
    expect(result.current[0]).toBe("vip");
  });

  it("strips invalid focus via replace", () => {
    mockSearch = new URLSearchParams("tab=crm&focus=garbage");
    const { result } = renderHook(() =>
      useOptionalUrlState({ key: "focus", valid: FOCUS }),
    );
    expect(result.current[0]).toBeNull();
    expect(mockReplace).toHaveBeenCalled();
    const href = mockReplace.mock.calls[0][0] as string;
    expect(href).not.toContain("focus=");
    expect(href).toContain("tab=crm");
  });

  it("writes focus on setValue and clears on null", () => {
    mockSearch = new URLSearchParams("tab=crm");
    const { result } = renderHook(() =>
      useOptionalUrlState({ key: "focus", valid: FOCUS }),
    );
    act(() => {
      result.current[1]("lapsed");
    });
    expect(mockReplace.mock.calls.at(-1)?.[0]).toContain("focus=lapsed");

    act(() => {
      result.current[1](null);
    });
    const cleared = mockReplace.mock.calls.at(-1)?.[0] as string;
    expect(cleared).not.toContain("focus=");
    expect(cleared).toContain("tab=crm");
  });
});
