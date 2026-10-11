/** @jest-environment jsdom */
/**
 * #912 — the es-AR register wizard must land on Argentina (so the venue's
 * currency and timezone defaults follow), and the preselect must survive the
 * two ways it used to be lost:
 *   1. SimpleTranslationProvider resolves the locale inside an effect, so the
 *      useState initializer usually runs while the locale is still `en`;
 *   2. the localStorage draft-restore effect spreads a stored (often empty)
 *      country over whatever the initializer picked.
 * A country already in the draft, or one the operator picked themselves, wins.
 */
import React from "react";
import { act, render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { REGISTRATION_DRAFT_STORAGE_KEY } from "../_registrationDraft";

// react-aria's overlay positioning (the NextUI Autocomplete) reaches for these
// two browser APIs; jsdom ships neither. Polyfilled locally rather than in the
// global setup so no other suite's behaviour changes.
if (typeof globalThis.ResizeObserver === "undefined") {
  globalThis.ResizeObserver = class {
    observe() {}
    unobserve() {}
    disconnect() {}
  } as unknown as typeof ResizeObserver;
}
if (!Element.prototype.scrollIntoView) {
  Element.prototype.scrollIntoView = function scrollIntoView() {};
}

const mockReplace = jest.fn();
let mockSearchParamsValue = new URLSearchParams();
let mockLocale = "en";

jest.mock("next/navigation", () => ({
  useRouter: () => ({ replace: mockReplace, back: jest.fn(), push: jest.fn() }),
  useSearchParams: () => mockSearchParamsValue,
}));

jest.mock("wagmi", () => ({
  useAccount: () => ({ address: undefined, isConnected: false }),
}));

jest.mock("@/providers/HybridAuthProvider", () => ({
  useAuth: () => ({
    isOAuthUser: false,
    isWeb3User: false,
    isStaffUser: false,
    oauthData: null,
    isLoading: false,
    isInitialized: true,
  }),
}));

jest.mock("@/store/useUserStore", () => {
  const setUser = jest.fn();
  return {
    useUserStore: Object.assign(() => ({ user: null }), {
      getState: () => ({ setUser }),
    }),
  };
});

jest.mock("@/i18n/SimpleTranslationProvider", () => ({
  useSimpleLocale: () => ({ locale: mockLocale }),
  getTranslation: (key: string) => key,
}));

jest.mock("@/hooks/useAnalytics", () => ({
  usePageTracking: () => undefined,
  useClickTracking: () => jest.fn(),
  useConversionTracking: () => jest.fn(),
}));

jest.mock("@/api/users/profile", () => ({
  getUserProfile: jest.fn().mockResolvedValue(null),
}));

import BusinessRegisterPage from "../page";

/**
 * The wizard persists the whole funnel draft on every formData change, so the
 * stored country is the value the wizard will actually submit — and it is the
 * exact value the country Autocomplete is bound to via `selectedKey`.
 */
function storedCountry(): string | undefined {
  const raw = localStorage.getItem(REGISTRATION_DRAFT_STORAGE_KEY);
  return raw ? JSON.parse(raw).address?.country : undefined;
}

/**
 * The visible control. Its text value is the localized country NAME, so it
 * also proves the operator sees "Argentina" and not a bare "AR" code.
 */
function countryField(): HTMLInputElement {
  return screen.getByLabelText(
    "businessRegister.businessInfo.fields.country.label",
  ) as HTMLInputElement;
}

function seedDraft(address: Record<string, string>) {
  localStorage.setItem(
    REGISTRATION_DRAFT_STORAGE_KEY,
    JSON.stringify({ name: "Bodegón", address }),
  );
}

async function renderWizard() {
  let view!: ReturnType<typeof render>;
  await act(async () => {
    view = render(<BusinessRegisterPage />);
  });
  return view;
}

beforeEach(() => {
  mockReplace.mockClear();
  mockLocale = "en";
  mockSearchParamsValue = new URLSearchParams("step=business");
  localStorage.clear();
});

describe("es-AR register country preselect (#912)", () => {
  it("preselects Argentina when the locale is es-AR from the first paint", async () => {
    mockLocale = "es-AR";
    await renderWizard();

    expect(storedCountry()).toBe("AR");
    expect(countryField().value).toBe("Argentina");
  });

  it("preselects Argentina when the locale only resolves after mount", async () => {
    // The provider settles a tick late; nothing used to re-sync afterwards.
    const view = await renderWizard();
    expect(storedCountry()).toBe("");

    mockLocale = "es-AR";
    await act(async () => {
      view.rerender(<BusinessRegisterPage />);
    });

    expect(storedCountry()).toBe("AR");
    expect(countryField().value).toBe("Argentina");
  });

  it("does not preselect anything on en or generic es", async () => {
    await renderWizard();
    expect(storedCountry()).toBe("");
    expect(countryField().value).toBe("");

    localStorage.clear();
    mockLocale = "es";
    await renderWizard();
    expect(storedCountry()).toBe("");
  });

  it("keeps a restored draft country instead of overwriting it with AR", async () => {
    seedDraft({ street: "Defensa 100", city: "Buenos Aires", country: "UY" });
    mockLocale = "es-AR";
    await renderWizard();

    expect(storedCountry()).toBe("UY");
    expect(countryField().value).toBe("Uruguay");
  });

  it("fills an empty draft country once the locale resolves", async () => {
    // The clobber path: the draft carries an empty country string, which used
    // to land on top of the initializer's "AR" with nothing to restore it.
    seedDraft({ street: "Defensa 100", city: "Buenos Aires", country: "" });
    mockLocale = "es-AR";
    await renderWizard();

    expect(storedCountry()).toBe("AR");
  });

  it("leaves the field empty once the operator clears it themselves", async () => {
    // Clearing marks the field touched, and `countryTouched` is itself an
    // effect dependency — so the preselect effect re-runs right after the
    // operator's edit and must bail out instead of snapping back to Argentina.
    const user = userEvent.setup();
    mockLocale = "es-AR";
    const view = await renderWizard();
    expect(storedCountry()).toBe("AR");

    await act(async () => {
      await user.clear(countryField());
    });
    await waitFor(() => expect(storedCountry()).toBe(""));

    // A later locale re-resolve must not undo the operator's edit either.
    await act(async () => {
      view.rerender(<BusinessRegisterPage />);
    });
    expect(storedCountry()).toBe("");
  });
});
