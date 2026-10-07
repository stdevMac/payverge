import { landedOnRequestedPage } from "../navigation.js";

test("same path and same tab param is a match", () => {
  expect(
    landedOnRequestedPage(
      "/business/9/dashboard?tab=crm",
      "https://payverge.io/business/9/dashboard?tab=crm",
    ),
  ).toBe(true);
});

test("auth bounce to /dashboard is NOT a match", () => {
  expect(
    landedOnRequestedPage(
      "/business/9/dashboard?tab=crm",
      "https://payverge.io/dashboard?redirect=%2Fbusiness%2F9%2Fdashboard",
    ),
  ).toBe(false);
});

test("extra query params added by the app are tolerated", () => {
  expect(
    landedOnRequestedPage("/t/ABC123/menu", "https://payverge.io/t/ABC123/menu?lang=es"),
  ).toBe(true);
});

test("app dropping the tab param is tolerated", () => {
  expect(
    landedOnRequestedPage(
      "/business/9/dashboard?tab=overview",
      "https://payverge.io/business/9/dashboard",
    ),
  ).toBe(true);
});

test("bounce to a DIFFERENT tab is NOT a match", () => {
  expect(
    landedOnRequestedPage(
      "/business/9/dashboard?tab=crm",
      "https://payverge.io/business/9/dashboard?tab=overview",
    ),
  ).toBe(false);
});

test("trailing slash differences are normalized", () => {
  expect(landedOnRequestedPage("/b/lounge", "https://payverge.io/b/lounge/")).toBe(true);
});

test("malformed actual URL is NOT a match", () => {
  expect(landedOnRequestedPage("/b/lounge", "not a url")).toBe(false);
});
