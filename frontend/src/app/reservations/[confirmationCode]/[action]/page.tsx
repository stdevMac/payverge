"use client";

import Link from "next/link";
import { useContext, useEffect, useRef, useState } from "react";
import { useParams, useRouter, useSearchParams } from "next/navigation";
import { Button, Card, CardBody, Spinner } from "@nextui-org/react";

import {
  guestReservationAPI,
  type Reservation,
  type ReservationActionResponse,
} from "@/api/reservations";
import { getRouteParam } from "@/utils/nextRouteParams";
import {
  GuestTranslationContext,
  GuestTranslationProvider,
} from "@/i18n/GuestTranslationProvider";
import { resolveReservationInitialLanguage } from "@/lib/reservationLocale";
import { guestReservationDetailsPath } from "@/lib/guestReservationPaths";
import {
  presentGuestReservationCancelError,
  presentGuestReservationCancelSuccess,
} from "@/lib/guestReservationErrors";
import {
  DATE_LONG,
  formatBusinessDateTime,
} from "@/utils/businessTime";

type ViewState = "awaiting_confirmation" | "loading" | "success" | "error";

function formatReservationTime(
  value: string | undefined,
  locale: string,
  timeZone?: string | null,
) {
  if (!value) {
    return "";
  }
  return formatBusinessDateTime(value, locale, timeZone ?? null, DATE_LONG);
}

function getBusinessHref(customUrl?: string) {
  return customUrl ? `/b/${customUrl}` : "/";
}

