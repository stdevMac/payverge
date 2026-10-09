/** @jest-environment jsdom */
//
// #883 — the kiosk's PIN errors in Argentine Spanish.
//
// QA saw an English "Incorrect PIN" on a Spanish terminal. The cause is not a
// missing backend translation: KioskClockIn never renders the server's
// `message` field. It maps the HTTP status to its OWN dashboardKiosk key, so
// the rendered copy is decided entirely on the client and any status the map
// misses degrades to the vague generic line.
//
// This suite therefore drives the real bundles (no COPY stub — that is what
// KioskClockIn.test.tsx does, and it is why the gap stayed invisible) at
// locale es-AR, asserting both the voseo deltas and the keys that correctly
// inherit the neutral `es` base.
import React from "react";
import { render, screen, fireEvent } from "@testing-library/react";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import KioskClockIn from "./KioskClockIn";
import { timeclockApi, type KioskStaffMember } from "@/api/timeclock";

// Keep the REAL getTranslation (and therefore the real en/es/es-ar message
// files) — only the locale hook is stubbed, standing in for an operator whose
// dashboard is set to Argentine Spanish.
jest.mock("@/i18n/SimpleTranslationProvider", () => {
  const actual = jest.requireActual("@/i18n/SimpleTranslationProvider");
  return {
    ...actual,
    useSimpleLocale: () => ({ locale: "es-AR", setLocale: jest.fn() }),
  };
});

jest.mock("@/api/timeclock", () => ({
  timeclockApi: { kioskRoster: jest.fn(), kioskPunch: jest.fn() },
}));

const mockedApi = timeclockApi as unknown as {
  kioskRoster: jest.Mock;
  kioskPunch: jest.Mock;
};

const roster: KioskStaffMember[] = [
  {
    staff_id: 7,
    name: "Camila",
    role: "server",
    has_pin: true,
    on_clock: false,
  },
];

beforeEach(() => {
  jest.clearAllMocks();
  mockedApi.kioskRoster.mockResolvedValue(roster);
});

async function punchWithStatus(status: number) {
  mockedApi.kioskPunch.mockRejectedValue({ response: { status } });
  const qc = new QueryClient({
    defaultOptions: { queries: { retry: false }, mutations: { retry: false } },
  });
  render(
    <QueryClientProvider client={qc}>
      <KioskClockIn businessId="141" onClose={jest.fn()} />
    </QueryClientProvider>,
  );
  fireEvent.click(await screen.findByRole("button", { name: /Camila/ }));
  for (const d of "0000") {
    fireEvent.click(screen.getByRole("button", { name: d }));
  }
  fireEvent.click(screen.getByRole("button", { name: "Fichar entrada" }));
}

describe("kiosk PIN errors render in es-AR (#883)", () => {
  it("403 shows the voseo incorrect-PIN line, never English", async () => {
    await punchWithStatus(403);
    expect(
      await screen.findByText("PIN incorrecto. Probá de nuevo."),
    ).toBeInTheDocument();
    expect(screen.queryByText(/Incorrect PIN/i)).toBeNull();
    // Still on the pad so the staffer can retry.
    expect(screen.getByText("Ingresá tu PIN")).toBeInTheDocument();
  });

  it("429 shows the voseo lockout line", async () => {
    await punchWithStatus(429);
    expect(
      await screen.findByText(
        "Demasiados intentos. Volvé a intentar en unos minutos.",
      ),
    ).toBeInTheDocument();
  });

  it("401 names the expired device session instead of inviting a retry", async () => {
    await punchWithStatus(401);
    expect(
      await screen.findByText(
        "La sesión de este dispositivo expiró. Volvé a iniciar sesión para seguir usando la terminal.",
      ),
    ).toBeInTheDocument();
    expect(screen.queryByText(/No se pudo registrar/)).toBeNull();
  });

  it("400 and 500 stay on the localized generic line", async () => {
    await punchWithStatus(500);
    expect(
      await screen.findByText("No se pudo registrar. Probá de nuevo."),
    ).toBeInTheDocument();
  });

  it("400 is not dressed up as a PIN complaint", async () => {
    await punchWithStatus(400);
    expect(
      await screen.findByText("No se pudo registrar. Probá de nuevo."),
    ).toBeInTheDocument();
  });

  it("404 and 409 inherit the neutral es copy (no voseo verb to differ on)", async () => {
    await punchWithStatus(404);
    expect(
      await screen.findByText("No se encontró a esa persona."),
    ).toBeInTheDocument();
  });

  it("409 inherits the neutral es no-PIN copy", async () => {
    await punchWithStatus(409);
    expect(
      await screen.findByText("Esta persona todavía no configuró un PIN."),
    ).toBeInTheDocument();
  });
});
