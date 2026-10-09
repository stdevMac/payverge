import React, { useState, useEffect, useCallback, useRef } from "react";
import {
  Modal,
  ModalContent,
  ModalHeader,
  ModalBody,
  ModalFooter,
  Button,
} from "@nextui-org/react";
import { AlertCircle, Calendar, Check, Power } from "lucide-react";
import toast from "react-hot-toast";
import { reservationAPI } from "@/api/reservations";
import {
  useSimpleLocale,
  getTranslation,
} from "@/i18n/SimpleTranslationProvider";
import { buildReservationTogglePayload } from "@/utils/partnerFallbackToggles";
import ActivationPanel from "./shared/ActivationPanel";
import { btnPrimaryNextUI } from "@/components/ui/buttonStyles";

interface ReservationToggleProps {
  businessId: number;
  isLocked: boolean;
  variant?: "card" | "button";
  // Called after the enabled flag is successfully persisted. The parent uses
  // this to flip its own reservation-status state and re-render the dashboard
  // in place — replacing the old window.location.reload(), which forced a full
  // auth re-initialization and could log staff users out.
  onToggled?: (enabled: boolean) => void;
  // Called with what THIS component's own settings read actually saw. The
  // parent runs `useReservationStatus`, a second, independent GET; when the two
  // disagree the card renders `null` (see the `variant === "card" && enabled`
  // bail-out below) and the tab paints a "Reservas pausadas" header over an
  // empty body (#681). Reporting the observed flag lets the parent reconcile.
  onStatusResolved?: (enabled: boolean) => void;
}

// Both readers below are plain effects, not React Query, and the axios client
// no longer retries GETs (React Query is the single retry owner). One dropped
// cold-load request therefore latched the tab into "paused" with no re-read
// until a settings round-trip. Retry the read a bounded number of times before
// believing a failure.
const SETTINGS_READ_RETRY_DELAYS_MS = [200, 400];

async function readReservationSettings(businessId: number) {
  let lastError: unknown;
  for (
    let attempt = 0;
    attempt <= SETTINGS_READ_RETRY_DELAYS_MS.length;
    attempt += 1
  ) {
    try {
      return await reservationAPI.getSettings(businessId);
    } catch (error) {
      lastError = error;
      const delayMs = SETTINGS_READ_RETRY_DELAYS_MS[attempt];
      if (delayMs === undefined) break;
      await new Promise((resolve) => setTimeout(resolve, delayMs));
    }
  }
  throw lastError;
}

export function useReservationStatus(businessId: number, isLocked: boolean) {
  const [enabled, setEnabledState] = useState(false);
  // Settings are unknown until a fetch for THIS business resolves. Treating
  // enabled=false + loading=false as "paused" before that paint (the unlock
  // frame after a access-gate, or a businessId swap) flashed "Reservas pausadas".
  const [knownForId, setKnownForId] = useState<number | null>(null);

  const setEnabled = useCallback(
    (value: boolean) => {
      setEnabledState(value);
      setKnownForId(businessId);
    },
    [businessId],
  );

  useEffect(() => {
    if (isLocked) {
      setEnabledState(false);
      setKnownForId(null);
      return;
    }

    let cancelled = false;
    const loadSettings = async () => {
      try {
        const settings = await readReservationSettings(businessId);
        if (!cancelled) {
          setEnabledState(settings.enabled);
          setKnownForId(businessId);
        }
      } catch (error) {
        console.error("Failed to load reservation settings:", error);
        if (!cancelled) {
          setEnabledState(false);
          setKnownForId(businessId);
        }
      }
    };

    loadSettings();
    return () => {
      cancelled = true;
    };
  }, [businessId, isLocked]);

  if (isLocked) {
    return { enabled: false, loading: false, setEnabled };
  }

  const loading = knownForId !== businessId;
  return {
    enabled: loading ? false : enabled,
    loading,
    setEnabled,
  };
}

