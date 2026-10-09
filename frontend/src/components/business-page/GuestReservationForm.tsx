"use client";

import React, {
  useCallback,
  useEffect,
  useMemo,
  useRef,
  useState,
} from "react";
import {
  Card,
  CardBody,
  Button,
  Select,
  SelectItem,
  Modal,
  ModalContent,
  ModalHeader,
  ModalBody,
  ModalFooter,
  Chip,
  Spinner,
} from "@nextui-org/react";
import { useGuestTranslation } from "@/i18n/GuestTranslationProvider";
import {
  Calendar,
  Users,
  Mail,
  Phone,
  User,
  CheckCircle,
  AlertCircle,
  ArrowRight,
  ChevronDown,
  Utensils,
  CalendarPlus,
} from "lucide-react";
import toast from "react-hot-toast";
import {
  guestReservationAPI,
  ReservationSettingsDto,
  ReservationAvailabilityDto,
  ReservationAvailabilitySlotDto,
} from "@/api/reservations";
import { useGuestApiErrorMessage } from "@/i18n/useApiErrorMessage";
import { appendOptionalSuffix } from "@/utils/appendOptionalSuffix";
import { localDateKey } from "@/lib/localDate";
import { guestReservationDetailsPath } from "@/lib/guestReservationPaths";
import { DATE_TIME_SHORT, formatBusinessDateTime } from "@/utils/businessTime";
import { getRadiusClass, getShadowClass } from "./designClasses";
import {
  getGuestSlotState,
  pickRecommendedSlotTime,
  type SlotState,
} from "./guestSlotState";
import {
  buildReservationCalendarInvite,
  downloadReservationIcs,
} from "./guestReservationCalendar";
import { reservationConfirmationHeading } from "./guestReservationCopy";
import { AccessibleInput } from "@/components/ui/AccessibleInput";
import { AccessibleTextarea } from "@/components/ui/AccessibleTextarea";
import { guestHourCycle } from "@/utils/guestClockTime";
import {
  RESERVATION_CUSTOMER_NAME_MAX_LENGTH,
  RESERVATION_CUSTOMER_PHONE_MAX_LENGTH,
  RESERVATION_SPECIAL_REQUESTS_MAX_LENGTH,
} from "@/components/business/reservations/reservationFieldLimits";
import { GuestDemoPersonalDataNotice } from "@/components/demo/DemoPersonalDataNotice";

interface GuestReservationFormProps {
  customUrl: string;
  businessName: string;
  /** IANA timezone of the business (e.g. "Asia/Dubai"); slots render in it. */
  timezone?: string;
  designSettings?: {
    primary_color: string;
    secondary_color: string;
    corner_radius?: string;
    shadow_intensity?: string;
    font_family?: string;
  };
  /**
   * Switch the storefront to the menu tab. Wired by the landing page so the
   * confirmation modal's "Check Out Menu" CTA actually navigates instead of
   * querying a non-existent DOM attribute.
   */
  onViewMenu?: () => void;
}

const fallbackCopy = {
  title: "Reserve Your Table",
  subtitle: "Book a table at {{businessName}}",
  yourInfo: "Your Information",
  fullName: "Full Name",
  email: "Email",
  phone: "Phone Number",
  reservationDetails: "Reservation Details",
  partySize: "Party Size",
  selectGuests: "Select number of guests",
  guest: "Guest",
  guests: "Guests",
  date: "Date",
  availableTimes: "Available Times",
  noTimesTitle: "No Available Times",
  noTimesDesc:
    "This date is fully booked or outside business hours. Please try another date.",
  specialRequests: "Special Requests",
  specialRequestsPlaceholder:
    "Any dietary restrictions, seating preferences, or special occasions...",
  submit: "Book",
  pleaseNote: "Please Note:",
  confirmationRequired: "Your reservation will be confirmed by the restaurant",
  arriveTime: "Please arrive within 15 minutes of your reservation time",
  cancellationPolicy:
    "Cancellations must be made at least {{hours}} hours in advance",
  modalTitle: "Reservation Requested!",
  modalThankYou: "Thank you for your reservation request at",
  confirmationRequiredTitle: "Confirmation Required",
  confirmationRequiredDesc:
    "The restaurant will review your request and contact you shortly to confirm your reservation.",
  confirmedTitle: "Reservation Confirmed",
  confirmedDesc:
    "Your reservation has been confirmed. We look forward to seeing you.",
  details: "Reservation Details:",
  name: "Name",
  confirmationCode: "Confirmation Code",
  modalDate: "Date",
  modalTime: "Time",
  party: "Party Size",
  important: "Important:",
  importantDesc:
    "Please arrive within 15 minutes of your reservation time. Your table will be held for 15 minutes after your reservation time.",
  viewReservation: "View Reservation",
  viewInfo: "View Restaurant Info",
  checkMenu: "Check Out Menu",
  addToCalendar: "Add to calendar",
  addToGoogleCalendar: "Google Calendar",
  calendarEventTitle: "Reservation at {{businessName}}",
  calendarEventDetails:
    "Reservation for {{partySize}} guests at {{businessName}}.",
  notAvailableTitle: "Reservations Not Available",
  notAvailableDesc: "This restaurant is not currently accepting reservations.",
  invalidEmail: "Please enter a valid email address.",
  emailRequired:
    "Please enter a valid email — we'll send your confirmation there.",
  hearBackBy:
    "The restaurant reviews each request. You'll hear back by {{deadline}}.",
  loadError: "Failed to load reservation settings",
  createError: "Failed to create reservation",
  addedToWaitlist: "You were added to the waitlist for this time.",
  requestSubmitted: "Reservation request submitted for confirmation.",
  reservationConfirmedToast: "Reservation confirmed.",
  invalidPhone: "Please enter a valid phone number.",
  fillAllFields: "Please complete the required fields before continuing.",
  fullNamePlaceholder: "John Doe",
  phonePlaceholder: "+1 (555) 123-4567",
  emailPlaceholder: "john@example.com",
  optional: "optional",
  summaryDate: "Date:",
  summaryTime: "Time:",
  summaryParty: "Party:",
  summaryStatusOnSubmit: "Status on submit:",
  summaryChooseSlot: "Choose a slot",
  summaryStatusWaitlist: "Waitlist request",
  summaryStatusPending: "Pending confirmation",
  summaryStatusConfirmed: "Confirmed booking",
  summaryStatusChooseSlot: "Choose a slot to see status",
  waitlistDoNotArrive: "Do not arrive unless the restaurant confirms a table.",
  waitlistWaitForContact: "Wait to be contacted if a table becomes available.",
  loadingAvailability: "Loading availability for {{date}}",
  modalImportantWaitlistDesc:
    "Do not come to the restaurant until this waitlist request is confirmed. The restaurant will contact you if a table opens.",
  quickToday: "Today",
  quickTomorrow: "Tomorrow",
  quickWeekend: "This weekend",
  pickADate: "Pick a date",
  periodEarly: "Early",
  periodLunch: "Lunch",
  periodDinner: "Dinner",
  periodLate: "Late",
  slotsCount: "{{count}} slots",
  // Legacy step header keys (kept for locale parity; single-page form uses title/subtitle)
  step1Title: "Choose party size and date",
  step1Subtitle:
    "Start with the basics so we can show the right service windows.",
  step2Title: "Choose a service time",
  step2Subtitle:
    "Open slots, waitlist-only slots, and unavailable times are separated clearly.",
  step3Title: "Add guest details",
  step3Subtitle:
    "We use your name, phone, and email to hold or confirm the booking.",
  // Date step + booking policy
  quickDates: "Quick dates",
  dateMinAdvance:
    "Reservations must be made at least {{minutes}} minutes in advance",
  bookingPolicy: "Booking policy",
  policyDirect:
    "Direct booking stays primary unless the business cannot fit your request.",
  policyReview:
    "This business reviews new reservations before they are confirmed.",
  policyInstant: "Confirmed reservations can be booked instantly.",
  policyCancellation: "Cancellations require {{hours}} hours notice.",
  policyWaitlist: "Full time slots can still accept waitlist requests.",
  // Service-time chips + slot rendering
  chipOpenCount: "{{count}} open",
  chipWaitlistOnlyCount: "{{count}} waitlist-only",
  chipNextOpen: "Next open: {{time}}",
  slotStateOpen: "open",
  slotStateWaitlist: "waitlist",
  slotStateUnavailable: "unavailable",
  recommendedBadge: "Recommended",
  hideUnavailableSlots: "Hide unavailable times",
  showFullSchedule: "Show full schedule",
  waitlistWarning:
    "This time is currently full. If you submit this request, you will join the waitlist for {{time}}.",
  // Per-slot availability reason text (rendered under every time-slot card).
  slotReasonTableReadyOne: "{{count}} table ready",
  slotReasonTableReadyOther: "{{count}} tables ready",
  slotReasonWaitlist: "Waitlist available",
  slotReasonBookAhead: "Book at least {{minutes}} minutes ahead",
  slotReasonFullyBooked: "This service window is fully booked",
  slotReasonNoTables: "No tables available at this time",
  slotReasonUnavailable: "Unavailable",
  // Footer review + nav
  finalReview: "Final review",
  backBtn: "Back",
  continueBtn: "Continue",
  joinWaitlistBtn: "Join Waitlist",
  // Waitlist confirmation surface
  waitlistReceived: "Waitlist request received",
  waitlistFollowUp:
    "The restaurant can promote this request if another table opens. Expect a follow-up on the phone number you provided.",
  requestIdLabel: "Request ID",
  statusLabel: "Status",
  statusWaitlistValue: "Waitlist",
};

