import { execFileSync } from "node:child_process";
import { createHash } from "node:crypto";
import path from "node:path";
import {
  request as apiRequestFactory,
  type APIRequestContext,
} from "@playwright/test";
import { loginStaff } from "./staff-login";

const COMPOSE_FILE = path.resolve(
  __dirname,
  "..",
  "..",
  "..",
  "docker-compose.yml",
);
const POSTGRES_USER = "payverge";
const POSTGRES_DB = "payverge";

export const API_BASE =
  process.env.PLAYWRIGHT_API_BASE || "http://localhost:8080/api/v1";
export const BUSINESS_SLUG =
  process.env.PLAYWRIGHT_DEMO_SLUG || "demo-core-kitchen";
/**
 * Table code minted by backend/scripts/demo_seed.sql for table `n` of a seeded
 * profile: upper(substr(md5('payverge-fixture-<profile>-table-' || n), 1, 10)).
 * Codes are high-entropy on purpose (#293); legacy enumerable codes such as
 * CORE-T01 or demo-2-ai-pro-table-02 are never seeded and 404 by design.
 */
export function seededTableCode(profile: "core" | "ai-pro", n: number): string {
  return createHash("md5")
    .update(`payverge-fixture-${profile}-table-${n}`)
    .digest("hex")
    .slice(0, 10)
    .toUpperCase();
}

/** Seeded core-business table "Indoor 1". */
export const TABLE_CODE =
  process.env.PLAYWRIGHT_TABLE_CODE || seededTableCode("core", 1);

/**
 * Seeded core-business manager (backend/scripts/demo_seed.sql). Overridable so
 * later journeys can log in as a different seeded staff role.
 */
export const MANAGER_EMAIL =
  process.env.PLAYWRIGHT_MANAGER_EMAIL || "manager@core-demo.payverge.example";
export const MARKETING_BUSINESS_SLUG =
  process.env.PLAYWRIGHT_MARKETING_DEMO_SLUG || "demo-ai-lounge";
export const MARKETING_MANAGER_EMAIL =
  process.env.PLAYWRIGHT_MARKETING_MANAGER_EMAIL ||
  "gm@ai-demo.payverge.example";

export const journeysEnabled = !!process.env.PLAYWRIGHT_RUN_JOURNEYS_E2E;

/** Resolve the seeded demo business id via the public endpoint (seed re-runs shift ids). */
export async function resolveBusinessId(
  slug: string = BUSINESS_SLUG,
): Promise<number> {
  const ctx = await apiRequestFactory.newContext();
  try {
    const resp = await ctx.get(`${API_BASE}/business/${slug}`);
    if (!resp.ok())
      throw new Error(`resolve business ${slug}: ${resp.status()}`);
    return Number((await resp.json()).id);
  } finally {
    await ctx.dispose();
  }
}

type ReservationAvailabilitySlot = {
  time: string; // RFC3339 UTC
  available_tables: number;
};

/**
 * Earliest bookable reservation slot (RFC3339 UTC) for `partySize`, taken from
 * the public availability endpoint
 * (GET /business/:customUrl/reservations/availability).
 *
 * The server builds slots from the venue's operating hours in the business
 * timezone and only marks a slot available when it clears min-advance and a
 * full `settings.default_duration` + service buffer fits before closing. So a
 * reservation created at the returned time WITHOUT an explicit `duration` is
 * accepted regardless of the UTC run time, the seeded hours, or whether an
 * earlier spec widened them (ensureBusinessOpenForJourney). Scans a week of
 * day keys from `minDaysAhead` to survive closed days.
 */
export async function findBookableReservationSlot(
  api: APIRequestContext,
  options: { partySize: number; minDaysAhead?: number; slug?: string },
): Promise<string> {
  const { partySize, minDaysAhead = 0, slug = BUSINESS_SLUG } = options;
  const failures: string[] = [];
  for (let ahead = minDaysAhead; ahead < minDaysAhead + 7; ahead++) {
    const date = new Date(Date.now() + ahead * 24 * 60 * 60 * 1000)
      .toISOString()
      .slice(0, 10); // YYYY-MM-DD, read as a civil day in the business timezone
    const resp = await api.get(
      `${API_BASE}/business/${slug}/reservations/availability?date=${date}&party_size=${partySize}`,
    );
    if (!resp.ok()) {
      failures.push(`${date}: ${resp.status()} ${await resp.text()}`);
      continue;
    }
    const body = (await resp.json()) as {
      available_slots?: ReservationAvailabilitySlot[];
    };
    const slot = (body.available_slots ?? []).find(
      (s) => s.available_tables > 0,
    );
    if (slot) return slot.time;
  }
  throw new Error(
    `no available reservation slot for ${slug} within a week — is the demo seed loaded?` +
      (failures.length ? ` Availability errors: ${failures.join("; ")}` : ""),
  );
}

/**
 * A logged-in staff (manager) API context. The returned context's cookie jar
 * carries the staff_token.
 *
 * ⚠️ Always call it with ABSOLUTE URLs, e.g.
 * `api.get(`${API_BASE}/inside/businesses/1/bills/open`)`. Playwright joins
 * leading-slash paths against the baseURL's ORIGIN only (`new URL(url, base)`),
 * silently dropping the `/api/v1` path segment — `api.get("/businesses/...")`
 * would hit `http://localhost:8080/businesses/...` and 404. Manager routes
 * live under `/api/v1/inside` (backend/cmd/app/main.go:1595).
 *
 * `loginStaff(request, email)` posts to `${API_BASE}/staff/verify-login-code`
 * with a seeded OTP — see tests/helpers/staff-login.ts.
 */
