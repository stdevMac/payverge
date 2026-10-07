import { axiosInstance } from "@/api/tools/instance";

/**
 * Account-level email notification preferences. These are stored on the USER
 * (resolved by user_id/address in the backend), NOT per-business — the same
 * record the dashboard's Settings → Notifications email section edits. Mirrors
 * `structs.NotificationPreferences` so untouched flags (email_enabled master,
 * statistics) round-trip and aren't clobbered to false on save.
 */
export interface AccountEmailPreferences {
  email_enabled?: boolean;
  transactional_enabled: boolean;
  reports_enabled: boolean;
  news_enabled: boolean;
  updates_enabled: boolean;
  security_enabled: boolean;
  statistics_enabled?: boolean;
}

const ENDPOINT = "/inside/settings/notifications";

export async function getEmailNotificationPreferences(): Promise<AccountEmailPreferences> {
  const res = await axiosInstance.get<{
    address: string;
    preferences: AccountEmailPreferences;
  }>(ENDPOINT);
  return res.data.preferences;
}

export async function updateEmailNotificationPreferences(
  preferences: AccountEmailPreferences,
): Promise<AccountEmailPreferences> {
  const res = await axiosInstance.put<{ preferences: AccountEmailPreferences }>(
    ENDPOINT,
    { preferences },
  );
  return res.data.preferences;
}
