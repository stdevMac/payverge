// Onboarding API — data-driven setup status + idempotent completion anchor.
// The IMP-06 per-step JSON state blob (get/update) was removed 2026-07:
// zero non-test consumers; OnboardingHub derives progress from setup-status.

import { axiosInstance } from "./tools/instance";

// Resource path shared with the (kept) completion endpoint.
const path = (businessId: string | number) =>
  `/inside/businesses/${businessId}/onboarding-state`;

export interface OnboardingCompleteResponse {
  completed_at: string | null;
}

/**
 * Marks first-run onboarding complete (idempotent). The old GET/PUT state-blob
 * endpoints were removed as never-consumed; completion is the only per-business
 * onboarding write the frontend performs (OnboardingHub auto-anchor).
 */
export async function completeOnboarding(
  businessId: string | number,
): Promise<OnboardingCompleteResponse> {
  const response = await axiosInstance.post<{ completed_at: string | null }>(
    `${path(businessId)}/complete`,
    {},
  );
  return { completed_at: response.data?.completed_at ?? null };
}

// --- Setup status (data-driven readiness) ---

interface SetupStepBase {
  done: boolean;
}

interface BusinessProfileStep extends SetupStepBase {
  has_name: boolean;
  has_address: boolean;
  has_currency: boolean;
}

interface CountStep extends SetupStepBase {
  count: number;
}

interface MenuSetupStep extends SetupStepBase {
  categories: number;
  items: number;
}

// Crypto plugins only count as a working payment route once the payout wallet
// (settlement address) exists; the backend surfaces that signal so the UI can
// say WHY the step is incomplete.
interface PaymentSetupStep extends CountStep {
  has_settlement_address: boolean;
}

export interface SetupStatusSteps {
  business_profile: BusinessProfileStep;
  tables: CountStep;
  menu: MenuSetupStep;
  staff: CountStep;
  payment: PaymentSetupStep;
  /**
   * Optional floor-plan layout (published space count). Never gates
   * required_done / all_done / completed_count — go-live stays profile+tables+menu.
   */
  layout?: CountStep;
}

export interface SetupStatusResponse {
  steps: SetupStatusSteps;
  completed_count: number;
  total_count: number;
  required_done: boolean;
  all_done: boolean;
  /** True once the business has at least one paid bill (activation signal). */
  has_first_paid_bill: boolean;
  /** Server-authoritative milestone, recorded when an owned table QR is previewed. */
  qr_previewed: boolean;
}

export async function getSetupStatus(
  businessId: string | number,
): Promise<SetupStatusResponse> {
  const response = await axiosInstance.get<SetupStatusResponse>(
    `/inside/businesses/${businessId}/setup-status`,
  );
  return response.data;
}

export async function markQRPreviewed(
  businessId: string | number,
  tableId: number,
): Promise<{ qr_previewed: true; qr_previewed_at: string }> {
  const response = await axiosInstance.post<{
    qr_previewed: true;
    qr_previewed_at: string;
  }>(`/inside/businesses/${businessId}/onboarding/qr-preview`, {
    table_id: tableId,
  });
  return response.data;
}
