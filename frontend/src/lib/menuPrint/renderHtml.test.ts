import {
  cjkRestaurantMenu,
  photoRichRestaurantMenu,
  rtlRestaurantMenu,
  sparseRestaurantMenu,
} from "./__fixtures__/restaurantMenus";
import { planFixtureDocument } from "./__fixtures__/plannedDocuments";
import { formatMenuPrice } from "./formatPrice";
import { escapeHtml, renderMenuPrintHtml } from "./renderHtml";
import type { PlannedMenuDocument, PrintMenuModel } from "./types";
import { PRINT_PRICE_TREATMENTS } from "./types";

const ORIGIN = "https://restaurant.test";

function cloneModel(model: PrintMenuModel): PrintMenuModel {
  return JSON.parse(JSON.stringify(model)) as PrintMenuModel;
}

function cloneDocument(document: PlannedMenuDocument): PlannedMenuDocument {
  return JSON.parse(JSON.stringify(document)) as PlannedMenuDocument;
}

function count(haystack: string, needle: string): number {
  return haystack.split(needle).length - 1;
}

describe("renderMenuPrintHtml", () => {
  it("keeps the historical public text-node escaping contract", () => {
    expect(escapeHtml(`chef's <pick> & "special"`)).toBe(
      `chef's &lt;pick&gt; &amp; &quot;special&quot;`,
    );
  });

  it("renders the supplied planned document with its exact page count and family structure", () => {
    const document = cloneDocument(
      planFixtureDocument({
        model: photoRichRestaurantMenu,
        familyId: "atelier",
        outputFormat: "two-page-spread",
      }),
    );
    document.pages = [
      document.pages[0],
      { index: 1, blocks: [] },
      { index: 2, blocks: [] },
    ];

    const html = renderMenuPrintHtml(photoRichRestaurantMenu, document, {
      origin: ORIGIN,
    });

    expect(count(html, 'class="print-page"')).toBe(3);
    expect(html).toContain('data-page="3"');
    expect(html).toContain("family-atelier");
    expect(html).toContain("atelier-asymmetric-masthead");
    expect(html).toContain("atelier-editorial-side-crop");
  });

  it("escapes restaurant and menu content at the stable entry point", () => {
    const model = cloneModel(sparseRestaurantMenu);
    model.business.name = '<script>alert("restaurant")</script>';
    model.sections[0].items[0].name = '<img src=x onerror="alert(1)">';
    model.sections[0].items[0].description =
      "</style><script>alert(2)</script>";
    const document = planFixtureDocument(model);

    const html = renderMenuPrintHtml(model, document, { origin: ORIGIN });

    expect(html).not.toMatch(/<script|<img[^>]+onerror/i);
    expect(html).toContain("&lt;script&gt;");
    expect(html).toContain("&lt;img src=x onerror=&quot;alert(1)&quot;&gt;");
  });

  it.each([
    ["en", "USD", 1234.5],
    ["es", "EUR", 1234.5],
    ["ja", "JPY", 1234],
    ["es-AR", "ARS", 1234.5],
  ])(
    "formats %s prices for %s through the per-family treatment",
    (language, currency, price) => {
      const model = cloneModel(cjkRestaurantMenu);
      model.language = language;
      model.currency = currency;
      model.sections[0].items[0].price = price;

      // Counter keeps the locale currency symbol per PRINT_PRICE_TREATMENTS.
      const html = renderMenuPrintHtml(
        model,
        planFixtureDocument({ model, familyId: "counter" }),
        { origin: ORIGIN },
      );
      const expected = formatMenuPrice(
        price,
        currency,
        PRINT_PRICE_TREATMENTS.counter,
        language,
      );

      expect(html).toContain(`lang="${language}"`);
      expect(html).toContain(`class="item-price">${expected}</span>`);
    },
  );

  it("renders bare numerals for formal families that suppress the symbol", () => {
    const model = cloneModel(cjkRestaurantMenu);
    model.language = "en";
    model.currency = "USD";
    model.sections[0].items[0].price = 1234.5;

    const html = renderMenuPrintHtml(
      model,
      planFixtureDocument({ model, familyId: "atelier" }),
      { origin: ORIGIN },
    );

    expect(html).toContain('class="item-price">1,234.50</span>');
    expect(html).not.toContain('class="item-price">$1,234.50</span>');
  });

  it("marks right-to-left languages without changing left-to-right CJK output", () => {
    const rtl = renderMenuPrintHtml(
      rtlRestaurantMenu,
      planFixtureDocument(rtlRestaurantMenu),
      { origin: ORIGIN },
    );
    const cjk = renderMenuPrintHtml(
      cjkRestaurantMenu,
      planFixtureDocument(cjkRestaurantMenu),
      { origin: ORIGIN },
    );

    expect(rtl).toContain('<html lang="ar" dir="rtl">');
    expect(cjk).toContain('<html lang="ja">');
    expect(cjk).not.toContain('<html lang="ja" dir="rtl">');
  });

  it("renders one safe QR with escaped caption copy", () => {
    const document = planFixtureDocument({
      model: sparseRestaurantMenu,
      outputFormat: "two-page-spread",
    });
    const qrDataUrl = "data:image/png;base64,AAAA";

    const html = renderMenuPrintHtml(sparseRestaurantMenu, document, {
      origin: ORIGIN,
      qrDataUrl,
      qrCaption: 'Scan <today> "only"',
    });

    expect(count(html, 'class="qr-image"')).toBe(1);
    expect(html).toContain(`src="${qrDataUrl}"`);
    expect(html).toContain("Scan &lt;today&gt; &quot;only&quot;");
  });

  it("embeds only restaurant-facing font assets from the supplied origin", () => {
    const model = cloneModel(sparseRestaurantMenu);
    const document = planFixtureDocument({
      model,
      familyId: "gallery",
    });

    const html = renderMenuPrintHtml(model, document, {
      origin: ORIGIN,
    });

    expect(html).toContain("@font-face");
    expect(html).toContain(`${ORIGIN}/fonts/`);
    expect(html).toContain("family-gallery");
    expect(html).not.toMatch(/payverge/i);
  });
});
