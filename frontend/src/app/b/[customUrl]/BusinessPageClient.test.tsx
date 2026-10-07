/** @jest-environment jsdom */
import { render, screen, waitFor } from "@testing-library/react";
import BusinessPageClient from "./BusinessPageClient";
import { getBusinessByCustomUrl } from "@/api/publicBusiness";
import { GuestTranslationProvider } from "@/i18n/GuestTranslationProvider";

jest.mock("@/api/publicBusiness", () => ({
  getBusinessByCustomUrl: jest.fn(),
}));

jest.mock("next/navigation", () => ({
  useRouter: () => ({ push: jest.fn() }),
}));

// Avoid pulling the full landing page into this test.
jest.mock(
  "@/components/business-page/ConvertingBusinessLandingPage",
  () => () => null,
);

// I18N-1: the loading/error copy now resolves from the GUEST tier (21 locales),
// not the operator tier. Mounting inside a Spanish guest provider proves the
// 404-vs-5xx branch picks the right guest-tier es string.
function renderEs(customUrl: string) {
  return render(
    <GuestTranslationProvider initialLanguage="es">
      <BusinessPageClient customUrl={customUrl} />
    </GuestTranslationProvider>,
  );
}

// L-1: the shared axiosInstance interceptor rejects with a SANITIZED plain
// Error (not an AxiosError) that carries .status / .response.status — see
// src/api/tools/instance.ts toSanitizedError. So the component must read the
// status off that shape; axios.isAxiosError() is always false here. This
// helper mirrors what callers actually receive (the previous test force-mocked
// axios.isAxiosError, masking the production shape).
function sanitizedError(status: number): Error {
  const err = new Error(
    `Request failed with status code ${status}`,
  ) as Error & { status?: number; response?: { status?: number } };
  err.status = status;
  err.response = { status };
  return err;
}

describe("BusinessPageClient error handling", () => {
  beforeEach(() => {
    jest.clearAllMocks();
  });

  it("shows the not-found (Compass) variant on a real 404, even for a non-English locale", async () => {
    (getBusinessByCustomUrl as jest.Mock).mockRejectedValue(sanitizedError(404));

    renderEs("missing");

    // Guest-tier es translation of businessPage.error.notFoundTitle.
    await waitFor(() => {
      expect(
        screen.getByText("No pudimos encontrar esa página"),
      ).toBeInTheDocument();
    });
    // Must NOT show the retryable "unavailable" outage title for a permanent 404.
    expect(
      screen.queryByText("El negocio no está disponible temporalmente"),
    ).not.toBeInTheDocument();
  });

  it("shows the retryable 'unavailable' variant on a 5xx/transient error", async () => {
    (getBusinessByCustomUrl as jest.Mock).mockRejectedValue(sanitizedError(500));

    renderEs("boom");

    // Guest-tier es translation of businessPage.error.unavailableTitle.
    await waitFor(() => {
      expect(
        screen.getByText("El negocio no está disponible temporalmente"),
      ).toBeInTheDocument();
    });
    expect(
      screen.queryByText("No pudimos encontrar esa página"),
    ).not.toBeInTheDocument();
  });
});
