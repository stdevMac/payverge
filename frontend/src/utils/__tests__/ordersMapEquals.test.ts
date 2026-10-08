import { ordersMapEquals } from "@/utils/ordersMapEquals";
import type { Order } from "@/api/orders";

const order = (
  id: number,
  status: Order["status"],
  updated_at: string,
): Order =>
  ({
    id,
    bill_id: 10,
    business_id: 1,
    order_number: `K-${id}`,
    status,
    items: "[]",
    currency: "USD",
    created_at: "2026-06-12T10:00:00Z",
    updated_at,
  }) as Order;

describe("ordersMapEquals (K-1)", () => {
  it("treats identical id+status+updated_at content as equal across fresh references", () => {
    const a = { 10: [order(1, "approved", "t1")] };
    const b = { 10: [order(1, "approved", "t1")] };
    expect(a).not.toBe(b);
    expect(ordersMapEquals(a, b)).toBe(true);
  });

  it("detects a status change", () => {
    expect(
      ordersMapEquals(
        { 10: [order(1, "approved", "t1")] },
        { 10: [order(1, "in_kitchen", "t1")] },
      ),
    ).toBe(false);
  });

  it("detects updated_at drift, added orders, and removed bills", () => {
    expect(
      ordersMapEquals(
        { 10: [order(1, "approved", "t1")] },
        { 10: [order(1, "approved", "t2")] },
      ),
    ).toBe(false);
    expect(
      ordersMapEquals(
        { 10: [order(1, "approved", "t1")] },
        { 10: [order(1, "approved", "t1"), order(2, "pending", "t1")] },
      ),
    ).toBe(false);
    expect(ordersMapEquals({ 10: [order(1, "approved", "t1")] }, {})).toBe(
      false,
    );
  });

  it("two empty maps are equal", () => {
    expect(ordersMapEquals({}, {})).toBe(true);
  });
});
