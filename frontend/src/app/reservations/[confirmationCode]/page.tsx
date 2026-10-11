"use client";

import Link from "next/link";
import { useEffect, useRef, useState } from "react";
import { useParams, useSearchParams } from "next/navigation";
import { Button, Card, CardBody, Chip, Spinner } from "@nextui-org/react";

import {
  guestReservationAPI,
  type PublicReservationDetailsResponse,
} from "@/api/reservations";
import { getRouteParam } from "@/utils/nextRouteParams";
import {
  presentGuestReservationLoadError,
  type GuestReservationLoadMessageKey,
} from "@/lib/guestReservationErrors";
import { GuestTranslationProvider, GUEST_SUPPORTED_LANGUAGES, useGuestTranslation } from "@/i18n/GuestTranslationProvider";
import { resolveReservationInitialLanguage } from "@/lib/reservationLocale";
import { guestReservationCancelPath } from "@/lib/guestReservationPaths";
import {
  DATE_LONG,
  formatBusinessDateTime,
} from "@/utils/businessTime";

type ViewState = "loading" | "success" | "error";

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

function getStatusColor(status: string) {
  switch (status) {
    case "confirmed":
      return "success";
    case "pending":
      return "warning";
    case "waitlist":
      return "secondary";
    case "cancelled":
    case "no_show":
      return "danger";
    case "seated":
    case "completed":
      return "primary";
    default:
      return "default";
  }
}

