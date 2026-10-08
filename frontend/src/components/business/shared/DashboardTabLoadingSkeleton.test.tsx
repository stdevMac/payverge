/** @jest-environment jsdom */
import React from "react";
import { render, screen } from "@testing-library/react";
import DashboardTabLoadingSkeleton from "./DashboardTabLoadingSkeleton";

jest.mock("@/i18n/SimpleTranslationProvider", () => {
  const actual = jest.requireActual("@/i18n/getTranslation");
  return {
    useSimpleLocale: () => ({ locale: "en", setLocale: jest.fn() }),
    getTranslation: actual.getTranslation,
  };
});

describe("DashboardTabLoadingSkeleton", () => {
  it("exposes a busy status region with an accessible label", () => {
    render(<DashboardTabLoadingSkeleton variant="overview" />);
    expect(screen.getByRole("status", { busy: true })).toBeInTheDocument();
    expect(screen.getByText("Loading overview…")).toBeInTheDocument();
  });
});
