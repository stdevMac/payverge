/** @jest-environment jsdom */
/**
 * Guest reservation picker UX: #397 recommended sparsity, #408 waitlist
 * policy copy, #409 party-size remount/focus, #410 stale live region,
 * #414 confirmed-status promise before a slot is selected.
 */
import React from "react";
import { act, fireEvent, render, screen, waitFor, within } from "@testing-library/react";
import GuestReservationForm from "../GuestReservationForm";
import { guestReservationAPI } from "@/api/reservations";
import { localDateKey } from "@/lib/localDate";

type Slot = {
  time: string;
  available_tables: number;
  recommended: boolean;
  reason_code: string;
};

const translationState: { locale: "en" | "es" } = { locale: "en" };

const ES: Record<string, string> = {
  "businessPage.info.reservationForm.summaryStatusConfirmed":
    "Reserva confirmada",
  "businessPage.info.reservationForm.summaryStatusWaitlist":
    "Solicitud para lista de espera",
  "businessPage.info.reservationForm.summaryStatusPending":
    "Confirmación pendiente",
  "businessPage.info.reservationForm.summaryStatusChooseSlot":
    "Elige un horario para ver el estado",
  "businessPage.info.reservationForm.arriveTime":
    "Llega dentro de los 15 minutos de tu horario de reserva",
  "businessPage.info.reservationForm.cancellationPolicy":
    "Las cancelaciones requieren {hours} horas de aviso",
  "businessPage.info.reservationForm.waitlistDoNotArrive":
    "No llegues al restaurante hasta que confirmen una mesa.",
  "businessPage.info.reservationForm.waitlistWaitForContact":
    "Espera a que te contacten si se libera una mesa.",
  "businessPage.info.reservationForm.loadingAvailability":
    "Cargando disponibilidad para {date}",
};

const translation = {
  t: (key: string, params?: Record<string, string | number>) => {
    let out = translationState.locale === "es" ? (ES[key] ?? key) : key;
    if (params) {
      for (const [name, value] of Object.entries(params)) {
        out = out.replace(new RegExp(`\\{\\{?${name}\\}?\\}`, "g"), String(value));
      }
    }
    return out;
  },
};

jest.mock("@/i18n/GuestTranslationProvider", () => ({
  useGuestTranslation: () => translation,
}));

jest.mock("react-hot-toast", () => ({
  __esModule: true,
  default: { error: jest.fn(), success: jest.fn() },
  error: jest.fn(),
  success: jest.fn(),
}));

jest.mock("@/api/reservations", () => ({
  guestReservationAPI: {
    getSettings: jest.fn(),
    getAvailability: jest.fn(),
    createReservation: jest.fn(),
  },
}));

const WAIT = { timeout: 15_000 } as const;

const defaultSettings = {
  enabled: true,
  min_party_size: 1,
  max_party_size: 12,
  max_advance_days: 30,
  min_advance_minutes: 0,
  approval_mode: "auto",
  allow_cancellation: true,
  allow_waitlist: true,
  default_duration: 60,
  cancellation_deadline: 24,
  external_partner_links: [],
};

function isoAt(hours: number, minutes = 0, dayOffset = 1): string {
  const date = new Date();
  date.setDate(date.getDate() + dayOffset);
  date.setHours(hours, minutes, 0, 0);
  return date.toISOString();
}

function openSlot(time: string, tables: number, recommended = true): Slot {
  return {
    time,
    available_tables: tables,
    recommended,
    reason_code: "available",
  };
}

function waitlistSlot(time: string): Slot {
  return {
    time,
    available_tables: 0,
    recommended: false,
    reason_code: "covers_limit",
  };
}

function availabilityPayload(
  slots: Slot[],
  extras: { next?: string | null; waitlist?: boolean } = {},
) {
  const nextOpen = extras.next ?? slots.find((slot) => slot.available_tables > 0)?.time ?? null;
  return {
    available_slots: slots,
    waitlist_available: extras.waitlist ?? slots.some((slot) => slot.available_tables === 0),
    next_available_slot: nextOpen,
  };
}

function tomorrowKey() {
  const date = new Date();
  date.setDate(date.getDate() + 1);
  return localDateKey(date);
}

function todayKey() {
  return localDateKey();
}

