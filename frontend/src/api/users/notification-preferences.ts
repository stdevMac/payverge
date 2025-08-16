import { axiosInstance } from "../tools/instance";

export interface NotificationPreferences {
  email_enabled: boolean;
  news_enabled: boolean;
  updates_enabled: boolean;
  investment_enabled: boolean;
  fleet_updates_enabled: boolean;
  car_purchase_enabled: boolean;
  car_sale_enabled: boolean;
  car_status_enabled: boolean;
  rewards_enabled: boolean;
  dividends_enabled: boolean;
  transactional_enabled: boolean;
  security_enabled: boolean;
  reports_enabled: boolean;
  statistics_enabled: boolean;
}

interface NotificationPreferencesResponse {
  preferences: NotificationPreferences;
}

export interface UpdateNotificationPreferencesRequest {
  preferences: Partial<NotificationPreferences>;
}

export async function getNotificationPreferences(): Promise<NotificationPreferencesResponse> {
  const response = await axiosInstance.get<NotificationPreferencesResponse>(
    "/inside/settings/notifications"
  );
  return response.data;
}

export async function updateNotificationPreferences(
  data: UpdateNotificationPreferencesRequest
): Promise<boolean> {
  const response = await axiosInstance.put(
    "/inside/settings/notifications",
    data
  );
  return response.status === 200;
}
