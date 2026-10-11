import { createHash } from "node:crypto";
import { runJourneySql } from "./delivery-test-setup";

const INVITE_RE = /^[A-Za-z0-9_-]{16,128}$/;
const EMAIL_RE = /^[A-Za-z0-9._%+-]+@[A-Za-z0-9.-]+\.[A-Za-z]{2,}$/;

/** Seed a short-lived launch cohort without persisting the plaintext code. */
export function seedOwnerLaunchInvite(code: string): void {
  if (!INVITE_RE.test(code)) {
    throw new Error("refusing to seed an invalid launch invite code");
  }
  const digest = createHash("sha256").update(code).digest("hex");
  runJourneySql(`
    INSERT INTO runtime_invite_batches (
      name, code_digest, cohort_cap, claimed_count, active, owner, reason,
      expires_at, created_by, created_at, updated_at
    ) VALUES (
      'Playwright owner onboarding', '${digest}', 10, 0, true,
      'ci-e2e', 'Verify invite-gated owner onboarding',
      NOW() + INTERVAL '1 hour', 'ci-e2e', NOW(), NOW()
    )
    ON CONFLICT (code_digest) DO UPDATE SET
      active = true,
      expires_at = EXCLUDED.expires_at,
      updated_at = NOW();
  `);
}

/**
 * Model the completed email-link step after registration. The production
 * plaintext verification token is intentionally not recoverable from storage,
 * so the isolated E2E database is advanced at this explicit seam.
 */
export function verifyOwnerEmailForJourney(email: string): void {
  if (!EMAIL_RE.test(email)) {
    throw new Error("refusing to verify a non-email owner identity");
  }
  runJourneySql(`
    BEGIN;
    UPDATE users
       SET email_verified = true, updated_at = NOW()
     WHERE LOWER(email) = LOWER($$${email}$$);
    UPDATE user_auths
       SET email_verified = true, updated_at = NOW()
     WHERE provider = 'email'
       AND LOWER(provider_user_id) = LOWER($$${email}$$);
    DO $$
    DECLARE verified_count integer;
    BEGIN
      SELECT COUNT(*) INTO verified_count
        FROM users u
        JOIN user_auths a ON a.user_id = u.id
       WHERE LOWER(u.email) = LOWER($email$${email}$email$)
         AND u.email_verified = true
         AND a.provider = 'email'
         AND a.email_verified = true;
      IF verified_count <> 1 THEN
        RAISE EXCEPTION 'owner verification seam did not update one identity';
      END IF;
    END
    $$;
    COMMIT;
  `);
}
