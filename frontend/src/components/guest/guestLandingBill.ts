export type GuestLandingBillInput = {
  total_amount: number;
  paid_amount?: number;
  status?: string;
};

export function guestLandingBillPresentation(bill: GuestLandingBillInput): {
  amount: number;
  statusKey: string;
} {
  const paid = Number(bill.paid_amount ?? 0);
  const total = Number(bill.total_amount ?? 0);
  const remaining = Math.max(total - paid, 0);
  if (bill.status === "partial" || (paid > 0 && remaining > 0)) {
    return { amount: remaining, statusKey: "partial" };
  }
  if (bill.status === "paid" || remaining <= 0) {
    return { amount: 0, statusKey: "paid" };
  }
  return { amount: total, statusKey: bill.status || "open" };
}
