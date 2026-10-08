import { asDollars } from "@/types/money";

import {
  buildCashRegisterZReportText,
  type ZReportLabels,
} from "./cashRegisterZReport";

const labels: ZReportLabels = {
  title: "Z-Report",
  session: "Session",
  status: "Status",
  opened: "Opened",
  closed: "Closed",
  closedBy: "Closed by",
  openingFloat: "Opening float",
  cashSales: "Cash sales",
  cashRefunds: "Cash refunds",
  cashIn: "Cash in",
  cashOut: "Cash out",
  expectedCash: "Expected cash",
  countedCash: "Counted cash",
  variance: "Variance",
  movements: "Movements",
  type: "Type",
  amount: "Amount",
  reason: "Reason",
  time: "Time",
  generatedAt: "Generated",
};

describe("cashRegisterZReport", () => {
  it("includes session totals and movements for export", () => {
    const text = buildCashRegisterZReportText({
      session: {
        id: 42,
        business_id: 1,
        status: "closed",
        opening_float: asDollars(100),
        opening_note: "",
        opened_by_user_id: null,
        opened_by_staff_id: null,
        opened_by_label: "Owner",
        opened_at: "2026-06-27T10:00:00Z",
        cash_sales: asDollars(50),
        cash_refunds: asDollars(0),
        cash_in: asDollars(10),
        cash_out: asDollars(5),
        expected_cash: asDollars(155),
        counted_cash: asDollars(150),
        variance: asDollars(-5),
        closing_note: "",
        closed_by_user_id: null,
        closed_by_staff_id: null,
        closed_by_label: "Owner",
        closed_at: "2026-06-27T20:00:00Z",
        created_at: "2026-06-27T10:00:00Z",
        updated_at: "2026-06-27T20:00:00Z",
        movements: [
          {
            id: 1,
            business_id: 1,
            session_id: 42,
            movement_type: "cash_sale",
            amount: asDollars(50),
            reason: "Table 4",
            note: "",
            alternative_payment_id: null,
            bill_id: 9,
            actor_user_id: null,
            actor_staff_id: null,
            actor_label: "Owner",
            occurred_at: "2026-06-27T12:00:00Z",
            created_at: "2026-06-27T12:00:00Z",
          },
        ],
      },
      labels,
      movementTypeLabels: { cash_sale: "Cash sale" },
      formatMoney: (value) => `$${Number(value ?? 0).toFixed(2)}`,
      formatDateTime: (value) => value ?? "-",
      generatedAt: "2026-06-27T21:00:00Z",
    });

    expect(text).toContain("Z-Report");
    expect(text).toContain("Session: #42");
    expect(text).toContain("Counted cash: $150.00");
    expect(text).toContain("Cash sale · $50.00 · Table 4");
  });
});
