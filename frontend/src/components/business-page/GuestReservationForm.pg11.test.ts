import fs from "fs";
import path from "path";

const SRC = path.resolve(__dirname, "GuestReservationForm.tsx");

describe("PG-11 reservation form residuals", () => {
  const src = fs.readFileSync(SRC, "utf8");

  it("quick-date presets expose aria-pressed and group labelling", () => {
    expect(src).toMatch(/role=["']group["']/);
    expect(src).toMatch(/aria-labelledby=["']reservation-quick-dates-label["']/);
    expect(src).toMatch(/aria-pressed=\{pressed\}/);
  });

  it("unavailable slots stay keyboard-reachable (aria-disabled, not disabled)", () => {
    expect(src).toMatch(/aria-disabled=\{isDisabled\}/);
    // The slot buttons must not use the native disabled attribute for unavailability.
    const slotBlock = src.slice(
      src.indexOf("aria-pressed={isSelected}"),
      src.indexOf("aria-pressed={isSelected}") + 400,
    );
    expect(slotBlock).not.toMatch(/(?<!aria-)disabled=\{isDisabled\}/);
  });

  it("disabled submit expose a title reason", () => {
    expect(src).toMatch(/title=\{[\s\S]*fillAllFields/);
  });

  it("special requests label references optional without a naive double suffix", () => {
    expect(src).toMatch(/appendOptionalSuffix\(/);
    expect(src).toMatch(/reservationT\(["']optional["']\)/);
    expect(src).not.toMatch(
      /label=\{`\$\{reservationT\(["']specialRequests["']\)\} \(\$\{reservationT\(["']optional["']\)\}\)`\}/,
    );
    expect(src).toMatch(/specialRequests:\s*["']Special Requests["']/);
  });
});
