import { axiosInstance } from "@/api/tools/instance";
import type { Dollars } from "@/types/money";

export type EmploymentType = "" | "hourly" | "salaried";

export interface StaffCompensation {
  employment_type: EmploymentType;
  hourly_rate: Dollars;
  annual_salary: Dollars;
}

export const staffCompensationApi = {
  getCompensation: async (
    businessId: string,
    staffId: string,
  ): Promise<StaffCompensation> => {
    const response = await axiosInstance.get(
      "/inside/businesses/" + businessId + "/staff/" + staffId + "/compensation",
    );
    return response.data.data;
  },

  updateCompensation: async (
    businessId: string,
    staffId: string,
    payload: StaffCompensation,
  ): Promise<StaffCompensation> => {
    const response = await axiosInstance.put(
      "/inside/businesses/" + businessId + "/staff/" + staffId + "/compensation",
      payload,
    );
    return response.data.data;
  },
};