export async function newStaffApi(
  playwright: {
    request: {
      newContext: (o: {
        baseURL: string;
        extraHTTPHeaders?: Record<string, string>;
      }) => Promise<APIRequestContext>;
    };
  },
  email: string = MANAGER_EMAIL,
): Promise<APIRequestContext> {
  const bootstrap = await playwright.request.newContext({ baseURL: API_BASE });
  try {
    const token = await loginStaff(bootstrap, email);
    return await playwright.request.newContext({
      baseURL: API_BASE,
      extraHTTPHeaders: { Authorization: `Bearer ${token}` },
    });
  } finally {
    await bootstrap.dispose();
  }
}

export async function newManagerApi(
  playwright: Parameters<typeof newStaffApi>[0],
): Promise<APIRequestContext> {
  return newStaffApi(playwright, MANAGER_EMAIL);
}

const SAFE_FIXTURE_VALUE = /^[A-Za-z0-9@._:\-\[\] ]+$/;

function assertSafeFixtureValue(value: string, label: string): void {
  if (!SAFE_FIXTURE_VALUE.test(value)) {
    throw new Error(`refusing unsafe ${label} fixture value`);
  }
}

/** Execute narrowly scoped demo-fixture SQL without passing SQL through a shell. */
function runJourneyFixtureSQL(sql: string): string {
  if (process.env.PLAYWRIGHT_DIRECT_PSQL === "1") {
    return execFileSync(
      "psql",
      [
        "-h",
        process.env.PLAYWRIGHT_DB_HOST || "localhost",
        "-p",
        process.env.PLAYWRIGHT_DB_PORT || "5432",
        "-U",
        POSTGRES_USER,
        "-d",
        POSTGRES_DB,
        "-v",
        "ON_ERROR_STOP=1",
        "-qAt",
      ],
      {
        input: sql,
        encoding: "utf8",
        env: {
          ...process.env,
          PGPASSWORD: process.env.PLAYWRIGHT_DB_PASSWORD || "payverge_password",
        },
      },
    );
  }

  return execFileSync(
    "docker",
    [
      "compose",
      "-f",
      COMPOSE_FILE,
      "exec",
      "-T",
      "postgres",
      "psql",
      "-U",
      POSTGRES_USER,
      "-d",
      POSTGRES_DB,
      "-v",
      "ON_ERROR_STOP=1",
      "-qAt",
    ],
    { input: sql, encoding: "utf8" },
  );
}

/**
 * Return the marketing journey to its safe baseline after an interrupted run.
 * Preserve the seeded creative profile/disabled plays, but always re-enable
 * suggestions and remove any deny for this seeded staff member.
 */
export function resetMarketingJourneyFixture(
  businessSlug: string,
  staffEmail: string,
  captionMarker: string,
): void {
  assertSafeFixtureValue(businessSlug, "business slug");
  assertSafeFixtureValue(staffEmail, "staff email");
  assertSafeFixtureValue(captionMarker, "caption marker");
  runJourneyFixtureSQL(`
BEGIN;
DELETE FROM staff_permission_denies AS deny
USING staff, businesses AS business
WHERE deny.staff_id = staff.id
  AND deny.business_id = business.id
  AND business.custom_url = '${businessSlug}'
  AND staff.email = '${staffEmail}'
  AND deny.permission = 'marketing:write';

DELETE FROM marketing_activities AS activity
USING businesses AS business
WHERE activity.business_id = business.id
  AND business.custom_url = '${businessSlug}'
  AND activity.creative_snapshot ->> 'caption' = '${captionMarker}';

UPDATE businesses
SET marketing_settings = jsonb_set(
  COALESCE(marketing_settings, '{}'::jsonb),
  '{enabled}',
  'true'::jsonb,
  true
)
WHERE custom_url = '${businessSlug}';
COMMIT;
`);
}

export function marketingWriteDenyExists(
  businessSlug: string,
  staffEmail: string,
): boolean {
  assertSafeFixtureValue(businessSlug, "business slug");
  assertSafeFixtureValue(staffEmail, "staff email");
  const output = runJourneyFixtureSQL(`
SELECT EXISTS (
  SELECT 1
  FROM staff_permission_denies AS deny
  JOIN staff ON staff.id = deny.staff_id
  JOIN businesses AS business ON business.id = deny.business_id
  WHERE business.custom_url = '${businessSlug}'
    AND staff.email = '${staffEmail}'
    AND deny.permission = 'marketing:write'
);
`);
  return output.trim() === "t";
}

/** Toggle one reversible deny used to prove the same seeded GM's read-only UI. */
export function setMarketingWriteDeny(
  businessSlug: string,
  staffEmail: string,
  denied: boolean,
): void {
  assertSafeFixtureValue(businessSlug, "business slug");
  assertSafeFixtureValue(staffEmail, "staff email");
  runJourneyFixtureSQL(
    denied
      ? `
INSERT INTO staff_permission_denies (
  business_id, staff_id, permission, created_by, reason
)
SELECT business.id, staff.id, 'marketing:write', 'playwright',
       'marketing approval journey read-only proof'
FROM businesses AS business
JOIN staff ON staff.business_id = business.id
WHERE business.custom_url = '${businessSlug}'
  AND staff.email = '${staffEmail}'
ON CONFLICT (business_id, staff_id, permission) DO NOTHING;
`
      : `
DELETE FROM staff_permission_denies AS deny
USING staff, businesses AS business
WHERE deny.staff_id = staff.id
  AND deny.business_id = business.id
  AND business.custom_url = '${businessSlug}'
  AND staff.email = '${staffEmail}'
  AND deny.permission = 'marketing:write';
`,
  );
}
