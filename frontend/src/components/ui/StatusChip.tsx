"use client";

/**
 * Shared StatusChip primitive.
 *
 * Round 4 audit finding (operations): status across bills, kitchen, tables,
 * and reservations was encoded ONLY in chip color, and the color meanings were
 * inverted per screen:
 *
 *   bills:         success = "open"
 *   kitchen:       primary = "in_kitchen"
 *   tables:        success = "available"
 *   reservations:  success = "confirmed"
 *
 * An operator scanning four panels at once could not rely on color alone to
 * understand a row's state. This primitive fixes that by rendering:
 *
 *   [semantic icon] [human label]   in a tone-colored pill
 *
 * The label is always visible, so color becomes a reinforcing signal rather
 * than the sole signal. Tones map to the app palette (emerald / amber /
 * rose / brand / warm-neutral).
 */

import React from "react";
import {
  CircleDollarSign,
  CheckCircle2,
  XCircle,
  Clock,
  ChefHat,
  PackageCheck,
  CircleCheck,
  AlertCircle,
  Calendar,
  UserCheck,
  UserX,
  CheckCheck,
  Ban,
  Hourglass,
  Bed,
  type LucideIcon,
} from "lucide-react";

type StatusKind = "bill" | "order" | "table" | "reservation";

export type StatusTone = "success" | "warn" | "danger" | "info" | "neutral";

interface StatusEntry {
  icon: LucideIcon;
  label: string;
  tone: StatusTone;
}

// ---------------------------------------------------------------------------
// Kind -> status -> (icon, label, tone) lookups
// ---------------------------------------------------------------------------

const BILL_STATUSES: Record<string, StatusEntry> = {
  open: { icon: CircleDollarSign, label: "Open", tone: "info" },
  partial: { icon: Clock, label: "Partial", tone: "warn" },
  paid: { icon: CheckCircle2, label: "Paid", tone: "success" },
  closed: { icon: XCircle, label: "Closed", tone: "neutral" },
  // IMP-15: a voided bill is reversed money — it must read as danger (rose),
  // not the neutral warm-gray of the UNKNOWN fallback. Restores the danger
  // signal the deleted getStatusColor applied before bills moved to StatusChip.
  voided: { icon: Ban, label: "Voided", tone: "danger" },
};

const ORDER_STATUSES: Record<string, StatusEntry> = {
  pending: { icon: Hourglass, label: "Pending", tone: "warn" },
  approved: { icon: Clock, label: "Approved", tone: "info" },
  in_kitchen: { icon: ChefHat, label: "In Kitchen", tone: "warn" },
  ready: { icon: PackageCheck, label: "Ready", tone: "success" },
  delivered: { icon: CheckCheck, label: "Delivered", tone: "neutral" },
  cancelled: { icon: Ban, label: "Cancelled", tone: "danger" },
};

const TABLE_STATUSES: Record<string, StatusEntry> = {
  available: { icon: CircleCheck, label: "Available", tone: "success" },
  occupied: { icon: AlertCircle, label: "Occupied", tone: "danger" },
  reserved: { icon: Clock, label: "Reserved", tone: "warn" },
};

const RESERVATION_STATUSES: Record<string, StatusEntry> = {
  pending: { icon: Hourglass, label: "Pending", tone: "warn" },
  confirmed: { icon: CheckCircle2, label: "Confirmed", tone: "success" },
  late: { icon: AlertCircle, label: "Late", tone: "warn" },
  waitlist: { icon: Calendar, label: "Waitlist", tone: "info" },
  seated: { icon: UserCheck, label: "Seated", tone: "info" },
  completed: { icon: CheckCheck, label: "Completed", tone: "neutral" },
  cancelled: { icon: UserX, label: "Cancelled", tone: "danger" },
  no_show: { icon: Bed, label: "No Show", tone: "danger" },
};

const KIND_LOOKUP: Record<StatusKind, Record<string, StatusEntry>> = {
  bill: BILL_STATUSES,
  order: ORDER_STATUSES,
  table: TABLE_STATUSES,
  reservation: RESERVATION_STATUSES,
};

