import { axiosInstance } from "@/api/tools/instance";
import type { Dollars } from "@/types/money";

type CashRegisterSessionStatus = "open" | "closed";
type CashRegisterMovementType = "cash_sale" | "cash_refund" | "cash_in" | "cash_out";

export interface CashRegisterMovement {
  id: number;
  business_id: number;
  session_id: number;
  movement_type: CashRegisterMovementType;
  amount: Dollars;
  reason: string;
  note: string;
  alternative_payment_id: number | null;
  bill_id: number | null;
  actor_user_id: number | null;
  actor_staff_id: number | null;
  actor_label: string;
  occurred_at: string;
  created_at: string;
}

export interface CashRegisterSession {
  id: number;
  business_id: number;
  status: CashRegisterSessionStatus;
  opening_float: Dollars;
  opening_note: string;
  opened_by_user_id: number | null;
  opened_by_staff_id: number | null;
  opened_by_label: string;
  opened_at: string;
  cash_sales: Dollars;
  cash_refunds: Dollars;
  cash_in: Dollars;
  cash_out: Dollars;
  expected_cash?: Dollars;
  counted_cash?: Dollars;
  variance?: Dollars;
  closing_note: string;
  closed_by_user_id: number | null;
  closed_by_staff_id: number | null;
  closed_by_label: string;
  closed_at: string | null;
  created_at: string;
  updated_at: string;
  movements?: CashRegisterMovement[];
}

export interface CashRegisterCurrentResponse {
  session: CashRegisterSession | null;
  unassigned_cash_total: Dollars;
  unassigned_cash_count: number;
  /** Last closed session's declared starting bank. Never counted cash (#652). */
  suggested_opening_float?: Dollars | null;
  /** Open cash drawer is the house rail when card/crypto plugins are off (#769). */
  house_rail?: "cash" | null;
}

export interface CashRegisterSessionsResponse {
  sessions: CashRegisterSession[];
  total: number;
}

export interface CashRegisterUnassignedResponse {
  total: Dollars;
  count: number;
}

interface CashRegisterUnassignedItem {
  id: number;
  bill_id: number;
  bill_number: string;
  table_name: string;
  participant_name: string;
  /** Cash value of this tender, in dollars (wire contract). */
  amount: Dollars;
  status: string;
  created_at: string;
}

export interface CashRegisterUnassignedListResponse {
  items: CashRegisterUnassignedItem[];
  total: number;
}

export const cashRegisterApi = {
  getCurrent: async (businessId: string): Promise<CashRegisterCurrentResponse> => {
    const response = await axiosInstance.get("/inside/businesses/" + businessId + "/cash-register/current");
    return response.data;
  },

  openSession: async (
    businessId: string,
    payload: { opening_float: Dollars; opening_note?: string },
  ): Promise<CashRegisterSession> => {
    const response = await axiosInstance.post("/inside/businesses/" + businessId + "/cash-register/sessions", payload);
    return response.data;
  },

  listSessions: async (
    businessId: string,
    params?: { limit?: number; offset?: number },
  ): Promise<CashRegisterSessionsResponse> => {
    const response = await axiosInstance.get("/inside/businesses/" + businessId + "/cash-register/sessions", {
      params,
    });
    return response.data;
  },

  getSession: async (businessId: string, sessionId: number): Promise<CashRegisterSession> => {
    const response = await axiosInstance.get(
      "/inside/businesses/" + businessId + "/cash-register/sessions/" + sessionId,
    );
    return response.data;
  },

  createMovement: async (
    businessId: string,
    sessionId: number,
    payload: { movement_type: "cash_in" | "cash_out"; amount: Dollars; reason: string; note?: string },
  ): Promise<{ movement: CashRegisterMovement; session: CashRegisterSession }> => {
    const response = await axiosInstance.post(
      "/inside/businesses/" + businessId + "/cash-register/sessions/" + sessionId + "/movements",
      payload,
    );
    return response.data;
  },

  closeSession: async (
    businessId: string,
    sessionId: number,
    payload: { counted_cash: Dollars; closing_note?: string },
  ): Promise<CashRegisterSession> => {
    const response = await axiosInstance.post(
      "/inside/businesses/" + businessId + "/cash-register/sessions/" + sessionId + "/close",
      payload,
    );
    return response.data;
  },

  getUnassigned: async (businessId: string): Promise<CashRegisterUnassignedResponse> => {
    const response = await axiosInstance.get("/inside/businesses/" + businessId + "/cash-register/unassigned");
    return response.data;
  },

  listUnassigned: async (
    businessId: string,
    params?: { limit?: number; offset?: number },
  ): Promise<CashRegisterUnassignedListResponse> => {
    const response = await axiosInstance.get(
      "/inside/businesses/" + businessId + "/cash-register/unassigned/list",
      { params },
    );
    return response.data;
  },
};
