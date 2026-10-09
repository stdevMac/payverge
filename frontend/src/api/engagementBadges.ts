import { axiosInstance } from "@/api/tools/instance";

// Staff-nav badge counts (Phase 5 · Slice 5a): the caller's own "needs attention"
// tallies for buried engagement surfaces, so the More menu + bottom nav can nudge
// toward work that's otherwise two taps deep. NO money — staff surfaces are
// money-free. Mirrors backend/internal/handlers/engagement_badges.go.
export interface EngagementBadges {
  unacked_announcements: number;
  pending_checklists: number;
}

const base = (businessId: string) =>
  "/inside/businesses/" + businessId + "/me/engagement-badges";

export const engagementBadgesApi = {
  // The caller's OWN badge counts (self-scoped by the backend from staff_id).
  get: async (businessId: string): Promise<EngagementBadges> => {
    const res = await axiosInstance.get(base(businessId));
    return res.data.data;
  },
};
