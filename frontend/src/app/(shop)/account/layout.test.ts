jest.mock("@/contexts/ToastContext", () => ({
  ToastProvider: ({ children }: { children: unknown }) => children,
}));

import { metadata } from "./layout";

describe("account route metadata", () => {
  it("provides an account document title instead of the marketing homepage string", () => {
    expect(metadata.title).toBe("Account — Payverge");
  });

  it("keeps the authenticated account surface out of search indexes", () => {
    expect(metadata.robots).toEqual({ index: false, follow: false });
  });
});
