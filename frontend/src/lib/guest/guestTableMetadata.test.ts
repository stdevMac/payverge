/** @jest-environment node */
import { guestTablePageMetadata } from "./guestTableMetadata";

describe("guestTablePageMetadata", () => {
  it("emits table-specific OG url/title/description for menu", async () => {
    const meta = await guestTablePageMetadata({
      tableCode: "M03Y18GB3P",
      surface: "menu",
      businessName: "Payverge AI Pro Demo Lounge",
    });
    expect(meta.title).toBe("Payverge AI Pro Demo Lounge – Menu | Payverge");
    expect(String(meta.openGraph?.url)).toContain("/t/M03Y18GB3P/menu");
    expect(meta.openGraph?.title).toContain("Payverge AI Pro Demo Lounge");
    expect(String(meta.openGraph?.description)).toMatch(/Menu at Payverge AI Pro Demo Lounge/i);
    expect(String(meta.openGraph?.url)).not.toBe("https://payverge.io");
    expect(String(meta.title)).not.toMatch(/AI-Powered Restaurant Management/i);
  });

  it("emits table-specific OG url/title/description for bill", async () => {
    const meta = await guestTablePageMetadata({
      tableCode: "M03Y18GB3P",
      surface: "bill",
      businessName: "Payverge AI Pro Demo Lounge",
    });
    expect(meta.title).toBe("Payverge AI Pro Demo Lounge – Bill | Payverge");
    expect(String(meta.openGraph?.url)).toContain("/t/M03Y18GB3P/bill");
    expect(String(meta.openGraph?.description)).toMatch(/bill at Payverge AI Pro Demo Lounge/i);
    expect(String(meta.openGraph?.url)).not.toBe("https://payverge.io");
  });

  it("self-canonicalizes /es/t/{code} with es_ES og:locale and Spanish twitter title (#922)", async () => {
    const meta = await guestTablePageMetadata({
      tableCode: "FV214XU12D",
      surface: "table",
      businessName: "Parrilla Quebracho Azul",
      locale: "es",
    });
    expect(String(meta.alternates?.canonical)).toBe(
      "https://payverge.io/es/t/FV214XU12D",
    );
    expect(String(meta.openGraph?.url)).toBe(
      "https://payverge.io/es/t/FV214XU12D",
    );
    expect(meta.openGraph?.locale).toBe("es_ES");
    expect(String((meta.twitter as { title?: string }).title)).toBe(
      String(meta.title),
    );
    expect(String((meta.twitter as { title?: string }).title)).not.toBe(
      "Payverge - AI-Driven Restaurant Operations",
    );
    expect(meta.alternates?.languages?.es).toBe(
      "https://payverge.io/es/t/FV214XU12D",
    );
  });

  it("uses es_AR og:locale and /es-ar prefix for es-AR (#922)", async () => {
    const meta = await guestTablePageMetadata({
      tableCode: "FV214XU12D",
      surface: "menu",
      businessName: "Parrilla Quebracho Azul",
      locale: "es-AR",
    });
    expect(String(meta.alternates?.canonical)).toBe(
      "https://payverge.io/es-ar/t/FV214XU12D/menu",
    );
    expect(meta.openGraph?.locale).toBe("es_AR");
  });

  it("localizes es/es-AR menu and bill document titles (#923)", async () => {
    const esMenu = await guestTablePageMetadata({
      tableCode: "FV214XU12D",
      surface: "menu",
      businessName: "Parrilla Quebracho Azul",
      locale: "es",
    });
    expect(esMenu.title).toBe("Parrilla Quebracho Azul – Menú | Payverge");
    expect(String(esMenu.title)).not.toMatch(/\bMenu\b/);

    const esBill = await guestTablePageMetadata({
      tableCode: "FV214XU12D",
      surface: "bill",
      businessName: "Parrilla Quebracho Azul",
      locale: "es",
    });
    expect(esBill.title).toBe("Parrilla Quebracho Azul – Cuenta | Payverge");
    expect(String(esBill.title)).not.toMatch(/\bBill\b/);

    const arMenu = await guestTablePageMetadata({
      tableCode: "FV214XU12D",
      surface: "menu",
      businessName: "Parrilla Quebracho Azul",
      locale: "es-AR",
    });
    expect(arMenu.title).toBe("Parrilla Quebracho Azul – Menú | Payverge");
  });
});
