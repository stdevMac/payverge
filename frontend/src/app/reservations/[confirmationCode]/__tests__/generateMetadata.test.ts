/** @jest-environment node */
/**
 * PG-10: /reservations/[code] must not inherit the marketing root <title>.
 * Honest per-page metadata, no confirmation-code / guest PII in the title.
 */
import { generateMetadata } from "../layout";

describe("generateMetadata for /reservations/[confirmationCode] (PG-10)", () => {
  it("returns an honest reservation title (not the marketing homepage title)", async () => {
    const meta = await generateMetadata({
      params: Promise.resolve({ confirmationCode: "ABC123XYZ" }),
    });
    expect(meta.title).toBe("Reservation | Payverge");
    expect(String(meta.title)).not.toMatch(/AI-Powered Restaurant Management/i);
  });

  it("does not embed the confirmation code (no PII / secret leakage in <title>)", async () => {
    const code = "SECRET-CODE-99";
    const meta = await generateMetadata({
      params: Promise.resolve({ confirmationCode: code }),
    });
    expect(String(meta.title)).not.toContain(code);
  });

  it("marks the page noindex (transactional guest micro-page)", async () => {
    const meta = await generateMetadata({
      params: Promise.resolve({ confirmationCode: "X" }),
    });
    const robots = meta.robots;
    if (typeof robots === "object" && robots !== null) {
      expect(robots.index).toBe(false);
    } else {
      expect(robots).toMatch(/noindex/i);
    }
  });
});
