import { isValidOrder, isValidBill, isValidReservation } from './sseValidation';

describe('SSE event validation', () => {
  describe('isValidOrder', () => {
    it('rejects null', () => expect(isValidOrder(null)).toBe(false));
    it('rejects undefined', () => expect(isValidOrder(undefined)).toBe(false));
    it('rejects empty object', () => expect(isValidOrder({})).toBe(false));
    it('rejects object missing status', () => expect(isValidOrder({ id: 1 })).toBe(false));
    it('rejects object with string id', () => expect(isValidOrder({ id: "1", status: "pending" })).toBe(false));
    it('accepts valid order', () => expect(isValidOrder({ id: 1, status: 'pending', business_id: 1 })).toBe(true));
  });

  describe('isValidBill', () => {
    it('rejects empty object', () => expect(isValidBill({})).toBe(false));
    it('accepts valid bill', () => expect(isValidBill({ id: 1, status: 'open', total_amount: 50 })).toBe(true));
  });

  describe('isValidReservation', () => {
    it('rejects empty object', () => expect(isValidReservation({})).toBe(false));
    it('accepts valid reservation', () => expect(isValidReservation({ id: 1, customer_name: 'John' })).toBe(true));
  });
});
