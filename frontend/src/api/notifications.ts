import { axiosInstance } from "@/api/tools/instance";

// Staff notification inbox (Phase 1). NO money anywhere on these types — staff
// surfaces are money-free. Mirrors backend/internal/database/staff_notification_service.go.
export interface StaffNotification {
  id: number;
  business_id: number;
  staff_id: number;
  kind: string;
  title: string;
  body: string;
  url: string;
  read_at: string | null;
  created_at: string;
}

const base = (businessId: string) => "/inside/businesses/" + businessId + "/me/notifications";

export const notificationsApi = {
  // The caller's OWN inbox, newest first. limit caps at 50; before is a keyset id.
  list: async (
    businessId: string,
    opts?: { limit?: number; before?: number },
  ): Promise<StaffNotification[]> => {
    const res = await axiosInstance.get(base(businessId), {
      params: { limit: opts?.limit, before: opts?.before || undefined },
    });
    return res.data.data;
  },

  unreadCount: async (businessId: string): Promise<number> => {
    const res = await axiosInstance.get(base(businessId) + "/unread-count");
    return res.data.data.count;
  },

  // Stamps read_at on the caller's own rows. Returns the number updated.
  markRead: async (
    businessId: string,
    body: { ids?: number[]; all?: boolean },
  ): Promise<number> => {
    const res = await axiosInstance.post(base(businessId) + "/read", body);
    return res.data.data.updated;
  },
};
