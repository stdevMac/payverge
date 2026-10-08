import { API_BASE_URL } from "@/api/tools/baseUrl";

/**
 * Public-demo endpoints (DEMO_MODE only; both answer 404 on a normal
 * install). See docs/self-hosting/public-demo.md.
 */

export type DemoRole = "owner" | "kitchen" | "waiter";

export interface DemoStaff {
  id: number;
  name: string;
  email: string;
  role: "manager" | "server" | "host" | "kitchen";
  business_id: number;
}

export type DemoLoginResult =
  | { kind: "owner"; redirect: string }
  | { kind: "staff"; staff: DemoStaff };

interface DemoTable {
  name: string;
  code: string;
}

export interface DemoVenue {
  name: string;
  custom_url: string;
  tables: DemoTable[];
}

async function errorFrom(res: Response, fallback: string): Promise<Error> {
  const data = (await res.json().catch(() => ({}))) as { error?: unknown };
  return new Error(typeof data.error === "string" && data.error ? data.error : fallback);
}

/** POST /auth/demo/login: one-click session for a fixed demo identity. */
export async function demoLogin(role: DemoRole): Promise<DemoLoginResult> {
  const res = await fetch(`${API_BASE_URL}/auth/demo/login`, {
    method: "POST",
    credentials: "include",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify({ role }),
  });
  if (!res.ok) throw await errorFrom(res, "Could not start the demo");
  const data = (await res.json()) as {
    redirect?: unknown;
    staff?: DemoStaff;
  };
  if (role === "owner") {
    const redirect =
      typeof data.redirect === "string" && data.redirect.startsWith("/") && !data.redirect.startsWith("//")
        ? data.redirect
        : "/dashboard";
    return { kind: "owner", redirect };
  }
  if (!data.staff || typeof data.staff.business_id !== "number") {
    throw new Error("Could not start the demo");
  }
  return { kind: "staff", staff: data.staff };
}

/** GET /demo/tables: a few guest table links per demo venue. */
export async function fetchDemoTables(): Promise<DemoVenue[]> {
  const res = await fetch(`${API_BASE_URL}/demo/tables`, { credentials: "omit" });
  if (!res.ok) throw await errorFrom(res, "Could not load the demo tables");
  const data = (await res.json()) as { venues?: unknown };
  if (!Array.isArray(data.venues)) return [];
  return data.venues
    .filter((v): v is DemoVenue => !!v && typeof v === "object" && Array.isArray((v as DemoVenue).tables))
    .map((v) => ({
      name: String(v.name ?? ""),
      custom_url: String(v.custom_url ?? ""),
      tables: v.tables
        .filter((t) => t && typeof t.code === "string" && /^[A-Za-z0-9_-]+$/.test(t.code))
        .map((t) => ({ name: String(t.name ?? ""), code: t.code })),
    }));
}
