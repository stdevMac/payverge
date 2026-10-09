import {
  cjkRestaurantMenu,
  photoRichRestaurantMenu,
  rtlRestaurantMenu,
  sparseRestaurantMenu,
} from "./__fixtures__/restaurantMenus";
import {
  makeDirection,
  planFixtureDocument,
} from "./__fixtures__/plannedDocuments";
import { getMenuDesignFamily } from "./families";
import { formatMenuPrice } from "./formatPrice";
import { PRINT_FONT_FAMILIES } from "./fonts";
import {
  escapeHtml,
  renderPlannedMenuHtml,
  renderPlannedPageHtml,
  type RenderPlannedMenuOptions,
} from "./renderPlannedHtml";
import {
  MENU_DESIGN_FAMILY_IDS,
  PRINT_PRICE_TREATMENTS,
  type MenuDesignFamilyId,
  type PlannedMenuBlock,
  type PlannedMenuDocument,
  type PrintMenuModel,
} from "./types";

const renderOptions: RenderPlannedMenuOptions = {
  origin: "https://payverge.test",
  qrCaption: "Scan for the live menu",
};

function cloneModel(model: PrintMenuModel): PrintMenuModel {
  return JSON.parse(JSON.stringify(model)) as PrintMenuModel;
}

function cloneDocument(document: PlannedMenuDocument): PlannedMenuDocument {
  return JSON.parse(JSON.stringify(document)) as PlannedMenuDocument;
}

function count(haystack: string, needle: string): number {
  return haystack.split(needle).length - 1;
}

function computeClassStyle(
  html: string,
  elementClasses: readonly string[],
  ancestorClasses: readonly string[],
): Record<string, string> {
  const css = html.match(/<style>\s*([\s\S]*?)\s*<\/style>/)?.[1] ?? "";
  const winners = new Map<string, { specificity: number; value: string }>();
  for (const rule of css.matchAll(/([^{}]+)\{([^{}]*)\}/g)) {
    for (const rawSelector of rule[1].split(",")) {
      const selector = rawSelector.trim();
      const segments = selector.split(/\s+/);
      const segmentClasses = segments.map((segment) =>
        [...segment.matchAll(/\.([a-z0-9-]+)/gi)].map((match) => match[1]),
      );
      const targetClasses = segmentClasses.at(-1) ?? [];
      const ancestorRequirements = segmentClasses.slice(0, -1).flat();
      if (
        !targetClasses.length ||
        !targetClasses.every((name) => elementClasses.includes(name)) ||
        !ancestorRequirements.every((name) => ancestorClasses.includes(name))
      ) {
        continue;
      }
      const specificity = segmentClasses.flat().length;
      for (const declaration of rule[2].split(";")) {
        const separator = declaration.indexOf(":");
        if (separator < 0) continue;
        const property = declaration.slice(0, separator).trim();
        const value = declaration.slice(separator + 1).trim();
        const current = winners.get(property);
        if (!current || specificity >= current.specificity) {
          winners.set(property, { specificity, value });
        }
      }
    }
  }
  return Object.fromEntries(
    [...winners].map(([property, winner]) => [property, winner.value]),
  );
}

