/** @jest-environment jsdom */
import React from "react";
import { render, screen } from "@testing-library/react";
import { AiProviderNotice } from "../AiProviderNotice";

const mockTier = jest.fn();
jest.mock("@/hooks/useBusinessAccess", () => ({
  useBusinessAccess: () => mockTier(),
}));
const mockLocale = { locale: "en" };
jest.mock("@/i18n/SimpleTranslationProvider", () => {
  const actual = jest.requireActual("@/i18n/getTranslation");
  return {
    useSimpleLocale: () => mockLocale,
    getTranslation: actual.getTranslation,
  };
});

describe("AiProviderNotice", () => {
  beforeEach(() => {
    jest.clearAllMocks();
    mockLocale.locale = "en";
  });

  it("stays hidden while the tier probe loads or when a provider is wired", () => {
    mockTier.mockReturnValue({ aiConfigured: false, loading: true });
    const { rerender } = render(<AiProviderNotice businessId={1} />);
    expect(screen.queryByTestId("ai-provider-notice")).not.toBeInTheDocument();

    mockTier.mockReturnValue({ aiConfigured: true, loading: false });
    rerender(<AiProviderNotice businessId={1} />);
    expect(screen.queryByTestId("ai-provider-notice")).not.toBeInTheDocument();
  });

  it("stays hidden when an older backend omits ai_configured", () => {
    // useBusinessAccess defaults the missing field to true; a mock that drops it
    // entirely must not surface a false alarm either.
    mockTier.mockReturnValue({ loading: false });
    render(<AiProviderNotice businessId={1} />);
    expect(screen.queryByTestId("ai-provider-notice")).not.toBeInTheDocument();
  });

  it("names the LLM env vars when the server has no provider", () => {
    mockTier.mockReturnValue({ aiConfigured: false, loading: false });
    render(<AiProviderNotice businessId={1} />);
    const notice = screen.getByTestId("ai-provider-notice");
    expect(notice).toHaveTextContent(
      "AI text generation isn't set up on this server",
    );
    expect(notice).toHaveTextContent("LLM_API_KEY");
    expect(notice).toHaveTextContent("LLM_BASE_URL");
    expect(notice.textContent).not.toMatch(/aiProviderNotice/);
  });

  it("renders the Spanish copy for es operators", () => {
    mockLocale.locale = "es";
    mockTier.mockReturnValue({ aiConfigured: false, loading: false });
    render(<AiProviderNotice businessId={1} />);
    expect(screen.getByTestId("ai-provider-notice")).toHaveTextContent(
      "La generación de texto con IA no está configurada en este servidor",
    );
  });
});
