/**
 * F4: LocalizedModal + ModalCloseAriaLocalizer must put a Spanish aria-label
 * on the visible close (X) button. NextUI hardcodes "Close" with no prop override.
 *
 * Assert rendered DOM after the localizer runs. Strip the localizer / effect
 * and leave raw NextUI Modal → this suite goes red.
 *
 * @jest-environment jsdom
 */
import React from "react";
import { render, screen, waitFor } from "@testing-library/react";
import { ModalContent, ModalHeader, ModalBody } from "@nextui-org/react";
import { LocalizedModal } from "./LocalizedModal";
import { ModalCloseAriaLocalizer } from "./ModalCloseAriaLocalizer";

const mockLocale = { current: "es" as string };

jest.mock("@/i18n/SimpleTranslationProvider", () => {
  const actual = jest.requireActual("@/i18n/SimpleTranslationProvider");
  return {
    ...actual,
    useSimpleLocale: () => ({
      locale: mockLocale.current,
      setLocale: jest.fn(),
    }),
  };
});

function renderOpenModal() {
  return render(
    <>
      <ModalCloseAriaLocalizer />
      <LocalizedModal isOpen onOpenChange={() => undefined}>
        <ModalContent>
          <ModalHeader>Título</ModalHeader>
          <ModalBody>
            <p>Cuerpo</p>
          </ModalBody>
        </ModalContent>
      </LocalizedModal>
    </>,
  );
}

describe("F4 LocalizedModal close aria-label", () => {
  beforeEach(() => {
    mockLocale.current = "es";
  });

  it("localizes the visible close button to Cerrar under es", async () => {
    renderOpenModal();

    await waitFor(() => {
      const closeBtn = screen.getByRole("button", { name: "Cerrar" });
      expect(closeBtn).toBeInTheDocument();
      expect(closeBtn.getAttribute("aria-label")).toBe("Cerrar");
    });
    expect(screen.queryByRole("button", { name: "Close" })).toBeNull();
  });

  it("leaves English Close under en", async () => {
    mockLocale.current = "en";
    render(
      <>
        <ModalCloseAriaLocalizer />
        <LocalizedModal isOpen onOpenChange={() => undefined}>
          <ModalContent>
            <ModalHeader>Title</ModalHeader>
            <ModalBody>
              <p>Body</p>
            </ModalBody>
          </ModalContent>
        </LocalizedModal>
      </>,
    );

    await waitFor(() => {
      expect(screen.getByRole("button", { name: "Close" })).toBeInTheDocument();
    });
  });
});
