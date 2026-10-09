import { axiosInstance } from "@/api/tools/instance";

// Logbook — shift-handover notes (Slice 8). Any staff member may log a note for
// the next shift; the feed is date-scoped (default today), newest-first. NO money
// on this wire: handover notes carry operational context, never dollars. Mirrors
// the backend logbook handler (envelope { success, data }; author taken from the
// staff session server-side, so it is never sent on create).

export type ShiftNoteCategory = "sales" | "guests" | "staffing" | "maintenance" | "other";

export interface ShiftNote {
  id: number;
  business_id: number;
  shift_id: number | null;
  for_date: string;
  author_staff_id: number;
  // Author display name, resolved server-side (Phase 5 · Slice 5b) so the operator
  // logbook can show who logged each handover. Empty for a since-removed author;
  // the staff feed ignores it.
  author_name?: string;
  category: ShiftNoteCategory;
  content: string;
  created_at: string;
}

export interface ShiftNoteInput {
  category: ShiftNoteCategory;
  content: string;
  date?: string; // YYYY-MM-DD; omit for today
  shift_id?: number;
}

const base = (businessId: string) => "/inside/businesses/" + businessId;

export const logbookApi = {
  list: async (businessId: string, date?: string): Promise<ShiftNote[]> => {
    const qs = date ? "?date=" + encodeURIComponent(date) : "";
    const res = await axiosInstance.get(base(businessId) + "/shift-notes" + qs);
    return res.data.data;
  },

  create: async (businessId: string, input: ShiftNoteInput): Promise<ShiftNote> => {
    const res = await axiosInstance.post(base(businessId) + "/shift-notes", input);
    return res.data.data;
  },
};