function ReservationDetailsView() {
  const params = useParams();
  const searchParams = useSearchParams();
  const confirmationCode = getRouteParam(params, "confirmationCode");
  const lastLoadedCodeRef = useRef("");
  const { t, currentLanguage, setLanguage } = useGuestTranslation();
  const locale = currentLanguage ?? "en";
  const langQuery = searchParams?.get("lang");

  const [viewState, setViewState] = useState<ViewState>("loading");
  const [errorKey, setErrorKey] = useState<GuestReservationLoadMessageKey | "">(
    "",
  );
  const [details, setDetails] = useState<PublicReservationDetailsResponse | null>(null);

  useEffect(() => {
    if (lastLoadedCodeRef.current === confirmationCode) {
      return;
    }
    lastLoadedCodeRef.current = confirmationCode;

    setViewState("loading");
    setErrorKey("");
    setDetails(null);

    if (!confirmationCode) {
      setViewState("error");
      return;
    }

    async function loadReservation() {
      try {
        const response = await guestReservationAPI.getReservation(confirmationCode);
        setDetails(response);
        // Prefer ?lang= (email deep link). Otherwise adopt the language stored
        // on the reservation row so confirmation chrome stays localized even
        // when the link was opened without a query param.
        if (!langQuery && response.reservation?.language) {
          setLanguage(
            resolveReservationInitialLanguage(response.reservation.language),
          );
        }
        setViewState("success");
      } catch (error) {
        setViewState("error");
        setErrorKey(presentGuestReservationLoadError(error));
      }
    }

    void loadReservation();
  }, [confirmationCode, langQuery, setLanguage]);

  const reservation = details?.reservation;
  const businessHref = getBusinessHref(details?.business_custom_url);
  const langForLinks =
    langQuery && langQuery in GUEST_SUPPORTED_LANGUAGES
      ? langQuery
      : reservation?.language && reservation.language in GUEST_SUPPORTED_LANGUAGES
        ? reservation.language
        : locale;

  let title: string;
  let message: string;
  if (viewState === "loading") {
    title = t("reservationConfirmation.loadingTitle");
    message = t("reservationConfirmation.loadingBody");
  } else if (viewState === "success") {
    title = t("reservationConfirmation.successTitle");
    message = t("reservationConfirmation.successBody");
  } else if (!confirmationCode) {
    title = t("reservationConfirmation.missingCodeTitle");
    message = t("reservationConfirmation.missingCodeBody");
  } else {
    title = t("reservationConfirmation.unableLoadTitle");
    message = t(errorKey || "reservationConfirmation.notFoundFallback");
  }

  const statusKey = reservation?.status ? `reservationConfirmation.status.${reservation.status}` : "";
  const statusLabel = reservation
    ? t(statusKey)
    : "";

  return (
    <main className="min-h-screen bg-warm-50 px-6 py-16 text-stone-900">
      <div className="mx-auto flex max-w-3xl items-center justify-center">
        <Card className="w-full border border-stone-200 bg-white shadow-xl">
          <CardBody className="gap-6 p-8">
            <div className="space-y-3 text-center">
              <p className="text-xs uppercase tracking-[0.3em] text-brand">
                {details?.business_name ||
                  t("reservationConfirmation.brandHeading")}
              </p>
              <h1 className="font-title text-3xl text-stone-900">{title}</h1>
              <p className="text-sm text-stone-600">{message}</p>
            </div>

            {viewState === "loading" ? (
              <div className="flex justify-center py-6">
                <Spinner color="primary" />
              </div>
            ) : null}

            {reservation ? (
              <>
                <div className="rounded-2xl border border-stone-200 bg-stone-50 p-6">
                  <div className="flex flex-wrap items-center justify-between gap-3">
                    <div>
                      <p className="text-xs uppercase tracking-[0.2em] text-stone-500">
                        {t("reservationConfirmation.confirmationCodeLabel")}
                      </p>
                      <p className="mt-1 text-lg font-semibold text-stone-900">
                        {reservation.confirmation_code}
                      </p>
                    </div>
                    <Chip
                      color={getStatusColor(reservation.status) as any}
                      variant="flat"
                    >
                      {statusLabel}
                    </Chip>
                  </div>

                  <div className="mt-6 grid gap-4 md:grid-cols-2">
                    <div className="space-y-2 text-sm text-stone-700">
                      <p>
                        <span className="text-stone-500">
                          {t("reservationConfirmation.guestLabel")}:
                        </span>{" "}
                        {reservation.customer_name}
                      </p>
                      {reservation.customer_phone ? (
                        <p>
                          <span className="text-stone-500">
                            {t("reservationConfirmation.phoneLabel")}:
                          </span>{" "}
                          {reservation.customer_phone}
                        </p>
                      ) : null}
                      {reservation.customer_email ? (
                        <p>
                          <span className="text-stone-500">
                            {t("reservationConfirmation.emailLabel")}:
                          </span>{" "}
                          {reservation.customer_email}
                        </p>
                      ) : null}
                      <p>
                        <span className="text-stone-500">
                          {t("reservationConfirmation.partySizeLabel")}:
                        </span>{" "}
                        {reservation.party_size}
                      </p>
                    </div>

                    <div className="space-y-2 text-sm text-stone-700">
                      <p>
                        <span className="text-stone-500">
                          {t("reservationConfirmation.whenLabel")}:
                        </span>{" "}
                        {formatReservationTime(
                          reservation.reservation_time,
                          locale,
                          details?.business_timezone,
                        )}
                      </p>
                      {reservation.table_name ? (
                        <p>
                          <span className="text-stone-500">
                            {t("reservationConfirmation.tableLabel")}:
                          </span>{" "}
                          {reservation.table_name}
                        </p>
                      ) : null}
                      {reservation.waitlist_position ? (
                        <p>
                          <span className="text-stone-500">
                            {t("reservationConfirmation.waitlistPositionLabel")}:
                          </span>{" "}
                          {reservation.waitlist_position}
                        </p>
                      ) : null}
                      {reservation.special_requests ? (
                        <p>
                          <span className="text-stone-500">
                            {t("reservationConfirmation.specialRequestsLabel")}:
                          </span>{" "}
                          {reservation.special_requests}
                        </p>
                      ) : null}
                    </div>
                  </div>
                </div>

                {(details?.business_address || details?.business_phone) ? (
                  <div className="rounded-2xl border border-stone-200 bg-stone-50 p-6">
                    <p className="text-xs uppercase tracking-[0.2em] text-stone-500">
                      {t("reservationConfirmation.restaurantSectionLabel")}
                    </p>
                    <div className="mt-3 space-y-2 text-sm text-stone-700">
                      {details.business_address ? <p>{details.business_address}</p> : null}
                      {details.business_phone ? <p>{details.business_phone}</p> : null}
                    </div>
                  </div>
                ) : null}

                <div className="flex flex-col items-center gap-3">
                  {details?.can_cancel ? (
                    <Button
                      as={Link}
                      href={guestReservationCancelPath(
                        reservation.confirmation_code,
                        langForLinks,
                      )}
                      color="danger"
                      variant="flat"
                    >
                      {t("reservationConfirmation.cancelCta")}
                    </Button>
                  ) : details &&
                    reservation.status !== "cancelled" &&
                    (details.can_cancel_reason === "never_open" ||
                      details.can_cancel_reason === "window_closed" ||
                      details.can_cancel_reason === "not_allowed") ? (
                    <p
                      data-testid="reservation-cancel-unavailable"
                      className="max-w-md text-center text-sm text-stone-600"
                    >
                      {details.can_cancel_reason === "not_allowed"
                        ? t("reservationConfirmation.action.notAllowedBody")
                        : details.can_cancel_reason === "never_open"
                          ? t("reservationConfirmation.onlineCancelNeverOpen", {
                              hours: details.cancellation_deadline_hours ?? 24,
                            })
                          : t("reservationConfirmation.onlineCancelWindowClosed", {
                              hours: details.cancellation_deadline_hours ?? 24,
                            })}
                      {details.business_phone
                        ? ` ${details.business_phone}`
                        : ""}
                    </p>
                  ) : null}

                  <Button as={Link} href={businessHref} variant="bordered">
                    {details?.business_custom_url
                      ? t("reservationConfirmation.backToRestaurantCta")
                      : t("reservationConfirmation.returnHomeCta")}
                  </Button>
                </div>
              </>
            ) : viewState !== "loading" ? (
              <div className="flex justify-center">
                <Button as={Link} href="/" variant="bordered">
                  {t("reservationConfirmation.returnHomeCta")}
                </Button>
              </div>
            ) : null}
          </CardBody>
        </Card>
      </div>
    </main>
  );
}

export default function ReservationDetailsPage() {
  const searchParams = useSearchParams();
  const initialLanguage = resolveReservationInitialLanguage(searchParams?.get("lang"));
  return (
    <GuestTranslationProvider initialLanguage={initialLanguage}>
      <ReservationDetailsView />
    </GuestTranslationProvider>
  );
}
