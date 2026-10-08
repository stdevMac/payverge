/** @jest-environment jsdom */
/**
 * Round-3 handoff residual — a user-facing failure must not discard its cause.
 *
 * `loadDeliveryData` caught with a bare `catch {` and showed
 * "deliverySettings.focused.toasts.loadError". That is correct for a 500 or an offline client.
 * It is actively misleading for a programmer error: if `ensureZoneClientKeys`
 * throws a TypeError on a malformed zone, the operator is told the load failed
 * (implying "retry / check your connection") and nobody — not the console, not
 * Sentry — ever learns a bug ran. The toast is indistinguishable in both cases,
 * which is what made L3-38 hard to see in the first place.
 *
 * The toast stays: an operator should not be shown a stack trace. What changes
 * is that the cause is reported through `captureClientError`, which already
 * runs `sanitizeObject` over any additional context.
 *
 * This is the template for the other 60 sites in this class (catch that raises
 * a user-facing message AND discards the error). They are deliberately NOT
 * swept here — a blind sweep would regress the sites that avoid logging on
 * purpose, e.g. BillDetailsModal.tsx:436 keeps a document number out of error
 * output. Each remaining site needs that one judgement made explicitly.
 */

import React from "react";
import { render, waitFor } from "@testing-library/react";

const mockCaptureClientError = jest.fn();
const mockShowError = jest.fn();
const mockGetDeliverySettings = jest.fn();

// Every hook mock must return STABLE identities. Returning a fresh `jest.fn()`
// per render changes `loadDeliveryData`'s useCallback deps, which refires the
// load effect, which setStates, which re-renders — an infinite loop that is an
// artefact of the harness, not of the component.
const mockToast = {
  showError: mockShowError,
  showSuccess: jest.fn(),
  showInfo: jest.fn(),
  showWarning: jest.fn(),
};
const mockTier = {
  hasAccess: true,
  loading: false,
  isSuspended: false,
  access: null,
  error: null,
  refetch: jest.fn(),
};
const mockDirtyForm = {
  isDirty: false,
  markClean: jest.fn(),
  markDirty: jest.fn(),
  reset: jest.fn(),
};
const mockDeliveryBusiness = { data: null, isLoading: false };

jest.mock("@/lib/sentry/reporting", () => ({
  captureClientError: (...args: unknown[]) => mockCaptureClientError(...args),
}));

jest.mock("@/contexts/ToastContext", () => ({
  useToast: () => mockToast,
}));

jest.mock("@/i18n/SimpleTranslationProvider", () => ({
  useSimpleLocale: () => ({ locale: "en", setLocale: jest.fn() }),
  getTranslation: (key: string) => key,
}));

jest.mock("@/hooks/useBusinessAccess", () => ({
  useBusinessAccess: () => mockTier,
}));

jest.mock("@/hooks/useDirtyForm", () => ({
  useDirtyForm: () => mockDirtyForm,
}));

jest.mock("@/hooks/useUnsavedChangesGuard", () => ({
  useUnsavedChangesGuard: () => {},
}));

jest.mock("@/api/delivery", () => ({
  deliveryApi: {
    getDeliverySettings: (...args: unknown[]) => mockGetDeliverySettings(...args),
    updateDeliverySettings: jest.fn(),
    getDeliveryOrders: jest.fn().mockResolvedValue([]),
    getBusinessDeliveries: jest.fn().mockResolvedValue({ deliveries: [] }),
  },
}));

jest.mock("../delivery/useDeliveryQueries", () => ({
  useDeliveryBusiness: () => mockDeliveryBusiness,
}));

// The section components are irrelevant to the load-failure path and pull in
// heavy NextUI trees; their own suites cover them.
jest.mock("../delivery/ConfigurationSection", () => ({
  ConfigurationSection: () => null,
}));
jest.mock("../delivery/ZonesSection", () => ({
  ZonesSection: () => null,
  ensureZoneClientKeys: (zones: unknown[]) => zones,
  reconcileZoneClientKeys: (zones: unknown[]) => zones,
}));
jest.mock("../delivery/HoursSection", () => ({ HoursSection: () => null }));
jest.mock("../delivery/PartnersSection", () => ({
  PartnersSection: () => null,
}));
jest.mock("../delivery/CustomerNoteSection", () => ({
  CustomerNoteSection: () => null,
}));

import DeliverySettings from "../DeliverySettings";

describe("DeliverySettings load failure reports its cause", () => {
  beforeEach(() => {
    jest.clearAllMocks();
  });

  it("still shows the operator an honest toast", async () => {
    mockGetDeliverySettings.mockRejectedValue(new TypeError("zones is not iterable"));

    render(<DeliverySettings businessId={1} />);

    await waitFor(() =>
      expect(mockShowError).toHaveBeenCalledWith("deliverySettings.focused.toasts.loadError"),
    );
  });

  it("reports the underlying error instead of discarding it", async () => {
    const boom = new TypeError("zones is not iterable");
    mockGetDeliverySettings.mockRejectedValue(boom);

    render(<DeliverySettings businessId={1} />);

    await waitFor(() => expect(mockCaptureClientError).toHaveBeenCalled());

    const arg = mockCaptureClientError.mock.calls[0][0];
    expect(arg.error).toBe(boom);
    expect(arg.component).toBe("DeliverySettings");
    expect(arg.functionName).toBe("loadDeliveryData");
  });

  it("reports a non-Error throw too — the cause must survive whatever was thrown", async () => {
    mockGetDeliverySettings.mockRejectedValue("string rejection");

    render(<DeliverySettings businessId={1} />);

    await waitFor(() => expect(mockCaptureClientError).toHaveBeenCalled());
    expect(mockCaptureClientError.mock.calls[0][0].error).toBe("string rejection");
  });
});
