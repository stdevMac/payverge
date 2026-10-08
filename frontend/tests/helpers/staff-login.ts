import { execFileSync } from "node:child_process";
import path from "node:path";

const API_BASE = process.env.PLAYWRIGHT_API_BASE || "http://localhost:8080/api/v1";
const COMPOSE_FILE = path.resolve(__dirname, "..", "..", "..", "docker-compose.yml");
const POSTGRES_USER = "payverge";
const POSTGRES_DB = "payverge";

const FIXED_OTP = "424242";

const EMAIL_RE = /^[A-Za-z0-9._%+-]+@[A-Za-z0-9.-]+\.[A-Za-z]{2,}$/;

/**
 * Inserts an OTP code directly into staff_login_codes for the given email,
 * bypassing the email-send step. Returns the code string.
 *
 * Uses execFileSync with the SQL piped over stdin so no string interpolation
 * touches a shell. The email is also validated against an allow-list regex
 * before going anywhere near the DB.
 *
 * Requires `docker compose` access — designed for local dev runs, not CI.
 */
function seedStaffLoginCode(email: string): string {
  if (!EMAIL_RE.test(email)) {
    throw new Error(`refusing to seed login code for non-email value: ${email}`);
  }

  const sql = [
    `BEGIN;`,
    `DELETE FROM staff_login_codes WHERE staff_id = (SELECT id FROM staff WHERE email = $$${email}$$);`,
    `INSERT INTO staff_login_codes (staff_id, code, expires_at, used, created_at)`,
    `SELECT id, '${FIXED_OTP}', NOW() + INTERVAL '15 minutes', false, NOW() FROM staff WHERE email = $$${email}$$;`,
    `COMMIT;`,
  ].join("\n");

  // execFileSync — no shell. Args are an array; SQL is piped via stdin so the
  // email never appears in argv. PLAYWRIGHT_DIRECT_PSQL=1 skips docker compose
  // and uses a host-local psql client; required when the dev stack runs
  // natively rather than via docker compose.
  if (process.env.PLAYWRIGHT_DIRECT_PSQL === "1") {
    const dbHost = process.env.PLAYWRIGHT_DB_HOST || "localhost";
    const dbPort = process.env.PLAYWRIGHT_DB_PORT || "5432";
    const dbPassword = process.env.PLAYWRIGHT_DB_PASSWORD || "payverge_password";
    execFileSync(
      "psql",
      [
        "-h", dbHost,
        "-p", dbPort,
        "-U", POSTGRES_USER,
        "-d", POSTGRES_DB,
        "-v", "ON_ERROR_STOP=1",
        "-q",
      ],
      {
        input: sql,
        stdio: ["pipe", "pipe", "pipe"],
        env: { ...process.env, PGPASSWORD: dbPassword },
      },
    );
  } else {
    execFileSync(
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
        "-q",
      ],
      { input: sql, stdio: ["pipe", "pipe", "pipe"] },
    );
  }
  return FIXED_OTP;
}

/**
 * Logs the staff member in via `verify-login-code`. The Playwright request
 * context's cookie jar will carry the staff_token cookie after this call;
 * subsequent page.goto's reuse it.
 */
export async function loginStaff(
  request: import("@playwright/test").APIRequestContext,
  email: string,
): Promise<string> {
  const code = seedStaffLoginCode(email);
  const res = await request.post(`${API_BASE}/staff/verify-login-code`, {
    data: { email, code },
  });
  if (!res.ok()) throw new Error(`staff login failed: ${res.status()} ${await res.text()}`);
  const body = (await res.json()) as { token?: string };
  if (!body.token) throw new Error("staff login succeeded without a token");
  return body.token;
}
