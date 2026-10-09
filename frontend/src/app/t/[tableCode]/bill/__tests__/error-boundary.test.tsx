/** @jest-environment jsdom */
import React from "react";
import { render, screen, fireEvent } from "@testing-library/react";

jest.mock("next/navigation", () => ({
  useParams: () => ({ tableCode: "TBL9" }),
}));

const mockLogError = jest.fn();
jest.mock("@/utils/errorLogger", () => ({
  logError: (...args: unknown[]) => mockLogError(...args),
}));

import BillError from "../error";

describe("/t/[tableCode]/bill scoped error boundary (NEW-4)", () => {
  beforeEach(() => {
    mockLogError.mockClear();
  });

  it("offers a retry and a back-to-table link that preserves the table code", () => {
    const reset = jest.fn();
    render(<BillError error={new Error("boom")} reset={reset} />);

    fireEvent.click(screen.getByRole("button", { name: /try again/i }));
    expect(reset).toHaveBeenCalled();

    const link = screen.getByRole("link", { name: /back to your table/i });
    expect(link.getAttribute("href")).toBe("/t/TBL9");
  });

  it("calls logError with BillErrorBoundary on mount", () => {
    const error = new Error("render crash");
    render(<BillError error={error} reset={() => {}} />);
    expect(mockLogError).toHaveBeenCalledWith(
      error,
      "BillErrorBoundary",
      "render",
      { digest: undefined },
    );
  });
});
