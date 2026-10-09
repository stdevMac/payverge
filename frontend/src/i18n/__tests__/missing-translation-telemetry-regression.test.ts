import fs from "node:fs";
import path from "node:path";

import { messages } from "../getTranslation";
import { storefrontLocales, type Locale } from "../localeRegistry";

function readPath(root: unknown, key: string): unknown {
  return key.split(".").reduce<unknown>((value, part) => {
    if (!value || typeof value !== "object") return undefined;
    return (value as Record<string, unknown>)[part];
  }, root);
}

function expectStringPath(root: unknown, key: string) {
  const value = readPath(root, key);
  expect(typeof value).toBe("string");
  expect(value).not.toBe("");
}

const operatorLocales: Locale[] = ["en", "es", "es-AR"];

// These paths are copied from the production missing-translation telemetry
// surface. Keeping them here makes the next namespace drift fail locally
// instead of silently filling the admin table with leaf fallbacks again.
export const operatorTelemetryKeys = [
  "businessDashboard.dashboard.liveBills.loading",
  "businessDashboard.dashboard.paymentHistory.loading",
  "businessDashboard.dateRange.startDate",
  "businessDashboard.dateRange.endDate",
  "businessDashboard.overview.serviceRollCall.menu",
  "businessDashboard.overview.serviceRollCall.bills",
  "businessDashboard.overview.serviceRollCall.tables",
  "businessDashboard.accountingDashboard.categories.private_room",
  "businessDashboard.accountingDashboard.categories.supplier",
  "businessDashboard.accountingDashboard.categories.staff_meal",
  "billCreator.inventoryWarnings.outBadge",
  "billCreator.inventoryWarnings.outDescription",
  "billManager.filters.fromDate",
  "billManager.filters.toDate",
  "billManager.preparedUnits",
  "businessSettings.notifications.operationalTypes.table_stale_occupied",
  "directorConsole.empty.title",
  "directorConsole.empty.subtitle",
  "fiscal.labels.US",
  "fiscal.labels.demo",
  "fiscal.labels.receipt",
  "fiscal.credentials.status.validated",
  "resetPassword.request.showPassword",
  "resetPassword.request.hidePassword",
  "businessDashboard.dashboard.tableManager.aging.openCheckWarning",
  "businessDashboard.dashboard.tableManager.aging.seatedUnknown",
];

export const guestTelemetryKeys = [
  "businessPage.galleryBadge",
  "businessPage.galleryTitle",
  "businessPage.glimpseRestaurant",
  "businessPage.featuresBadge",
  "businessPage.experienceBest",
  "businessPage.whyChooseUs",
  "businessPage.ourStory",
  "businessPage.contact",
  "businessPage.tabsAria",
  "businessPage.menuTab",
  "businessPage.about",
  "businessPage.hero.callCta",
  "businessPage.hero.viewMenuCta",
  "businessPage.info.reservationForm.confirmationCode",
  "businessPage.info.reservationForm.viewReservation",
  "businessPage.poweringHospitality",
  "businessPage.openNow",
  "businessPage.closed",
  "businessPage.skipToMainContent",
  "languageSelector.dropdownAria",
  "languageSelector.default",
  "languageSelector.changeLanguage",
  "menu.viewMode.label",
  "menu.viewMode.categoryTabs",
  "menu.viewMode.compact",
  "menu.viewMode.grid",
  "menu.viewMode.detailed",
];

function loadGuestMessages(locale: string): unknown {
  const file = path.resolve(
    __dirname,
    "..",
    "guest-messages",
    `${locale}.json`,
  );
  return JSON.parse(fs.readFileSync(file, "utf8"));
}

describe("missing translation telemetry regression keys", () => {
  it.each(operatorLocales)(
    "resolves production operator telemetry keys in %s",
    (locale) => {
      for (const key of operatorTelemetryKeys) {
        expectStringPath(messages[locale], key);
      }
    },
  );

  it.each(storefrontLocales)(
    "resolves production guest telemetry keys in %s",
    (locale) => {
      const bundle = loadGuestMessages(locale);
      for (const key of guestTelemetryKeys) {
        expectStringPath(bundle, key);
      }
    },
  );
});