function ReservationActionView() {
  const params = useParams();
  const router = useRouter();
  const searchParams = useSearchParams();
  const confirmationCode = getRouteParam(params, "confirmationCode");
  const action = getRouteParam(params, "action");
  const translation = useContext(GuestTranslationContext);
  const t = (key: string, fallback: string) =>
    translation ? translation.t(key) === key ? fallback : translation.t(key) : fallback;
  const locale = translation?.currentLanguage ?? "en";

  const [viewState, setViewState] = useState<ViewState>("awaiting_confirmation");
  const [reservation, setReservation] = useState<Reservation | null>(null);
  const [businessName, setBusinessName] = useState("");
  const [businessCustomUrl, setBusinessCustomUrl] = useState("");
  const [businessTimezone, setBusinessTimezone] = useState<string | null>(null);
  const [responseMessage, setResponseMessage] = useState("");
  const [errorMessage, setErrorMessage] = useState("");
  // A reservation cancel is non-idempotent from the user's POV
  // (a double-cancel would have to be dedup'd on the server, and many providers
  // return an error on the second call). Guard against React 18 strict-mode
  // double-invocation and back-button re-mounts by requiring a user click
  // before running the action.
  const inFlightRef = useRef(false);

  // Guest self-confirmation has been removed — approval belongs to the
  // business. Stale "confirm" links from old pending emails degrade to the
  // read-only details page instead of erroring.
  const isLegacyConfirmLink = action === "confirm";
  const isValidAction = action === "cancel";

  useEffect(() => {
    if (isLegacyConfirmLink && confirmationCode) {
      const lang = searchParams?.get("lang");
      router.replace(
        `/reservations/${encodeURIComponent(confirmationCode)}${lang ? `?lang=${encodeURIComponent(lang)}` : ""}`,
      );
    }
  }, [isLegacyConfirmLink, confirmationCode, router, searchParams]);

  useEffect(() => {
    setViewState((prev) =>
      prev === "success" || prev === "error" ? prev : "awaiting_confirmation",
    );
  }, [action, confirmationCode]);

  async function runAction() {
    if (inFlightRef.current || !isValidAction || !confirmationCode) {
      return;
    }
    inFlightRef.current = true;
    setViewState("loading");
    setResponseMessage("");
    setErrorMessage("");

    try {
      const response: ReservationActionResponse =
        await guestReservationAPI.cancelReservation(confirmationCode);

      setReservation(response.reservation);
      setBusinessName(response.business_name || response.businessName || "");
      setBusinessCustomUrl(response.business_custom_url || "");
      setBusinessTimezone(response.business_timezone || null);
      // Map backend success message/code → guest key; never surface raw English.
      const successKey = presentGuestReservationCancelSuccess({
        message: response.message,
      });
      setResponseMessage(t(successKey, ""));
      // Adopt the booking's business so the guest's saved per-business language
      // preference (guest-language-<id>) is honored, layering on top of ?lang=.
      if (response.reservation?.business_id) {
        translation?.setBusinessId(response.reservation.business_id);
      }
      setViewState("success");
    } catch (error) {
      const errorKey = presentGuestReservationCancelError(error);
      setErrorMessage(
        t(errorKey, "We could not cancel this reservation."),
      );
      setViewState("error");
    } finally {
      inFlightRef.current = false;
    }
  }

  const reservationSummary = formatReservationTime(
    reservation?.reservation_time,
    locale,
    businessTimezone,
  );

  let title: string;
  let message: string;
  if (!confirmationCode) {
    title = t("reservationConfirmation.missingCodeTitle", "Missing reservation code");
    message = t("reservationConfirmation.missingCodeBody", "The reservation link is incomplete.");
  } else if (isLegacyConfirmLink) {
    // Redirecting to the read-only details page (see effect above).
    title = t("reservationConfirmation.loadingTitle", "Loading reservation...");
    message = t("reservationConfirmation.loadingBody", "Fetching your reservation details.");
  } else if (!isValidAction) {
    title = t("reservationConfirmation.action.invalidTitle", "Invalid reservation action");
    message = t("reservationConfirmation.action.invalidBody", "This reservation link is not supported.");
  } else if (viewState === "loading") {
    title = t("reservationConfirmation.action.cancellingTitle", "Cancelling reservation...");
    message = t("reservationConfirmation.action.pleaseWait", "Please wait.");
  } else if (viewState === "success") {
    title = t("reservationConfirmation.action.cancelledTitle", "Reservation Cancelled");
    // responseMessage is already a localized string from the guest catalog.
    message =
      responseMessage ||
      t(
        "reservationConfirmation.action.cancelledBody",
        "Your reservation has been cancelled.",
      );
  } else if (viewState === "error") {
    title = t("reservationConfirmation.action.cancelErrorTitle", "Unable to Cancel Reservation");
    // errorMessage is already localized via presentGuestReservationCancelError.
    message =
      errorMessage ||
      t(
        "reservationConfirmation.action.cancelErrorBody",
        "We could not cancel this reservation.",
      );
  } else {
    title = t("reservationConfirmation.action.cancelPrompt", "Cancel your reservation");
    message = t("reservationConfirmation.action.cancelBody", "Tap the button below to cancel your reservation. This cannot be undone.");
  }

  const statusKey = reservation?.status ? `reservationConfirmation.status.${reservation.status}` : "";
  const statusLabel = reservation
    ? t(statusKey, reservation.status.replace("_", " "))
    : "";

  return (
    <main className="min-h-screen bg-warm-50 px-6 py-16 text-stone-900">
      <div className="mx-auto flex max-w-2xl items-center justify-center">
        <Card className="w-full border border-stone-200 bg-white shadow-xl">
          <CardBody className="gap-6 p-8">
            <div className="space-y-3 text-center">
              <p className="text-xs uppercase tracking-[0.3em] text-brand">
                {businessName ||
                  t("reservationConfirmation.brandHeading", "Payverge Reservations")}
              </p>
              <h1 className="font-title text-3xl text-stone-900">{title}</h1>
              <p className="text-sm text-stone-600">{message}</p>
            </div>

            {viewState === "loading" ? (
              <div className="flex justify-center py-6">
                <Spinner color="primary" />
              </div>
            ) : null}

            {viewState === "awaiting_confirmation" && isValidAction ? (
              <div className="flex justify-center">
                <Button
                  color="danger"
                  variant="solid"
                  onPress={() => void runAction()}
                >
                  {t("reservationConfirmation.cancelCta", "Cancel Reservation")}
                </Button>
              </div>
            ) : null}

            {reservation ? (
              <div className="rounded-2xl border border-stone-200 bg-stone-50 p-5">
                <div className="space-y-2 text-sm text-stone-700">
                  <p>
                    <span className="text-stone-500">
                      {t("reservationConfirmation.guestLabel", "Guest")}:
                    </span>{" "}
                    {reservation.customer_name}
                  </p>
                  <p>
                    <span className="text-stone-500">
                      {t("reservationConfirmation.partySizeLabel", "Party size")}:
                    </span>{" "}
                    {reservation.party_size}
                  </p>
                  {reservationSummary ? (
                    <p>
                      <span className="text-stone-500">
                        {t("reservationConfirmation.whenLabel", "When")}:
                      </span>{" "}
                      {reservationSummary}
                    </p>
                  ) : null}
                  <p>
                    <span className="text-stone-500">
                      {t("reservationConfirmation.statusLabel", "Status")}:
                    </span>{" "}
                    {statusLabel}
                  </p>
                </div>
              </div>
            ) : null}

            {viewState !== "loading" ? (
              <div className="flex justify-center">
                <div className="flex flex-wrap justify-center gap-3">
                  {viewState === "error" && isValidAction ? (
                    // Reuses the existing guest key bill.retry ("Try Again")
                    // so no 21-locale key addition is needed.
                    <Button
                      color="danger"
                      variant="solid"
                      onPress={() => void runAction()}
                    >
                      {t("bill.retry", "Try Again")}
                    </Button>
                  ) : null}
                  {confirmationCode ? (
                    // Built from the URL code, so this recovery link renders
                    // even when the reservation object never loaded.
                    <Button
                      as={Link}
                      href={guestReservationDetailsPath(
                        reservation?.confirmation_code || confirmationCode,
                        searchParams?.get("lang") ||
                          reservation?.language ||
                          locale,
                      )}
                      color="primary"
                      variant={viewState === "error" ? "bordered" : "solid"}
                    >
                      {t("reservationConfirmation.viewReservationCta", "View Reservation")}
                    </Button>
                  ) : null}
                  <Button as={Link} href={getBusinessHref(businessCustomUrl)} variant="bordered">
                    {businessCustomUrl
                      ? t("reservationConfirmation.backToRestaurantCta", "Back to Restaurant")
                      : t("reservationConfirmation.returnHomeCta", "Return Home")}
                  </Button>
                </div>
              </div>
            ) : null}
          </CardBody>
        </Card>
      </div>
    </main>
  );
}

export default function ReservationActionPage() {
  const searchParams = useSearchParams();
  // PG-12: honor guest cookie when email link has no ?lang=
  const initialLanguage = resolveReservationInitialLanguage(
    searchParams?.get("lang"),
  );
  return (
    <GuestTranslationProvider initialLanguage={initialLanguage}>
      <ReservationActionView />
    </GuestTranslationProvider>
  );
}