// ---------------------------------------------------------------------------
// Tone -> Tailwind class mapping
// Tone classes are fixed literals so Tailwind's JIT picks them up.
// ---------------------------------------------------------------------------

const TONE_CLASSES: Record<StatusTone, string> = {
  success: "bg-emerald-50 text-emerald-800 border-emerald-200",
  warn: "bg-amber-50 text-amber-800 border-amber-200",
  danger: "bg-rose-50 text-rose-800 border-rose-200",
  info: "bg-brand/10 text-brand-dark border-brand/20",
  neutral: "bg-warm-100 text-ink-700 border-ink-200",
};

// ---------------------------------------------------------------------------
// Component
// ---------------------------------------------------------------------------

/**
 * Domain path: resolve tone/icon/label from the kind + raw status lookup.
 * This is the original, and still the primary, API.
 */
interface StatusByKindProps {
  /** Which lookup table to use. */
  kind: StatusKind;
  /** The raw status string from the domain object. Unknown values fall back to neutral. */
  status: string;
  /**
   * Optional pre-translated label. If provided, overrides the English
   * internal default — callers that already have an i18n helper should pass
   * the translated string here so status remains readable in non-English
   * locales.
   */
  labelOverride?: string;
  /** Extra classes appended after the tone classes. */
  className?: string;
  // Discriminant guards: the generic-path props are forbidden here so the two
  // shapes never mix. TS narrows on the presence of `tone`.
  tone?: never;
  label?: never;
}

/**
 * Generic escape hatch: caller supplies a tone + label directly, bypassing the
 * kind/status lookup. For statuses that don't belong to a domain kind — e.g. a
 * one-off "Beta" / "Active" badge — where the caller owns the wording and
 * just wants the shared pill styling and palette.
 *
 * Icon-less by design: the generic tone path renders label-only (no icon slot).
 * A chip carries exactly ONE encoding — tonal color + label — so the two chip
 * shapes stay legible and symmetric (the domain path's icon is a fixed,
 * lookup-owned semantic, not a caller affordance). Consumers that need a richer
 * affordance (e.g. surfacing an error's detail on hover) wrap the chip
 * externally rather than smuggling an icon/element into the label, as
 * PluginManager does with a Tooltip around its error StatusChip.
 */
interface StatusByToneProps {
  /** Semantic tone that selects the pill palette. */
  tone: StatusTone;
  /** The (already-localized) text to render. */
  label: string;
  /** Extra classes appended after the tone classes. */
  className?: string;
  kind?: never;
  status?: never;
  labelOverride?: never;
}

export type StatusChipProps = StatusByKindProps | StatusByToneProps;

const UNKNOWN_ENTRY: StatusEntry = {
  icon: AlertCircle,
  label: "Unknown",
  tone: "neutral",
};

const PILL_CLASSES =
  "inline-flex items-center gap-1.5 rounded-full border px-2.5 py-0.5 text-xs font-medium";

export function StatusChip(props: StatusChipProps) {
  const className = props.className ?? "";

  // Generic tone/label path: no domain lookup, no semantic icon — just the
  // label in a tone-colored pill.
  if (props.tone !== undefined) {
    const toneClasses = TONE_CLASSES[props.tone];
    return (
      <span
        className={`${PILL_CLASSES} ${toneClasses} ${className}`.trim()}
        data-status-tone={props.tone}
      >
        {props.label}
      </span>
    );
  }

  const { kind, status, labelOverride } = props;
  const table = KIND_LOOKUP[kind];
  const entry = (table && table[status]) ?? UNKNOWN_ENTRY;
  const Icon = entry.icon;
  const label = labelOverride ?? entry.label;
  const toneClasses = TONE_CLASSES[entry.tone];

  return (
    <span
      className={`${PILL_CLASSES} ${toneClasses} ${className}`.trim()}
      data-status={status}
      data-status-kind={kind}
      data-status-tone={entry.tone}
    >
      <Icon className="w-3.5 h-3.5" aria-hidden />
      {label}
    </span>
  );
}
