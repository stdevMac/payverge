/**
 * F4: global observer rewrites dialog Close → Cerrar under es.
 * @jest-environment jsdom
 */
import React from "react";
import { act, render, waitFor } from "@testing-library/react";
import { ModalCloseAriaLocalizer } from "./ModalCloseAriaLocalizer";

const mockLocale = { current: "es" as string };

jest.mock("@/i18n/OperatorLocaleProvider", () => {
  const actual = jest.requireActual("@/i18n/OperatorLocaleProvider");
  return {
    ...actual,
    useSimpleLocale: () => ({
      locale: mockLocale.current,
      setLocale: jest.fn(),
    }),
  };
});

describe("F4 ModalCloseAriaLocalizer", () => {
  beforeEach(() => {
    mockLocale.current = "es";
    document.body.innerHTML = "";
  });

  it("rewrites a dialog button with aria-label Close to Cerrar", async () => {
    // Simulate NextUI's hardcoded close control.
    const dialog = document.createElement("div");
    dialog.setAttribute("role", "dialog");
    const btn = document.createElement("button");
    btn.setAttribute("aria-label", "Close");
    dialog.appendChild(btn);
    document.body.appendChild(dialog);

    render(<ModalCloseAriaLocalizer />);

    await waitFor(() => {
      expect(btn.getAttribute("aria-label")).toBe("Cerrar");
    });
  });

  it("rewrites Close buttons added after mount (MutationObserver)", async () => {
    render(<ModalCloseAriaLocalizer />);

    const dialog = document.createElement("div");
    dialog.setAttribute("role", "dialog");
    const btn = document.createElement("button");
    btn.setAttribute("aria-label", "Close");
    dialog.appendChild(btn);
    document.body.appendChild(dialog);

    await waitFor(() => {
      expect(btn.getAttribute("aria-label")).toBe("Cerrar");
    });
  });

  it("rewrites Close to Cerrar after an in-app guest es-AR switch while operator locale stays en", async () => {
    mockLocale.current = "en";
    const dialog = document.createElement("div");
    dialog.setAttribute("role", "dialog");
    const btn = document.createElement("button");
    btn.setAttribute("aria-label", "Close");
    dialog.appendChild(btn);
    document.body.appendChild(dialog);

    render(<ModalCloseAriaLocalizer />);
    await new Promise((r) => setTimeout(r, 20));
    expect(btn.getAttribute("aria-label")).toBe("Close");

    await act(async () => {
      window.dispatchEvent(
        new CustomEvent("guestLanguageChange", {
          detail: { language: "es-AR", businessId: 1 },
        }),
      );
    });

    await waitFor(() => {
      expect(btn.getAttribute("aria-label")).toBe("Cerrar");
    });
  });

  it("does not rewrite under en", async () => {
    mockLocale.current = "en";
    const dialog = document.createElement("div");
    dialog.setAttribute("role", "dialog");
    const btn = document.createElement("button");
    btn.setAttribute("aria-label", "Close");
    dialog.appendChild(btn);
    document.body.appendChild(dialog);

    render(<ModalCloseAriaLocalizer />);

    // Give the effect a tick; label must stay English.
    await new Promise((r) => setTimeout(r, 30));
    expect(btn.getAttribute("aria-label")).toBe("Close");
  });
});
