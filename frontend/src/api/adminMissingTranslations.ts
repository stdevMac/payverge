import { axiosInstance } from "@/api/tools/instance";

export type MissingTranslationStatus = "open" | "resolved" | "ignored";

export interface MissingTranslationRow {
  id: number;
  locale: string;
  key_path: string;
  page: string;
  fallback_used: string;
  hit_count: number;
  occurrence_count?: number;
  first_seen_at: string;
  last_seen_at: string;
  status: MissingTranslationStatus;
  status_updated_at: string | null;
}

export interface MissingTranslationsResponse {
  rows: MissingTranslationRow[];
  total: number;
  limit: number;
  offset: number;
}

export async function getMissingTranslations(params?: {
  limit?: number;
  offset?: number;
  locale?: string;
  status?: MissingTranslationStatus;
}): Promise<MissingTranslationsResponse> {
  const response = await axiosInstance.get<MissingTranslationsResponse>(
    "/admin/analytics/missing-translations",
    { params, _useCache: false },
  );
  const rows = (response.data.rows || []).map((row) => ({
    ...row,
    hit_count: row.hit_count ?? row.occurrence_count ?? 0,
  }));

  return {
    rows,
    total: response.data.total || 0,
    limit: response.data.limit || params?.limit || 100,
    offset: response.data.offset || params?.offset || 0,
  };
}

export async function updateMissingTranslationStatus(
  id: number,
  status: MissingTranslationStatus,
): Promise<void> {
  await axiosInstance.patch(
    `/admin/analytics/missing-translations/${id}/status`,
    { status },
  );
}
