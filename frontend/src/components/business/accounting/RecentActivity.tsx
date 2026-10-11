import {
  AlertTriangle,
  CheckCircle2,
  Clock3,
  Coins,
  CreditCard,
  ReceiptText,
  Users,
  type LucideIcon,
} from "lucide-react";

type ActivityKind =
  | "entry_created"
  | "entry_voided"
  | "payroll_created"
  | "payroll_paid"
  | "payroll_voided"
  | "invoice_authorized"
  | "invoice_pending"
  | "invoice_failed";

export interface ActivityItem {
  id: string;
  kind: ActivityKind;
  title: string;
  detail?: string;
  occurredAt: string;
}

const KIND_META: Record<
  ActivityKind,
  { Icon: LucideIcon; iconClass: string }
> = {
  entry_created: { Icon: Coins, iconClass: "text-emerald-600 bg-emerald-50" },
  entry_voided: { Icon: AlertTriangle, iconClass: "text-amber-700 bg-amber-50" },
  payroll_created: { Icon: Users, iconClass: "text-ink-700 bg-warm-100" },
  payroll_paid: { Icon: CheckCircle2, iconClass: "text-emerald-600 bg-emerald-50" },
  payroll_voided: { Icon: AlertTriangle, iconClass: "text-amber-700 bg-amber-50" },
  invoice_authorized: {
    Icon: ReceiptText,
    iconClass: "text-emerald-600 bg-emerald-50",
  },
  invoice_pending: { Icon: Clock3, iconClass: "text-amber-700 bg-amber-50" },
  invoice_failed: { Icon: AlertTriangle, iconClass: "text-rose-600 bg-rose-50" },
};

interface RecentActivityProps {
  items: ActivityItem[];
  emptyLabel: string;
  title: string;
  Icon?: LucideIcon;
  formatOccurredAt: (iso: string) => string;
}

export default function RecentActivity({
  items,
  emptyLabel,
  title,
  Icon = CreditCard,
  formatOccurredAt,
}: RecentActivityProps) {
  return (
    <div className="rounded-3xl border border-warm-200 bg-white p-4 shadow-sm shadow-warm-900/5">
      <div className="flex items-center gap-2 text-sm font-semibold text-ink-950">
        <Icon className="h-4 w-4 text-ink-500" aria-hidden />
        {title}
      </div>
      {items.length === 0 ? (
        <p className="mt-3 text-sm text-ink-500">{emptyLabel}</p>
      ) : (
        <ul className="mt-3 space-y-3">
          {items.map((item) => {
            const meta = KIND_META[item.kind];
            const ItemIcon = meta.Icon;
            return (
              <li key={item.id} className="flex items-start gap-3 text-sm">
                <span
                  className={`mt-0.5 flex h-7 w-7 flex-shrink-0 items-center justify-center rounded-full ${meta.iconClass}`}
                >
                  <ItemIcon className="h-3.5 w-3.5" aria-hidden />
                </span>
                <div className="min-w-0 flex-1">
                  <p className="font-medium text-ink-900">{item.title}</p>
                  {item.detail ? (
                    <p className="truncate text-xs text-ink-500">
                      {item.detail}
                    </p>
                  ) : null}
                  <p className="text-xs text-ink-500">
                    {formatOccurredAt(item.occurredAt)}
                  </p>
                </div>
              </li>
            );
          })}
        </ul>
      )}
    </div>
  );
}
