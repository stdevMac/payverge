import { reservationPhoneGate } from "./reservationPhoneValidation";

describe("reservationPhoneGate (L1-16 create+edit)", () => {
  it("flags missing / whitespace", () => {
    expect(reservationPhoneGate("")).toBe("missing");
    expect(reservationPhoneGate("   ")).toBe("missing");
    expect(reservationPhoneGate(undefined)).toBe("missing");
    expect(reservationPhoneGate(null)).toBe("missing");
  });

  it('rejects garbage phones like "abc"', () => {
    expect(reservationPhoneGate("abc")).toBe("invalid");
    expect(reservationPhoneGate("letters")).toBe("invalid");
    expect(reservationPhoneGate("123")).toBe("invalid");
  });

  it("accepts real phone shapes", () => {
    expect(reservationPhoneGate("5551112222")).toBe("ok");
    expect(reservationPhoneGate("+54 11 5555-1234")).toBe("ok");
    expect(reservationPhoneGate("(11) 5555-1234")).toBe("ok");
  });
});

describe("ReservationManager wires gate on create and update (L1-16)", () => {
  it("both handlers call reservationPhoneGate", async () => {
    const fs = await import("fs");
    const path = await import("path");
    const src = fs.readFileSync(
      path.join(__dirname, "ReservationManager.tsx"),
      "utf8",
    );
    // Gate imported and used — must appear at least twice (create + update).
    expect(src).toMatch(/import \{ reservationPhoneGate \}/);
    const uses = src.match(/reservationPhoneGate\(/g) || [];
    expect(uses.length).toBeGreaterThanOrEqual(2);
  });
});
