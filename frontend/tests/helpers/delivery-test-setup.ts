import { execFileSync } from "node:child_process";
import path from "node:path";

const POSTGRES_USER = "payverge";
const POSTGRES_DB = "payverge";
const COMPOSE_FILE = path.resolve(
  __dirname,
  "..",
  "..",
  "..",
  "docker-compose.yml",
);

/**
 * Ensures the seeded business has enough delivery capacity for a test
 * to place a fresh order. The demo seed runs the dashboard at near-max
 * (5 cap, 8 in-flight) on purpose so operators can see the "capacity
 * full" UI; the e2e needs headroom.
 *
 * Bumps `max_concurrent_deliveries` without touching in-flight rows and opens
 * the delivery window for the deterministic journey. The nightly run starts
 * before the seeded venue's normal 10:30 opening time, so leaving real hours
 * in place would make the test fail every day for the correct product behavior.
 */
export function ensureDeliveryCapacity(
  businessId: number,
  capacity = 50,
): void {
  const sql =
    `UPDATE delivery_settings SET max_concurrent_deliveries = ${capacity}, ` +
    `delivery_hours_same_as_business = false, delivery_start_time = '00:00', ` +
    `delivery_end_time = '23:59' WHERE business_id = ${businessId};`;

  runJourneySql(sql);
}

/** Make time-dependent dine-in journeys deterministic regardless of UTC run time. */
export function ensureBusinessOpenForJourney(businessId: number): void {
  runJourneySql(
    `UPDATE business_operating_hours SET is_closed = false, open_time = '00:00', ` +
      `close_time = '23:59', updated_at = NOW() WHERE business_id = ${businessId};`,
  );
}

export function runJourneySql(sql: string): void {
  if (process.env.PLAYWRIGHT_DIRECT_PSQL === "1") {
    const dbHost = process.env.PLAYWRIGHT_DB_HOST || "localhost";
    const dbPort = process.env.PLAYWRIGHT_DB_PORT || "5432";
    const dbPassword =
      process.env.PLAYWRIGHT_DB_PASSWORD || "payverge_password";
    execFileSync(
      "psql",
      [
        "-h",
        dbHost,
        "-p",
        dbPort,
        "-U",
        POSTGRES_USER,
        "-d",
        POSTGRES_DB,
        "-v",
        "ON_ERROR_STOP=1",
        "-q",
      ],
      {
        input: sql,
        stdio: ["pipe", "pipe", "pipe"],
        env: { ...process.env, PGPASSWORD: dbPassword },
      },
    );
    return;
  }

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
