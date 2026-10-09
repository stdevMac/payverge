/** @jest-environment jsdom */
import React from "react";
import { render, screen, fireEvent } from "@testing-library/react";

const mockLogError = jest.fn();
jest.mock("@/utils/errorLogger", () => ({
  logError: (...args: unknown[]) => mockLogError(...args),
}));

import BusinessIdError from "../error";

describe("(shop)/business/[businessId] scoped error boundary", () => {
  beforeEach(() => mockLogError.mockClear());

  it("renders operator copy and a dashboard link, not guest cart copy", () => {
    render(<BusinessIdError error={new Error("boom")} reset={() => {}} />);
    // Operator-worded, not the diner "your cart is safe / Browse menu".
    expect(screen.queryByText(/your cart is safe/i)).not.toBeInTheDocument();
    expect(screen.queryByRole("link", { name: /browse menu/i })).not.toBeInTheDocument();
    const link = screen.getByRole("link", { name: /back to dashboard/i });
    expect(link.getAttribute("href")).toBe("/dashboard?venues=all");
  });

  it("offers a retry that calls reset", () => {
    const reset = jest.fn();
    render(<BusinessIdError error={new Error("boom")} reset={reset} />);
    fireEvent.click(screen.getByRole("button", { name: /try again/i }));
    expect(reset).toHaveBeenCalled();
  });

  it("logs with a BusinessIdErrorBoundary tag on mount", () => {
    const error = new Error("render crash");
    render(<BusinessIdError error={error} reset={() => {}} />);
    expect(mockLogError).toHaveBeenCalledWith(
      error,
      "BusinessIdErrorBoundary",
      "render",
      { digest: undefined }
    );
  });
});