describe("renderPlannedMenuHtml", () => {
  it.each([
    [
      "Arabic",
      rtlRestaurantMenu,
      "Noto Naskh Arabic",
      "Noto Sans Arabic",
      "DM Serif Display",
      "DM Sans",
      "/fonts/print/noto-naskh-arabic/NotoNaskhArabic-Variable.woff2",
    ],
    [
      "CJK",
      cjkRestaurantMenu,
      "Noto Serif CJK JP",
      "Noto Sans CJK JP",
      "DM Serif Display",
      "DM Sans",
      "/fonts/print/noto-serif-cjk-jp/NotoSerifCJKjp-Variable.woff2",
    ],
  ] as const)(
    "loads deterministic %s print faces into the active type stacks",
    (
      _script,
      model,
      displayFace,
      bodyFace,
      familyDisplay,
      familyBody,
      facePath,
    ) => {
      const html = renderPlannedMenuHtml(
        model,
        planFixtureDocument({ model }),
        { ...renderOptions, origin: "" },
      );

      expect(html).toContain(`font-family: "${displayFace}";`);
      expect(html).toContain(`url("${facePath}")`);
      expect(html).toContain(
        `--display-font:"${familyDisplay}","${displayFace}",`,
      );
      expect(html).toContain(`--body-font:"${familyBody}","${bodyFace}",`);
    },
  );

  it.each([
    ["ko-KR", "Noto Sans CJK KR", "Noto Sans CJK KR"],
    ["zh-Hans", "Noto Sans CJK SC", "Noto Sans CJK SC"],
    ["zh-Hant-TW", "Noto Sans CJK TC", "Noto Sans CJK TC"],
  ] as const)(
    "keeps the family art direction before the %s script fallbacks",
    (language, displayFace, bodyFace) => {
      const model = cloneModel(cjkRestaurantMenu);
      model.language = language;
      const html = renderPlannedMenuHtml(
        model,
        planFixtureDocument({ model }),
        { ...renderOptions, origin: "" },
      );

      expect(html).toContain(
        `--display-font:"DM Serif Display","${displayFace}",`,
      );
      expect(html).toContain(`--body-font:"DM Sans","${bodyFace}",`);
    },
  );

  it("keeps variable identity and item copy inside their planned boxes", () => {
    const document = planFixtureDocument({
      model: sparseRestaurantMenu,
      familyId: "field",
      treatment: "balanced",
      outputFormat: "single-sheet",
      paperFormat: "half-letter",
    });
    const html = renderPlannedMenuHtml(
      sparseRestaurantMenu,
      document,
      renderOptions,
    );

    expect(html).toContain(
      ".block-masthead{overflow:hidden;overflow-wrap:anywhere}",
    );
    expect(html).toContain(".item-name{min-width:0;overflow-wrap:anywhere;");
    expect(html).toContain(".item-price{flex:none;");
  });

  it.each(["atelier", "maison", "night-house", "gallery"] as const)(
    "keeps the %s logo and restaurant name on one deliberate identity axis",
    (familyId) => {
      const document = planFixtureDocument({
        model: photoRichRestaurantMenu,
        familyId,
        treatment: familyId === "gallery" ? "balanced" : "type-led",
      });
      const html = renderPlannedMenuHtml(
        photoRichRestaurantMenu,
        document,
        renderOptions,
      );

      expect(html).toContain('<div class="identity-lockup">');
      expect(html).toContain(
        ".identity-lockup{display:flex;flex-direction:column;align-items:center",
      );
      expect(html).toContain(
        "justify-content:flex-start;gap:2mm;width:100%;height:100%;text-align:center",
      );
      expect(html).toContain(
        ".restaurant-logo{flex:none;object-position:center}",
      );
    },
  );

  it("layers a photographic cover without repeating a second masthead", () => {
    const document = planFixtureDocument({
      model: photoRichRestaurantMenu,
      familyId: "gallery",
      treatment: "photo-led",
      outputFormat: "two-page-spread",
      coverMode: "photographic",
      imageSelections: [
        { itemId: "harvest-bowl", url: "harvest.jpg", role: "cover" },
      ],
    });
    const html = renderPlannedMenuHtml(
      photoRichRestaurantMenu,
      document,
      renderOptions,
    );

    expect(html).not.toContain('<h1 class="restaurant-name">');
    expect(count(html, '<p class="cover-restaurant">')).toBe(1);
    expect(
      count(html, 'class="restaurant-logo cover-logo logo-contained"'),
    ).toBe(1);
    expect(html).toContain(
      ".block-cover .cover-type{position:relative;z-index:1;display:flex;height:100%;width:100%;flex-direction:column;align-items:center",
    );
    expect(html).toContain("text-align:center");
    expect(html).toContain(
      '.block-cover.has-cover-image::after{content:"";position:absolute;inset:0',
    );
    expect(html).toContain(
      ".block-cover.has-cover-image .cover-type{color:var(--ground)",
    );
  });

  it("prints typography personality and contact placement as document design", () => {
    const document = planFixtureDocument({
      model: sparseRestaurantMenu,
      direction: {
        ...makeDirection(),
        typographyPersonality: "refined",
        contactPlacement: "cover",
      },
    });

    const html = renderPlannedMenuHtml(
      sparseRestaurantMenu,
      document,
      renderOptions,
    );

    expect(html).toContain("typography-refined");
    expect(html).toContain("contact-cover");
    expect(html).toContain(
      '<p class="cover-contact">18 Paseo del Mar, Valencia</p>',
    );
    expect(html).not.toContain('class="footer-address"');
    expect(html).toContain(
      ".typography-refined .restaurant-name,.typography-refined .category-name",
    );
  });

  it("places and renders every selected photo across menu sections", () => {
    const selections = [
      { itemId: "harvest-bowl", url: "harvest.jpg", role: "full-width" },
      {
        itemId: "lemon-trout",
        url: "https://images.payverge.test/lemon-trout-1800x1200.jpg",
        role: "editorial-crop",
      },
    ] as const;
    const document = planFixtureDocument({
      model: photoRichRestaurantMenu,
      familyId: "atelier",
      treatment: "balanced",
      outputFormat: "two-page-spread",
      imageSelections: selections.map((selection) => ({ ...selection })),
    });
    const placedUrls = document.pages.flatMap((page) =>
      page.blocks.flatMap((block) => (block.image ? [block.image.url] : [])),
    );
    const html = renderPlannedMenuHtml(
      photoRichRestaurantMenu,
      document,
      renderOptions,
    );

    expect(new Set(placedUrls)).toEqual(
      new Set(selections.map(({ url }) => url)),
    );
    expect(
      document.diagnostics.filter(({ messageKey }) =>
        messageKey.endsWith("imageUnplaced"),
      ),
    ).toHaveLength(0);
    for (const { url } of selections) expect(html).toContain(`src="${url}"`);
  });

  it.each(MENU_DESIGN_FAMILY_IDS)(
    "renders explicit %s pages without product chrome",
    (familyId) => {
      const model = cloneModel(photoRichRestaurantMenu);
      model.business.logoUrl = "https://images.restaurant.test/logo.svg";
      const document = planFixtureDocument({
        model,
        familyId,
      });

      const html = renderPlannedMenuHtml(model, document, {
        ...renderOptions,
        origin: "",
      });

      expect(html).toMatch(
        new RegExp(
          `<section class="print-page" data-page="1" data-family="${familyId}"`,
        ),
      );
      expect(html).toContain(`data-family="${familyId}"`);
      expect(count(html, 'class="print-page"')).toBe(document.pages.length);
      expect(html).not.toMatch(/payverge/i);
      expect(html).not.toContain("data-print-hidden");
    },
  );

  it("renders only explicitly planned photos with clamped focal points", () => {
    const harvest = photoRichRestaurantMenu.sections[0].items[0];
    const imageUrl = harvest.imageCandidates![1];
    const document = planFixtureDocument({
      model: photoRichRestaurantMenu,
      familyId: "gallery",
      treatment: "photo-led",
      imageSelections: [
        { itemId: harvest.id, url: imageUrl, role: "editorial-crop" },
      ],
      imageFocalPoints: { [imageUrl]: { x: 0.72, y: 0.31 } },
      failedUrls: ["broken.jpg"],
    });

    const html = renderPlannedMenuHtml(
      photoRichRestaurantMenu,
      document,
      renderOptions,
    );

    expect(html).toContain(`src="${imageUrl}"`);
    expect(html).toContain("object-position:72% 31%");
    expect(html).not.toContain("broken.jpg");
    expect(html).not.toContain("image-placeholder");

    const clamped = cloneDocument(document);
    clamped.direction.imageFocalPoints[imageUrl] = { x: 4, y: -2 };
    expect(
      renderPlannedMenuHtml(photoRichRestaurantMenu, clamped, renderOptions),
    ).toContain("object-position:100% 0%");
  });

  it("escapes untrusted content and rejects CSS/origin injection", () => {
    const model = cloneModel(sparseRestaurantMenu);
    model.business.name = '<script>alert("business")</script>';
    model.business.tagline = '"><img src=x onerror=alert(1)>';
    model.business.address = "</style><script>alert('address')</script>";
    model.business.logoUrl = "javascript:alert(1)";
    model.sections[0].name = '<svg onload="alert(2)">';
    model.sections[0].items[0].name = '<img src=x onerror="alert(3)">';
    model.sections[0].items[0].description = "<script>alert(4)</script>";
    model.sections[0].items[0].dietaryTags = ["vegan", "<tag>"];
    model.sections[0].items[0].allergens = ['nuts" onerror="alert(5)'];
    const document = planFixtureDocument(model);
    const html = renderPlannedMenuHtml(model, document, {
      ...renderOptions,
      qrCaption: '"><script>alert(6)</script>',
      qrDataUrl: "javascript:alert(7)",
    });

    expect(html).not.toMatch(
      /<script|<svg|<img[^>]+\sonerror\s*=|<svg[^>]+\sonload\s*=|javascript:/i,
    );
    expect(html).toContain("&lt;script&gt;");
    expect(html).toContain("&quot;&gt;&lt;img");
    expect(html).toContain("&#39;address&#39;");
    expect(html).not.toContain('<img class="qr-image"');

    const poisonPalette = cloneDocument(document);
    poisonPalette.direction.palette.ink = "#fff;}body{display:none";
    expect(() =>
      renderPlannedMenuHtml(model, poisonPalette, renderOptions),
    ).toThrow("Invalid print palette color: ink");
    expect(() =>
      renderPlannedMenuHtml(model, document, {
        origin: 'https://safe.test\");}script{display:block}',
      }),
    ).toThrow("Invalid print origin");
  });

  it.each([
    ["en", "USD", 1234.5],
    ["es", "EUR", 1234.5],
    ["ja", "JPY", 1234.5],
    ["es-AR", "ARS", 1234.5],
  ])(
    "formats %s %s prices from exact dollar amounts",
    (language, currency, price) => {
      const model = cloneModel(sparseRestaurantMenu);
      model.language = language;
      model.currency = currency;
      model.sections[0].items[0].price = price;
      const document = planFixtureDocument(model);
      const expected = escapeHtml(
        formatMenuPrice(
          price,
          currency,
          PRINT_PRICE_TREATMENTS[document.direction.familyId],
          language,
        ),
      );

      const html = renderPlannedMenuHtml(model, document, renderOptions);

      expect(html).toContain(`lang="${language}"`);
      expect(html).toContain(`class="item-price">${expected}</span>`);
    },
  );

  it("nests the maison price beneath the copy instead of inside an item-line", () => {
    const maisonHtml = renderPlannedMenuHtml(
      sparseRestaurantMenu,
      planFixtureDocument({ model: sparseRestaurantMenu, familyId: "maison" }),
      { ...renderOptions, origin: "" },
    );
    const atelierHtml = renderPlannedMenuHtml(
      sparseRestaurantMenu,
      planFixtureDocument({ model: sparseRestaurantMenu, familyId: "atelier" }),
      { ...renderOptions, origin: "" },
    );

    expect(maisonHtml).not.toContain('<div class="item-line">');
    expect(maisonHtml).toMatch(
      /<h3 class="item-name">[^<]*<\/h3>(?:<p class="item-description">[\s\S]*?<\/p>)?<span class="item-price">/,
    );
    expect(atelierHtml).toMatch(
      /<div class="item-line"><h3 class="item-name">[^<]*<\/h3><span class="price-leaders" aria-hidden="true"><\/span><span class="item-price">/,
    );
  });

  it("renders osteria price leaders between the item name and price", () => {
    const html = renderPlannedMenuHtml(
      sparseRestaurantMenu,
      planFixtureDocument({ model: sparseRestaurantMenu, familyId: "osteria" }),
      { ...renderOptions, origin: "" },
    );

    expect(html).toMatch(
      /<div class="item-line"><h3 class="item-name">[^<]*<\/h3><span class="price-leaders" aria-hidden="true"><\/span><span class="item-price">/,
    );
  });

  it.each(["ar", "fa-IR", "ur", "he-IL", "en-Arab"])(
    "sets RTL direction for %s from explicit script or base language",
    (language) => {
      const model = cloneModel(sparseRestaurantMenu);
      model.language = language;

      expect(
        renderPlannedMenuHtml(model, planFixtureDocument(model), renderOptions),
      ).toContain(`<html lang="${language}" dir="rtl">`);
    },
  );

  it.each(["ja", "ar-Latn"])(
    "keeps %s LTR when its explicit script or language is LTR",
    (language) => {
      const model =
        language === "ja"
          ? cloneModel(cjkRestaurantMenu)
          : cloneModel(rtlRestaurantMenu);
      model.language = language;
      const html = renderPlannedMenuHtml(
        model,
        planFixtureDocument(model),
        renderOptions,
      );

      expect(html).toContain(`<html lang="${language}">`);
      expect(html).not.toContain(`<html lang="${language}" dir="rtl">`);
    },
  );

  it("uses logical inline CSS and explicit RTL mirrors", () => {
    const documents = MENU_DESIGN_FAMILY_IDS.map((familyId) =>
      renderPlannedMenuHtml(
        rtlRestaurantMenu,
        planFixtureDocument({ model: rtlRestaurantMenu, familyId }),
        { ...renderOptions, origin: "" },
      ),
    );
    const css = documents
      .map((html) => html.match(/<style>\s*([\s\S]*?)\s*<\/style>/)?.[1] ?? "")
      .join("\n");

    expect(
      documents.every((html) => html.includes('<html lang="ar" dir="rtl">')),
    ).toBe(true);
    expect(css).not.toMatch(
      /(?:^|[;{])(?:right|padding-left|border-left|border-right|margin-left|margin-right):/,
    );
    expect(css).not.toContain("object-position:left");
    expect(css).not.toContain("transform-origin:left");
    expect(css).not.toContain("translateX(3mm)");
    expect(css).toContain("inset-inline-end:0");
    expect(css).toContain("padding-block-start:3mm");
    expect(css).toContain("border-block-start:.55mm solid var(--ink)");
    expect(css).toContain("border-inline-end:2mm solid var(--ground)");
    expect(css).toContain("margin-inline-start:8mm");
    expect(css).toContain("object-position:center");
    expect(css).toContain('[dir="rtl"] .logo-wordmark{object-position:center}');
    expect(css).toContain(
      ".osteria-unequal-columns.block-masthead{border-block-start:.6mm solid var(--accent);padding-block-start:2.5mm;text-align:center}",
    );
    expect(css).not.toContain("transform:rotate(var(--inline-tilt))");
    // --inline-shift/--inline-tilt are declared for RTL mirroring but no
    // longer consumed by any transform: the offset-heading/feature-shift
    // treatments that used them were replaced by border/price-leader
    // treatments (see FAMILY_CSS atelier/osteria).
    expect(css).not.toContain("translateX(var(--inline-shift))");
    expect(css).toContain(
      '[dir="rtl"]{--inline-shift:-3mm;--inline-tilt:1deg}',
    );
  });

  it("renders all block kinds, structural roles, semantic labels, and exact mm rectangles", () => {
    const model = cloneModel(sparseRestaurantMenu);
    model.sections[0].items[0].allergens = ["gluten"];
    const document = cloneDocument(planFixtureDocument(model));
    const section = model.sections[0];
    const item = section.items[0];
    const blocks: PlannedMenuBlock[] = [
      {
        id: 'mast"><script>',
        kind: "masthead",
        rect: { xMm: 1, yMm: 2, widthMm: 3, heightMm: 4 },
      },
      {
        id: "cover",
        kind: "cover",
        rect: { xMm: 5, yMm: 6, widthMm: 7, heightMm: 8 },
      },
      {
        id: "heading",
        kind: "category-heading",
        sectionId: section.id,
        rect: { xMm: 9, yMm: 10, widthMm: 11, heightMm: 12 },
      },
      {
        id: "list",
        kind: "item-list",
        sectionId: section.id,
        itemIds: [item.id],
        rect: { xMm: 13, yMm: 14, widthMm: 15, heightMm: 16 },
      },
      {
        id: "feature",
        kind: "item-feature",
        sectionId: section.id,
        itemIds: [item.id],
        rect: { xMm: 17, yMm: 18, widthMm: 19, heightMm: 20 },
      },
      {
        id: "image",
        kind: "image",
        sectionId: section.id,
        itemIds: [item.id],
        image: { itemId: item.id, url: "harvest.jpg", role: "compact-tile" },
        rect: { xMm: 21, yMm: 22, widthMm: 23, heightMm: 24 },
      },
      {
        id: "panel-note:story",
        kind: "panel-note",
        rect: { xMm: 23, yMm: 24, widthMm: 25, heightMm: 26 },
      },
      {
        id: "footer",
        kind: "footer",
        rect: { xMm: 25, yMm: 26, widthMm: 27, heightMm: 28 },
      },
      {
        id: "folio",
        kind: "folio",
        rect: { xMm: 29, yMm: 30, widthMm: 31, heightMm: 32 },
      },
    ];
    document.pages = [{ index: 0, blocks }];
    document.direction.imageFocalPoints = { "harvest.jpg": { x: 0.5, y: 0.5 } };
    const html = renderPlannedMenuHtml(model, document, renderOptions);

    for (const kind of [
      "masthead",
      "cover",
      "category-heading",
      "item-list",
      "item-feature",
      "image",
      "panel-note",
      "footer",
      "folio",
    ]) {
      expect(html).toContain(`kind-${kind}`);
      expect(html).toContain(`data-kind="${kind}"`);
    }
    expect(html).toContain("role-compact-tile");
    expect(html).toContain('data-block-id="mast&quot;&gt;&lt;script&gt;"');
    expect(html).toContain(
      "inset-inline-start:13mm;top:14mm;width:15mm;height:16mm",
    );
    expect(html).not.toMatch(/(?:^|[;"\s])left:\d/m);
    expect(html).toContain('aria-label="vegan"');
    expect(html).toContain('aria-label="gluten"');
    expect(html).toContain('class="semantic-mark dietary-mark"');
    expect(html).not.toMatch(/[😀-🙏]/u);
    expect(html).toContain('class="folio-number">1</span>');

    expect(
      renderPlannedPageHtml(model, document, document.pages[0], renderOptions),
    ).toContain('data-page="1"');
  });

  it("keeps multi-item lists in normal flow and reserves full height for a single feature", () => {
    const document = cloneDocument(planFixtureDocument(sparseRestaurantMenu));
    const section = sparseRestaurantMenu.sections[0];
    const listBlock = document.pages
      .flatMap((page) => page.blocks)
      .find((block) => block.kind === "item-list")!;
    listBlock.itemIds = section.items.map((item) => item.id);

    const html = renderPlannedMenuHtml(sparseRestaurantMenu, document, {
      ...renderOptions,
      origin: "",
    });

    for (const item of section.items) {
      expect(html).toContain(`data-item-id="${item.id}"`);
    }
    expect(html).not.toContain(".menu-item{height:100%");
    expect(html).toContain(".block-item-list{overflow:hidden}");
    expect(html).toContain(
      ".block-item-list .menu-item{height:auto;overflow:visible}",
    );
    expect(html).toContain(
      ".block-item-list .menu-item+.menu-item{margin-block-start:2mm}",
    );
    expect(html).toContain(
      ".block-item-feature>.menu-item:only-child{height:100%;overflow:hidden}",
    );
  });

  it.each(["contained", "mark-only", "wordmark", "hidden"] as const)(
    "honors the %s logo treatment",
    (logoTreatment) => {
      const model = cloneModel(photoRichRestaurantMenu);
      model.business.logoUrl = "/images/restaurant-logo.png";
      const document = planFixtureDocument({ model, logoTreatment });
      const html = renderPlannedMenuHtml(model, document, renderOptions);

      if (logoTreatment === "hidden") {
        expect(html).not.toContain('<img class="restaurant-logo');
      } else {
        expect(html).toContain(`logo-${logoTreatment}`);
        expect(html).toContain('src="/images/restaurant-logo.png"');
      }
      expect(html).toContain(
        `<h1 class="restaurant-name">${escapeHtml(model.business.name)}</h1>`,
      );
      expect(html).not.toContain(model.business.customUrl!);
    },
  );

  describe("folded cover identity tiers", () => {
    const extractTier = (html: string, tier: "mark" | "anchor"): string => {
      const match = html.match(
        new RegExp(
          `<div class="cover-tier cover-tier-${tier}">([\\s\\S]*?)</div>`,
        ),
      );
      expect(match?.[1]).toBeDefined();
      return match![1];
    };

    it("composes monogram, title, and place-line tiers on a folded cover", () => {
      const model = cloneModel(photoRichRestaurantMenu);
      model.business.name = "Mercado del Puerto";
      model.business.logoUrl = undefined;
      model.business.address = "246 Defensa, San Telmo";
      const document = planFixtureDocument({
        model,
        familyId: "osteria",
        outputFormat: "folded-booklet",
        coverMode: "none",
        contactPlacement: "footer",
      });
      const html = renderPlannedMenuHtml(model, document, renderOptions);

      expect(html).toContain('<div class="identity-lockup cover-lockup">');
      // Monogram skips particles: "Mercado del Puerto" → MP.
      expect(extractTier(html, "mark")).toContain(
        '<span class="cover-monogram" aria-hidden="true">MP</span>',
      );
      // The place line anchors the cover foot even when it also prints in
      // the interior footer — standard folded-menu identity.
      expect(extractTier(html, "anchor")).toContain(
        escapeHtml(model.business.address!),
      );
      // Osteria hairlines frame the title tier, not the whole lockup.
      expect(html).toContain(".osteria-unequal-columns .cover-tier-title::before");
    });

    it("uses the logo as the cover mark with an empty alt", () => {
      const model = cloneModel(photoRichRestaurantMenu);
      model.business.logoUrl = "/images/restaurant-logo.png";
      const document = planFixtureDocument({
        model,
        familyId: "street",
        outputFormat: "takeaway-trifold",
        coverMode: "none",
        logoTreatment: "contained",
      });
      const html = renderPlannedMenuHtml(model, document, renderOptions);

      const mark = extractTier(html, "mark");
      expect(mark).toContain('src="/images/restaurant-logo.png" alt=""');
      expect(mark).not.toContain("cover-monogram");
    });

    it("falls back to a composed ornament anchor when contact is suppressed", () => {
      const model = cloneModel(photoRichRestaurantMenu);
      model.business.logoUrl = undefined;
      model.business.address = "7 Harbour Lane, Portland";
      const document = planFixtureDocument({
        model,
        familyId: "street",
        outputFormat: "takeaway-trifold",
        coverMode: "none",
        contactPlacement: "none",
      });
      const html = renderPlannedMenuHtml(model, document, renderOptions);

      const anchor = extractTier(html, "anchor");
      expect(anchor).toContain("cover-foot-ornament");
      expect(anchor).toContain("cover-foot-gem");
      expect(anchor).not.toContain(escapeHtml(model.business.address!));
    });

    it("keeps flat-sheet mastheads on the plain lockup", () => {
      const model = cloneModel(photoRichRestaurantMenu);
      const document = planFixtureDocument({
        model,
        familyId: "atelier",
        outputFormat: "single-sheet",
        coverMode: "none",
      });
      const html = renderPlannedMenuHtml(model, document, renderOptions);

      expect(html).toContain('<div class="identity-lockup">');
      expect(html).not.toContain('<div class="identity-lockup cover-lockup">');
      expect(html).not.toContain('class="cover-tier');
    });
  });

  it.each([
    ["takeaway-trifold", "street"] as const,
    ["folded-booklet", "osteria"] as const,
  ])(
    "never prints customUrl and dedupes panel-note copy on %s",
    (outputFormat, familyId) => {
      const model = cloneModel(photoRichRestaurantMenu);
      // Unique slug that cannot appear in fixture logo/image URLs.
      model.business.customUrl = "INTERNAL-SLUG-XYZ-NEVER-PRINT";
      model.business.tagline = "Garden-led cooking by the water";
      model.business.address = "7 Harbour Lane, Portland";
      const document = cloneDocument(
        planFixtureDocument({
          model,
          familyId,
          outputFormat,
          contactPlacement: "footer",
        }),
      );

      // Controlled empty panels in document order: story, story, contact.
      for (const page of document.pages) {
        page.blocks = page.blocks.filter(
          (block) => block.kind !== "panel-note",
        );
      }
      document.pages[0].blocks.push(
        {
          id: "panel-note:story",
          kind: "panel-note",
          rect: { xMm: 10, yMm: 40, widthMm: 40, heightMm: 30 },
        },
        {
          id: "panel-note:story-2",
          kind: "panel-note",
          rect: { xMm: 60, yMm: 40, widthMm: 40, heightMm: 30 },
        },
        {
          id: "panel-note:contact",
          kind: "panel-note",
          rect: { xMm: 110, yMm: 40, widthMm: 40, heightMm: 30 },
        },
      );

      const html = renderPlannedMenuHtml(model, document, {
        ...renderOptions,
        origin: "",
      });

      // (a) customUrl never appears anywhere in printed output.
      expect(html).not.toContain("INTERNAL-SLUG-XYZ-NEVER-PRINT");
      expect(html).not.toContain(model.business.customUrl!);

      const extractPanel = (id: string): string => {
        const match = html.match(
          new RegExp(
            `data-block-id="${id}"[^>]*>([\\s\\S]*?)<\\/div>\\s*<\\/div>`,
          ),
        );
        expect(match?.[1]).toBeDefined();
        return match![1];
      };

      const contact = extractPanel("panel-note:contact");
      expect(contact).toContain(escapeHtml(model.business.address!));
      expect(contact).not.toContain(escapeHtml(model.business.tagline!));
      expect(contact).not.toContain("INTERNAL-SLUG");

      const story1 = extractPanel("panel-note:story");
      expect(story1).toContain(escapeHtml(model.business.tagline!));
      // Address already prints in footer — not repeated as story secondary.
      expect(story1).not.toContain(
        `<span>${escapeHtml(model.business.address!)}</span>`,
      );

      const story2 = extractPanel("panel-note:story-2");
      expect(story2).toContain("panel-note-ornament");
      expect(story2).not.toContain(escapeHtml(model.business.tagline!));
      expect(story2).not.toContain(escapeHtml(model.business.address!));

      // Tagline only once across panel-notes.
      const panelBodies = ["panel-note:story", "panel-note:story-2", "panel-note:contact"]
        .map(extractPanel)
        .join("\n");
      expect(count(panelBodies, escapeHtml(model.business.tagline!))).toBe(1);
    },
  );

  it("shows story-panel address only when contactPlacement is none", () => {
    const model = cloneModel(photoRichRestaurantMenu);
    model.business.customUrl = "NEVER-PRINT-ME-SLUG";
    const document = cloneDocument(
      planFixtureDocument({
        model,
        familyId: "street",
        outputFormat: "takeaway-trifold",
        contactPlacement: "none",
      }),
    );
    // Drop planner panel-notes so this is the first (and only) story panel.
    for (const page of document.pages) {
      page.blocks = page.blocks.filter((block) => block.kind !== "panel-note");
    }
    document.pages[0].blocks.push({
      id: "panel-note:story-only",
      kind: "panel-note",
      rect: { xMm: 10, yMm: 40, widthMm: 40, heightMm: 30 },
    });
    const html = renderPlannedMenuHtml(model, document, {
      ...renderOptions,
      origin: "",
    });
    expect(html).not.toContain("NEVER-PRINT-ME-SLUG");
    const story = html.match(
      /data-block-id="panel-note:story-only"[^>]*>([\s\S]*?)<\/div>\s*<\/div>/,
    )?.[1];
    expect(story).toContain(escapeHtml(model.business.tagline!));
    expect(story).toContain(
      `<span>${escapeHtml(model.business.address!)}</span>`,
    );
  });

  it("renders continuation headings as subordinate with is-continuation CSS", () => {
    const model = cloneModel(sparseRestaurantMenu);
    const document = cloneDocument(planFixtureDocument(model));
    const section = model.sections[0];
    document.pages[0].blocks.push({
      id: "heading-continued",
      kind: "category-heading",
      sectionId: section.id,
      continuation: true,
      rect: { xMm: 12, yMm: 80, widthMm: 60, heightMm: 10 },
    });
    const html = renderPlannedMenuHtml(model, document, {
      ...renderOptions,
      origin: "",
    });

    expect(html).toContain("is-continuation");
    expect(html).toMatch(/class="[^"]*\bis-continuation\b[^"]*"/);
    // Continuation headings stay muted but keep display scale — a "(continued)"
    // label smaller than the dish names beneath it reads as a rendering bug.
    expect(html).toMatch(
      /\.block-category-heading\.is-continuation \.category-name\{font-size:[\d.]+pt;color:var\(--muted\)\}/,
    );
    expect(html).toContain(
      ".block-category-heading.is-continuation .category-name::after{content:none",
    );
    // Style-only — no hardcoded English continuation copy.
    expect(html.toLowerCase()).not.toContain("(continued)");
    expect(html.toLowerCase()).not.toContain("continued)");
  });

  it("prints a marker legend in the footer without duplicates", () => {
    const model = cloneModel(sparseRestaurantMenu);
    for (const section of model.sections) {
      for (const item of section.items) {
        item.dietaryTags = [];
        item.allergens = [];
      }
    }
    model.sections[0].items[0].dietaryTags = ["vegan", "gluten-free"];
    model.sections[0].items[0].allergens = ["fish"];
    // Second item reuses vegan so legend must not duplicate VG.
    model.sections[0].items[1].dietaryTags = ["vegan"];
    model.sections[0].items[1].allergens = [];
    const document = planFixtureDocument({ model, contactPlacement: "footer" });
    const html = renderPlannedMenuHtml(model, document, {
      ...renderOptions,
      origin: "",
    });

    expect(html).toContain('class="footer-marker-legend"');
    const legend = html.match(
      /class="footer-marker-legend">([^<]*)<\/span>/,
    )?.[1];
    expect(legend).toBeDefined();
    expect(legend).toBe("VG Vegan · GF Gluten-free · F Fish");
    expect(count(legend!, "VG ")).toBe(1);
    expect(html).toContain(
      ".footer-marker-legend{display:-webkit-box;-webkit-box-orient:vertical;-webkit-line-clamp:2",
    );
  });

  it("omits the marker legend when no items carry semantic markers", () => {
    const model = cloneModel(sparseRestaurantMenu);
    for (const section of model.sections) {
      for (const item of section.items) {
        item.dietaryTags = [];
        item.allergens = [];
      }
    }
    const document = planFixtureDocument({ model, contactPlacement: "footer" });
    const html = renderPlannedMenuHtml(model, document, {
      ...renderOptions,
      origin: "",
    });
    expect(html).not.toContain('class="footer-marker-legend"');
  });

  it("renders a safe QR once in the primary footer and omits unsafe QR data", () => {
    const document = planFixtureDocument({
      model: sparseRestaurantMenu,
      outputFormat: "two-page-spread",
    });
    const qr = "data:image/png;base64,iVBORw0KGgoAAAANSUhEUgAAAAEAAAAB";
    const html = renderPlannedMenuHtml(sparseRestaurantMenu, document, {
      ...renderOptions,
      qrDataUrl: qr,
      qrCaption: 'Live <menu> "today"',
    });

    expect(count(html, 'class="qr-image"')).toBe(1);
    expect(html).toContain(`src="${qr}"`);
    expect(html).toContain("Live &lt;menu&gt; &quot;today&quot;");

    const unsafe = renderPlannedMenuHtml(sparseRestaurantMenu, document, {
      ...renderOptions,
      qrDataUrl: "data:text/html;base64,PHNjcmlwdD4=",
    });
    expect(unsafe).not.toContain('<img class="qr-image"');
    expect(unsafe).not.toContain("data:text/html");
  });

  it.each([
    ["LTR", sparseRestaurantMenu, '<html lang="en">', "Live menu"],
    [
      "RTL",
      rtlRestaurantMenu,
      '<html lang="ar" dir="rtl">',
      "القائمة المباشرة",
    ],
  ] as const)(
    "contains long %s footer copy beside a compact QR on A5 trifold panels",
    (_writingMode, fixture, htmlDirection, qrCaption) => {
      const model = cloneModel(fixture);
      model.business.address = `${model.business.address} — ${"very long address ".repeat(20)}`;
      model.business.tagline = `${model.business.tagline} — ${"very long tagline ".repeat(20)}`;
      const document = planFixtureDocument({
        model,
        familyId: "counter",
        outputFormat: "takeaway-trifold",
        paperFormat: "a5",
      });
      const qrDataUrl = "data:image/png;base64,AAAA";
      const html = renderPlannedMenuHtml(model, document, {
        ...renderOptions,
        origin: "",
        qrDataUrl,
        qrCaption,
      });

      expect(html).toContain(htmlDirection);
      expect(count(html, '<img class="qr-image"')).toBe(1);
      expect(html).toContain(`alt="${escapeHtml(qrCaption)}"`);
      expect(html).toContain(
        'class="footer-copy has-qr-clearance qr-clearance-compact"',
      );
      expect(html).toContain('class="footer-qr footer-qr-compact"');
      expect(html).not.toContain("<figcaption>");
      expect(html).toContain(
        ".block-footer{max-block-size:12mm;overflow:hidden}",
      );
      expect(html).toContain(
        ".footer-copy{block-size:12mm;max-block-size:12mm;overflow:hidden",
      );
      expect(html).toContain(
        ".footer-copy>*{min-width:0;overflow:hidden;overflow-wrap:anywhere;line-height:1.4}",
      );
      expect(html).toContain(
        ".footer-copy.qr-clearance-compact{padding-inline-end:14mm}",
      );
      expect(html).toContain(
        ".footer-qr-compact{inline-size:14mm;grid-template-columns:10mm",
      );
      expect(html).toContain(".qr-image{width:10mm;height:10mm");

      for (const page of document.pages) {
        const footers = page.blocks.filter((block) => block.kind === "footer");
        expect(footers).toHaveLength(page.index === 0 ? 1 : 0);
        for (const footer of footers) {
          expect(footer.rect.widthMm).toBeLessThan(64);
          expect(footer.rect.heightMm).toBe(12);
          for (const block of page.blocks.filter(
            (candidate) => candidate.kind !== "footer",
          )) {
            const overlapsFooterInline =
              block.rect.xMm < footer.rect.xMm + footer.rect.widthMm &&
              block.rect.xMm + block.rect.widthMm > footer.rect.xMm;
            if (overlapsFooterInline) {
              expect(block.rect.yMm + block.rect.heightMm).toBeLessThanOrEqual(
                footer.rect.yMm,
              );
            }
          }
        }
      }

      const disabled = renderPlannedMenuHtml(model, document, {
        ...renderOptions,
        origin: "",
      });
      expect(disabled).not.toContain("has-qr-clearance");
    },
  );

  it("retains a captioned QR treatment on a wide footer", () => {
    const document = planFixtureDocument({
      model: sparseRestaurantMenu,
      outputFormat: "single-sheet",
      paperFormat: "a4",
    });
    const qrCaption = "Scan for the live menu";
    const html = renderPlannedMenuHtml(sparseRestaurantMenu, document, {
      ...renderOptions,
      origin: "",
      qrDataUrl: "data:image/png;base64,AAAA",
      qrCaption,
    });
    const footer = document.pages[0].blocks.find(
      (block) => block.kind === "footer",
    )!;

    expect(footer.rect.widthMm).toBeGreaterThanOrEqual(64);
    expect(html).toContain(
      'class="footer-copy has-qr-clearance qr-clearance-captioned"',
    );
    expect(html).toContain('class="footer-qr footer-qr-captioned"');
    expect(html).toContain(`<figcaption>${qrCaption}</figcaption>`);
    expect(html).toContain(
      ".footer-copy.qr-clearance-captioned{padding-inline-end:32mm}",
    );
    expect(html).toContain(
      ".footer-qr-captioned{inline-size:30mm;grid-template-columns:10mm 1fr}",
    );
  });

  it("throws deterministic errors for missing section and item references", () => {
    const sectionDocument = cloneDocument(
      planFixtureDocument(sparseRestaurantMenu),
    );
    const sectionBlock = sectionDocument.pages
      .flatMap((page) => page.blocks)
      .find((block) => block.kind === "category-heading")!;
    sectionBlock.sectionId = "missing-section";
    expect(() =>
      renderPlannedMenuHtml(
        sparseRestaurantMenu,
        sectionDocument,
        renderOptions,
      ),
    ).toThrow("Planned block references missing section: missing-section");

    const itemDocument = cloneDocument(
      planFixtureDocument(sparseRestaurantMenu),
    );
    const itemBlock = itemDocument.pages
      .flatMap((page) => page.blocks)
      .find((block) => block.kind === "item-list")!;
    itemBlock.itemIds = ["missing-item"];
    expect(() =>
      renderPlannedMenuHtml(sparseRestaurantMenu, itemDocument, renderOptions),
    ).toThrow("Planned block references missing item: missing-item");
  });

  it("rejects incomplete palettes and unsafe runtime document tokens", () => {
    const incompletePalette = cloneDocument(
      planFixtureDocument(sparseRestaurantMenu),
    );
    delete (
      incompletePalette.direction.palette as Partial<
        typeof incompletePalette.direction.palette
      >
    ).muted;
    expect(() =>
      renderPlannedMenuHtml(
        sparseRestaurantMenu,
        incompletePalette,
        renderOptions,
      ),
    ).toThrow("Invalid print palette color: muted");

    const unsafeToken = cloneDocument(
      planFixtureDocument(sparseRestaurantMenu),
    );
    unsafeToken.direction.treatment = 'compact\" onload=\"alert(1)' as never;
    expect(() =>
      renderPlannedMenuHtml(sparseRestaurantMenu, unsafeToken, renderOptions),
    ).toThrow("Invalid print document token: treatment");
  });

  it("rejects a detached page object in the page renderer", () => {
    const document = planFixtureDocument(sparseRestaurantMenu);
    const detachedPage = cloneDocument(document).pages[0];

    expect(() =>
      renderPlannedPageHtml(
        sparseRestaurantMenu,
        document,
        detachedPage,
        renderOptions,
      ),
    ).toThrow("Planned page does not belong to document: 0");
  });

  it.each<[string, number, (document: PlannedMenuDocument) => void]>([
    ["duplicate", 1, (document) => (document.pages[1].index = 0)],
    ["non-integer", 0, (document) => (document.pages[0].index = 0.5)],
    ["nonsequential", 0, (document) => (document.pages[0].index = 1)],
    ["negative", 0, (document) => (document.pages[0].index = -1)],
    [
      "runtime string/injection",
      0,
      (document) =>
        ((document.pages[0] as unknown as { index: unknown }).index =
          '0\" onload=\"alert(1)'),
    ],
  ])("rejects %s planned page indices", (_name, position, mutate) => {
    const document = cloneDocument(
      planFixtureDocument({
        model: sparseRestaurantMenu,
        outputFormat: "two-page-spread",
      }),
    );
    mutate(document);

    expect(() =>
      renderPlannedMenuHtml(sparseRestaurantMenu, document, renderOptions),
    ).toThrow(`Invalid planned page index at position ${position}`);
  });

  it("rejects unsafe runtime block kind and image role tokens", () => {
    const unsafeKind = cloneDocument(planFixtureDocument(sparseRestaurantMenu));
    unsafeKind.pages[0].blocks[0].kind =
      'masthead\" onload=\"alert(1)' as never;
    expect(() =>
      renderPlannedMenuHtml(sparseRestaurantMenu, unsafeKind, renderOptions),
    ).toThrow("Invalid print document token: blockKind");

    const harvest = photoRichRestaurantMenu.sections[0].items[0];
    const imageDocument = cloneDocument(
      planFixtureDocument({
        model: photoRichRestaurantMenu,
        familyId: "gallery",
        treatment: "photo-led",
        imageSelections: [
          {
            itemId: harvest.id,
            url: harvest.imageCandidates![1],
            role: "editorial-crop",
          },
        ],
      }),
    );
    const imageBlock = imageDocument.pages
      .flatMap((page) => page.blocks)
      .find((block) => block.kind === "image")!;
    imageBlock.image!.role = 'cover\" onload=\"alert(2)' as never;
    expect(() =>
      renderPlannedMenuHtml(
        photoRichRestaurantMenu,
        imageDocument,
        renderOptions,
      ),
    ).toThrow("Invalid print document token: imageRole");
  });

  it.each(MENU_DESIGN_FAMILY_IDS)(
    "includes only the active %s display/body font families",
    (familyId) => {
      const family = getMenuDesignFamily(familyId);
      const active = new Set([
        family.typography.display,
        family.typography.body,
      ]);
      const html = renderPlannedMenuHtml(
        sparseRestaurantMenu,
        planFixtureDocument({ model: sparseRestaurantMenu, familyId }),
        renderOptions,
      );

      for (const familyName of Object.keys(PRINT_FONT_FAMILIES)) {
        const hasFontFace = html.includes(`font-family: "${familyName}";`);
        expect(hasFontFace).toBe(active.has(familyName as never));
      }
      expect(count(html, `src: url("${renderOptions.origin}/fonts/`)).toBe(
        [...active].reduce(
          (total, familyName) => total + PRINT_FONT_FAMILIES[familyName].length,
          0,
        ),
      );
    },
  );

  it("preserves root-relative active font URLs for an empty origin", () => {
    const family = getMenuDesignFamily("atelier");
    const html = renderPlannedMenuHtml(
      sparseRestaurantMenu,
      planFixtureDocument({ model: sparseRestaurantMenu, familyId: family.id }),
      { ...renderOptions, origin: "" },
    );

    expect(html).toContain('src: url("/fonts/');
    expect(html).not.toContain('src: url("https://');
    expect(html).toContain(`font-family: "${family.typography.display}";`);
    expect(html).toContain(`font-family: "${family.typography.body}";`);
  });

  it("uses exact physical page CSS and planned trifold panel coordinates", () => {
    const document = planFixtureDocument({
      model: sparseRestaurantMenu,
      familyId: "osteria",
      outputFormat: "takeaway-trifold",
    });
    const html = renderPlannedMenuHtml(
      sparseRestaurantMenu,
      document,
      renderOptions,
    );

    expect(html).toContain(
      "@page { size: var(--page-width) var(--page-height); margin:0; }",
    );
    expect(html).toContain(".planned-block{position:absolute}");
    expect(html).toContain("break-after:page");
    expect(html).toContain("overflow:hidden");
    expect(html).toContain("print-color-adjust:exact");
    expect(html).toContain("-webkit-print-color-adjust:exact");
    expect(html).toContain(".print-page:last-child{break-after:auto}");

    for (const block of document.pages.flatMap((page) => page.blocks)) {
      expect(html).toContain(
        `inset-inline-start:${block.rect.xMm}mm;top:${block.rect.yMm}mm;width:${block.rect.widthMm}mm;height:${block.rect.heightMm}mm`,
      );
    }
  });

  it("places QR content once even if malformed plans repeat footer ids", () => {
    const document = cloneDocument(
      planFixtureDocument({
        model: sparseRestaurantMenu,
        outputFormat: "two-page-spread",
      }),
    );
    const footers = document.pages
      .flatMap((page) => page.blocks)
      .filter((block) => block.kind === "footer");
    expect(footers).toHaveLength(2);
    footers[1].id = footers[0].id;

    const html = renderPlannedMenuHtml(sparseRestaurantMenu, document, {
      ...renderOptions,
      qrDataUrl: "data:image/png;base64,AAAA",
    });

    expect(count(html, '<img class="qr-image"')).toBe(1);
  });
});

const FAMILY_SIGNATURES: Record<MenuDesignFamilyId, readonly string[]> = {
  atelier: [
    "atelier-asymmetric-masthead",
    "atelier-offset-heading",
    "atelier-editorial-side-crop",
  ],
  maison: ["maison-course-axis", "maison-double-rules", "maison-inset-frame"],
  osteria: [
    "osteria-unequal-columns",
    "osteria-category-opener",
    "osteria-lively-rhythm",
  ],
  "night-house": [
    "night-house-cinematic-surface",
    "night-house-frame",
    "night-house-wide-band",
  ],
  counter: [
    "counter-modular-tile",
    "counter-filled-label",
    "counter-dominant-special",
  ],
  street: [
    "street-condensed-scan",
    "street-scan-bar",
    "street-price-rail",
    "street-full-description",
  ],
  field: [
    "field-ingredient-spacing",
    "field-narrow-measure",
    "field-organic-restraint",
  ],
  gallery: [
    "gallery-image-dominant",
    "gallery-negative-space",
    "gallery-photo-field",
  ],
};

// Still assigned to blocks by structureClassForBlock (see renderPlannedHtml.ts)
// but no longer targeted by any CSS selector: their families now style the
// affected elements through family+element selectors (e.g. .family-atelier
// .item-price) instead of a dedicated structural-slot class.
const UNSTYLED_STRUCTURE_CLASSES = new Set<string>([
  "atelier-offset-heading",
  "osteria-lively-rhythm",
  "street-price-rail",
  "gallery-image-dominant",
]);

describe("family visual systems", () => {
  it.each(MENU_DESIGN_FAMILY_IDS)(
    "emits a unique structural CSS system for %s",
    (familyId) => {
      const document = planFixtureDocument({
        model: photoRichRestaurantMenu,
        familyId,
      });
      const html = renderPlannedMenuHtml(
        photoRichRestaurantMenu,
        document,
        renderOptions,
      );

      for (const signature of FAMILY_SIGNATURES[familyId]) {
        if (UNSTYLED_STRUCTURE_CLASSES.has(signature)) continue;
        expect(html).toContain(`.${signature}`);
      }
      for (const otherId of MENU_DESIGN_FAMILY_IDS) {
        if (otherId === familyId) continue;
        const otherSignature = FAMILY_SIGNATURES[otherId][0];
        if (UNSTYLED_STRUCTURE_CLASSES.has(otherSignature)) continue;
        expect(html).not.toContain(`.${otherSignature}`);
      }
      expect(html).toContain(`family-${familyId}`);
      expect(html).toContain(
        `data-composition="${getMenuDesignFamily(familyId).compositions[0]}"`,
      );
      expect(html).toContain(
        `data-treatment="${document.direction.treatment}"`,
      );
      expect(html).toContain(
        `data-format="${document.direction.outputFormat}"`,
      );
    },
  );

  it.each([
    [
      "atelier",
      {
        "padding-inline-start": "5mm",
        "border-inline-start": ".3mm solid var(--accent)",
      },
    ],
    [
      "counter",
      {
        "border-block-start": ".6mm solid var(--accent)",
        "padding-block-start": "2mm",
      },
    ],
    ["gallery", { "margin-inline-start": "8mm" }],
  ] as const)(
    "applies %s feature styling to the emitted block-item-feature class",
    (familyId, expectedStyle) => {
      const document = cloneDocument(
        planFixtureDocument({ model: sparseRestaurantMenu, familyId }),
      );
      const feature = document.pages
        .flatMap((page) => page.blocks)
        .find(
          (block) =>
            block.kind === "item-list" || block.kind === "item-feature",
        )!;
      feature.kind = "item-feature";

      const html = renderPlannedMenuHtml(sparseRestaurantMenu, document, {
        ...renderOptions,
        origin: "",
      });
      const featureClasses = html
        .match(/class="([^"]+)"[^>]+data-kind="item-feature"/)?.[1]
        .split(/\s+/);
      const computed = computeClassStyle(
        html,
        ["planned-block", "block-item-feature", `family-${familyId}`],
        [`family-${familyId}`],
      );

      expect(featureClasses).toContain("block-item-feature");
      expect(featureClasses).not.toContain("item-feature");
      expect(html).toContain(`.family-${familyId} .block-item-feature`);
      expect(computed).toMatchObject(expectedStyle);
    },
  );

  const ORNAMENT_RULES: Record<
    ReturnType<typeof getMenuDesignFamily>["ornament"],
    {
      restrainedTarget: string;
      expressiveTarget: string;
      restrained: string;
      expressive: string;
    }
  > = {
    none: {
      restrainedTarget: ".block-category-heading",
      expressiveTarget: ".block-category-heading",
      restrained: "border:0;background:none",
      expressive: "border:0;background:none",
    },
    // Restrained "rule" decoration moved off the heading box onto a
    // ::after underline beneath the category name; expressive still
    // decorates the heading box itself, now via a logical border-block-start.
    rule: {
      restrainedTarget: ".category-name::after",
      expressiveTarget: ".block-category-heading",
      restrained:
        'content:"";display:block;inline-size:11mm;border-block-start:.5mm solid var(--accent);margin-block-start:1.8mm',
      expressive:
        "border-block-start:.75mm double var(--accent);padding-block-start:2mm",
    },
    "double-rule": {
      restrainedTarget: ".block-category-heading",
      expressiveTarget: ".block-category-heading",
      restrained: "border-block:.45mm double var(--accent)",
      expressive: "border-block:1mm double var(--accent)",
    },
    frame: {
      restrainedTarget: ".print-page",
      expressiveTarget: ".print-page",
      restrained: "outline:.3mm solid var(--accent)",
      expressive: "outline:.8mm double var(--accent)",
    },
    // Restrained "block" decoration is family-specific now (see
    // RESTRAINED_BLOCK_RULES); expressive still fills the heading box, and
    // resets --muted between the ink swap and the box-shadow.
    block: {
      restrainedTarget: ".block-category-heading",
      expressiveTarget: ".block-category-heading",
      restrained: "",
      expressive:
        "background:var(--ink);color:var(--ground);--muted:color-mix(in srgb,currentColor 75%,transparent);box-shadow:inset 0 -.8mm 0 var(--accent)",
    },
  };

  // The two block-native families diverge in restrained mode: street keeps
  // its full-width ink scan bar; counter drops the fill for a typographic
  // label with an accent underline on the category name.
  const RESTRAINED_BLOCK_RULES: Record<
    string,
    { selector: string; css: string }
  > = {
    street: {
      selector: ".family-street.ornament-restrained .block-category-heading",
      css: "background:var(--ink);color:var(--ground)",
    },
    counter: {
      selector: ".family-counter.ornament-restrained .category-name::after",
      css: 'content:"";display:block;inline-size:100%;border-block-start:1.1mm solid var(--accent);margin-block-start:1.2mm',
    },
  };

  it.each(MENU_DESIGN_FAMILY_IDS)(
    "connects %s native ornament to none, restrained, and expressive CSS",
    (familyId) => {
      const family = getMenuDesignFamily(familyId);
      const rule = ORNAMENT_RULES[family.ornament];
      const render = (
        ornamentIntensity: "none" | "restrained" | "expressive",
      ) =>
        renderPlannedMenuHtml(
          sparseRestaurantMenu,
          planFixtureDocument({
            model: sparseRestaurantMenu,
            familyId,
            ornamentIntensity,
          }),
          { ...renderOptions, origin: "" },
        );

      const none = render("none");
      const restrained = render("restrained");
      const expressive = render("expressive");

      for (const html of [none, restrained, expressive]) {
        expect(html).toContain(`native-ornament-${family.ornament}`);
        expect(html).toContain(".ornament-none .block-category-heading");
        expect(html).toContain(".ornament-none .print-page");
      }
      expect(none).toContain("ornament-none");
      expect(restrained).toContain("ornament-restrained");
      expect(expressive).toContain("ornament-expressive");
      const restrainedBlockRule = RESTRAINED_BLOCK_RULES[familyId];
      if (restrainedBlockRule) {
        expect(restrained).toContain(
          `${restrainedBlockRule.selector}{${restrainedBlockRule.css}`,
        );
      } else {
        expect(restrained).toContain(
          `.ornament-restrained.native-ornament-${family.ornament} ${rule.restrainedTarget}{${rule.restrained}`,
        );
      }
      expect(expressive).toContain(
        `.ornament-expressive.native-ornament-${family.ornament} ${rule.expressiveTarget}{${rule.expressive}`,
      );
    },
  );

  it.each(MENU_DESIGN_FAMILY_IDS)(
    "computes %s ornament intensity after family decoration rules",
    (familyId) => {
      const family = getMenuDesignFamily(familyId);
      const headingClass = FAMILY_SIGNATURES[familyId][1];
      const render = (
        ornamentIntensity: "none" | "restrained" | "expressive",
      ) =>
        renderPlannedMenuHtml(
          sparseRestaurantMenu,
          planFixtureDocument({
            model: sparseRestaurantMenu,
            familyId,
            ornamentIntensity,
          }),
          { ...renderOptions, origin: "" },
        );
      const computed = (
        html: string,
        ornamentIntensity: "none" | "restrained" | "expressive",
        elementClasses: readonly string[],
      ) =>
        computeClassStyle(html, elementClasses, [
          `family-${familyId}`,
          `ornament-${ornamentIntensity}`,
          `native-ornament-${family.ornament}`,
        ]);

      const noneHtml = render("none");
      const noneHeading = computed(noneHtml, "none", [
        "block-category-heading",
        headingClass,
      ]);
      const nonePage = computed(noneHtml, "none", ["print-page"]);
      expect(noneHeading).toMatchObject({
        border: "0",
        background: "none",
        color: "inherit",
      });
      expect(nonePage.outline).toBe("0");

      for (const ornamentIntensity of ["restrained", "expressive"] as const) {
        const html = render(ornamentIntensity);
        const heading = computed(html, ornamentIntensity, [
          "block-category-heading",
          headingClass,
        ]);
        const page = computed(html, ornamentIntensity, ["print-page"]);
        const expected = ORNAMENT_RULES[family.ornament][ornamentIntensity];

        if (family.ornament === "rule") {
          // Restrained mode no longer decorates the heading box directly
          // (the underline moved to .category-name::after, a different
          // element than the synthetic heading we're computing here).
          if (ornamentIntensity === "restrained") {
            expect(heading["border-block-start"]).toBeUndefined();
          } else {
            expect(heading["border-block-start"]).toBe(
              expected.split(";")[0].split(":")[1],
            );
          }
          expect(heading.border).toBe("0");
          expect(heading.background).toBe("none");
        } else if (family.ornament === "double-rule") {
          expect(heading["border-block"]).toBe(
            expected.split(":")[1].split(";")[0],
          );
          expect(page.outline).toBe("0");
        } else if (family.ornament === "frame") {
          expect(page.outline).toBe(expected.split(":")[1].split(";")[0]);
          expect(heading.border).toBe("0");
        } else if (family.ornament === "block") {
          if (ornamentIntensity === "restrained") {
            // Restrained block treatments diverge per family: street keeps
            // its full-width ink scan bar, while counter drops the fill for
            // a typographic label whose accent underline lives on
            // .category-name::after (a different element than the synthetic
            // heading computed here).
            if (familyId === "street") {
              expect(heading).toMatchObject({
                background: "var(--ink)",
                color: "var(--ground)",
              });
            } else {
              expect(heading.background).toBe("none");
            }
          } else {
            expect(heading.background).toBe(
              expected.split(":")[1].split(";")[0],
            );
          }
        } else {
          expect(heading).toMatchObject({ border: "0", background: "none" });
          expect(page.outline).toBe("0");
        }
      }
    },
  );
});

describe("escapeHtml", () => {
  it("escapes text and quoted attribute delimiters", () => {
    expect(escapeHtml(`&<>"'`)).toBe("&amp;&lt;&gt;&quot;&#39;");
  });
});

describe("test fixture sanity", () => {
  it("keeps family defaults compatible", () => {
    for (const familyId of MENU_DESIGN_FAMILY_IDS) {
      expect(makeDirection({ familyId }).familyId).toBe(familyId);
    }
  });
});
