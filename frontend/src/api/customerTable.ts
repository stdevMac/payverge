import { axiosInstance } from "@/api/tools/instance";

export interface CustomerTableCheckInResponse {
  customer_business: unknown;
  bill_attached: boolean;
  bill_id?: number | null;
}

export async function checkInCustomerToTable(
  tableCode: string,
): Promise<CustomerTableCheckInResponse> {
  const response = await axiosInstance.post<CustomerTableCheckInResponse>(
    `/customer/table/${encodeURIComponent(tableCode)}/check-in`,
    undefined,
  );
  return response.data;
}
