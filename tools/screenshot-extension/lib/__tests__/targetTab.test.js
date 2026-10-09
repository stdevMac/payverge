import { chooseTargetTab } from "../targetTab.js";

const dash = (over = {}) => ({
  id: 1,
  url: "https://payverge.io/business/9/dashboard?tab=overview",
  active: false,
  ...over,
});

test("prefers the active tab when it is a business dashboard", () => {
  const tabs = [
    dash({ id: 1 }),
    dash({ id: 2, active: true }),
  ];
  expect(chooseTargetTab(tabs).id).toBe(2);
});

test("falls back to any business-dashboard tab when the active tab is unrelated", () => {
  const tabs = [
    { id: 1, url: "chrome-extension://abc/popup.html", active: true },
    dash({ id: 2 }),
  ];
  expect(chooseTargetTab(tabs).id).toBe(2);
});

test("picks the most recently accessed dashboard tab when several exist", () => {
  const tabs = [
    dash({ id: 1, lastAccessed: 100 }),
    dash({ id: 2, lastAccessed: 300 }),
    dash({ id: 3, lastAccessed: 200 }),
  ];
  expect(chooseTargetTab(tabs).id).toBe(2);
});

test("returns null when no business-dashboard tab exists", () => {
  const tabs = [
    { id: 1, url: "https://example.com/", active: true },
    { id: 2, url: "https://payverge.io/", active: false },
  ];
  expect(chooseTargetTab(tabs)).toBeNull();
});

test("tolerates tabs without a url (unloaded/discarded)", () => {
  const tabs = [{ id: 1, active: true }, dash({ id: 2 })];
  expect(chooseTargetTab(tabs).id).toBe(2);
});