export default function ReservationToggle({
  businessId,
  isLocked,
  variant = "card",
  onToggled,
  onStatusResolved,
}: ReservationToggleProps) {
  const { locale } = useSimpleLocale();
  const [currentLocale, setCurrentLocale] = useState(locale);
  const [enabled, setEnabled] = useState(false);
  const [loading, setLoading] = useState(true);
  const [actionLoading, setActionLoading] = useState(false);
  const [showEnableModal, setShowEnableModal] = useState(false);
  const [showDisableModal, setShowDisableModal] = useState(false);

  useEffect(() => {
    setCurrentLocale(locale);
  }, [locale]);

  const onStatusResolvedRef = useRef(onStatusResolved);
  useEffect(() => {
    onStatusResolvedRef.current = onStatusResolved;
  }, [onStatusResolved]);

  const t = useCallback(
    (key: string): string => {
      const fullKey = `businessDashboard.reservations.${key}`;
      const result = getTranslation(fullKey, currentLocale);
      return Array.isArray(result) ? result[0] || key : (result as string);
    },
    [currentLocale],
  );

  useEffect(() => {
    if (isLocked) {
      setEnabled(false);
      setLoading(false);
      return;
    }

    let cancelled = false;
    const loadSettings = async () => {
      try {
        const settings = await readReservationSettings(businessId);
        if (cancelled) return;
        setEnabled(settings.enabled);
        onStatusResolvedRef.current?.(settings.enabled);
      } catch (error) {
        console.error("Failed to load reservation settings:", error);
      } finally {
        if (!cancelled) setLoading(false);
      }
    };

    loadSettings();
    return () => {
      cancelled = true;
    };
  }, [businessId, isLocked]);

  const handleToggle = async (newEnabled: boolean) => {
    if (isLocked) return;

    setActionLoading(true);
    try {
      const payload = buildReservationTogglePayload(newEnabled);
      await reservationAPI.updateSettings(businessId, payload);
      setEnabled(newEnabled);
      setShowEnableModal(false);
      setShowDisableModal(false);
      // Refresh sibling components in place instead of window.location.reload().
      // A hard reload re-runs the auth bootstrap, which could spuriously log
      // staff users out; letting the parent flip its reservation-status state
      // re-renders the dashboard without touching the session.
      onToggled?.(newEnabled);
      setActionLoading(false);
    } catch (error) {
      console.error("Failed to update reservation settings:", error);
      toast.error(t("toggle.updateError"));
      setActionLoading(false);
    }
  };

  const handleEnableClick = () => {
    setShowEnableModal(true);
  };

  const handleDisableClick = () => {
    setShowDisableModal(true);
  };

  if (isLocked) {
    return null;
  }

  // Hold layout while the status loads instead of popping in afterwards.
  if (loading) {
    return (
      <div
        className="h-8 w-32 animate-pulse rounded-full border border-warm-200/80 bg-white/80 shadow-sm shadow-warm-900/5"
        aria-hidden="true"
      />
    );
  }

  // Button variant - only show if enabled
  if (variant === "button") {
    if (!enabled) return null;

    return (
      <>
        <button
          onClick={handleDisableClick}
          disabled={actionLoading}
          className="inline-flex items-center gap-2 whitespace-nowrap rounded-xl border border-warm-200/90 bg-white/80 px-4 py-2 font-medium text-ink-700 shadow-sm shadow-warm-300/20 transition-all duration-200 hover:-translate-y-0.5 hover:border-rose-300 hover:bg-rose-50 hover:text-rose-700 active:translate-y-0 disabled:cursor-not-allowed disabled:opacity-55"
        >
          <Power className="w-4 h-4" />
          {t("toggle.disable")}
        </button>

        {/* Disable Confirmation Modal */}
        <Modal
          isOpen={showDisableModal}
          onClose={() => setShowDisableModal(false)}
        >
          <ModalContent className="rounded-2xl border border-warm-200/90 bg-white shadow-panel">
            {(onClose) => (
              <>
                <ModalHeader className="flex flex-col gap-1 border-b border-warm-200/80 pb-4">
                  <div className="flex items-center gap-3 text-amber-700">
                    <span className="flex h-9 w-9 items-center justify-center rounded-xl border border-amber-200 bg-amber-50">
                      <AlertCircle className="h-5 w-5" aria-hidden="true" />
                    </span>
                    <span className="font-semibold text-ink-950">
                      {t("toggle.disableModal.title")}
                    </span>
                  </div>
                </ModalHeader>
                <ModalBody className="gap-4 py-5">
                  <p className="text-sm leading-6 text-ink-600">
                    {t("toggle.disableModal.description")}
                  </p>
                </ModalBody>
                <ModalFooter className="border-t border-warm-200/80">
                  <Button
                    variant="light"
                    onPress={onClose}
                    disabled={actionLoading}
                    radius="full"
                    className="font-medium text-ink-700 hover:bg-warm-100"
                  >
                    {t("toggle.disableModal.cancel")}
                  </Button>
                  <Button
                    onPress={() => handleToggle(false)}
                    isLoading={actionLoading}
                    radius="full"
                    className="bg-rose-600 font-medium text-white hover:bg-rose-700"
                  >
                    {t("toggle.disableModal.confirm")}
                  </Button>
                </ModalFooter>
              </>
            )}
          </ModalContent>
        </Modal>
      </>
    );
  }

  // Card variant - only show when disabled
  if (variant === "card" && enabled) {
    return null;
  }

  return (
    <>
      <ActivationPanel
        icon={Calendar}
        title={t("toggle.title")}
        description={t("toggle.description")}
        features={[
          {
            title: t("toggle.features.booking"),
            description: t("toggle.features.bookingDesc"),
          },
          {
            title: t("toggle.features.management"),
            description: t("toggle.features.managementDesc"),
          },
          {
            title: t("toggle.features.automation"),
            description: t("toggle.features.automationDesc"),
          },
          {
            title: t("toggle.features.notifications"),
            description: t("toggle.features.notificationsDesc"),
          },
        ]}
        action={
          <Button
            onPress={handleEnableClick}
            isLoading={actionLoading}
            radius="full"
            className="bg-brand font-medium text-white hover:bg-brand-dark"
          >
            {t("toggle.enable")}
          </Button>
        }
        footnote={t("toggle.activationHint")}
      />

      {/* Enable Confirmation Modal */}
      <Modal
        isOpen={showEnableModal}
        onClose={() => setShowEnableModal(false)}
        size="lg"
      >
        <ModalContent className="rounded-2xl border border-warm-200/90 bg-white shadow-panel">
          {(onClose) => (
            <>
              <ModalHeader className="flex flex-col gap-1 border-b border-warm-200/80 pb-4">
                <div className="flex items-center gap-3">
                  <span className="flex h-9 w-9 items-center justify-center rounded-xl border border-brand/15 bg-brand/5 text-brand">
                    <Calendar className="h-5 w-5" aria-hidden="true" />
                  </span>
                  <span className="font-semibold text-ink-950">
                    {t("toggle.enableModal.title")}
                  </span>
                </div>
              </ModalHeader>
              <ModalBody className="gap-4 py-5">
                <div className="space-y-4">
                  <p className="text-sm leading-6 text-ink-600">
                    {t("toggle.enableModal.description")}
                  </p>
                  <div className="rounded-2xl border border-warm-200/80 bg-white/80 p-4 shadow-sm shadow-warm-900/5">
                    <h4 className="mb-3 text-sm font-semibold text-ink-950">
                      {t("toggle.enableModal.features.title")}
                    </h4>
                    <ModalFeatureList
                      items={[
                        t("toggle.enableModal.features.feature1"),
                        t("toggle.enableModal.features.feature2"),
                        t("toggle.enableModal.features.feature3"),
                      ]}
                    />
                  </div>
                </div>
              </ModalBody>
              <ModalFooter className="border-t border-warm-200/80">
                <Button
                  variant="light"
                  onPress={onClose}
                  disabled={actionLoading}
                  radius="full"
                  className="font-medium text-ink-700 hover:bg-warm-100"
                >
                  {t("toggle.enableModal.cancel")}
                </Button>
                <Button
                  radius="full"
                  className={btnPrimaryNextUI}
                  onPress={() => handleToggle(true)}
                  isLoading={actionLoading}
                >
                  {t("toggle.enableModal.confirm")}
                </Button>
              </ModalFooter>
            </>
          )}
        </ModalContent>
      </Modal>
    </>
  );
}

function ModalFeatureList({ items }: { items: string[] }) {
  return (
    <ul className="grid gap-2">
      {items.map((item) => (
        <li key={item} className="flex items-start gap-2 text-sm text-ink-700">
          <span className="mt-0.5 flex h-5 w-5 shrink-0 items-center justify-center rounded-full bg-brand/10 text-brand">
            <Check className="h-3.5 w-3.5" aria-hidden="true" />
          </span>
          <span className="leading-5">{item}</span>
        </li>
      ))}
    </ul>
  );
}
