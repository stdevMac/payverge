/** @jest-environment node */
import fs from "node:fs";
import path from "node:path";
import { GET, dynamic } from "@/app/.well-known/security.txt/route";
import { buildSecurityTxt, SECURITY_TXT_TTL_DAYS } from "@/lib/security/securityTxt";
import { getSecurityEmail } from "@/config/serverConfig";

const KEYS = ["PUBLIC_URL", "SECURITY_EMAIL", "SUPPORT_EMAIL", "NEXT_PUBLIC_SUPPORT_EMAIL"] as const;
const saved = Object.fromEntries(KEYS.map((k) => [k, process.env[k]]));

beforeEach(() => {
  for (const key of KEYS) delete process.env[key];
});

afterEach(() => {
  for (const [key, value] of Object.entries(saved)) {
    if (value === undefined) delete process.env[key];
    else process.env[key] = value;
  }
});

describe("RFC 9116 security.txt (#291)", () => {
  it("is a per-request route, not a static file with a baked-in origin", () => {
    expect(dynamic).toBe("force-dynamic");
    expect(
      fs.existsSync(path.join(process.cwd(), "public/.well-known/security.txt")),
    ).toBe(false);
  });

  it.each([
    ["https://a.example.test", "security@a.example.test"],
    ["https://b.example.test:8443", "sec@b.example.test"],
  ])("renders contact and canonical for PUBLIC_URL=%s", async (origin, email) => {
    process.env.PUBLIC_URL = `${origin}/`;
    process.env.SECURITY_EMAIL = email;
    const res = GET();
    expect(res.status).toBe(200);
    expect(res.headers.get("content-type")).toBe("text/plain; charset=utf-8");
    const body = await res.text();
    expect(body).toContain(`Contact: mailto:${email}`);
    expect(body).toContain("Preferred-Languages:");
    expect(body).toContain(`Canonical: ${origin}/.well-known/security.txt`);
    expect(body).toMatch(/Expires:\s*\d{4}-\d{2}-\d{2}T/);
    expect(body).not.toContain("payverge.io");
  });

  it("falls back to SUPPORT_EMAIL when SECURITY_EMAIL is unset or malformed", async () => {
    process.env.PUBLIC_URL = "https://a.example.test";
    process.env.SUPPORT_EMAIL = "help@a.example.test";
    process.env.SECURITY_EMAIL = "not an email";
    const body = await GET().text();
    expect(body).toContain("Contact: mailto:help@a.example.test");
  });

  it("404s when no contact is configured instead of naming another inbox", async () => {
    process.env.PUBLIC_URL = "https://a.example.test";
    const res = GET();
    expect(res.status).toBe(404);
    expect(await res.text()).not.toContain("Contact:");
  });

  it("keeps Expires within a year and stable across one UTC day", () => {
    const morning = buildSecurityTxt({
      contactEmail: "s@x.test",
      siteUrl: "https://x.test",
      now: new Date("2026-10-03T00:05:00Z"),
    });
    const evening = buildSecurityTxt({
      contactEmail: "s@x.test",
      siteUrl: "https://x.test",
      now: new Date("2026-10-03T23:55:00Z"),
    });
    expect(morning).toBe(evening);
    const expires = new Date(/Expires: (\S+)/.exec(morning)![1]);
    const days = (expires.getTime() - Date.UTC(2026, 9, 3)) / 86_400_000;
    expect(days).toBe(SECURITY_TXT_TTL_DAYS);
    expect(days).toBeLessThan(365);
  });

  it("getSecurityEmail honours the env it is given", () => {
    expect(getSecurityEmail({ SUPPORT_EMAIL: "help@env.test" })).toBe("help@env.test");
    expect(getSecurityEmail({ SECURITY_EMAIL: "sec@env.test", SUPPORT_EMAIL: "help@env.test" })).toBe(
      "sec@env.test",
    );
    expect(getSecurityEmail({})).toBe("");
  });
});
