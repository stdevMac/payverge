/** @jest-environment jsdom */
/**
 * i18n Batch B Task 3: create payload carries the guest language so the
 * backend can persist it and stamp email deep links with ?lang=.
 */
import React from "react";
import { render, screen, fireEvent, waitFor } from "@testing-library/react";
import GuestReservationForm from "../GuestReservationForm";

const mockTranslation = {
  t: (k: string) => k,
  currentLanguage: "es",
};
jest.mock("@/i18n/GuestTranslationProvider", () => ({
  useGuestTranslation: () => mockTranslation,
}));

jest.mock("react-hot-toast", () => ({
  __esModule: true,
  default: { error: jest.fn(), success: jest.fn() },
  error: jest.fn(),
  success: jest.fn(),
}));

const slotDate = new Date();
slotDate.setDate(slotDate.getDate() + 1);
slotDate.setHours(19, 0, 0, 0);
const mockSlotTime = slotDate.toISOString();

const mockCreateReservation = jest.fn().mockResolvedValue({
  id: 42,
  status: "confirmed",
  confirmation_code: "ABC123",
});

jest.mock("@/api/reservations", () => ({
  guestReservationAPI: {
    getSettings: jest.fn().mockResolvedValue({
      enabled: true,
      min_party_size: 1,
      max_party_size: 8,
      max_advance_days: 30,
      min_advance_minutes: 0,
      approval_mode: "auto",
      allow_cancellation: true,
      allow_waitlist: false,
      default_duration: 60,
      cancellation_deadline: 24,
      external_partner_links: [],
    }),
    getAvailability: jest.fn().mockImplementation(() =>
      Promise.resolve({
        available_slots: [
          { time: mockSlotTime, available_tables: 2, recommended: false },
        ],
        waitlist_available: false,
        next_available_slot: null,
      }),
    ),
    createReservation: (...args: unknown[]) => mockCreateReservation(...args),
  },
}));

async function renderAtContactStep() {
  render(<GuestReservationForm customUrl="test" businessName="Test" />);

  fireEvent.click(await screen.findByText("Tomorrow"));
  await waitFor(() => {
    const slots = Array.from(
      document.querySelectorAll("button[aria-pressed]"),
    ).filter(
      (b) => !b.closest('[aria-labelledby="reservation-quick-dates-label"]'),
    );
    expect(slots.length).toBeGreaterThan(0);
  });
  const slotButton = Array.from(
    document.querySelectorAll("button[aria-pressed]"),
  ).find(
    (b) =>
      !b.closest('[aria-labelledby="reservation-quick-dates-label"]') &&
      b.getAttribute("aria-disabled") !== "true",
  ) as HTMLElement;
  fireEvent.click(slotButton);

  const nameInput = await screen.findByPlaceholderText("John Doe");
  fireEvent.change(nameInput, { target: { value: "Jane Guest" } });
  const phoneInput = screen.getByPlaceholderText("+1 (555) 123-4567");
  fireEvent.change(phoneInput, { target: { value: "+1 555 000 1234" } });
}

describe("GuestReservationForm language continuity (Batch B)", () => {
  beforeEach(() => {
    jest.clearAllMocks();
    mockCreateReservation.mockResolvedValue({
      id: 42,
      status: "confirmed",
      confirmation_code: "ABC123",
    });
  });

  it("includes language: es in the create payload", async () => {
    await renderAtContactStep();

    fireEvent.change(screen.getByPlaceholderText("john@example.com"), {
      target: { value: "jane@example.com" },
    });
    fireEvent.click(screen.getByRole("button", { name: /Book/ }));

    await waitFor(() => {
      expect(mockCreateReservation).toHaveBeenCalledTimes(1);
    });
    expect(mockCreateReservation.mock.calls[0][1]).toMatchObject({
      language: "es",
    });
  });
});
