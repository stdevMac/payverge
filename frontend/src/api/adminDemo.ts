import { axiosInstance } from "@/api/tools/instance";

interface AdminDemoInstance {
  id: number;
  admin_user_id: number;
  primary_business_id?: number | null;
  secondary_business_id?: number | null;
  status: "creating" | "ready" | "failed" | "resetting" | string;
  seed_version: string;
  baseline_start_date: string;
  last_simulated_business_date?: string | null;
  timezone: string;
  last_error?: string;
  last_verified_at?: string | null;
  created_at: string;
  updated_at: string;
}

export interface AdminDemoBusiness {
  id: number;
  business_id?: string;
  name: string;
  custom_url?: string;
  is_demo?: boolean;
  demo_owner_user_id?: number | null;
  logo?: string;
  created_at?: string;
  updated_at?: string;
}

interface AdminDemoRun {
  id: number;
  demo_instance_id?: number;
  admin_user_id: number;
  run_type: string;
  status: string;
  seed_version?: string;
  started_at?: string;
  finished_at?: string | null;
  business_date_from?: string | null;
  business_date_to?: string | null;
  records_created?: Record<string, number>;
  verification?: Record<string, unknown>;
  error?: string;
}

export interface AdminDemoAccessIdentity {
  business_id: number;
  business_name: string;
  role: string;
  name: string;
  email: string;
  login_path: string;
  pin_hint?: string;
}

export interface AdminDemoCoverageCheck {
  key: string;
  status: "passed" | "failed" | string;
  count: number;
  message?: string;
}

export interface AdminDemoVerification {
  status: "passed" | "failed" | string;
  errors: string[];
  coverage: AdminDemoCoverageCheck[];
}

/** Append-worker liveness from Task 17 honesty pass. */
interface AdminDemoHeartbeat {
  status: "fresh" | "stale" | "unknown" | string;
  last_append_at?: string | null;
  append_interval_seconds?: number;
  stale_after_seconds?: number;
}

export interface AdminDemoSummary {
  instance: AdminDemoInstance | null;
  businesses: AdminDemoBusiness[];
  access: AdminDemoAccessIdentity[];
  runs: AdminDemoRun[];
  verification: AdminDemoVerification;
  heartbeat?: AdminDemoHeartbeat | null;
}

interface VerifyResponse {
  verification: AdminDemoVerification;
}

export async function getAdminDemo(): Promise<AdminDemoSummary> {
  const response = await axiosInstance.get<AdminDemoSummary>("/admin/demo", {
    _useCache: false,
  });
  return response.data;
}

export async function ensureAdminDemo(): Promise<AdminDemoSummary> {
  const response =
    await axiosInstance.post<AdminDemoSummary>("/admin/demo/ensure");
  return response.data;
}

export async function resetAdminDemo(): Promise<AdminDemoSummary> {
  const response =
    await axiosInstance.post<AdminDemoSummary>("/admin/demo/reset");
  return response.data;
}

export async function appendAdminDemoDay(): Promise<AdminDemoSummary> {
  const response = await axiosInstance.post<AdminDemoSummary>(
    "/admin/demo/append-day",
  );
  return response.data;
}

export async function verifyAdminDemo(): Promise<AdminDemoVerification> {
  const response =
    await axiosInstance.post<VerifyResponse>("/admin/demo/verify");
  return response.data.verification;
}
