import { axiosInstance } from "./tools/instance";

export interface RedemptionResult {
  discount_cents: number;
  points_deducted: number;
  remaining_points: number;
}

export async function redeemPoints(
  tableCode: string,
  points: number,
  billNumber: string,
): Promise<RedemptionResult> {
  const response = await axiosInstance.post(
    `/guest/table/${tableCode}/redeem-points`,
    { points, bill_number: billNumber },
  );
  return response.data as RedemptionResult;
}

/**
 * Points required for ONE unit of the venue's own currency (points per ARS 1 on
 * a peso carta, per $1 on a dollar one) — loyalty never converts to USD (#895).
 */
export async function getLoyaltyRate(tableCode: string): Promise<number> {
  const response = await axiosInstance.get(
    `/guest/table/${tableCode}/loyalty-rate`,
  );
  const data = response.data as { points_per_currency_unit?: number };
  return data.points_per_currency_unit ?? 0;
}

export async function undoRedemption(
  tableCode: string,
  billNumber: string,
): Promise<void> {
  await axiosInstance.post(`/guest/table/${tableCode}/undo-redemption`, {
    bill_number: billNumber,
  });
}

export interface PointsEarnedResult {
  points_earned: number;
  total_points: number;
}

export async function getPointsEarned(
  tableCode: string,
  billNumber: string,
): Promise<PointsEarnedResult> {
  const response = await axiosInstance.get(
    `/guest/table/${tableCode}/points-earned`,
    { params: { bill_number: billNumber } },
  );
  return response.data as PointsEarnedResult;
}

export interface LoyaltyTier {
  id?: number;
  name: string;
  min_lifetime_spent: number;
  sort_order: number;
  color?: string;
}

export interface LoyaltyProgram {
  id: number;
  business_id: number;
  enabled: boolean;
  /** Earn: points awarded per $1 spent. */
  points_per_dollar: number;
  /** Redeem: points required for $1 discount. Independent of earn rate. */
  redemption_points_per_dollar: number;
}

export interface LoyaltyResponse {
  program: LoyaltyProgram;
  tiers: LoyaltyTier[];
  valid?: boolean;
  validation_error?: string;
}

export interface PreviewResponse {
  total_customers: number;
  tier_distribution: Record<string, number>;
}

export async function getLoyalty(businessId: number): Promise<LoyaltyResponse> {
  const res = await axiosInstance.get(
    `/inside/businesses/${businessId}/crm/loyalty`,
  );
  return res.data as LoyaltyResponse;
}

export async function putLoyalty(
  businessId: number,
  body: {
    enabled: boolean;
    points_per_dollar: number;
    redemption_points_per_dollar: number;
    tiers: LoyaltyTier[];
  },
): Promise<{ ok: boolean }> {
  const res = await axiosInstance.put(
    `/inside/businesses/${businessId}/crm/loyalty`,
    body,
  );
  return res.data as { ok: boolean };
}

export async function previewLoyalty(
  businessId: number,
  body: {
    points_per_dollar: number;
    redemption_points_per_dollar?: number;
    tiers: LoyaltyTier[];
  },
): Promise<PreviewResponse> {
  const res = await axiosInstance.post(
    `/inside/businesses/${businessId}/crm/loyalty/preview`,
    body,
  );
  return res.data as PreviewResponse;
}
