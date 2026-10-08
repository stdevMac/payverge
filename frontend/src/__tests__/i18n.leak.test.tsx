/** @jest-environment jsdom */
/**
 * Task 8.1 — i18n leak safety net for dashboard tabs.
 *
 * The Drivers tab regression (Task 1.3) shipped because `DriversManager` was
 * resolving keys against the wrong namespace, so the unresolved dotted path
 * (e.g. `deliverySettings.drivers.title`) was echoed straight into the DOM.
 * `getTranslation` returns the raw key on miss, which makes those leaks
 * visually obvious in QA but completely invisible to existing render tests
 * that key-echo their own translation mock.
 *
 * This test renders three full-page dashboard subjects with the REAL
 * `SimpleTranslationProvider` (no key-echo stub) and asserts that the
 * rendered text contains no tokens that look like raw i18n keys. Each
 * subject runs in its own `it()` so one leak doesn't mask another.
 *
 * IMPORTANT: if a leak surfaces here, fix the component's translations or
 * the namespace it resolves against — do NOT weaken the regex.
 */
import React from "react";
import { render, waitFor } from "@testing-library/react";
import { I18N_KEY_RE } from "./_i18nUtil";

// --- API mocks ---------------------------------------------------------------
// Keep these realistic enough that each component renders both its loading
// and loaded states. We don't need to cover every method — just the ones the
// happy path calls on mount.

jest.mock("@/api/delivery", () => ({
  VEHICLE_TYPES: ["bicycle", "scooter", "motorcycle", "car", "van"],
  deliveryApi: {
    getBusinessDrivers: jest.fn(() => Promise.resolve([])),
    createDriver: jest.fn(),
    updateDriver: jest.fn(),
    deleteDriver: jest.fn(),
  },
}));

jest.mock("@/api/crm", () => ({
  __esModule: true,
  businessCRMAPI: {
    getCustomers: jest.fn(() =>
      Promise.resolve({ customers: [], total_pages: 1 }),
    ),
    getCustomerDetails: jest.fn(() => Promise.resolve({ customer: null })),
    updateCustomerNotes: jest.fn(() => Promise.resolve({})),
    updateCustomerTags: jest.fn(() => Promise.resolve({})),
    exportCustomers: jest.fn(() => Promise.resolve(new Blob())),
    getCRMStatus: jest.fn(() => Promise.resolve({ enabled: true })),
    setCRMStatus: jest.fn(() => Promise.resolve({ enabled: true })),
  },
  getSegments: jest.fn(() => Promise.resolve({ segments: [] })),
}));

jest.mock("@/api/business", () => ({
  __esModule: true,
  businessApi: {
    getBusinessTables: jest.fn(() => Promise.resolve({ tables: [] })),
    getTablesWithStatus: jest.fn(() => Promise.resolve({ tables: [] })),
    createTableWithQR: jest.fn(),
    updateTableDetails: jest.fn(),
    deleteTable: jest.fn(),
    updateBusinessTable: jest.fn(),
    updateBusiness: jest.fn(),
  },
  getBusiness: jest.fn(() =>
    Promise.resolve({ default_currency: "USD" }),
  ),
}));

// --- Context / hook mocks ---------------------------------------------------

jest.mock("@/contexts/ToastContext", () => ({
  useToast: () => ({ showSuccess: jest.fn(), showError: jest.fn() }),
}));

jest.mock("@/hooks/useBusinessAccess", () => ({
  useBusinessAccess: () => ({
    access: null,
    loading: false,
    error: null,
    hasAccess: true,
    isSuspended: false,
    lockState: "active",
    aiConfigured: false,
    refetch: jest.fn(),
  }),
}));

// We deliberately do NOT mock SimpleTranslationProvider — the whole point of
// this suite is to exercise the real translation layer.

// --- Subjects ---------------------------------------------------------------

import CRMManager from "@/components/business/CRMManager";
import DriversManager from "@/components/business/delivery/DriversManager";
import TableManager from "@/components/business/TableManager";

interface Subject {
  name: string;
  mount: () => ReturnType<typeof render>;
}

const SUBJECTS: Subject[] = [
  {
    name: "CRMManager",
    mount: () => render(<CRMManager businessId={1} />),
  },
  {
    name: "DriversManager",
    mount: () => render(<DriversManager businessId={1} />),
  },
  {
    name: "TableManager",
    mount: () => render(<TableManager businessId={1} />),
  },
];

function findLeaks(text: string): string[] {
  // Split on whitespace AND a small set of punctuation that commonly wraps
  // visible labels (parentheses, brackets, slashes). This catches keys that
  // got rendered inside e.g. "(deliverySettings.drivers.title)" without
  // letting punctuation false-positive the regex match.
  return Array.from(
    new Set(
      text
        .split(/[\s()[\]{}<>"'`,;]+/)
        .map((s) => s.trim())
        .filter((s) => s.length > 0)
        .filter((s) => I18N_KEY_RE.test(s)),
    ),
  );
}

describe("i18n leak safety net", () => {
  SUBJECTS.forEach((subject) => {
    it(`${subject.name} renders no raw i18n keys`, async () => {
      const { container } = subject.mount();
      // Let async effects settle (data loads, locale sync, etc.) before
      // sampling the DOM. We poll the container text — if a leak appears
      // and stays, waitFor will eventually report it; if a transient
      // unresolved key flashes and then resolves, this still catches the
      // stable end state.
      await waitFor(() => {
        const text = container.textContent || "";
        const leaks = findLeaks(text);
        if (leaks.length > 0) {
          throw new Error(
            `${subject.name} leaked raw i18n keys: ${leaks.join(", ")}`,
          );
        }
      });
    });
  });
});
