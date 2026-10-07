import { z } from 'zod';
import { logError } from '@/utils/errorLogger';

const BusinessAddressSchema = z.object({
  street: z.string(),
  city: z.string(),
  state: z.string(),
  postal_code: z.string(),
  country: z.string(),
}).loose();

const BusinessSchema = z.object({
  id: z.number(),
  business_id: z.string().optional(),
  owner_address: z.string(),
  owner_name: z.string().optional(),
  name: z.string(),
  logo: z.string(),
  address: BusinessAddressSchema,
  settlement_address: z.string(),
  tipping_address: z.string(),
  tax_rate: z.number(),
  service_fee_rate: z.number(),
  tax_inclusive: z.boolean(),
  service_inclusive: z.boolean(),
  is_active: z.boolean(),
  closed_at: z.string().nullish(),
  created_at: z.string(),
  updated_at: z.string(),
}).loose();

export function validateBusiness<T>(data: T): T {
  const result = BusinessSchema.safeParse(data);
  if (!result.success) {
    void logError(
      `Business response validation failed: ${result.error.message}`,
      'ApiValidation',
      'validateBusiness'
    );
  }
  return data;
}

export function validateBusinessList<T>(data: T): T {
  const result = z.array(BusinessSchema).safeParse(data);
  if (!result.success) {
    void logError(
      `Business list response validation failed: ${result.error.message}`,
      'ApiValidation',
      'validateBusinessList'
    );
  }
  return data;
}