const formatWithParams = (
  template: string,
  params?: Record<string, string | number>,
) => {
  if (!params) {
    return template;
  }

  return Object.entries(params).reduce(
    (result, [key, value]) =>
      result.replace(new RegExp(`{{${key}}}`, "g"), String(value)),
    template,
  );
};

const quickDateOptions = () => {
  const today = new Date();
  const tomorrow = new Date(today);
  tomorrow.setDate(today.getDate() + 1);

  const weekend = new Date(today);
  while (![0, 6].includes(weekend.getDay())) {
    weekend.setDate(weekend.getDate() + 1);
  }

  // Use the LOCAL calendar day, not the UTC day (toISOString). Otherwise the
  // "Today" quick option resolves to tomorrow for evening guests behind UTC
  // (US Pacific/Mountain/Central/Eastern), blocking same-day booking.
  const toInput = (date: Date) => localDateKey(date);

  // labels are looked up by parent via i18n; we only return stable keys
  return [
    { key: "today" as const, value: toInput(today) },
    { key: "tomorrow" as const, value: toInput(tomorrow) },
    { key: "weekend" as const, value: toInput(weekend) },
  ];
};

// Slots are absolute UTC instants from the backend; group/format them in the
// BUSINESS timezone (falling back to the viewer's locale/timezone when the
// business timezone is unknown). Rendering in the viewer tz shifted the
// displayed time for any guest not in the venue's timezone. (L-5)
export const getServicePeriod = (isoTime: string, timeZone?: string) => {
  const date = new Date(isoTime);
  const hour = timeZone
    ? Number(
        // "en-US" here is a STABLE PARSING locale, not user-facing display: we
        // read back the numeric hour with Number(), so it must stay Latin-digit
        // (a localized digit system like ar/hi would break Number()). Guest-
        // facing times are formatted via formatTimeSlot in the active locale.
        new Intl.DateTimeFormat("en-US", {
          timeZone,
          hour: "2-digit",
          hourCycle: "h23",
        })
          .formatToParts(date)
          .find((part) => part.type === "hour")?.value ?? "0",
      )
    : date.getHours();

  if (hour < 11) return "Early";
  if (hour < 16) return "Lunch";
  if (hour < 22) return "Dinner";
  return "Late";
};

export const formatTimeSlot = (
  time: string,
  timeZone?: string,
  locale?: string,
) => {
  const date = new Date(time);
  // I18N-4: format in the active GUEST locale (12h vs 24h follows the locale),
  // not the browser default. The guest codes are valid BCP-47 tags; fall back
  // to the browser default ([]) only when no locale is supplied.
  return date.toLocaleTimeString(locale || [], {
    hour: "numeric",
    minute: "2-digit",
    // #949: es-AR must read 23:00, not "11:00 p. m." — one shared hour-cycle
    // rule for every guest-facing clock string.
    hourCycle: guestHourCycle(locale),
    ...(timeZone ? { timeZone } : {}),
  });
};

