import { axiosInstance } from "@/api/tools/instance";
import { asDollars, type Dollars } from "@/types/money";

export interface Position {
  id: number;
  business_id: number;
  name: string;
  color_hex: string;
  department: string;
  is_active: boolean;
  sort_order: number;
}

export interface PositionInput {
  name: string;
  color_hex?: string;
  department?: string;
  sort_order?: number;
}

export interface StaffPositionLink {
  id: number;
  business_id: number;
  staff_id: number;
  position_id: number;
  is_primary: boolean;
}

const base = (businessId: string) => "/inside/businesses/" + businessId;

export const positionsApi = {
  list: async (businessId: string): Promise<Position[]> => {
    const res = await axiosInstance.get(base(businessId) + "/positions");
    return res.data.data;
  },

  create: async (businessId: string, input: PositionInput): Promise<Position> => {
    const res = await axiosInstance.post(base(businessId) + "/positions", input);
    return res.data.data;
  },

  update: async (businessId: string, positionId: number, input: PositionInput): Promise<void> => {
    await axiosInstance.patch(base(businessId) + "/positions/" + positionId, input);
  },

  remove: async (businessId: string, positionId: number): Promise<void> => {
    await axiosInstance.delete(base(businessId) + "/positions/" + positionId);
  },

  listForStaff: async (businessId: string, staffId: number): Promise<StaffPositionLink[]> => {
    const res = await axiosInstance.get(base(businessId) + "/staff/" + staffId + "/positions");
    return res.data.data;
  },

  assign: async (
    businessId: string,
    staffId: number,
    positionId: number,
    isPrimary: boolean,
  ): Promise<void> => {
    await axiosInstance.post(base(businessId) + "/staff/" + staffId + "/positions", {
      position_id: positionId,
      is_primary: isPrimary,
    });
  },

  unassign: async (businessId: string, staffId: number, positionId: number): Promise<void> => {
    await axiosInstance.delete(
      base(businessId) + "/staff/" + staffId + "/positions/" + positionId,
    );
  },

  // Owner-only (server returns 403 for non-owners). Pay rate is in dollars.
  getRate: async (businessId: string, staffId: number, positionId: number): Promise<Dollars> => {
    const res = await axiosInstance.get(
      base(businessId) + "/staff/" + staffId + "/positions/" + positionId + "/rate",
    );
    return asDollars(res.data.data.pay_rate);
  },

  setRate: async (
    businessId: string,
    staffId: number,
    positionId: number,
    payRate: Dollars,
  ): Promise<void> => {
    await axiosInstance.put(
      base(businessId) + "/staff/" + staffId + "/positions/" + positionId + "/rate",
      { pay_rate: payRate },
    );
  },
};
