/** @jest-environment jsdom */
import React from "react";
import { render, screen, fireEvent } from "@testing-library/react";

jest.mock("next/navigation", () => ({
  useParams: () => ({ tableCode: "TBL7" }),
}));

const mockLogError = jest.fn();
jest.mock("@/utils/errorLogger", () => ({
  logError: (...args: unknown[]) => mockLogError(...args),
}));

import TableError from "../error";

describe("/t/[tableCode] scoped error boundary", () => {
  beforeEach(() => {
    mockLogError.mockClear();
  });

  it("offers a retry and a back-to-table link that preserves the table code", () => {
    const reset = jest.fn();
    render(<TableError error={new Error("boom")} reset={reset} />);

    fireEvent.click(screen.getByRole("button", { name: /try again/i }));
    expect(reset).toHaveBeenCalled();

    const link = screen.getByRole("link", { name: /back to your table/i });
    expect(link.getAttribute("href")).toBe("/t/TBL7");
  });

  it("calls logError with TableErrorBoundary on mount", () => {
    const error = new Error("render crash");
    render(<TableError error={error} reset={() => {}} />);
    expect(mockLogError).toHaveBeenCalledWith(
      error,
      "TableErrorBoundary",
      "render",
      { digest: undefined }
    );
  });
});
