"use client";

import React from "react";
import {
  Button,
  Modal,
  ModalBody,
  ModalContent,
  ModalFooter,
  ModalHeader,
} from "@nextui-org/react";
import type {
  CampaignSuggestion,
  MarketingCreativeSnapshot,
} from "@/api/marketing";

export interface MarketingCreativeSelection {
  suggestion: CampaignSuggestion;
  creative: MarketingCreativeSnapshot;
}

export interface MarketingCreativeHandoff extends MarketingCreativeSelection {
  mode: "shared" | "downloaded" | "copied";
  /** Resolved destination display name for outcome copy (optional). */
  destinationLabel?: string;
  /**
   * S3-Loop: optional freeform channel the operator typed when confirming
   * mark_posted. Never a schedule field.
   */
  postedChannel?: string;
}

interface Props {
  handoff: MarketingCreativeHandoff | null;
  onNotYet: () => void;
  onConfirmPublished: (handoff: MarketingCreativeHandoff) => void;
  error?: string | null;
  onRetry?: () => void;
  isConfirming?: boolean;
  t: (key: string, params?: Record<string, string | number>) => string;
}

/** Server max for creative_snapshot.posted_channel (runes ≈ chars for Latin). */
const POSTED_CHANNEL_MAX_LEN = 40;

function outcomeBodyKey(
  mode: MarketingCreativeHandoff["mode"],
  hasDestination: boolean,
): string {
  if (!hasDestination) return `outcome.${mode}`;
  if (mode === "shared") return "outcome.sharedWithDestination";
  if (mode === "copied") return "outcome.copiedWithDestination";
  return "outcome.downloadedWithDestination";
}

export function ExportOutcomeDialog({
  handoff,
  onNotYet,
  onConfirmPublished,
  error,
  onRetry,
  isConfirming = false,
  t,
}: Props) {
  const mode = handoff?.mode ?? "downloaded";
  const destinationLabel = handoff?.destinationLabel?.trim() || "";
  const bodyKey = outcomeBodyKey(mode, destinationLabel.length > 0);
  // Seed freeform channel from destination label when present; operator can edit.
  const [channel, setChannel] = React.useState("");
  React.useEffect(() => {
    if (!handoff) {
      setChannel("");
      return;
    }
    const seed =
      handoff.postedChannel?.trim() ||
      handoff.destinationLabel?.trim() ||
      "";
    setChannel(seed.slice(0, POSTED_CHANNEL_MAX_LEN));
  }, [handoff]);

  const confirm = () => {
    if (!handoff) return;
    const trimmed = channel.trim().slice(0, POSTED_CHANNEL_MAX_LEN);
    onConfirmPublished({
      ...handoff,
      ...(trimmed ? { postedChannel: trimmed } : {}),
    });
  };

  return (
    <Modal
      isOpen={handoff != null}
      onClose={onNotYet}
      isDismissable={!isConfirming}
      isKeyboardDismissDisabled={isConfirming}
      hideCloseButton={isConfirming}
    >
      <ModalContent>
        <ModalHeader>{t("outcome.title")}</ModalHeader>
        <ModalBody>
          <p className="text-sm leading-6 text-ink-600">
            {destinationLabel
              ? t(bodyKey, { destination: destinationLabel })
              : t(bodyKey)}
          </p>
          <label className="mt-3 block space-y-1.5">
            <span className="text-xs font-medium text-ink-500">
              {t("outcome.channelLabel")}
            </span>
            <input
              type="text"
              data-testid="outcome-posted-channel"
              value={channel}
              maxLength={POSTED_CHANNEL_MAX_LEN}
              disabled={isConfirming}
              onChange={(e) =>
                setChannel(e.target.value.slice(0, POSTED_CHANNEL_MAX_LEN))
              }
              placeholder={t("outcome.channelPlaceholder")}
              className="w-full rounded-xl border border-warm-200 bg-white px-3 py-2 text-sm text-ink-900 placeholder:text-ink-300 focus:border-brand/40 focus:outline-none focus:ring-2 focus:ring-brand/20 disabled:opacity-60"
              autoComplete="off"
            />
            <span className="block text-xs text-ink-400">
              {t("outcome.channelHint")}
            </span>
          </label>
          {error ? (
            <p className="text-sm text-amber-700" role="alert">
              {t("outcome.error")}
            </p>
          ) : null}
        </ModalBody>
        <ModalFooter>
          <Button variant="flat" onPress={onNotYet} isDisabled={isConfirming}>
            {t("outcome.notYet")}
          </Button>
          {error && onRetry ? (
            <Button color="primary" onPress={onRetry}>
              {t("outcome.retry")}
            </Button>
          ) : (
            <Button
              color="primary"
              isLoading={isConfirming}
              isDisabled={!handoff}
              onPress={confirm}
            >
              {t("outcome.confirmPublished")}
            </Button>
          )}
        </ModalFooter>
      </ModalContent>
    </Modal>
  );
}
