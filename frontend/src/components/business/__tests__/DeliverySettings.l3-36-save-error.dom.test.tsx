/**
 * D1 / L3-36: delivery settings save 400 must toast the structured backend
 * message (catalog / domain copy), not silently revert or a generic only path.
 *
 * Pure deliverySaveErrorMessage unit tests pass if handleSave never calls them.
 * This mounts DeliverySettings, triggers save with a rejected update, and
 * asserts showError received the coded Spanish catalog string.
 *
 * Revert-proof: showError(tString("focused.toasts.saveError")) only → toast is
 * the generic key, not delivery_zone_priority_duplicate.
 *
 * @jest-environment jsdom
 */
import React from "react";
import { fireEvent, render, screen, waitFor } from "@testing-library/react";
import { asDollars } from "@/types/money";
import esApiErrors from "@/i18n/locales/es/apiErrors.json";

const mockShowError = jest.fn();
const mockShowSuccess = jest.fn();
const mockGetDeliverySettings = jest.fn();
const mockUpdateDeliverySettings = jest.fn();

const mockToast = {
  showError: mockShowError,
  showSuccess: mockShowSuccess,
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
  dirty: true,
  markClean: jest.fn(),
  markDirty: jest.fn(),
  reset: jest.fn(),
};
const mockDeliveryBusiness = { data: null, isLoading: false };

jest.mock("@/lib/sentry/reporting", () => ({
  captureClientError: jest.fn(),
}));

jest.mock("@/contexts/ToastContext", () => ({
  useToast: () => mockToast,
}));

jest.mock("@/i18n/SimpleTranslationProvider", () => ({
  useSimpleLocale: () => ({ locale: "es", setLocale: jest.fn() }),
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
    getDeliverySettings: (...args: unknown[]) =>
      mockGetDeliverySettings(...args),
    updateDeliverySettings: (...args: unknown[]) =>
      mockUpdateDeliverySettings(...args),
    getDeliveryOrders: jest.fn().mockResolvedValue([]),
    getBusinessDeliveries: jest.fn().mockResolvedValue({ deliveries: [] }),
  },
}));

jest.mock("../delivery/useDeliveryQueries", () => ({
  useDeliveryBusiness: () => mockDeliveryBusiness,
}));

jest.mock("../delivery/ConfigurationSection", () => ({
  ConfigurationSection: () => null,
}));
jest.mock("../delivery/ZonesSection", () => ({
  ZonesSection: () => null,
  ensureZoneClientKeys: (zones: unknown[]) => zones ?? [],
  reconcileZoneClientKeys: (_prev: unknown, next: unknown[]) => next ?? [],
}));
jest.mock("../delivery/HoursSection", () => ({ HoursSection: () => null }));
jest.mock("../delivery/PartnersSection", () => ({
  PartnersSection: () => null,
}));
jest.mock("../delivery/CustomerNoteSection", () => ({
  CustomerNoteSection: () => null,
}));

import DeliverySettings from "../DeliverySettings";

const settings = {
  business_id: 1,
  delivery_enabled: true,
  in_house_delivery_enabled: true,
  third_party_enabled: false,
  payment_mode: "cash_on_delivery" as const,
  online_payment_available: false,
  flat_delivery_fee: asDollars(3.99),
  free_delivery_minimum: asDollars(25),
  minimum_order_amount: asDollars(10),
  estimated_prep_time: 15,
  max_concurrent_deliveries: 5,
  delivery_hours_same_as_business: true,
  delivery_instructions: "",
  external_partner_links: [],
  zones: [
    {
      id: 1,
      name: "Zone A",
      delivery_fee: asDollars(4),
      minimum_order_amount: asDollars(15),
      estimated_time: 30,
      priority: 1,
      cutoff_buffer_minutes: 0,
      is_active: true,
      boundaries: {},
    },
  ],
  partner_fallback_available: false,
  estimated_delivery_minutes: 0,
};

beforeEach(() => {
  jest.clearAllMocks();
  mockGetDeliverySettings.mockResolvedValue(settings);
  mockUpdateDeliverySettings.mockRejectedValue({
    response: {
      status: 400,
      data: {
        code: "delivery_zone_priority_duplicate",
        error: "duplicate active zone priority: 1",
      },
    },
    status: 400,
  });
});

describe("DeliverySettings L3-36 save error surface (DOM)", () => {
  it("toasts coded zone priority error, not only the generic saveError key", async () => {
    render(<DeliverySettings businessId={1} />);

    // Wait for load, then find and press Save (SaveBar dirty path).
    await waitFor(() => {
      expect(mockGetDeliverySettings).toHaveBeenCalled();
    });

    // Save control — various SaveBar copy keys; find by role + name regex.
    const saveBtn = await screen.findByRole("button", {
      name: /save|guardar|focused\.toasts\.saved|saveBar|Save/i,
    });
    fireEvent.click(saveBtn);

    await waitFor(() => {
      expect(mockUpdateDeliverySettings).toHaveBeenCalled();
    });

    await waitFor(() => {
      expect(mockShowError).toHaveBeenCalled();
    });

    const msg = String(mockShowError.mock.calls[0][0]);
    expect(msg).toBe(esApiErrors.delivery_zone_priority_duplicate);
    expect(msg).not.toBe("deliverySettings.focused.toasts.saveError");
    expect(msg.toLowerCase()).not.toMatch(/failed to save delivery settings/);
  });
});
