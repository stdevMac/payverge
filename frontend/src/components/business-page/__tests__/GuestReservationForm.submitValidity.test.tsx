/** @jest-environment jsdom */
/**
 * GUEST-006 — reservation submit affordance uses the same phone/email
 * validators as handleSubmit (disabled when malformed; re-enables when fixed).
 */
import React from "react";
import { render, screen, fireEvent, waitFor } from "@testing-library/react";
import GuestReservationForm from "../GuestReservationForm";

const mockTranslation = { t: (k: string) => k };
jest.mock("@/i18n/GuestTranslationProvider", () => ({
  useGuestTranslation: () => mockTranslation,
}));

jest.mock("react-hot-toast", () => ({
  __esModule: true,
  default: { error: jest.fn(), success: jest.fn() },
  error: jest.fn(),
  success: jest.fn(),
}));

const mockSlotDate = new Date();
mockSlotDate.setDate(mockSlotDate.getDate() + 1);
mockSlotDate.setHours(19, 0, 0, 0);
const mockSlotTime = mockSlotDate.toISOString();

const mockCreateReservation = jest.fn();
const mockApprovalMode: { value: "manual" | "auto" } = { value: "manual" };

jest.mock("@/api/reservations", () => ({
  guestReservationAPI: {
    getSettings: jest.fn().mockImplementation(() =>
      Promise.resolve({
        enabled: true,
        min_party_size: 1,
        max_party_size: 8,
        max_advance_days: 30,
        min_advance_minutes: 0,
        approval_mode: mockApprovalMode.value,
        allow_cancellation: true,
        allow_waitlist: false,
        default_duration: 60,
        cancellation_deadline: 24,
        external_partner_links: [],
      }),
    ),
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

const isDisabled = (button: HTMLElement) =>
  button.hasAttribute("disabled") ||
  button.getAttribute("aria-disabled") === "true";

const slotButtons = () =>
  (
    Array.from(
      document.querySelectorAll("button[aria-pressed]"),
    ) as HTMLButtonElement[]
  ).filter(
    (b) => !b.closest('[aria-labelledby="reservation-quick-dates-label"]'),
  );

async function goToContactStep() {
  render(<GuestReservationForm customUrl="test" businessName="Test" />);
  fireEvent.click(await screen.findByText("Tomorrow"));
  await waitFor(() => {
    expect(slotButtons().length).toBeGreaterThan(0);
  });
  const slot = slotButtons().find(
    (b) => b.getAttribute("aria-disabled") !== "true",
  );
  expect(slot).toBeTruthy();
  fireEvent.click(slot!);
  await screen.findByPlaceholderText("John Doe");
}

describe.each([
  ["manual-approval", "manual" as const],
  ["instant-confirmation", "auto" as const],
])("GuestReservationForm submit validity (%s)", (_label, mode) => {
  beforeEach(() => {
    mockApprovalMode.value = mode;
    mockCreateReservation.mockReset();
    mockCreateReservation.mockResolvedValue({
      id: 7,
      status: mode === "manual" ? "pending" : "confirmed",
      confirmation_code: "XYZ999",
      approval_deadline:
        mode === "manual" ? "2026-06-11T18:00:00Z" : undefined,
    });
  });

  it("keeps submit disabled for blank email and malformed phone/email", async () => {
    await goToContactStep();
    const name = screen.getByPlaceholderText("John Doe");
    const phone = screen.getByPlaceholderText("+1 (555) 123-4567");
    const email = screen.getByPlaceholderText("john@example.com");
    const submit = () =>
      screen.getByRole("button", { name: /Book/ });

    fireEvent.change(name, { target: { value: "Alex" } });
    fireEvent.change(phone, { target: { value: "555" } });
    fireEvent.change(email, { target: { value: "not-an-email" } });
    fireEvent.blur(phone);
    fireEvent.blur(email);

    expect(isDisabled(submit())).toBe(true);
    expect(
      screen.getByText("Please enter a valid phone number."),
    ).toBeInTheDocument();
    expect(
      screen.getByText("Please enter a valid email address."),
    ).toBeInTheDocument();

    fireEvent.change(email, { target: { value: "" } });
    expect(isDisabled(submit())).toBe(true);
    fireEvent.click(submit());
    expect(mockCreateReservation).not.toHaveBeenCalled();
  });

  it("enables submit after corrected values and submits successfully", async () => {
    await goToContactStep();
    const name = screen.getByPlaceholderText("John Doe");
    const phone = screen.getByPlaceholderText("+1 (555) 123-4567");
    const email = screen.getByPlaceholderText("john@example.com");
    const submit = () =>
      screen.getByRole("button", { name: /Book/ });

    fireEvent.change(name, { target: { value: "Alex Guest" } });
    fireEvent.change(phone, { target: { value: "555" } });
    fireEvent.change(email, { target: { value: "bad" } });
    fireEvent.blur(phone);
    fireEvent.blur(email);
    expect(isDisabled(submit())).toBe(true);

    fireEvent.change(phone, { target: { value: "+1 555 000 9999" } });
    fireEvent.change(email, { target: { value: "alex@example.com" } });
    fireEvent.blur(phone);
    fireEvent.blur(email);
    expect(isDisabled(submit())).toBe(false);

    fireEvent.click(submit());

    await waitFor(() => {
      expect(mockCreateReservation).toHaveBeenCalledTimes(1);
    });
    expect(mockCreateReservation.mock.calls[0][1]).toMatchObject({
      customer_email: "alex@example.com",
      customer_phone: "+1 555 000 9999",
    });
  });
});
