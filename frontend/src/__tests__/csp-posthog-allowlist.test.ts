import { resolvePublicEnv, toPublicConfig } from "@/config/publicConfig";
import { buildContentSecurityPolicy } from "@/lib/security/csp";

// The future PostHog hookup needs no CSP edit: setting POSTHOG_HOST at
// runtime adds exactly that origin to connect-src. Unset, no analytics host
// is allowed at all (a fresh self-hosted install talks to nobody).
describe("CSP connect-src follows POSTHOG_HOST (no change required for the gate)", () => {
  const csp = (env: Record<string, string>) =>
    buildContentSecurityPolicy({
      nonce: "bm9uY2U=",
      dev: false,
      config: toPublicConfig(
        resolvePublicEnv(env, "production", { includeBuildFallback: false }),
      ),
      cloudflareInsights: false,
    });

  it("allows the configured PostHog host", () => {
    expect(csp({ POSTHOG_HOST: "https://us.i.posthog.com" })).toMatch(
      /connect-src[^;]*https:\/\/us\.i\.posthog\.com/,
    );
  });

  it("allows no PostHog host when unset", () => {
    expect(csp({})).not.toMatch(/posthog/);
  });
});
