"use client";

import React from "react";
import { Button, Chip } from "@nextui-org/react";
import { AlertTriangle, ArrowRight, Check, Sparkles, Undo2 } from "lucide-react";

import {
  DirectorProposedAction,
  applyDirectorAction,
  directorActionErrorInfo,
  undoDirectorAction,
} from "@/api/directorConsole";
import {
  formatProposalExpiry,
  mapUndoErrorKey,
  UNDO_WINDOW_HOURS,
} from "./proposalActions";
import { countKey } from "@/i18n/countForm";
import {
  isEnglishLocale,
  localizedProposalDescription,
  localizedProposalSummary,
  localizedProposalWarning,
  localizedProposalTitle,
} from "./proposalCopy";

type Translator = (
  key: string,
  params?: Record<string, string | number>,
) => string;

interface ProposalCardProps {
  proposal: DirectorProposedAction;
  businessId: number;
  t: Translator;
  /** Operator locale for L4-18 presentation strings. */
  locale?: string;
  /** Parent removes the card from its list (client-side only; proposals expire server-side). */
  onDismiss: (proposalId: string) => void;
  /** Optional analytics seam, fired once on successful apply. */
  onApplied?: (proposalId: string) => void;
  /** Wave 4: navigate to the Menu tab after a menu.* proposal is applied. */
  onViewInMenu?: () => void;
}

/**
 * Card lifecycle. `confirm` is the armed second-press state for
 * `requires_reconfirm` proposals (spec §5.7: "a typed/secondary confirm").
 * `terminal` means the proposal can no longer be applied (409 menu drift /
 * 410 expired) and only the explanation + dismiss remain.
 */
type Phase =
  | "idle"
  | "confirm"
  | "applying"
  | "applied"
  | "undoing"
  | "undone"
  | "terminal";

/**
 * Render one side of a before→after pair. Server-computed values are
 * locale-neutral (spec §6.2): numbers pass through as-is, booleans map to the
 * availability words, objects fall back to compact JSON.
 */
function formatPreviewValue(value: unknown, t: Translator): string {
  if (typeof value === "boolean") {
    return value ? t("proposal.valueAvailable") : t("proposal.valueUnavailable");
  }
  if (value === null || value === undefined) return "—";
  if (typeof value === "object") return JSON.stringify(value);
  return String(value);
}