function slotButtons() {
  return (
    Array.from(document.querySelectorAll("button[aria-pressed]")) as HTMLButtonElement[]
  ).filter((button) => !button.closest('[aria-labelledby="reservation-quick-dates-label"]'));
}

function recommendedBadges() {
  return screen.queryAllByText("Recommended");
}

function primeSettings(overrides: Partial<typeof defaultSettings> = {}) {
  (guestReservationAPI.getSettings as jest.Mock).mockResolvedValue({
    ...defaultSettings,
    ...overrides,
  });
}

async function renderForm() {
  const view = render(
    <GuestReservationForm
      customUrl="demo-kitchen"
      businessName="Core Demo Kitchen"
      timezone="UTC"
    />,
  );
  await screen.findByText("Tomorrow", {}, WAIT);
  return view;
}

async function chooseTomorrow() {
  fireEvent.click(await screen.findByRole("button", { name: "Tomorrow" }, WAIT));
}

async function chooseToday() {
  fireEvent.click(await screen.findByRole("button", { name: "Today" }, WAIT));
}

function setPartySize(container: HTMLElement, size: number) {
  const select = container.querySelector("select") as HTMLSelectElement | null;
  expect(select).not.toBeNull();
  fireEvent.change(select as HTMLSelectElement, { target: { value: String(size) } });
}

function statusLine() {
  return screen.getByText(/Status on submit:/).closest("p")?.textContent ?? "";
}

async function resolvePending(resolve: ((value: unknown) => void) | null, value: unknown) {
  expect(resolve).not.toBeNull();
  await act(async () => {
    resolve!(value);
  });
}

