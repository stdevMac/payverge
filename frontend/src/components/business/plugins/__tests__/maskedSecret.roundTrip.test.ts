/**
 * Stream 9 #7 — guard the masked-credential round-trip before the payment
 * plugin forms move to NextUI. Behavior must not change: the sentinel
 * "••••••••" means "keep stored secret" and must pass validation / Save.
 */
import {
  MASKED_SECRET,
  hasSecretFormatError,
  isMaskedSecret,
  isSecretSatisfied,
} from "../configFields";

describe("masked secret sentinel (plugin credential round-trip)", () => {
  it("recognizes the backend mask sentinel", () => {
    expect(MASKED_SECRET).toBe("••••••••");
    expect(isMaskedSecret(MASKED_SECRET)).toBe(true);
    expect(isMaskedSecret("••••••••")).toBe(true);
    expect(isMaskedSecret("live_sk_real")).toBe(false);
    expect(isMaskedSecret("")).toBe(false);
    expect(isMaskedSecret(null)).toBe(false);
  });

  it("treats the sentinel as satisfied without format validation", () => {
    const rejectAll = () => false;
    expect(isSecretSatisfied(MASKED_SECRET, rejectAll)).toBe(true);
    expect(hasSecretFormatError(MASKED_SECRET, rejectAll)).toBe(false);
  });

  it("still validates genuinely new secrets", () => {
    const looksLikeKey = (v: string) => v.startsWith("sk_");
    expect(isSecretSatisfied("sk_live_abc", looksLikeKey)).toBe(true);
    expect(isSecretSatisfied("not-a-key", looksLikeKey)).toBe(false);
    expect(hasSecretFormatError("not-a-key", looksLikeKey)).toBe(true);
    expect(hasSecretFormatError("", looksLikeKey)).toBe(false);
  });

  it("preserves the sentinel through a form-style config merge", () => {
    // Simulates an editor that loads masked config and re-submits without retype.
    const loaded = {
      client_id: "AeA1B2",
      client_secret: MASKED_SECRET,
      environment: "sandbox",
    };
    const submitted = { ...loaded }; // operator didn't touch the secret field
    expect(submitted.client_secret).toBe(MASKED_SECRET);
    expect(isMaskedSecret(submitted.client_secret)).toBe(true);
    // Backend restoreMaskedSecrets keeps the real secret when it sees this.
  });
});
