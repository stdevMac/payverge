/** @jest-environment jsdom */
import { renderHook, act } from "@testing-library/react";
import fs from "fs";
import path from "path";
import { useDirtyForm } from "@/hooks/useDirtyForm";
import type { CreateReservationRequest } from "@/api/reservations";

/**
 * L1-15: refresh mid-form must not discard, but the guard must NOT fire on a
 * pristine open. Create seeds `reservation_time` via firstBookableSlot(); edit
 * always has customer_name/time. Dirty must be baseline-diff, not "any field
 * non-empty".
 */
describe("ReservationManager L1-15 dirty baseline", () => {
  const managerSrc = fs.readFileSync(
    path.join(__dirname, "..", "ReservationManager.tsx"),
    "utf8",
  );

  // Seeded create form — mirrors resetForm() after firstBookableSlot().
  const pristineCreate: CreateReservationRequest = {
    table_id: undefined,
    customer_name: "",
    customer_phone: "",
    customer_email: "",
    party_size: 2,
    reservation_time: "2026-08-06T19:00",
    duration: 120,
    special_requests: "",
    notes: "",
  };

  // Seeded edit form — always has name + time from the reservation row.
  const pristineEdit: CreateReservationRequest = {
    table_id: 3,
    customer_name: "Ada",
    customer_phone: "+15551212",
    customer_email: "ada@example.com",
    party_size: 4,
    reservation_time: "2026-08-07T20:00",
    duration: 90,
    special_requests: "",
    notes: "window",
  };

  it("ships useDirtyForm(formData) + markClean on open (not always-nonempty)", () => {
    // Regression: the always-nonempty Boolean(...) form treated every open as dirty.
    expect(managerSrc).not.toMatch(
      /Boolean\(\s*\n?\s*formData\.customer_name/,
    );
    expect(managerSrc).toMatch(/useDirtyForm\(\s*formData\s*\)/);
    expect(managerSrc).toMatch(/markReservationFormClean/);
    expect(managerSrc).toMatch(/useUnsavedChangesGuard/);
    expect(managerSrc).toMatch(/reservation-manager-form/);
  });

  it("pristine create open with seeded reservation_time is not dirty", () => {
    const { result, rerender } = renderHook(
      ({ value }) => useDirtyForm(value),
      { initialProps: { value: pristineCreate } },
    );
    // Component markCleans the snapshot immediately after resetForm/open.
    act(() => {
      result.current.markClean(pristineCreate);
    });
    const formOpen = true;
    expect(formOpen && result.current.dirty).toBe(false);

    // User edit → dirty.
    rerender({
      value: { ...pristineCreate, customer_name: "Grace" },
    });
    expect(formOpen && result.current.dirty).toBe(true);
  });

  it("pristine edit open with existing name/time is not dirty until edited", () => {
    const { result, rerender } = renderHook(
      ({ value }) => useDirtyForm(value),
      { initialProps: { value: pristineEdit } },
    );
    act(() => {
      result.current.markClean(pristineEdit);
    });
    expect(result.current.dirty).toBe(false);

    rerender({
      value: { ...pristineEdit, party_size: 5 },
    });
    expect(result.current.dirty).toBe(true);
  });
});
