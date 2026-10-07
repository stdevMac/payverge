import { axiosInstance } from "@/api/tools/instance";
import { apiErrorDetail } from "@/utils/apiError";

export interface ScheduleSettings {
  id: number;
  business_id: number;
  week_start_day: number;
  default_shift_minutes: number;
  reminder_lead_hours: number;
  overtime_weekly_minutes: number;
  posted_lead_days: number;
  minor_cutoff_min: number | null;
  quiet_hours_start_min: number | null;
  quiet_hours_end_min: number | null;
  created_at: string;
  updated_at: string;
}

export type ScheduleSettingsUpdate = Partial<
  Pick<
    ScheduleSettings,
    | "week_start_day"
    | "default_shift_minutes"
    | "reminder_lead_hours"
    | "overtime_weekly_minutes"
    | "posted_lead_days"
    | "minor_cutoff_min"
    | "quiet_hours_start_min"
    | "quiet_hours_end_min"
  >
>;

const base = (businessId: string) => `/inside/businesses/${businessId}/schedule/settings`;

export const scheduleSettingsApi = {
  async get(businessId: string): Promise<ScheduleSettings> {
    try {
      const res = await axiosInstance.get(base(businessId));
      return res.data.data as ScheduleSettings;
    } catch (error) {
      throw new Error(apiErrorDetail(error) || "Failed to load schedule settings");
    }
  },
  async update(businessId: string, body: ScheduleSettingsUpdate): Promise<ScheduleSettings> {
    try {
      const res = await axiosInstance.put(base(businessId), body);
      return res.data.data as ScheduleSettings;
    } catch (error) {
      throw new Error(apiErrorDetail(error) || "Failed to update schedule settings");
    }
  },
};
