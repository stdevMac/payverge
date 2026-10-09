/** @jest-environment jsdom */
import { act, renderHook } from "@testing-library/react";
import { useSetUrlParams, useUrlState } from "../useUrlState";

const mockReplace = jest.fn();
const mockPush = jest.fn();
let mockSearch = new URLSearchParams();
let mockPathname = "/business/1/dashboard";

jest.mock("next/navigation", () => ({
  useSearchParams: () => mockSearch,
  usePathname: () => mockPathname,
  useRouter: () => ({ replace: mockReplace, push: mockPush }),
}));

const SECTIONS = ["profile", "payments", "localization", "notifications"] as const;

beforeEach(() => {
  mockReplace.mockClear();
  mockPush.mockClear();
  mockSearch = new URLSearchParams();
  mockPathname = "/business/1/dashboard";
});

describe("useUrlState", () => {
  it("returns a valid URL value", () => {
    mockSearch = new URLSearchParams("section=payments");
    const { result } = renderHook(() =>
      useUrlState({
        key: "section",
        valid: SECTIONS,
        fallback: "profile",
      }),
    );
    expect(result.current[0]).toBe("payments");
    expect(mockReplace).not.toHaveBeenCalled();
  });

  it("falls back when the param is missing", () => {
    mockSearch = new URLSearchParams("tab=settings");
    const { result } = renderHook(() =>
      useUrlState({
        key: "section",
        valid: SECTIONS,
        fallback: "profile",
      }),
    );
    expect(result.current[0]).toBe("profile");
    // Absent default does not rewrite
    expect(mockReplace).not.toHaveBeenCalled();
  });

  it("normalizes invalid values and rewrites the URL via replace", () => {
    mockSearch = new URLSearchParams("tab=settings&section=garbage");
    const { result } = renderHook(() =>
      useUrlState({
        key: "section",
        valid: SECTIONS,
        fallback: "profile",
      }),
    );
    expect(result.current[0]).toBe("profile");
    expect(mockReplace).toHaveBeenCalledTimes(1);
    const href = mockReplace.mock.calls[0][0] as string;
    expect(href).toContain("/business/1/dashboard");
    // Default is omitted from URL
    expect(href).not.toContain("section=");
    expect(href).toContain("tab=settings");
    // Normalization never pushes
    expect(mockPush).not.toHaveBeenCalled();
  });

  it("writes back on setValue (replace by default)", () => {
    mockSearch = new URLSearchParams("tab=settings");
    const { result } = renderHook(() =>
      useUrlState({
        key: "section",
        valid: SECTIONS,
        fallback: "profile",
      }),
    );

    act(() => {
      result.current[1]("notifications");
    });

    expect(mockReplace).toHaveBeenCalled();
    const href = mockReplace.mock.calls[mockReplace.mock.calls.length - 1][0] as string;
    expect(href).toContain("section=notifications");
    expect(href).toContain("tab=settings");
  });

  it("uses push when history is push", () => {
    mockSearch = new URLSearchParams("tab=settings");
    const { result } = renderHook(() =>
      useUrlState({
        key: "section",
        valid: SECTIONS,
        fallback: "profile",
        history: "push",
      }),
    );

    act(() => {
      result.current[1]("payments");
    });

    expect(mockPush).toHaveBeenCalled();
    expect(mockReplace).not.toHaveBeenCalled();
    const href = mockPush.mock.calls[0][0] as string;
    expect(href).toContain("section=payments");
  });

  it("clears the param when setValue writes the fallback (omitDefault)", () => {
    mockSearch = new URLSearchParams("tab=settings&section=payments");
    const { result } = renderHook(() =>
      useUrlState({
        key: "section",
        valid: SECTIONS,
        fallback: "profile",
      }),
    );

    act(() => {
      result.current[1]("profile");
    });

    const href = mockReplace.mock.calls[mockReplace.mock.calls.length - 1][0] as string;
    expect(href).not.toContain("section=");
    expect(href).toContain("tab=settings");
  });

  it("does not rewrite a foreign tab's shared section param (#225)", () => {
    mockSearch = new URLSearchParams("tab=business-page&section=contact");
    const { result } = renderHook(() =>
      useUrlState({
        key: "section",
        valid: SECTIONS,
        fallback: "profile",
        activeWhen: { key: "tab", values: ["settings"] },
      }),
    );
    // Settings still falls back internally, but must not strip Contact.
    expect(result.current[0]).toBe("profile");
    expect(mockReplace).not.toHaveBeenCalled();
    expect(mockPush).not.toHaveBeenCalled();
  });

  it("still normalizes garbage while its owner tab is active", () => {
    mockSearch = new URLSearchParams("tab=settings&section=contact");
    const { result } = renderHook(() =>
      useUrlState({
        key: "section",
        valid: SECTIONS,
        fallback: "profile",
        activeWhen: { key: "tab", values: ["settings"] },
      }),
    );
    expect(result.current[0]).toBe("profile");
    expect(mockReplace).toHaveBeenCalledTimes(1);
    const href = mockReplace.mock.calls[0][0] as string;
    expect(href).not.toContain("section=");
    expect(href).toContain("tab=settings");
  });
});

describe("useSetUrlParams", () => {
  it("writes tab+sub+focus in a single replace (#376)", () => {
    mockSearch = new URLSearchParams("tab=crm&sub=segments");
    const { result } = renderHook(() => useSetUrlParams());

    act(() => {
      result.current([
        { key: "tab", value: "crm" },
        { key: "sub", value: "customers" },
        { key: "focus", value: "at-risk" },
      ]);
    });

    expect(mockReplace).toHaveBeenCalledTimes(1);
    const href = String(mockReplace.mock.calls[0][0]);
    expect(href).toContain("tab=crm");
    expect(href).toContain("sub=customers");
    expect(href).toContain("focus=at-risk");
    expect(mockPush).not.toHaveBeenCalled();
  });
});
