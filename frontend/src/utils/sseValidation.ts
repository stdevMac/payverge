/**
 * Type guard functions for SSE event data validation.
 * Prevents unsafe `as unknown as T` casts from causing crashes when the
 * backend sends malformed or unexpected JSON payloads.
 */

export function isValidOrder(data: unknown): data is { id: number; status: string; business_id: number } {
  return (
    typeof data === 'object' &&
    data !== null &&
    'id' in data &&
    typeof (data as Record<string, unknown>).id === 'number' &&
    'status' in data &&
    typeof (data as Record<string, unknown>).status === 'string'
  );
}

export function isValidBill(data: unknown): data is { id: number; status: string; total_amount: number } {
  return (
    typeof data === 'object' &&
    data !== null &&
    'id' in data &&
    typeof (data as Record<string, unknown>).id === 'number' &&
    'status' in data &&
    typeof (data as Record<string, unknown>).status === 'string'
  );
}

export function isValidReservation(data: unknown): data is { id: number; customer_name: string } {
  return (
    typeof data === 'object' &&
    data !== null &&
    'id' in data &&
    typeof (data as Record<string, unknown>).id === 'number'
  );
}
