/** @jest-environment jsdom */
import { render, screen, waitFor } from "@testing-library/react";
import BusinessPageClient from "./BusinessPageClient";
import { GuestTranslationProvider } from "@/i18n/GuestTranslationProvider";

jest.mock("@/api/publicBusiness", () => ({
  ...jest.requireActual("@/api/publicBusiness"),
  getBusinessByCustomUrl: jest.fn(),
}));
jest.mock("next/navigation", () => ({ useRouter: () => ({ push: jest.fn() }) }));
jest.mock(
  "@/components/business-page/ConvertingBusinessLandingPage",
  () => () => null,
);

describe("BusinessPageClient — localized states (I18N-1)", () => {
  it("renders the not-found copy from the GUEST tier for a guest-only locale (de)", async () => {
    // Seed a known not-found reason so no fetch is needed, and mount inside a
    // German guest provider so the copy must come from the guest bundle.
    render(
      <GuestTranslationProvider initialLanguage="de">
        <BusinessPageClient
          customUrl="missing"
          initialBusiness={null}
          initialReason="not_found"
        />
      </GuestTranslationProvider>,
    );
    // The guest bundle loads asynchronously (useEffect), so wait for the German
    // not-found title (businessPage.error.notFoundTitle in de.json) to resolve.
    // Asserting the LITERAL German string proves guest-tier resolution and would
    // catch a regression where the non-en error keys were missing/wrong (a
    // sentenceCaseLeaf fallback would render different text).
    await waitFor(() =>
      expect(
        screen.getByText("Wir konnten diese Seite nicht finden"),
      ).toBeInTheDocument(),
    );
    // And it is NOT the English string.
    expect(
      screen.queryByText("We couldn't find that page"),
    ).not.toBeInTheDocument();
    // The localized "Go to homepage" CTA is present (German: "Zur Startseite").
    expect(screen.getByText("Zur Startseite")).toBeInTheDocument();
  });
});
