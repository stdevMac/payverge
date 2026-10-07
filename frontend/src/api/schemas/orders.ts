import { z } from 'zod';
import { logError } from '@/utils/errorLogger';

const nullableString = z.string().nullable().optional();
const nullableTableID = z.union([z.number(), z.string()]).nullable().optional();

const OrderSchema = z.object({
  id: z.number(),
  bill_id: z.number(),
  business_id: z.number(),
  order_number: z.string(),
  status: z.enum(['pending', 'approved', 'in_kitchen', 'ready', 'delivered', 'cancelled']),
  created_by: z.string().nullable().optional(),
  approved_by: z.string().nullable().optional(),
  cancelled_by: nullableString,
  notes: z.string().nullable().optional(),
  items: z.union([z.array(z.unknown()), z.string()]),
  currency: z.string(),
  cancel_reason: nullableString,
  created_at: z.string(),
  updated_at: z.string(),
  approved_at: nullableString,
  cancelled_at: nullableString,
  bill: z.any().optional(),
  business: z.any().optional(),
  table_id: nullableTableID,
}).loose();

const OrdersResponseSchema = z.object({
  orders: z.array(OrderSchema),
  total: z.number(),
  page: z.number().optional(),
  page_size: z.number().optional(),
  total_pages: z.number().optional(),
}).loose();

export function validateOrdersResponse<T>(data: T): T {
  const result = OrdersResponseSchema.safeParse(data);
  if (!result.success) {
    void logError(
      `Orders response validation failed: ${result.error.message}`,
      'ApiValidation',
      'validateOrdersResponse'
    );
  }
  return data;
}

export function validateOrder<T>(data: T): T {
  const result = OrderSchema.safeParse(data);
  if (!result.success) {
    void logError(
      `Order response validation failed: ${result.error.message}`,
      'ApiValidation',
      'validateOrder'
    );
  }
  return data;
}