describe("GuestReservationForm availability UX", () => {
  beforeEach(() => {
    translationState.locale = "en";
    jest.clearAllMocks();
    primeSettings();
    (guestReservationAPI.getAvailability as jest.Mock).mockResolvedValue(
      availabilityPayload([openSlot(isoAt(19), 2, false)]),
    );
  });

  describe("#397 recommended badge is sparse", () => {
    it("shows no Recommended badge when every open slot has the same capacity", async () => {
      const slots = [11, 12, 13, 17, 18, 19, 20, 21].map((hour) =>
        openSlot(isoAt(hour), 9, true),
      );
      (guestReservationAPI.getAvailability as jest.Mock).mockResolvedValue(
        availabilityPayload(slots, { waitlist: false }),
      );

      await renderForm();
      await chooseTomorrow();
      await waitFor(() => expect(slotButtons().length).toBe(slots.length), WAIT);

      expect(recommendedBadges()).toHaveLength(0);
    });

    it("badges only the soonest higher-capacity slot when capacities differ", async () => {
      const soonerHigh = isoAt(17, 30);
      const laterHigh = isoAt(18, 0);
      const slots = [
        openSlot(isoAt(17, 0), 9, true),
        openSlot(soonerHigh, 10, true),
        openSlot(laterHigh, 10, true),
        openSlot(isoAt(18, 30), 9, true),
      ];
      (guestReservationAPI.getAvailability as jest.Mock).mockResolvedValue(
        availabilityPayload(slots, { waitlist: false }),
      );

      await renderForm();
      await chooseTomorrow();
      await waitFor(() => expect(slotButtons().length).toBe(4), WAIT);

      expect(recommendedBadges()).toHaveLength(1);
      const recommendedCard = recommendedBadges()[0].closest("button");
      expect(recommendedCard).not.toBeNull();
      expect(recommendedCard?.textContent).toMatch(/10 tables ready/i);
    });

    it("does not badge waitlist-only or fully equivalent constrained windows", async () => {
      const slots = [
        waitlistSlot(isoAt(11)),
        waitlistSlot(isoAt(12)),
        waitlistSlot(isoAt(13)),
      ];
      (guestReservationAPI.getAvailability as jest.Mock).mockResolvedValue(
        availabilityPayload(slots, { next: null, waitlist: true }),
      );

      await renderForm();
      await chooseTomorrow();
      await waitFor(() => expect(slotButtons().length).toBe(3), WAIT);

      expect(recommendedBadges()).toHaveLength(0);
    });
  });

  describe("#408 waitlist review policy", () => {
    it("replaces arrival and cancellation rules with waitlist-only notes", async () => {
      const waitlisted = isoAt(11);
      (guestReservationAPI.getAvailability as jest.Mock).mockResolvedValue(
        availabilityPayload(
          [waitlistSlot(waitlisted), waitlistSlot(isoAt(12))],
          { next: null, waitlist: true },
        ),
      );

      await renderForm();
      await chooseTomorrow();
      await waitFor(() => expect(slotButtons().length).toBe(2), WAIT);
      fireEvent.click(slotButtons()[0]);

      const notes = screen.getByText("Please Note:").closest("div");
      expect(notes).not.toBeNull();
      expect(
        within(notes as HTMLElement).getByText(
          /Do not arrive unless the restaurant confirms a table/i,
        ),
      ).toBeInTheDocument();
      expect(
        within(notes as HTMLElement).getByText(
          /Wait to be contacted if a table becomes available/i,
        ),
      ).toBeInTheDocument();
      expect(notes).not.toHaveTextContent("Please arrive within 15 minutes");
      expect(notes).not.toHaveTextContent("Cancellations must be made");
    });

    it("keeps arrival and cancellation rules for an open confirmed slot", async () => {
      const open = isoAt(19);
      (guestReservationAPI.getAvailability as jest.Mock).mockResolvedValue(
        availabilityPayload([openSlot(open, 4, false)], { waitlist: true }),
      );

      await renderForm();
      await chooseTomorrow();
      await waitFor(() => expect(slotButtons().length).toBe(1), WAIT);
      fireEvent.click(slotButtons()[0]);

      const notes = screen.getByText("Please Note:").closest("div");
      expect(notes).toHaveTextContent("Please arrive within 15 minutes");
      expect(notes).toHaveTextContent("Cancellations must be made at least 24 hours");
      expect(notes).not.toHaveTextContent("Do not arrive unless");
    });

    it("restores waitlist Spanish notes when the guest locale is es", async () => {
      translationState.locale = "es";
      const waitlisted = isoAt(11);
      (guestReservationAPI.getAvailability as jest.Mock).mockResolvedValue(
        availabilityPayload([waitlistSlot(waitlisted)], { next: null, waitlist: true }),
      );

      await renderForm();
      await chooseTomorrow();
      await waitFor(() => expect(slotButtons().length).toBe(1), WAIT);
      fireEvent.click(slotButtons()[0]);

      expect(
        screen.getByText(/No llegues al restaurante hasta que confirmen una mesa/),
      ).toBeInTheDocument();
      expect(screen.queryByText(/Llega dentro de los 15 minutos/)).toBeNull();
    });
  });

  describe("#414 status stays neutral until a slot is chosen", () => {
    it("does not promise Confirmed booking on a waitlist-only day before a time is selected", async () => {
      const slots = Array.from({ length: 4 }, (_, index) => waitlistSlot(isoAt(11 + index)));
      (guestReservationAPI.getAvailability as jest.Mock).mockResolvedValue(
        availabilityPayload(slots, { next: null, waitlist: true }),
      );

      await renderForm();
      await chooseTomorrow();
      await waitFor(() => expect(slotButtons().length).toBe(4), WAIT);

      expect(statusLine()).toMatch(/Choose a slot to see status/i);
      expect(screen.queryByText("Confirmed booking")).toBeNull();

      fireEvent.click(slotButtons()[0]);
      expect(statusLine()).toMatch(/Waitlist request/i);
    });

    it("resolves open slots to confirmed or pending after selection", async () => {
      primeSettings({ approval_mode: "manual" });
      (guestReservationAPI.getAvailability as jest.Mock).mockResolvedValue(
        availabilityPayload([openSlot(isoAt(19), 3, false)], { waitlist: false }),
      );

      await renderForm();
      await chooseTomorrow();
      await waitFor(() => expect(slotButtons().length).toBe(1), WAIT);

      expect(statusLine()).toMatch(/Choose a slot to see status/i);
      fireEvent.click(slotButtons()[0]);
      expect(statusLine()).toMatch(/Pending confirmation/i);
    });

    it("returns to the Spanish neutral status when party size clears the slot", async () => {
      translationState.locale = "es";
      let partySeen = 2;
      (guestReservationAPI.getAvailability as jest.Mock).mockImplementation(
        (_url: string, _date: string, party: number) => {
          partySeen = party;
          return Promise.resolve(
            availabilityPayload(
              party >= 12
                ? [waitlistSlot(isoAt(11))]
                : [openSlot(isoAt(19), 4, false)],
              { waitlist: party >= 12, next: party >= 12 ? null : isoAt(19) },
            ),
          );
        },
      );

      const { container } = await renderForm();
      await chooseTomorrow();
      await waitFor(() => expect(slotButtons().length).toBe(1), WAIT);
      fireEvent.click(slotButtons()[0]);
      expect(statusLine()).toMatch(/Reserva confirmada/i);

      setPartySize(container, 12);
      await waitFor(() => expect(partySeen).toBe(12), WAIT);
      expect(statusLine()).toMatch(/Elige un horario para ver el estado/i);
      expect(screen.queryByText("Reserva confirmada")).toBeNull();
    });
  });

  describe("#409 party-size change keeps the form mounted", () => {
    it("does not refetch settings or unmount the shell while availability reloads", async () => {
      let resolveAvailability: ((value: unknown) => void) | null = null;
      let availabilityCalls = 0;
      (guestReservationAPI.getAvailability as jest.Mock).mockImplementation(
        (_url: string, _date: string, party: number) => {
          availabilityCalls += 1;
          if (availabilityCalls === 1) {
            return Promise.resolve(
              availabilityPayload([openSlot(isoAt(19), 4, false)]),
            );
          }
          return new Promise((resolve) => {
            resolveAvailability = resolve;
            void party;
          });
        },
      );

      const { container } = await renderForm();
      expect(guestReservationAPI.getSettings).toHaveBeenCalledTimes(1);

      const name = await screen.findByPlaceholderText("John Doe", {}, WAIT);
      fireEvent.change(name, { target: { value: "Ada Guest" } });

      await chooseTomorrow();
      await waitFor(() => expect(slotButtons().length).toBe(1), WAIT);

      const select = container.querySelector("select") as HTMLSelectElement;
      select.focus();
      setPartySize(container, 4);

      await waitFor(() => expect(resolveAvailability).not.toBeNull(), WAIT);

      expect(guestReservationAPI.getSettings).toHaveBeenCalledTimes(1);
      expect(screen.getByPlaceholderText("John Doe")).toHaveValue("Ada Guest");
      expect(screen.getByText("Party Size")).toBeInTheDocument();
      expect(container.querySelector("select")).not.toBeNull();
      expect(document.body.contains(select)).toBe(true);

      await resolvePending(
        resolveAvailability,
        availabilityPayload([openSlot(isoAt(18), 3, false), openSlot(isoAt(19), 3, false)]),
      );
      await waitFor(() => expect(slotButtons().length).toBe(2), WAIT);
      expect(screen.getByPlaceholderText("John Doe")).toHaveValue("Ada Guest");
    });
  });

  describe("#410 live region is pending while slots reload", () => {
    it("replaces stale Today counts while Tomorrow is loading", async () => {
      const todaySlots = [
        openSlot(isoAt(17, 30, 0), 2, false),
        openSlot(isoAt(18, 0, 0), 2, false),
      ];
      const tomorrowSlots = Array.from({ length: 5 }, (_, index) =>
        openSlot(isoAt(11 + index, 0, 1), 4, false),
      );

      let resolveTomorrow: ((value: unknown) => void) | null = null;
      (guestReservationAPI.getAvailability as jest.Mock).mockImplementation(
        (_url: string, date: string) => {
          if (date === todayKey()) {
            return Promise.resolve(
              availabilityPayload(todaySlots, {
                next: todaySlots[0].time,
                waitlist: false,
              }),
            );
          }
          return new Promise((resolve) => {
            resolveTomorrow = resolve;
          });
        },
      );

      await renderForm();
      await chooseToday();
      await waitFor(() => expect(screen.getByText("2 open")).toBeInTheDocument(), WAIT);

      await chooseTomorrow();

      await waitFor(() => {
        expect(screen.getByText(/Loading availability for/i)).toBeInTheDocument();
      }, WAIT);
      expect(screen.queryByText("2 open")).toBeNull();
      const busyRegion = document.querySelector('[aria-busy="true"]');
      expect(busyRegion).not.toBeNull();

      await resolvePending(
        resolveTomorrow,
        availabilityPayload(tomorrowSlots, {
          next: tomorrowSlots[0].time,
          waitlist: false,
        }),
      );
      await waitFor(() => expect(screen.getByText("5 open")).toBeInTheDocument(), WAIT);
      expect(screen.queryByText(/Loading availability for/i)).toBeNull();
    });

    it("does not flash 0 open on a cold date request", async () => {
      let resolveSlots: ((value: unknown) => void) | null = null;
      (guestReservationAPI.getAvailability as jest.Mock).mockImplementation(
        () =>
          new Promise((resolve) => {
            resolveSlots = resolve;
          }),
      );

      await renderForm();
      await chooseTomorrow();

      await waitFor(() => {
        expect(screen.getByText(/Loading availability for/i)).toBeInTheDocument();
      }, WAIT);
      expect(screen.queryByText("0 open")).toBeNull();
      expect(screen.queryByText("0 waitlist-only")).toBeNull();

      await resolvePending(
        resolveSlots,
        availabilityPayload([openSlot(isoAt(19), 3, false)], { waitlist: false }),
      );
      await waitFor(() => expect(screen.getByText("1 open")).toBeInTheDocument(), WAIT);
    });

    it("ignores an out-of-order slower response after a newer date is chosen", async () => {
      const pending = new Map<string, (value: unknown) => void>();
      (guestReservationAPI.getAvailability as jest.Mock).mockImplementation(
        (_url: string, date: string) =>
          new Promise((resolve) => {
            pending.set(date, resolve);
          }),
      );

      const { container } = await renderForm();
      await chooseToday();
      await waitFor(() => expect(pending.has(todayKey())).toBe(true), WAIT);
      await resolvePending(
        pending.get(todayKey()) ?? null,
        availabilityPayload([openSlot(isoAt(17, 30, 0), 7, false)], {
          waitlist: false,
        }),
      );
      await waitFor(() => expect(screen.getByText("1 open")).toBeInTheDocument(), WAIT);

      await chooseTomorrow();
      await waitFor(() => expect(pending.has(tomorrowKey())).toBe(true), WAIT);

      const later = new Date();
      later.setDate(later.getDate() + 3);
      const laterKey = localDateKey(later);
      const dateInput = container.querySelector('input[type="date"]') as HTMLInputElement;
      fireEvent.change(dateInput, { target: { value: laterKey } });
      await waitFor(() => expect(pending.has(laterKey)).toBe(true), WAIT);

      await resolvePending(
        pending.get(laterKey) ?? null,
        availabilityPayload(
          Array.from({ length: 3 }, (_, index) => openSlot(isoAt(18 + index, 0, 3), 2, false)),
          { waitlist: false },
        ),
      );
      await waitFor(() => expect(screen.getByText("3 open")).toBeInTheDocument(), WAIT);

      await resolvePending(
        pending.get(tomorrowKey()) ?? null,
        availabilityPayload(
          Array.from({ length: 8 }, (_, index) => waitlistSlot(isoAt(11 + index))),
          { next: null, waitlist: true },
        ),
      );
      await new Promise((resolve) => setTimeout(resolve, 40));
      expect(screen.getByText("3 open")).toBeInTheDocument();
      expect(screen.queryByText("8 waitlist-only")).toBeNull();
    });

    it("announces a waitlist-only result only after the request settles", async () => {
      let resolveSlots: ((value: unknown) => void) | null = null;
      (guestReservationAPI.getAvailability as jest.Mock).mockImplementation(
        () =>
          new Promise((resolve) => {
            resolveSlots = resolve;
          }),
      );

      await renderForm();
      await chooseTomorrow();
      await waitFor(() => {
        expect(screen.getByText(/Loading availability for/i)).toBeInTheDocument();
      }, WAIT);

      await resolvePending(
        resolveSlots,
        availabilityPayload(
          [waitlistSlot(isoAt(11)), waitlistSlot(isoAt(12))],
          { next: null, waitlist: true },
        ),
      );
      await waitFor(() => expect(screen.getByText("0 open")).toBeInTheDocument(), WAIT);
      expect(screen.getByText("2 waitlist-only")).toBeInTheDocument();
    });
  });
});
