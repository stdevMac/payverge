import { type LucideIcon } from "lucide-react";

export type StatusTone =
  | "success"
  | "info"
  | "pending"
  | "warning"
  | "danger"
  | "neutral";

const TONE_CLASSES: Record<StatusTone, string> = {
  success: "border-emerald-200 bg-emerald-50 text-emerald-700",
  info: "border-brand/30 bg-brand/[0.08] text-brand",
  pending: "border-amber-200 bg-amber-50 text-amber-700",
  warning: "border-amber-200 bg-amber-50 text-amber-800",
  danger: "border-rose-200 bg-rose-50 text-rose-700",
  neutral: "border-warm-200 bg-warm-100 text-ink-700",
};

interface StatusBadgeProps {
  tone: StatusTone;
  label: string;
  Icon?: LucideIcon;
  size?: "sm" | "md";
}

export default function StatusBadge({
  tone,
  label,
  Icon,
  size = "md",
}: StatusBadgeProps) {
  const sizeClasses =
    size === "sm" ? "px-2 py-0.5 text-[10px]" : "px-2.5 py-1 text-xs";
  return (
    <span
      className={`inline-flex items-center gap-1.5 rounded-full border font-medium ${sizeClasses} ${TONE_CLASSES[tone]}`}
    >
      {Icon ? (
        <Icon className={size === "sm" ? "h-2.5 w-2.5" : "h-3 w-3"} aria-hidden />
      ) : null}
      {label}
    </span>
  );
}
