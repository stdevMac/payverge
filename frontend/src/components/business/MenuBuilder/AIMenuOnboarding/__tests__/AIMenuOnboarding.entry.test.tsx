/** @jest-environment jsdom */
/**
 * L3-21: AI onboarding opens straight into the tools — there is no plan gate.
 */
import React from "react";
import { render, screen } from "@testing-library/react";

jest.mock("../PDFDigitizer", () => ({
  __esModule: true,
  default: () => <div data-testid="pdf-digitizer">PDF</div>,
}));
jest.mock("../AIWizard", () => ({
  __esModule: true,
  default: () => <div data-testid="ai-wizard">Wizard</div>,
}));
jest.mock("../MenuReviewEditor", () => ({
  __esModule: true,
  default: () => null,
}));
jest.mock("@/i18n/SimpleTranslationProvider", () => ({
  useSimpleLocale: () => ({ locale: "en" }),
  getTranslation: (key: string) => key,
}));

import AIMenuOnboarding from "../index";

describe("L3-21 AI onboarding entry", () => {
  it("opens on the PDF tab with no upsell", () => {
    render(
      <AIMenuOnboarding businessId={1} onImportComplete={jest.fn()} />,
    );
    expect(screen.getByTestId("pdf-digitizer")).toBeInTheDocument();
    expect(screen.queryByTestId("ai-upsell-pdf")).not.toBeInTheDocument();
    expect(screen.queryByTestId("ai-upsell-wizard")).not.toBeInTheDocument();
  });
});
