import { isGuestPath } from "./guestPath";

describe("isGuestPath", () => {
  it.each([
    "/",
    "/es",
    "/es-ar/",
    "/b/acme-bistro",
    "/es/b/acme-bistro",
    "/t/FV214XU12D",
    "/scan",
    "/scan/abc",
    "/es-ar/scan",
    "/delivery/12/track",
    "/es/delivery/12/track",
    "/reservations/ABC123",
    "/reservations/ABC123/",
  ])("treats %s as a guest path", (path) => {
    expect(isGuestPath(path)).toBe(true);
  });

  it.each([
    "/dashboard",
    "/es/dashboard",
    "/business/acme/settings",
    "/pricing",
    "/staff/login",
    "/delivery",
    "/reservations",
    "/bistro",
    "/scanner",
    "/tables/1",
  ])("treats %s as an operator or marketing path", (path) => {
    expect(isGuestPath(path)).toBe(false);
  });
});
