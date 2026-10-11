/**
 * F4: modal close / dismiss label helpers.
 * toReactAriaLocale drives NextUIProvider so DismissButtons use es-ES "Descartar".
 * modalCloseAriaLabel drives the visible X button (NextUI hardcodes "Close").
 */
import {
  modalCloseAriaLabel,
  modalCloseButtonProps,
  toReactAriaLocale,
} from "./modalCloseLabel";

describe("F4 modalCloseLabel", () => {
  it("maps operator es / es-AR to es-ES for react-aria Dismiss strings", () => {
    expect(toReactAriaLocale("es")).toBe("es-ES");
    expect(toReactAriaLocale("es-AR")).toBe("es-ES");
    expect(toReactAriaLocale("en")).toBe("en-US");
    expect(toReactAriaLocale("en-US")).toBe("en-US");
  });

  it("returns Spanish Cerrar for the visible close control", () => {
    expect(modalCloseAriaLabel("es")).toBe("Cerrar");
    expect(modalCloseAriaLabel("es-AR")).toBe("Cerrar");
  });

  it("returns English Close for en", () => {
    expect(modalCloseAriaLabel("en")).toBe("Close");
  });

  it("builds closeButtonProps with the localized aria-label", () => {
    expect(modalCloseButtonProps("es")).toEqual({ "aria-label": "Cerrar" });
    expect(modalCloseButtonProps("en")).toEqual({ "aria-label": "Close" });
  });
});
