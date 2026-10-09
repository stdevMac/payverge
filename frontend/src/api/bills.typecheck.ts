import type { Payment } from "@/api/bills";

const validPaymentStatus: Payment["status"] = "refunded";

// @ts-expect-error Payment status must reject typos instead of accepting any string.
const invalidPaymentStatus: Payment["status"] = "refundded";

void validPaymentStatus;
void invalidPaymentStatus;
