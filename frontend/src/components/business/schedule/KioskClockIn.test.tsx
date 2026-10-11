/** @jest-environment jsdom */
import React from "react";
import { render, screen, waitFor, fireEvent } from "@testing-library/react";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import KioskClockIn from "./KioskClockIn";
import {
  timeclockApi,
  type KioskStaffMember,
  type KioskPunchResult,
} from "@/api/timeclock";

const COPY: Record<string, string> = {
  title: "Clock in / out",
  subtitle: "Tap your name, then enter your PIN",
  close: "Close",
  back: "Back",
  cancel: "Cancel",
  enterPin: "Enter your PIN",
  pinEntry: "PIN entry",
  backspace: "Delete",
  clockIn: "Clock in",
  clockOut: "Clock out",
  since: "Since {time}",
  offClock: "Tap to clock in",
  needPinNote: "{count} more staff still need a PIN",
  emptyTitle: "No staff have a PIN yet",
  emptyBody: "Set a PIN for each staffer in Team to use the clock-in kiosk.",
  clockedInFlash: "{name} clocked in",
  clockedOutFlash: "{name} clocked out",
  errWrongPin: "Incorrect PIN. Try again.",
  errLocked: "Too many attempts. Try again in a few minutes.",
  errNotFound: "That staffer wasn't found.",
  errNoPin: "This staffer hasn't set a PIN yet.",
  errGeneric: "Couldn't record that. Try again.",
};

jest.mock("@/i18n/SimpleTranslationProvider", () => ({
  useSimpleLocale: () => ({ locale: "en", setLocale: jest.fn() }),
  getTranslation: (key: string) =>
    COPY[key.replace("dashboardKiosk.", "")] ?? key,
}));

jest.mock("@/api/timeclock", () => ({
  timeclockApi: { kioskRoster: jest.fn(), kioskPunch: jest.fn() },
}));

const mockedApi = timeclockApi as unknown as {
  kioskRoster: jest.Mock;
  kioskPunch: jest.Mock;
};

const roster: KioskStaffMember[] = [
  { staff_id: 7, name: "Dana", role: "server", has_pin: true, on_clock: false },
  {
    staff_id: 8,
    name: "Eli",
    role: "server",
    has_pin: true,
    on_clock: true,
    clock_in_at: "2026-06-30T17:00:00.000Z",
  },
  { staff_id: 9, name: "Fio", role: "server", has_pin: false, on_clock: false },
];

function renderKiosk() {
  const onClose = jest.fn();
  const qc = new QueryClient({
    defaultOptions: { queries: { retry: false }, mutations: { retry: false } },
  });
  render(
    <QueryClientProvider client={qc}>
      <KioskClockIn businessId="42" onClose={onClose} />
    </QueryClientProvider>,
  );
  return { onClose };
}

beforeEach(() => {
  jest.clearAllMocks();
  mockedApi.kioskRoster.mockResolvedValue(roster);
});

async function enterPin(digits: string) {
  for (const d of digits) {
    fireEvent.click(screen.getByRole("button", { name: d }));
  }
}

test("shows only PIN-enrolled staff as tiles, with a footnote for the rest", async () => {
  renderKiosk();
  expect(await screen.findByRole("button", { name: /Dana/ })).toBeInTheDocument();
  expect(screen.getByRole("button", { name: /Eli/ })).toBeInTheDocument();
  // Fio has no PIN → not a tappable tile.
  expect(screen.queryByRole("button", { name: /Fio/ })).toBeNull();
  expect(screen.getByText("1 more staff still need a PIN")).toBeInTheDocument();
});

test("tap a name → enter PIN → clock in, and shows the success flash", async () => {
  const result: KioskPunchResult = {
    action: "clocked_in",
    staff_id: 7,
    name: "Dana",
    entry: {} as KioskPunchResult["entry"],
  };
  mockedApi.kioskPunch.mockResolvedValue(result);
  renderKiosk();

  fireEvent.click(await screen.findByRole("button", { name: /Dana/ }));
  expect(screen.getByText("Enter your PIN")).toBeInTheDocument();

  await enterPin("1234");
  fireEvent.click(screen.getByRole("button", { name: "Clock in" }));

  await waitFor(() =>
    expect(mockedApi.kioskPunch).toHaveBeenCalledWith("42", 7, "1234"),
  );
  expect(await screen.findByText("Dana clocked in")).toBeInTheDocument();
});

test("an on-clock staffer's action button reads Clock out", async () => {
  renderKiosk();
  fireEvent.click(await screen.findByRole("button", { name: /Eli/ }));
  await enterPin("1234");
  expect(screen.getByRole("button", { name: "Clock out" })).toBeInTheDocument();
});

test("wrong PIN (403) shows the incorrect-PIN message and stays on the pad", async () => {
  mockedApi.kioskPunch.mockRejectedValue({ response: { status: 403 } });
  renderKiosk();

  fireEvent.click(await screen.findByRole("button", { name: /Dana/ }));
  await enterPin("0000");
  fireEvent.click(screen.getByRole("button", { name: "Clock in" }));

  expect(await screen.findByText("Incorrect PIN. Try again.")).toBeInTheDocument();
  // Still on the pad (the name prompt is visible), no flash.
  expect(screen.getByText("Enter your PIN")).toBeInTheDocument();
  expect(screen.queryByText("Dana clocked in")).toBeNull();
});

test("too-many-attempts (429) surfaces the locked message", async () => {
  mockedApi.kioskPunch.mockRejectedValue({ response: { status: 429 } });
  renderKiosk();

  fireEvent.click(await screen.findByRole("button", { name: /Dana/ }));
  await enterPin("1234");
  fireEvent.click(screen.getByRole("button", { name: "Clock in" }));

  expect(
    await screen.findByText("Too many attempts. Try again in a few minutes."),
  ).toBeInTheDocument();
});

test("empty roster (nobody enrolled) shows the guidance, not a grid", async () => {
  mockedApi.kioskRoster.mockResolvedValue([
    { staff_id: 9, name: "Fio", role: "server", has_pin: false, on_clock: false },
  ]);
  renderKiosk();
  expect(await screen.findByText("No staff have a PIN yet")).toBeInTheDocument();
});

test("money-free: no dollar sign anywhere on the kiosk", async () => {
  const { onClose } = renderKiosk();
  await screen.findByRole("button", { name: /Dana/ });
  expect(document.body.textContent).not.toContain("$");
  expect(onClose).not.toHaveBeenCalled();
});
