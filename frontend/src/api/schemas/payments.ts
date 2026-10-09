import { z } from 'zod';
import { logError } from '@/utils/errorLogger';

const PaymentHistoryItemSchema = z.object({
  id: z.number(),
  bill_id: z.number(),
  bill_number: z.string(),
  table_name: z.string(),
  payer_address: z.string(),
  amount: z.number(),
  tip_amount: z.number(),
  currency: z.string(),
  tx_hash: z.string(),
  status: z.enum(['pending', 'confirmed', 'failed', 'reversed', 'refunded']),
  created_at: z.string(),
  updated_at: z.string(),
}).loose();

const PaymentHistoryResponseSchema = z.array(PaymentHistoryItemSchema);

export function validatePaymentHistoryResponse<T>(data: T): T {
  const result = PaymentHistoryResponseSchema.safeParse(data);
  if (!result.success) {
    void logError(
      `Payment history response validation failed: ${result.error.message}`,
      'ApiValidation',
      'validatePaymentHistoryResponse'
    );
  }
  return data;
}