export default function GuestReservationForm({
  customUrl,
  businessName,
  timezone,
  /* eslint-disable no-restricted-syntax -- API-stored design defaults; not Tailwind classes */
  designSettings = {
    primary_color: "#1a6b6a",
    secondary_color: "#2a8b8a",
    corner_radius: "medium",
    shadow_intensity: "subtle",
    font_family: "Inter",
  },
  /* eslint-enable no-restricted-syntax */
  onViewMenu,
}: GuestReservationFormProps) {
  const { t, currentLanguage } = useGuestTranslation();
  // Coded backend errors (e.g. reservation_email_required) localize from
  // apiErrors.json in the guest's storefront language; uncoded ones keep the
  // raw backend string.
  const localizeGuestError = useGuestApiErrorMessage();
  const [settings, setSettings] = useState<ReservationSettingsDto | null>(null);
  const [loading, setLoading] = useState(true);
  const [submitting, setSubmitting] = useState(false);
  const [showConfirmation, setShowConfirmation] = useState(false);
  const [reservationId, setReservationId] = useState<number | null>(null);
  const [confirmationCode, setConfirmationCode] = useState("");
  const [reservationStatus, setReservationStatus] = useState<
    "pending" | "confirmed" | "waitlist" | null
  >(null);
  // RFC3339 deadline by which a manual-approval business must answer a
  // pending request; only set when the create response carries one.
  const [pendingDeadline, setPendingDeadline] = useState<string | null>(null);

  const [customerName, setCustomerName] = useState("");
  const [customerEmail, setCustomerEmail] = useState("");
  const [customerPhone, setCustomerPhone] = useState("");
  const [partySize, setPartySize] = useState(2);
  const [selectedDate, setSelectedDate] = useState("");
  const [selectedTime, setSelectedTime] = useState("");
  const [specialRequests, setSpecialRequests] = useState("");
  const [showUnavailableSlots, setShowUnavailableSlots] = useState(false);

  const [availability, setAvailability] =
    useState<ReservationAvailabilityDto | null>(null);
  const [availableSlots, setAvailableSlots] = useState<
    ReservationAvailabilitySlotDto[]
  >([]);
  const [loadingSlots, setLoadingSlots] = useState(false);
  const [emailError, setEmailError] = useState("");
  const [phoneError, setPhoneError] = useState("");
  const availabilityRequestSeq = useRef(0);
  const settingsLoadedRef = useRef(false);

  const reservationT = useCallback(
    (
      key: keyof typeof fallbackCopy,
      params?: Record<string, string | number>,
    ): string => {
      const translationKey = key.startsWith("modal")
        ? `businessPage.info.reservationForm.modal.${key
            .replace("modal", "")
            .replace(/^./, (value) => value.toLowerCase())}`
        : `businessPage.info.reservationForm.${key}`;
      const translated = t(translationKey, params);
      if (translated !== translationKey) {
        return translated;
      }
      return formatWithParams(fallbackCopy[key], params);
    },
    [t],
  );

  // Resolve a confirmation-modal string. Every modal label lives under
  // businessPage.info.reservationForm.modal.* in all 21 guest bundles, but the
  // fallbackCopy keys for most of them (details, name, party, important, …) are
  // NOT "modal"-prefixed, so reservationT would look them up at the flat path
  // and miss. modalT always targets the modal.* namespace with the matching
  // leaf, falling back to the supplied English literal.
  const modalT = useCallback(
    (
      leaf: string,
      fallback: string,
      params?: Record<string, string | number>,
    ): string => {
      const translationKey = `businessPage.info.reservationForm.modal.${leaf}`;
      const translated = t(translationKey, params);
      if (translated !== translationKey) {
        return translated;
      }
      return formatWithParams(fallback, params);
    },
    [t],
  );

  // Email is REQUIRED for online booking: the backend rejects guest creates
  // without a valid email, and confirmations are delivered there.
  // Shared pure validators power both handleSubmit and the submit button
  // disabled state so the affordance matches validation (GUEST-006).
  const isEmailValid = (email: string): boolean => {
    const trimmed = email.trim();
    return trimmed.length > 0 && /^[^\s@]+@[^\s@]+\.[^\s@]+$/.test(trimmed);
  };

  const isPhoneValid = (phone: string): boolean => {
    if (!phone) return false;
    return /^[+]?[\d\s()-]{7,20}$/.test(phone);
  };

  const validateEmail = (email: string) => {
    const trimmed = email.trim();
    if (!trimmed) {
      setEmailError(reservationT("emailRequired"));
      return false;
    }

    const valid = isEmailValid(email);
    setEmailError(valid ? "" : reservationT("invalidEmail"));
    return valid;
  };

  const validatePhone = (phone: string) => {
    if (!phone) {
      setPhoneError("");
      return false;
    }

    const valid = isPhoneValid(phone);
    setPhoneError(valid ? "" : reservationT("invalidPhone"));
    return valid;
  };

  const canSubmitReservation =
    Boolean(customerName?.trim()) &&
    isPhoneValid(customerPhone) &&
    isEmailValid(customerEmail) &&
    Boolean(selectedTime);

  const radiusClass = getRadiusClass(designSettings.corner_radius);
  const shadowClass = getShadowClass(designSettings.shadow_intensity);

  const loadSettings = useCallback(async () => {
    try {
      if (!settingsLoadedRef.current) {
        setLoading(true);
      }
      const data = await guestReservationAPI.getSettings(customUrl);
      setSettings(data);
      settingsLoadedRef.current = true;
      setPartySize((current) => {
        if (data.min_party_size > current || data.max_party_size < current) {
          return Math.max(
            data.min_party_size,
            Math.min(data.max_party_size, current),
          );
        }
        return current;
      });
    } catch (error) {
      console.error("Failed to load reservation settings:", error);
      toast.error(reservationT("loadError"));
    } finally {
      setLoading(false);
    }
  }, [customUrl, reservationT]);

  const invalidateInFlightAvailability = useCallback(() => {
    availabilityRequestSeq.current += 1;
  }, []);

  const handleDateChange = useCallback(
    (value: string) => {
      if (value === selectedDate) {
        return;
      }
      invalidateInFlightAvailability();
      setSelectedDate(value);
      setSelectedTime("");
      setShowUnavailableSlots(false);
      setLoadingSlots(Boolean(value));
    },
    [invalidateInFlightAvailability, selectedDate],
  );

  const handlePartySizeChange = useCallback(
    (size: number) => {
      if (!Number.isFinite(size) || size === partySize) {
        return;
      }
      invalidateInFlightAvailability();
      setPartySize(size);
      setSelectedTime("");
      setShowUnavailableSlots(false);
      if (selectedDate) {
        setLoadingSlots(true);
      }
    },
    [invalidateInFlightAvailability, partySize, selectedDate],
  );

  const loadAvailability = useCallback(async () => {
    if (!selectedDate) {
      return;
    }

    const seq = ++availabilityRequestSeq.current;
    setLoadingSlots(true);
    try {
      const data = await guestReservationAPI.getAvailability(
        customUrl,
        selectedDate,
        partySize,
      );
      if (seq !== availabilityRequestSeq.current) {
        return;
      }
      setAvailability(data);
      setAvailableSlots(data.available_slots);
      setSelectedTime((currentSelection) => {
        if (!currentSelection) {
          return currentSelection;
        }

        const stillSelectable = data.available_slots.some(
          (slot) =>
            slot.time === currentSelection &&
            getGuestSlotState(slot, data.waitlist_available) !== "unavailable",
        );
        return stillSelectable ? currentSelection : "";
      });
    } catch (error) {
      if (seq !== availabilityRequestSeq.current) {
        return;
      }
      console.error("Failed to load availability:", error);
      setAvailability(null);
      setAvailableSlots([]);
      setSelectedTime("");
    } finally {
      if (seq === availabilityRequestSeq.current) {
        setLoadingSlots(false);
      }
    }
  }, [customUrl, selectedDate, partySize]);

  useEffect(() => {
    loadSettings();
  }, [loadSettings]);

  useEffect(() => {
    if (selectedDate) {
      loadAvailability();
    }
  }, [loadAvailability, selectedDate, partySize]);

  const getMinDate = () => localDateKey();

  const getMaxDate = () => {
    if (!settings) return "";
    const maxDate = new Date();
    maxDate.setDate(maxDate.getDate() + settings.max_advance_days);
    return localDateKey(maxDate);
  };

  const getSlotState = useCallback(
    (slot: ReservationAvailabilitySlotDto): SlotState =>
      getGuestSlotState(slot, availability?.waitlist_available ?? false),
    [availability?.waitlist_available],
  );

  const getSlotReasonText = useCallback(
    (slot: ReservationAvailabilitySlotDto) => {
      const state = getSlotState(slot);
      if (state === "open") {
        return reservationT(
          slot.available_tables === 1
            ? "slotReasonTableReadyOne"
            : "slotReasonTableReadyOther",
          { count: slot.available_tables },
        );
      }

      if (state === "waitlist") {
        return reservationT("slotReasonWaitlist");
      }

      switch (slot.reason_code) {
        case "outside_advance_window":
          return reservationT("slotReasonBookAhead", {
            minutes: settings?.min_advance_minutes ?? 0,
          });
        case "covers_limit":
          return reservationT("slotReasonFullyBooked");
        case "table_unavailable":
          return reservationT("slotReasonNoTables");
        // outside_operating_window (too close to closing for a full seating)
        // intentionally falls through to the generic unavailable copy.
        default:
          return reservationT("slotReasonUnavailable");
      }
    },
    [getSlotState, reservationT, settings?.min_advance_minutes],
  );

  const availableNowCount = availableSlots.filter(
    (slot) => getSlotState(slot) === "open",
  ).length;
  const waitlistOnlyCount = availableSlots.filter(
    (slot) => getSlotState(slot) === "waitlist",
  ).length;
  const selectedSlot = availableSlots.find(
    (slot) => slot.time === selectedTime,
  );
  const recommendedSlotTime = useMemo(
    () =>
      pickRecommendedSlotTime(
        availableSlots,
        availability?.waitlist_available ?? false,
      ),
    [availableSlots, availability?.waitlist_available],
  );
  const selectedSlotState = selectedSlot ? getSlotState(selectedSlot) : null;
  const isWaitlistSelection = selectedSlotState === "waitlist";
  const quickDates = quickDateOptions().filter(
    (option, index, array) =>
      array.findIndex((item) => item.value === option.value) === index,
  );

  const groupedSlots = useMemo(() => {
    const orderedSlots = [...availableSlots].sort((left, right) => {
      const leftState = getSlotState(left);
      const rightState = getSlotState(right);
      const priority = { open: 0, waitlist: 1, unavailable: 2 };
      if (priority[leftState] !== priority[rightState]) {
        return priority[leftState] - priority[rightState];
      }
      if (left.time === recommendedSlotTime) return -1;
      if (right.time === recommendedSlotTime) return 1;
      return new Date(left.time).getTime() - new Date(right.time).getTime();
    });

    const visibleSlots = showUnavailableSlots
      ? orderedSlots
      : orderedSlots.filter((slot) => getSlotState(slot) !== "unavailable");

    return visibleSlots.reduce(
      (groups, slot) => {
        const key = getServicePeriod(slot.time, timezone);
        groups[key] = groups[key] || [];
        groups[key].push(slot);
        return groups;
      },
      {} as Record<string, ReservationAvailabilitySlotDto[]>,
    );
  }, [
    availableSlots,
    getSlotState,
    recommendedSlotTime,
    showUnavailableSlots,
    timezone,
  ]);

  const selectedDateLabel = selectedDate
    ? new Date(`${selectedDate}T00:00:00`).toLocaleDateString(
        currentLanguage || [],
        {
          weekday: "long",
          month: "short",
          day: "numeric",
        },
      )
    : reservationT("pickADate");

  // Guest locale for month/hour names; venue timezone for wall-clock so the
  // approval deadline matches restaurant time (not the guest device TZ).
  const formatDeadline = (iso: string): string | null => {
    const date = new Date(iso);
    if (Number.isNaN(date.getTime())) return null;
    return formatBusinessDateTime(
      iso,
      currentLanguage || "en",
      timezone ?? null,
      DATE_TIME_SHORT,
    );
  };

  const handleSubmit = async () => {
    if (!customerName || !customerPhone || !selectedDate || !selectedTime) {
      toast.error(reservationT("fillAllFields"));
      return;
    }

    const emailValid =
      validateEmail(customerEmail) && customerEmail.trim().length > 0;
    const phoneValid = validatePhone(customerPhone);
    if (!emailValid || !phoneValid) {
      return;
    }

    try {
      setSubmitting(true);

      const reservation = await guestReservationAPI.createReservation(
        customUrl,
        {
          customer_name: customerName,
          customer_email: customerEmail.trim(),
          customer_phone: customerPhone,
          party_size: partySize,
          reservation_time: selectedTime,
          special_requests: specialRequests || undefined,
          // Send the booking page's locale only when we actually have one —
          // omitting it lets the backend stamp the venue's default language
          // instead of silently forcing English.
          language: currentLanguage || undefined,
        },
      );

      setReservationId(reservation.id);
      setConfirmationCode(reservation.confirmation_code || "");
      setPendingDeadline(
        reservation.status === "pending"
          ? (reservation.approval_deadline ?? null)
          : null,
      );
      if (
        reservation.status === "pending" ||
        reservation.status === "confirmed" ||
        reservation.status === "waitlist"
      ) {
        setReservationStatus(reservation.status);
      } else {
        setReservationStatus("confirmed");
      }
      setShowConfirmation(true);
      if (reservation.status === "waitlist") {
        toast.success(reservationT("addedToWaitlist"));
      } else if (reservation.status === "pending") {
        const deadline = reservation.approval_deadline
          ? formatDeadline(reservation.approval_deadline)
          : null;
        toast.success(
          deadline
            ? `${reservationT("requestSubmitted")} ${reservationT(
                "hearBackBy",
                {
                  deadline,
                },
              )}`
            : reservationT("requestSubmitted"),
        );
      } else {
        toast.success(reservationT("reservationConfirmedToast"));
      }
    } catch (error) {
      console.error("Failed to create reservation:", error);
      toast.error(localizeGuestError(error) || reservationT("createError"));
    } finally {
      setSubmitting(false);
    }
  };

  const calendarInvite = () => {
    if (!selectedTime || !settings) {
      return null;
    }
    const start = new Date(selectedTime);
    const end = new Date(start.getTime() + settings.default_duration * 60_000);
    return buildReservationCalendarInvite({
      title: reservationT("calendarEventTitle", { businessName }),
      details: reservationT("calendarEventDetails", {
        partySize,
        businessName,
      }),
      start,
      end,
    });
  };

  const downloadCalendarInvite = () => {
    const invite = calendarInvite();
    if (!invite) return;
    downloadReservationIcs(invite.icsDataUri, invite.filename);
  };

  const openGoogleCalendarInvite = () => {
    const invite = calendarInvite();
    if (!invite) return;
    window.open(invite.googleUrl, "_blank", "noopener,noreferrer");
  };

  if (loading && !settings) {
    return (
      <div
        className="flex items-center justify-center py-12"
        role="status"
        aria-live="polite"
      >
        <Spinner size="lg" />
      </div>
    );
  }

  if (!settings?.enabled) {
    return (
      <Card className="max-w-2xl mx-auto">
        <CardBody className="text-center py-12">
          <AlertCircle className="w-16 h-16 text-gray-400 mx-auto mb-4" />
          <h3 className="text-xl font-semibold text-gray-900 mb-2">
            {reservationT("notAvailableTitle")}
          </h3>
          <p className="text-gray-600">{reservationT("notAvailableDesc")}</p>
        </CardBody>
      </Card>
    );
  }

  return (
    <>
      <div className="max-w-5xl mx-auto">
        <Card
          className={`border border-gray-200 ${shadowClass} ${radiusClass} bg-white`}
        >
          <CardBody className="p-8">
            <div className="mb-8">
              <h3 className="text-2xl font-semibold text-gray-900">
                {reservationT("title")}
              </h3>
              <p className="text-sm text-gray-500 mt-1">
                {reservationT("subtitle", { businessName })}
              </p>
              <GuestDemoPersonalDataNotice className="mt-4" />
            </div>

            <div className="space-y-10">
              <div className="grid grid-cols-1 lg:grid-cols-[1.1fr,0.9fr] gap-8">
                <div className="space-y-6">
                  <div>
                    <label
                      id="guest-reservation-party-size-label"
                      className="text-sm font-medium text-gray-700 mb-2 block"
                    >
                      {reservationT("partySize")}
                    </label>
                    <Select
                      aria-labelledby="guest-reservation-party-size-label"
                      selectedKeys={new Set([partySize.toString()])}
                      onChange={(event) => {
                        const value = (event.target as HTMLSelectElement).value;
                        if (value) {
                          handlePartySizeChange(Number.parseInt(value, 10));
                        }
                      }}
                      onSelectionChange={(keys) => {
                        const selected = Array.from(keys)[0];
                        if (selected) {
                          handlePartySizeChange(
                            Number.parseInt(selected.toString(), 10),
                          );
                        }
                      }}
                      startContent={<Users className="w-4 h-4 text-gray-400" />}
                    >
                      {Array.from(
                        {
                          length:
                            settings.max_party_size -
                            settings.min_party_size +
                            1,
                        },
                        (_, index) => settings.min_party_size + index,
                      ).map((size) => (
                        <SelectItem
                          key={size.toString()}
                          value={size.toString()}
                          textValue={`${size} ${size === 1 ? reservationT("guest") : reservationT("guests")}`}
                        >
                          {size}{" "}
                          {size === 1
                            ? reservationT("guest")
                            : reservationT("guests")}
                        </SelectItem>
                      ))}
                    </Select>
                  </div>

                  <div
                    role="group"
                    aria-labelledby="reservation-quick-dates-label"
                  >
                    <p
                      id="reservation-quick-dates-label"
                      className="text-sm font-medium text-gray-700 mb-2 block"
                    >
                      {reservationT("quickDates")}
                    </p>
                    <div className="flex flex-wrap gap-2">
                      {quickDates.map((option) => {
                        const pressed = selectedDate === option.value;
                        const label =
                          option.key === "today"
                            ? reservationT("quickToday")
                            : option.key === "tomorrow"
                              ? reservationT("quickTomorrow")
                              : reservationT("quickWeekend");
                        return (
                          <Button
                            key={option.key}
                            variant={pressed ? "solid" : "flat"}
                            className={pressed ? "bg-brand text-white" : ""}
                            onPress={() => handleDateChange(option.value)}
                            aria-pressed={pressed}
                          >
                            {label}
                          </Button>
                        );
                      })}
                    </div>
                  </div>

                  <AccessibleInput
                    label={reservationT("date")}
                    type="date"
                    value={selectedDate}
                    onChange={(event) => handleDateChange(event.target.value)}
                    min={getMinDate()}
                    max={getMaxDate()}
                    startContent={
                      <Calendar className="w-4 h-4 text-gray-400" />
                    }
                    description={reservationT("dateMinAdvance", {
                      minutes: settings.min_advance_minutes,
                    })}
                  />
                </div>

                <details className="group h-fit rounded-2xl border border-gray-200 bg-gray-50 p-5">
                  <summary className="flex cursor-pointer list-none items-center justify-between gap-3 text-sm font-medium text-gray-800">
                    <span className="uppercase tracking-[0.2em] text-gray-500">
                      {reservationT("bookingPolicy")}
                    </span>
                    <ChevronDown className="h-4 w-4 text-gray-400 transition-transform group-open:rotate-180" />
                  </summary>
                  <div className="mt-4 space-y-3 text-sm text-gray-700">
                    {isWaitlistSelection ? (
                      <p>{reservationT("policyWaitlist")}</p>
                    ) : (
                      <>
                        <p>{reservationT("policyDirect")}</p>
                        <p>
                          {settings.approval_mode === "manual"
                            ? reservationT("policyReview")
                            : reservationT("policyInstant")}
                        </p>
                        {settings.allow_cancellation ? (
                          <p>
                            {reservationT("policyCancellation", {
                              hours: settings.cancellation_deadline,
                            })}
                          </p>
                        ) : null}
                        {settings.allow_waitlist ? (
                          <p>{reservationT("policyWaitlist")}</p>
                        ) : null}
                      </>
                    )}
                  </div>
                </details>
              </div>

              <div className="space-y-6" aria-busy={loadingSlots}>
                <div>
                  <h4 className="text-sm font-semibold uppercase tracking-[0.2em] text-gray-500 mb-3">
                    {reservationT("availableTimes")}
                  </h4>
                  {selectedDate ? (
                    <div
                      className="rounded-2xl border border-gray-200 bg-gray-50 p-4"
                      aria-busy={loadingSlots}
                    >
                      {loadingSlots ? (
                        <p
                          className="text-sm text-gray-600"
                          aria-live="polite"
                          aria-atomic="true"
                          role="status"
                        >
                          {reservationT("loadingAvailability", {
                            date: selectedDateLabel,
                          })}
                        </p>
                      ) : (
                        <div className="flex flex-wrap gap-2 text-xs">
                          <Chip size="sm" color="success" variant="flat">
                            {reservationT("chipOpenCount", {
                              count: availableNowCount,
                            })}
                          </Chip>
                          <Chip size="sm" color="warning" variant="flat">
                            {reservationT("chipWaitlistOnlyCount", {
                              count: waitlistOnlyCount,
                            })}
                          </Chip>
                          {availability?.next_available_slot ? (
                            <Chip size="sm" color="primary" variant="flat">
                              {reservationT("chipNextOpen", {
                                time: formatTimeSlot(
                                  availability.next_available_slot,
                                  timezone,
                                  currentLanguage,
                                ),
                              })}
                            </Chip>
                          ) : null}
                        </div>
                      )}
                    </div>
                  ) : (
                    <p className="text-sm text-gray-500">
                      {reservationT("pickADate")}
                    </p>
                  )}
                </div>

                {selectedDate ? (
                  loadingSlots ? (
                    <div
                      className="flex items-center justify-center py-12"
                      aria-hidden="true"
                    >
                      <Spinner size="sm" />
                    </div>
                  ) : availableSlots.length > 0 ? (
                    <div className="space-y-6">
                      {Object.entries(groupedSlots).map(([period, slots]) => (
                        <div key={period}>
                          <div className="flex items-center justify-between mb-3">
                            <h4 className="text-sm font-semibold uppercase tracking-[0.2em] text-gray-500">
                              {period === "Early"
                                ? reservationT("periodEarly")
                                : period === "Lunch"
                                  ? reservationT("periodLunch")
                                  : period === "Dinner"
                                    ? reservationT("periodDinner")
                                    : period === "Late"
                                      ? reservationT("periodLate")
                                      : period}
                            </h4>
                            <span className="text-xs text-gray-400">
                              {reservationT("slotsCount", {
                                count: slots.length,
                              })}
                            </span>
                          </div>
                          <div className="grid grid-cols-2 md:grid-cols-3 lg:grid-cols-4 gap-3">
                            {slots.map((slot) => {
                              const slotState = getSlotState(slot);
                              const isSelected = selectedTime === slot.time;
                              const isDisabled = slotState === "unavailable";

                              return (
                                <button
                                  key={slot.time}
                                  type="button"
                                  aria-pressed={isSelected}
                                  aria-disabled={isDisabled}
                                  tabIndex={0}
                                  onClick={() => {
                                    if (isDisabled) return;
                                    setSelectedTime(slot.time);
                                  }}
                                  className={`min-h-[84px] rounded-2xl border px-3 py-3 text-left transition ${
                                    isSelected
                                      ? "border-brand bg-brand text-white"
                                      : slotState === "open"
                                        ? "border-emerald-200 bg-emerald-50 text-gray-900"
                                        : slotState === "waitlist"
                                          ? "border-amber-200 bg-amber-50 text-gray-900"
                                          : "border-gray-200 bg-gray-50 text-gray-400"
                                  } ${isDisabled ? "cursor-not-allowed opacity-70" : "hover:-translate-y-0.5"}`}
                                >
                                  <div className="flex items-center justify-between">
                                    <span className="font-semibold">
                                      {formatTimeSlot(
                                        slot.time,
                                        timezone,
                                        currentLanguage,
                                      )}
                                    </span>
                                    <span className="text-[11px] uppercase tracking-[0.2em]">
                                      {slotState === "open"
                                        ? reservationT("slotStateOpen")
                                        : slotState === "waitlist"
                                          ? reservationT("slotStateWaitlist")
                                          : reservationT(
                                              "slotStateUnavailable",
                                            )}
                                    </span>
                                  </div>
                                  <p className="mt-2 text-xs opacity-90">
                                    {getSlotReasonText(slot)}
                                  </p>
                                  {recommendedSlotTime === slot.time &&
                                  slotState === "open" ? (
                                    <p className="mt-2 text-[11px] font-semibold uppercase tracking-[0.2em]">
                                      {reservationT("recommendedBadge")}
                                    </p>
                                  ) : null}
                                </button>
                              );
                            })}
                          </div>
                        </div>
                      ))}

                      {availableSlots.some(
                        (slot) => getSlotState(slot) === "unavailable",
                      ) ? (
                        <Button
                          variant="light"
                          onPress={() =>
                            setShowUnavailableSlots((current) => !current)
                          }
                        >
                          {showUnavailableSlots
                            ? reservationT("hideUnavailableSlots")
                            : reservationT("showFullSchedule")}
                        </Button>
                      ) : null}

                      {isWaitlistSelection && selectedSlot ? (
                        <div className="rounded-lg border border-amber-200 bg-amber-50 p-3 text-sm text-amber-900">
                          {reservationT("waitlistWarning", {
                            time: formatTimeSlot(
                              selectedSlot.time,
                              timezone,
                              currentLanguage,
                            ),
                          })}
                        </div>
                      ) : null}
                    </div>
                  ) : (
                    <div className="text-center py-8 bg-amber-50 border border-amber-200 rounded-lg">
                      <AlertCircle className="w-8 h-8 mx-auto mb-2 text-amber-600" />
                      <p className="text-sm font-semibold text-amber-900 mb-1">
                        {reservationT("noTimesTitle")}
                      </p>
                      <p className="text-xs text-amber-700">
                        {reservationT("noTimesDesc")}
                      </p>
                    </div>
                  )
                ) : null}
              </div>

              <div className="grid grid-cols-1 lg:grid-cols-[1.1fr,0.9fr] gap-8">
                <div className="space-y-4">
                  <h4 className="text-sm font-semibold uppercase tracking-[0.2em] text-gray-500">
                    {reservationT("yourInfo")}
                  </h4>
                  <AccessibleInput
                    label={reservationT("fullName")}
                    placeholder={reservationT("fullNamePlaceholder")}
                    value={customerName}
                    onChange={(event) => setCustomerName(event.target.value)}
                    startContent={<User className="w-4 h-4 text-gray-400" />}
                    autoComplete="name"
                    maxLength={RESERVATION_CUSTOMER_NAME_MAX_LENGTH}
                    isRequired
                  />
                  <AccessibleInput
                    label={reservationT("phone")}
                    type="tel"
                    placeholder={reservationT("phonePlaceholder")}
                    value={customerPhone}
                    autoComplete="tel"
                    inputMode="tel"
                    maxLength={RESERVATION_CUSTOMER_PHONE_MAX_LENGTH}
                    onChange={(event) => {
                      setCustomerPhone(event.target.value);
                      if (phoneError) setPhoneError("");
                    }}
                    onBlur={(event) => validatePhone(event.target.value)}
                    startContent={<Phone className="w-4 h-4 text-gray-400" />}
                    isRequired
                    isInvalid={!!phoneError}
                    errorMessage={phoneError}
                  />
                  <AccessibleInput
                    label={reservationT("email")}
                    type="email"
                    placeholder={reservationT("emailPlaceholder")}
                    value={customerEmail}
                    autoComplete="email"
                    inputMode="email"
                    onChange={(event) => {
                      setCustomerEmail(event.target.value);
                      if (emailError) setEmailError("");
                    }}
                    onBlur={(event) => validateEmail(event.target.value)}
                    startContent={<Mail className="w-4 h-4 text-gray-400" />}
                    isRequired
                    isInvalid={!!emailError}
                    errorMessage={emailError}
                  />
                  <AccessibleTextarea
                    label={appendOptionalSuffix(
                      reservationT("specialRequests"),
                      reservationT("optional"),
                    )}
                    placeholder={reservationT("specialRequestsPlaceholder")}
                    value={specialRequests}
                    onChange={(event) => setSpecialRequests(event.target.value)}
                    maxLength={RESERVATION_SPECIAL_REQUESTS_MAX_LENGTH}
                    minRows={4}
                  />
                </div>

                <div className="rounded-2xl border border-gray-200 bg-gray-50 p-5 space-y-4 h-fit">
                  <p className="text-sm uppercase tracking-[0.2em] text-gray-500">
                    {reservationT("finalReview")}
                  </p>
                  <div className="space-y-2 text-sm text-gray-700">
                    <p>
                      <strong>{reservationT("summaryDate")}</strong>{" "}
                      {selectedDateLabel}
                    </p>
                    <p>
                      <strong>{reservationT("summaryTime")}</strong>{" "}
                      {selectedTime
                        ? formatTimeSlot(
                            selectedTime,
                            timezone,
                            currentLanguage,
                          )
                        : reservationT("summaryChooseSlot")}
                    </p>
                    <p>
                      <strong>{reservationT("summaryParty")}</strong>{" "}
                      {partySize}{" "}
                      {partySize === 1
                        ? reservationT("guest")
                        : reservationT("guests")}
                    </p>
                    <p>
                      <strong>{reservationT("summaryStatusOnSubmit")}</strong>{" "}
                      {!selectedSlot
                        ? reservationT("summaryStatusChooseSlot")
                        : isWaitlistSelection
                          ? reservationT("summaryStatusWaitlist")
                          : settings.approval_mode === "manual"
                            ? reservationT("summaryStatusPending")
                            : reservationT("summaryStatusConfirmed")}
                    </p>
                  </div>
                  <div className="rounded-xl border border-brand/10 bg-brand/10 p-4 text-sm text-brand-dark">
                    <p className="font-semibold mb-2">
                      {reservationT("pleaseNote")}
                    </p>
                    <ul className="space-y-1 text-brand-dark">
                      {isWaitlistSelection ? (
                        <>
                          <li>• {reservationT("waitlistDoNotArrive")}</li>
                          <li>• {reservationT("waitlistWaitForContact")}</li>
                        </>
                      ) : (
                        <>
                          {settings.approval_mode === "manual" ? (
                            <li>• {reservationT("confirmationRequired")}</li>
                          ) : null}
                          <li>• {reservationT("arriveTime")}</li>
                          {settings.allow_cancellation ? (
                            <li>
                              •{" "}
                              {reservationT("cancellationPolicy", {
                                hours: settings.cancellation_deadline,
                              })}
                            </li>
                          ) : null}
                        </>
                      )}
                    </ul>
                  </div>
                </div>
              </div>
            </div>

            <div className="mt-8 flex justify-end">
              <Button
                color="primary"
                onPress={handleSubmit}
                isLoading={submitting}
                isDisabled={!canSubmitReservation}
                title={
                  !canSubmitReservation
                    ? reservationT("fillAllFields")
                    : undefined
                }
                className="bg-brand text-white font-semibold tracking-wide px-8 hover:bg-brand-dark"
                endContent={<ArrowRight className="w-5 h-5" />}
              >
                {isWaitlistSelection
                  ? reservationT("joinWaitlistBtn")
                  : reservationT("submit")}
              </Button>
            </div>
          </CardBody>
        </Card>
      </div>

      <Modal
        isOpen={showConfirmation}
        onClose={() => setShowConfirmation(false)}
        size="2xl"
      >
        <ModalContent>
          {(onClose) => (
            <>
              <ModalHeader className="flex flex-col gap-1">
                <div className="flex items-center gap-3">
                  <div className="w-12 h-12 rounded-full bg-green-100 flex items-center justify-center">
                    <CheckCircle className="w-6 h-6 text-green-600" />
                  </div>
                  <div>
                    <h3 className="text-2xl font-semibold">
                      {reservationConfirmationHeading(reservationStatus, {
                        requested: modalT("title", fallbackCopy.modalTitle),
                        confirmed: modalT(
                          "confirmedTitle",
                          fallbackCopy.confirmedTitle,
                        ),
                        waitlist: reservationT("waitlistReceived"),
                      })}
                    </h3>
                  </div>
                </div>
              </ModalHeader>
              <ModalBody>
                <div className="space-y-4">
                  <p className="text-gray-700">
                    {modalT("thankYou", fallbackCopy.modalThankYou)}{" "}
                    <strong>{businessName}</strong>.
                  </p>

                  {reservationStatus === "waitlist" ? (
                    <div className="bg-amber-50 border border-amber-200 rounded-lg p-4">
                      <p className="text-amber-900 font-medium mb-2">
                        {reservationT("waitlistReceived")}
                      </p>
                      <p className="text-amber-800 text-sm">
                        {reservationT("waitlistFollowUp")}
                      </p>
                    </div>
                  ) : settings?.approval_mode === "manual" ? (
                    <div className="bg-yellow-50 border border-yellow-200 rounded-lg p-4">
                      <p className="text-yellow-900 font-medium mb-2">
                        {modalT(
                          "confirmationRequiredTitle",
                          fallbackCopy.confirmationRequiredTitle,
                        )}
                      </p>
                      <p className="text-yellow-800 text-sm">
                        {modalT(
                          "confirmationRequiredDesc",
                          fallbackCopy.confirmationRequiredDesc,
                        )}
                      </p>
                      {pendingDeadline && formatDeadline(pendingDeadline) ? (
                        <p className="text-yellow-800 text-sm mt-2 font-medium">
                          {reservationT("hearBackBy", {
                            deadline: formatDeadline(pendingDeadline)!,
                          })}
                        </p>
                      ) : null}
                    </div>
                  ) : (
                    <div className="bg-green-50 border border-green-200 rounded-lg p-4">
                      <p className="text-green-900 font-medium mb-2">
                        {modalT("confirmedTitle", fallbackCopy.confirmedTitle)}
                      </p>
                      <p className="text-green-800 text-sm">
                        {modalT("confirmedDesc", fallbackCopy.confirmedDesc)}
                      </p>
                    </div>
                  )}

                  <div className="bg-gray-50 rounded-lg p-4 space-y-2">
                    <p className="font-semibold text-gray-900">
                      {modalT("details", fallbackCopy.details)}
                    </p>
                    <div className="text-sm text-gray-700 space-y-1">
                      <p>
                        • {modalT("name", fallbackCopy.name)}: {customerName}
                      </p>
                      <p>
                        • {modalT("party", fallbackCopy.party)}: {partySize}{" "}
                        {partySize === 1
                          ? reservationT("guest")
                          : reservationT("guests")}
                      </p>
                      <p>
                        • {modalT("date", fallbackCopy.modalDate)}:{" "}
                        {selectedDateLabel}
                      </p>
                      <p>
                        • {modalT("time", fallbackCopy.modalTime)}:{" "}
                        {selectedTime
                          ? formatTimeSlot(
                              selectedTime,
                              timezone,
                              currentLanguage,
                            )
                          : ""}
                      </p>
                      {reservationId ? (
                        <p>
                          • {reservationT("requestIdLabel")}: #{reservationId}
                        </p>
                      ) : null}
                      {confirmationCode ? (
                        <p>
                          • {reservationT("confirmationCode")}:{" "}
                          {confirmationCode}
                        </p>
                      ) : null}
                      {reservationStatus === "waitlist" ? (
                        <p>
                          • {reservationT("statusLabel")}:{" "}
                          {reservationT("statusWaitlistValue")}
                        </p>
                      ) : null}
                    </div>
                  </div>

                  <div className="bg-brand/10 border border-brand/20 rounded-lg p-4">
                    <p className="text-brand-dark text-sm">
                      <strong>
                        {modalT("important", fallbackCopy.important)}
                      </strong>{" "}
                      {reservationStatus === "waitlist"
                        ? modalT(
                            "importantWaitlistDesc",
                            fallbackCopy.modalImportantWaitlistDesc,
                          )
                        : modalT("importantDesc", fallbackCopy.importantDesc)}
                    </p>
                  </div>
                </div>
              </ModalBody>
              <ModalFooter className="flex flex-wrap gap-3">
                {confirmationCode ? (
                  <Button
                    color="primary"
                    variant="flat"
                    onPress={() => {
                      window.location.assign(
                        guestReservationDetailsPath(
                          confirmationCode,
                          currentLanguage || "en",
                        ),
                      );
                    }}
                    className="font-semibold"
                  >
                    {reservationT("viewReservation")}
                  </Button>
                ) : null}
                <Button
                  variant="bordered"
                  onPress={() => {
                    onClose();
                    window.scrollTo({ top: 0, behavior: "smooth" });
                  }}
                  className="font-semibold"
                >
                  {modalT("viewInfo", fallbackCopy.viewInfo)}
                </Button>
                <Button
                  variant="flat"
                  onPress={downloadCalendarInvite}
                  startContent={<CalendarPlus className="w-4 h-4" />}
                >
                  {reservationT("addToCalendar")}
                </Button>
                <Button variant="light" onPress={openGoogleCalendarInvite}>
                  {reservationT("addToGoogleCalendar")}
                </Button>
                <Button
                  color="primary"
                  onPress={() => {
                    onClose();
                    // Switch the storefront to the menu tab via the wired
                    // callback. The old querySelector('[data-tab="menu"]') hit
                    // nothing (the tabs use id="tab-menu" + onClick), so this
                    // CTA silently did nothing.
                    onViewMenu?.();
                  }}
                  className="bg-brand text-white font-semibold hover:bg-brand-dark"
                  endContent={<Utensils className="w-4 h-4" />}
                >
                  {modalT("checkMenu", fallbackCopy.checkMenu)}
                </Button>
              </ModalFooter>
            </>
          )}
        </ModalContent>
      </Modal>
    </>
  );
}
