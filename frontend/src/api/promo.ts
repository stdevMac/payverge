import { axiosInstance } from "./tools/instance";

export interface PromoOffer {
  id: number;
  name: string;
  code?: string;
  discount_type: "percentage" | "fixed";
  discount_value: number;
  applicable_to: string;
  target_id: string | null;
}

export async function validatePromoCode(
  tableCode: string,
  code: string,
): Promise<{ offer: PromoOffer }> {
  const response = await axiosInstance.post(
    `/guest/table/${tableCode}/validate-promo`,
    { code },
  );
  return response.data;
}
