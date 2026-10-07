/** @jest-environment node */
import { localDateKey } from "@/lib/localDate";
import {
  isClientOnlyReservationFiltersActive,
  isDerivedClientQuickFilter,
  resolveServerReservationFilters,
  shouldLoadCompleteReservations,
  shouldLoadPagedReservations,
} from "./reservationFilterMode";

describe("reservationFilterMode — server pagination default path", () => {
  const noon = new Date(2026, 5, 15, 12, 0, 0); // Jun 15 2026 local

  describe("isClientOnlyReservationFiltersActive", () => {
    it("is FALSE when attention is none (default)", () => {
      expect(isClientOnlyReservationFiltersActive("", "none")).toBe(false);
    });

    it("is TRUE for derived attention filters", () => {
      expect(isClientOnlyReservationFiltersActive("", "next2hours")).toBe(true);
      expect(isClientOnlyReservationFiltersActive("", "no_show_risk")).toBe(
        true,
      );
      expect(isClientOnlyReservationFiltersActive("", "needs_table")).toBe(
        true,
      );
    });

    it("is TRUE for non-empty search until search is server-side", () => {
      expect(isClientOnlyReservationFiltersActive("jane", "none")).toBe(true);
      expect(
        isClientOnlyReservationFiltersActive("jane", "none", {
          searchIsServerSide: true,
        }),
      ).toBe(false);
    });
  });

  describe("isDerivedClientQuickFilter", () => {
    it("classifies only time/null-column derived filters", () => {
      expect(isDerivedClientQuickFilter("next2hours")).toBe(true);
      expect(isDerivedClientQuickFilter("no_show_risk")).toBe(true);
      expect(isDerivedClientQuickFilter("needs_table")).toBe(true);
      expect(isDerivedClientQuickFilter("none")).toBe(false);
      expect(isDerivedClientQuickFilter("today")).toBe(false);
      expect(isDerivedClientQuickFilter("waitlist")).toBe(false);
    });
  });

  describe("resolveServerReservationFilters", () => {
    it("honors dateFilter today as the sole time window", () => {
      const resolved = resolveServerReservationFilters(
        "none",
        "today",
        "all",
        noon,
        30,
      );
      const key = localDateKey(noon);
      expect(resolved.startDate).toBe(key);
      expect(resolved.endDate).toBe(key);
      expect(resolved.status).toBe("all");
    });

    it("honors statusFilter waitlist without attention rewriting it", () => {
      const resolved = resolveServerReservationFilters(
        "none",
        "upcoming",
        "waitlist",
        noon,
        30,
      );
      expect(resolved.status).toBe("waitlist");
      expect(resolved.startDate).toBe(localDateKey(noon));
    });

    it("honors dateFilter + statusFilter independently of attention", () => {
      const resolved = resolveServerReservationFilters(
        "next2hours",
        "past",
        "confirmed",
        noon,
        30,
      );
      expect(resolved.status).toBe("confirmed");
      expect(resolved.endDate).toBe(localDateKey(noon));
      const minus30 = new Date(noon);
      minus30.setDate(minus30.getDate() - 30);
      expect(resolved.startDate).toBe(localDateKey(minus30));
    });
  });

  describe("loader path selection", () => {
    it("default list + no attention uses paged loader, not complete hydrate", () => {
      const clientOnly = isClientOnlyReservationFiltersActive("", "none");
      expect(shouldLoadPagedReservations(clientOnly, "list")).toBe(true);
      expect(shouldLoadCompleteReservations(clientOnly, "list")).toBe(false);
    });

    it("operations board still loads complete data", () => {
      const clientOnly = isClientOnlyReservationFiltersActive("", "none");
      expect(shouldLoadCompleteReservations(clientOnly, "operations")).toBe(
        true,
      );
      expect(shouldLoadPagedReservations(clientOnly, "operations")).toBe(false);
    });

    it("derived filters force complete hydrate on list", () => {
      const clientOnly = isClientOnlyReservationFiltersActive(
        "",
        "next2hours",
      );
      expect(shouldLoadCompleteReservations(clientOnly, "list")).toBe(true);
      expect(shouldLoadPagedReservations(clientOnly, "list")).toBe(false);
    });
  });
});