export default function ProposalCard({
  proposal,
  businessId,
  t,
  locale = "en",
  onDismiss,
  onApplied,
  onViewInMenu,
}: ProposalCardProps) {
  const [phase, setPhase] = React.useState<Phase>("idle");
  const [notice, setNotice] = React.useState<string | null>(null);
  // Undo can become unavailable (menu drifted / window elapsed) but the
  // affordance never vanishes — it stays visible and disabled so the operator
  // can see undo existed and read why it's gone (audit L4-17).
  const [undoDisabled, setUndoDisabled] = React.useState(false);
  const isMenuProposal = proposal.kind.startsWith("menu.");
  // A proposal whose preview matches zero items can only fail to apply
  // (409 no_items_match) — never offer Apply for it (audit L4-17).
  const zeroMatch = proposal.preview?.affected_count === 0;

  const handleApply = async () => {
    if (proposal.requires_reconfirm && phase === "idle") {
      setPhase("confirm");
      setNotice(t("proposal.reconfirmHint"));
      return;
    }
    const reconfirm = phase === "confirm";
    setPhase("applying");
    setNotice(null);
    try {
      await applyDirectorAction(businessId, proposal.id, reconfirm);
      // The footer's applied/undone status line communicates success;
      // `notice` is reserved for hints and errors.
      setPhase("applied");
      onApplied?.(proposal.id);
    } catch (err) {
      const { status, code } = directorActionErrorInfo(err);
      if (status === 409 && code === "no_items_match") {
        // Nothing matches — nothing changed, so "menu changed" would be a lie.
        setPhase("terminal");
        setNotice(t("proposal.errors.noItemsMatch"));
      } else if (status === 409) {
        // Menu changed since preview — unrecoverable; the operator re-asks.
        setPhase("terminal");
        setNotice(t("proposal.errors.menuChanged"));
      } else if (status === 428 || code === "reconfirm_required") {
        setPhase("confirm");
        setNotice(t("proposal.reconfirmHint"));
      } else if (status === 410 || status === 404) {
        setPhase("terminal");
        setNotice(t("proposal.errors.gone"));
      } else {
        setPhase(reconfirm ? "confirm" : "idle");
        setNotice(t("proposal.errors.applyFailed"));
      }
    }
  };

  const handleUndo = async () => {
    setPhase("undoing");
    setNotice(null);
    try {
      await undoDirectorAction(businessId, proposal.id);
      setPhase("undone");
    } catch (err) {
      const info = directorActionErrorInfo(err);
      const key = mapUndoErrorKey(info);
      if (info.code === "already_undone") {
        setPhase("undone");
        setNotice(t(key));
      } else if (info.status === 409 || info.status === 410) {
        // Menu drifted / window elapsed — keep applied; disable undo (L4-19).
        setPhase("applied");
        setUndoDisabled(true);
        setNotice(t(key, { hours: UNDO_WINDOW_HOURS }));
      } else {
        setPhase("applied");
        setNotice(t(key, { hours: UNDO_WINDOW_HOURS }));
      }
    }
  };

  // The countdown must keep moving while the card is on screen. Capturing
  // Date.now() once froze the label at whatever it read on mount ("Expires in
  // 58 min" forever) and it never flipped to expired, so an operator could
  // apply a proposal the server had already let lapse. Tick once a minute —
  // the smallest unit the copy renders — and stop at expiry.
  const [nowMs, setNowMs] = React.useState(() => Date.now());
  const expiresAtMs = React.useMemo(() => {
    if (!proposal.expires_at) return null;
    const parsed = Date.parse(proposal.expires_at);
    return Number.isNaN(parsed) ? null : parsed;
  }, [proposal.expires_at]);

  React.useEffect(() => {
    if (expiresAtMs === null) return;
    if (Date.now() >= expiresAtMs) return;
    const id = setInterval(() => {
      const now = Date.now();
      setNowMs(now);
      if (now >= expiresAtMs) clearInterval(id);
    }, 60_000);
    return () => clearInterval(id);
  }, [expiresAtMs]);

  const expiryLine = React.useMemo(
    () => formatProposalExpiry(proposal.expires_at, nowMs, t),
    [proposal.expires_at, nowMs, t],
  );

  const examples = proposal.preview?.examples || [];
  const warnings = proposal.warnings || [];
  const applied = phase === "applied" || phase === "undoing" || phase === "undone";
  const busy = phase === "applying" || phase === "undoing";

  return (
    <div
      data-testid="dc-proposal-card"
      className="rounded-xl border border-brand/25 bg-brand/[0.03] px-4 py-3 space-y-3"
    >
      {/* EU AI Act posture (§6.3): the change is AI-proposed, a human applies it. */}
      <div
        data-testid="dc-proposal-disclosure"
        className="flex items-center gap-1.5 text-label text-ink-500"
      >
        <Sparkles className="w-3.5 h-3.5 text-brand" strokeWidth={1.75} />
        <span>{t("proposal.disclosure")}</span>
        <Chip size="sm" variant="flat" className="ml-auto">
          {/* Proposal kinds are dotted (e.g. "menu.adjust_prices"), but the
              operator translation lookup splits keys on ".", so a dotted key
              never resolved — it fell to English in BOTH locales and spammed the
              missing-translation reporter every render. Map dots to underscores
              to match the flattened bundle keys. (R3-AI-6) */}
          {t(`proposal.kinds.${proposal.kind.replace(/\./g, "_")}`)}
        </Chip>
      </div>

      <div>
        <div className="text-body-sm font-semibold text-ink-900">
          {localizedProposalTitle(proposal, locale, t)}
        </div>
        {proposal.description || !isEnglishLocale(locale) ? (
          <p className="text-label text-ink-700 mt-0.5">
            {localizedProposalDescription(proposal, locale, t)}
          </p>
        ) : null}
      </div>

      {proposal.preview ? (
        <div className="space-y-1.5">
          <div className="text-label text-ink-600">
            {localizedProposalSummary(proposal, locale, t)}
            {proposal.preview.affected_count > 0 ? (
              <span className="text-ink-500">
                {" · "}
                {t(
                  countKey(
                    "proposal.affectedCount",
                    proposal.preview.affected_count,
                  ),
                  {
                    count: proposal.preview.affected_count,
                  },
                )}
              </span>
            ) : null}
          </div>
          {examples.length > 0 ? (
            <ul className="space-y-1">
              {examples.map((example, idx) => (
                <li
                  key={`${proposal.id}-ex-${idx}`}
                  data-testid="dc-proposal-example"
                  className="flex items-center gap-2 rounded-lg border border-warm-200 bg-white px-3 py-1.5 text-body-sm text-ink-800"
                >
                  <span className="font-medium truncate">{example.name}</span>
                  <span className="ml-auto text-ink-500 whitespace-nowrap">
                    {formatPreviewValue(example.before, t)}
                  </span>
                  <ArrowRight className="w-3.5 h-3.5 text-ink-400 flex-none" />
                  <span className="text-ink-900 font-medium whitespace-nowrap">
                    {formatPreviewValue(example.after, t)}
                  </span>
                </li>
              ))}
            </ul>
          ) : null}
        </div>
      ) : null}

      {warnings.length > 0 ? (
        <div className="rounded-lg border border-amber-200 bg-amber-50 px-3 py-2">
          <div className="flex items-center gap-1.5 text-label font-semibold text-amber-800">
            <AlertTriangle className="w-3.5 h-3.5" />
            {t("proposal.warningsTitle")}
          </div>
          <ul className="mt-1 list-disc pl-5 text-label text-amber-800 space-y-0.5">
            {warnings.map((warning, idx) => (
              <li key={`${proposal.id}-warn-${idx}`}>
                {localizedProposalWarning(warning, locale, t)}
              </li>
            ))}
          </ul>
        </div>
      ) : null}

      {zeroMatch ? (
        <p data-testid="dc-proposal-nomatch" className="text-label text-ink-600">
          {t("proposal.noMatchNotice")}
        </p>
      ) : null}

      {notice ? (
        <p data-testid="dc-proposal-notice" className="text-label text-ink-600">
          {notice}
        </p>
      ) : null}

      <div className="flex items-center gap-2">
        {phase === "idle" || phase === "confirm" || phase === "applying" ? (
          <>
            {!zeroMatch ? (
              <Button
                size="sm"
                className={
                  phase === "confirm"
                    ? "bg-amber-600 text-white"
                    : "bg-brand text-white"
                }
                isLoading={phase === "applying"}
                onPress={handleApply}
                data-testid="dc-proposal-apply"
              >
                {phase === "applying"
                  ? t("proposal.applying")
                  : phase === "confirm"
                    ? t("proposal.confirmApply")
                    : t("proposal.apply")}
              </Button>
            ) : null}
            <Button
              size="sm"
              variant="flat"
              isDisabled={busy}
              onPress={() => onDismiss(proposal.id)}
              data-testid="dc-proposal-dismiss"
            >
              {t("proposal.dismiss")}
            </Button>
          </>
        ) : null}

        {expiryLine && phase !== "applied" && phase !== "undone" && phase !== "undoing" ? (
          <span
            data-testid="dc-proposal-expiry"
            className="text-label text-ink-500"
          >
            {expiryLine}
          </span>
        ) : null}

        {applied && phase !== "undone" ? (
          <>
            <span className="inline-flex items-center gap-1 text-body-sm text-emerald-700">
              <Check className="w-4 h-4" />
              {t("proposal.appliedNote")}
            </span>
            <Button
              size="sm"
              variant="flat"
              startContent={<Undo2 className="w-3.5 h-3.5" />}
              isLoading={phase === "undoing"}
              isDisabled={undoDisabled}
              onPress={handleUndo}
              data-testid="dc-proposal-undo"
            >
              {phase === "undoing" ? t("proposal.undoing") : t("proposal.undo")}
            </Button>
          </>
        ) : null}

        {phase === "undone" ? (
          <span className="inline-flex items-center gap-1 text-body-sm text-ink-600">
            <Undo2 className="w-4 h-4" />
            {t("proposal.undoneNote")}
          </span>
        ) : null}

        {applied && isMenuProposal && onViewInMenu ? (
          <Button
            size="sm"
            variant="flat"
            startContent={<ArrowRight className="w-3.5 h-3.5" />}
            onPress={onViewInMenu}
            data-testid="dc-proposal-view-in-menu"
          >
            {t("proposal.viewInMenu")}
          </Button>
        ) : null}

        {phase === "terminal" ? (
          <Button
            size="sm"
            variant="flat"
            onPress={() => onDismiss(proposal.id)}
            data-testid="dc-proposal-dismiss"
          >
            {t("proposal.dismiss")}
          </Button>
        ) : null}
      </div>
    </div>
  );
}
