import type { CashRegisterMovement, CashRegisterSession } from "@/api/cashRegister";

export type ZReportMoneyFormatter = (value: number | null | undefined) => string;

export type ZReportLabels = {
  title: string;
  session: string;
  status: string;
  opened: string;
  closed: string;
  closedBy: string;
  openingFloat: string;
  cashSales: string;
  cashRefunds: string;
  cashIn: string;
  cashOut: string;
  expectedCash: string;
  countedCash: string;
  variance: string;
  movements: string;
  type: string;
  amount: string;
  reason: string;
  time: string;
  generatedAt: string;
};

function movementTypeLabel(
  labels: Record<string, string>,
  movementType: CashRegisterMovement["movement_type"],
) {
  return labels[movementType] ?? movementType;
}

/** Plain-text Z-report for download / clipboard (dinner QA #127). */
export function buildCashRegisterZReportText(params: {
  session: CashRegisterSession;
  labels: ZReportLabels;
  movementTypeLabels: Record<string, string>;
  formatMoney: ZReportMoneyFormatter;
  formatDateTime: (value: string | null) => string;
  generatedAt: string;
}): string {
  const { session, labels, movementTypeLabels, formatMoney, formatDateTime, generatedAt } =
    params;
  const lines = [
    labels.title,
    `${labels.session}: #${session.id}`,
    `${labels.status}: ${session.status}`,
    `${labels.opened}: ${formatDateTime(session.opened_at)}`,
    `${labels.closed}: ${formatDateTime(session.closed_at)}`,
    `${labels.closedBy}: ${session.closed_by_label || "—"}`,
    "",
    `${labels.openingFloat}: ${formatMoney(session.opening_float)}`,
    `${labels.cashSales}: ${formatMoney(session.cash_sales)}`,
    `${labels.cashRefunds}: ${formatMoney(session.cash_refunds)}`,
    `${labels.cashIn}: ${formatMoney(session.cash_in)}`,
    `${labels.cashOut}: ${formatMoney(session.cash_out)}`,
    `${labels.expectedCash}: ${formatMoney(session.expected_cash)}`,
    `${labels.countedCash}: ${formatMoney(session.counted_cash)}`,
    `${labels.variance}: ${formatMoney(session.variance)}`,
    "",
    labels.movements,
  ];

  const movements = [...(session.movements ?? [])].sort((a, b) =>
    b.occurred_at.localeCompare(a.occurred_at),
  );
  if (movements.length === 0) {
    lines.push("—");
  } else {
    for (const movement of movements) {
      lines.push(
        [
          movementTypeLabel(movementTypeLabels, movement.movement_type),
          formatMoney(movement.amount),
          movement.reason || "—",
          formatDateTime(movement.occurred_at),
        ].join(" · "),
      );
    }
  }

  lines.push("", `${labels.generatedAt}: ${generatedAt}`);
  return lines.join("\n");
}

/** Minimal printable HTML Z-report opened in a new window. */
export function buildCashRegisterZReportHTML(params: {
  session: CashRegisterSession;
  labels: ZReportLabels;
  movementTypeLabels: Record<string, string>;
  formatMoney: ZReportMoneyFormatter;
  formatDateTime: (value: string | null) => string;
  generatedAt: string;
}): string {
  const text = buildCashRegisterZReportText(params)
    .split("\n")
    .map((line) => `<div>${escapeHtml(line) || "&nbsp;"}</div>`)
    .join("");
  return `<!doctype html><html><head><meta charset="utf-8"><title>${escapeHtml(
    params.labels.title,
  )}</title>
<style>
  body { font-family: ui-monospace, SFMono-Regular, Menlo, monospace; font-size: 12px; padding: 16px; color: #1c1917; }
  div { white-space: pre-wrap; }
</style></head><body>${text}</body></html>`;
}

function escapeHtml(value: string): string {
  return value
    .replace(/&/g, "&amp;")
    .replace(/</g, "&lt;")
    .replace(/>/g, "&gt;")
    .replace(/"/g, "&quot;");
}

export function downloadTextFile(filename: string, contents: string) {
  const blob = new Blob([contents], { type: "text/plain;charset=utf-8" });
  const url = URL.createObjectURL(blob);
  const anchor = document.createElement("a");
  anchor.href = url;
  anchor.download = filename;
  anchor.click();
  URL.revokeObjectURL(url);
}
