import { axiosInstance } from "@/api/tools/instance";

// Coverage — open-shift claims & shift swaps/give-ups (Slice 5). NO money on any
// of these types: staff pick up open shifts, request swaps, and managers approve
// — pay never crosses this wire. Mirrors backend coverage_service.go + the
// coverage handlers (envelope { success, data }; role-branched open feed).

export type OpenClaimStatus = "pending" | "approved" | "denied" | "withdrawn";
export type SwapStatus =
  | "open"
  | "accepted"
  | "pending_approval"
  | "approved"
  | "denied"
  | "cancelled";
export type SwapKind = "swap" | "giveup";

/** An open (unassigned) shift the caller is eligible to claim. */
interface OpenShiftItem {
  shift_id: number;
  position_id: number;
  starts_at: string; // RFC3339
  ends_at: string; // RFC3339
  break_minutes: number;
}

/** A swap a coworker offered that the caller is eligible to accept. */
interface SwapInboxItem {
  swap_id: number;
  shift_id: number;
  requesting_staff_id: number;
  position_id: number;
  starts_at: string; // RFC3339
  ends_at: string; // RFC3339
}

/** A pending coverage decision in the manager queue (swap, give-up, or claim). */
export interface PendingApprovalItem {
  id: number;
  shift_id: number;
  position_id: number;
  kind: SwapKind | "open_claim";
  status: SwapStatus | OpenClaimStatus;
  requesting_staff_id: number;
  accepting_staff_id: number | null;
  claiming_staff_id: number | null;
  starts_at: string; // RFC3339
  ends_at: string; // RFC3339
}

/**
 * GET /coverage/open — role-branched in one route. Every staff member gets the
 * open shifts they may claim + the swap inbox they may accept; approvers
 * additionally get `pending_approvals` (the manager queue feeding ApprovalsPanel,
 * empty for non-approvers).
 */
export interface OpenCoverage {
  open_shifts: OpenShiftItem[];
  swap_inbox: SwapInboxItem[];
  pending_approvals: { swaps: PendingApprovalItem[]; claims: PendingApprovalItem[] };
}

export interface OpenShiftClaim {
  id: number;
  shift_id: number;
  status: OpenClaimStatus;
  created_at: string;
}

export interface ShiftSwap {
  id: number;
  shift_id: number;
  kind: SwapKind;
  status: SwapStatus;
  requesting_staff_id: number;
  accepting_staff_id: number | null;
  created_at: string;
}

/** GET /coverage/mine — the caller's own claims + swaps they requested. */
export interface MyCoverage {
  claims: OpenShiftClaim[];
  swaps: ShiftSwap[];
}

export interface RequestSwapInput {
  kind: SwapKind;
  target?: "all_in_role" | "specific";
  target_staff_id?: number;
}

/**
 * The `kind` discriminator for the cancel route: a swap/giveup the caller offered,
 * or an open-shift claim they filed. The backend maps swap|giveup → CancelSwap and
 * open_claim → WithdrawClaim, both ownership-scoped to the caller.
 */
export type CancelKind = SwapKind | "open_claim";

/**
 * A resolved coverage event for the manager history view (Phase 5 · Slice 5c):
 * an approved/denied/cancelled swap or give-up, or an approved/denied/withdrawn
 * open-shift claim. Staff carried as ids (the operator surface resolves names from
 * its roster). NO money. Mirrors backend CoverageHistoryItem.
 */
export interface CoverageHistoryItem {
  kind: SwapKind | "open_claim";
  request_id: number;
  shift_id: number;
  position_id: number;
  starts_at: string; // RFC3339
  ends_at: string; // RFC3339
  status: SwapStatus | OpenClaimStatus;
  requester_staff_id: number;
  decider_staff_id: number | null; // null for a self-cancel/withdraw
  resolved_at: string; // RFC3339
}

export interface DecisionInput {
  /**
   * Polymorphic discriminator: the single locked decision route covers both
   * swaps and open-shift claims. `:requestId` is the swap id when "swap" and the
   * claim id when "open_claim".
   */
  kind: "swap" | "open_claim";
  decision: "approve" | "deny";
}

const base = (businessId: string) => "/inside/businesses/" + businessId;

export const coverageApi = {
  // Role-branched open feed: open shifts + swap inbox for everyone; pending
  // approvals only for approvers (empty otherwise).
  listOpen: async (businessId: string): Promise<OpenCoverage> => {
    const res = await axiosInstance.get(base(businessId) + "/coverage/open");
    return res.data.data;
  },

  // The caller's OWN coverage activity (claims filed + swaps requested).
  listMine: async (businessId: string): Promise<MyCoverage> => {
    const res = await axiosInstance.get(base(businessId) + "/coverage/mine");
    return res.data.data;
  },

  // Resolved coverage events, newest resolution first — the manager history feed.
  // Approver-only server-side (a non-approver gets 403, which the caller treats as
  // "stay hidden"). `limit` caps the page (server default 50).
  history: async (businessId: string, limit?: number): Promise<CoverageHistoryItem[]> => {
    const res = await axiosInstance.get(base(businessId) + "/coverage/history", {
      params: { limit: limit || undefined },
    });
    return res.data.data;
  },

  // Claim an open shift (empty body). 201 with the pending claim.
  claim: async (businessId: string, shiftId: number): Promise<OpenShiftClaim> => {
    const res = await axiosInstance.post(base(businessId) + "/shifts/" + shiftId + "/claim");
    return res.data.data;
  },

  // Request a swap or give-up of the caller's own shift. `kind` discriminates:
  // "swap" needs a coworker to accept; "giveup" goes straight to the manager.
  requestSwap: async (
    businessId: string,
    shiftId: number,
    body: RequestSwapInput,
  ): Promise<ShiftSwap> => {
    const res = await axiosInstance.post(base(businessId) + "/shifts/" + shiftId + "/swap", body);
    return res.data.data;
  },

  // Accept an open swap the caller is eligible to cover (empty body).
  acceptSwap: async (businessId: string, swapId: number): Promise<ShiftSwap> => {
    const res = await axiosInstance.post(base(businessId) + "/swaps/" + swapId + "/accept");
    return res.data.data;
  },

  // Retract the caller's OWN still-live request — a swap/giveup they offered
  // (kind swap|giveup → the swap id) or an open-shift claim they filed
  // (kind open_claim → the claim id). schedule:self; the backend enforces
  // ownership + that the request is non-terminal.
  cancel: async (
    businessId: string,
    requestId: number,
    kind: CancelKind,
  ): Promise<ShiftSwap | OpenShiftClaim> => {
    const res = await axiosInstance.post(
      base(businessId) + "/coverage/" + requestId + "/cancel",
      { kind },
    );
    return res.data.data;
  },

  // Manager decision. The `kind` discriminator routes the same URL to either the
  // swap or the open-claim state machine — `requestId` is the matching id.
  decide: async (
    businessId: string,
    requestId: number,
    body: DecisionInput,
  ): Promise<ShiftSwap | OpenShiftClaim> => {
    const res = await axiosInstance.post(
      base(businessId) + "/swaps/" + requestId + "/decision",
      body,
    );
    return res.data.data;
  },
};
