/* eslint-disable no-restricted-syntax -- deterministic SVG fixture colors are test data */
import { readFile } from "node:fs/promises";
import path from "node:path";

import { expect, test, type Page } from "@playwright/test";

import {
  cjkRestaurantMenu,
  denseRestaurantMenu,
  hebrewRestaurantMenu,
  photoRichRestaurantMenu,
  rtlRestaurantMenu,
  sparseRestaurantMenu,
} from "../src/lib/menuPrint/__fixtures__/restaurantMenus";
import {
  makeDirection,
  makeAudit,
} from "../src/lib/menuPrint/__fixtures__/plannedDocuments";
import { getMenuDesignFamily } from "../src/lib/menuPrint/families";
import {
  getPrintScriptFontFallback,
  type PrintFontFamilyName,
} from "../src/lib/menuPrint/fonts";
import { planMenuDocument } from "../src/lib/menuPrint/planner/planDocument";
import { renderMenuPrintHtml } from "../src/lib/menuPrint/renderHtml";
import type {
  FeaturedImageSelection,
  MenuArtDirection,
  MenuDesignFamilyId,
  MenuOutputFormat,
  MenuTreatment,
  PaperFormat,
  PrintMenuModel,
} from "../src/lib/menuPrint/types";

const ASSET_ORIGIN = "https://assets.payverge.test";
const FONT_TEST_WEIGHT: Record<PrintFontFamilyName, number> = {
  "DM Serif Display": 400,
  "DM Sans": 400,
  "EB Garamond": 400,
  "Cormorant Garamond": 500,
  Oswald: 600,
  "Noto Naskh Arabic": 400,
  "Noto Sans Arabic": 400,
  "Noto Sans Hebrew": 400,
  "Noto Serif CJK JP": 400,
  "Noto Sans CJK JP": 400,
  "Noto Sans CJK KR": 400,
  "Noto Sans CJK SC": 400,
  "Noto Sans CJK TC": 400,
};

const photoCompactRestaurantMenu: PrintMenuModel = {
  ...photoRichRestaurantMenu,
  sections: [
    {
      ...photoRichRestaurantMenu.sections[0]!,
      items: photoRichRestaurantMenu.sections[0]!.items.slice(0, 2),
    },
  ],
};

interface VisualCase {
  name: string;
  familyId: MenuDesignFamilyId;
  model: PrintMenuModel;
  treatment: MenuTreatment;
  outputFormat: MenuOutputFormat;
  paperFormat: PaperFormat;
  withPhotos?: boolean;
}

const visualCases: VisualCase[] = [
  {
    name: "atelier-editorial",
    familyId: "atelier",
    model: sparseRestaurantMenu,
    treatment: "type-led",
    outputFormat: "single-sheet",
    paperFormat: "a4",
  },
  {
    name: "maison-formal",
    familyId: "maison",
    model: photoRichRestaurantMenu,
    treatment: "photo-led",
    outputFormat: "two-page-spread",
    paperFormat: "letter",
    withPhotos: true,
  },
  {
    name: "osteria-abundant",
    familyId: "osteria",
    model: denseRestaurantMenu,
    treatment: "balanced",
    outputFormat: "folded-booklet",
    paperFormat: "a4",
  },
  {
    name: "night-house-cinematic",
    familyId: "night-house",
    model: photoCompactRestaurantMenu,
    treatment: "photo-led",
    outputFormat: "drinks-card",
    paperFormat: "a5",
    withPhotos: true,
  },
  {
    name: "counter-modular",
    familyId: "counter",
    model: sparseRestaurantMenu,
    treatment: "compact",
    outputFormat: "counter-menu",
    paperFormat: "letter",
  },
  {
    name: "street-graphic",
    familyId: "street",
    model: photoRichRestaurantMenu,
    treatment: "photo-led",
    outputFormat: "takeaway-trifold",
    paperFormat: "a4",
    withPhotos: true,
  },
  {
    name: "field-seasonal",
    familyId: "field",
    model: sparseRestaurantMenu,
    treatment: "balanced",
    outputFormat: "single-sheet",
    paperFormat: "half-letter",
  },
  {
    name: "gallery-image-led",
    familyId: "gallery",
    model: photoRichRestaurantMenu,
    treatment: "photo-led",
    outputFormat: "two-page-spread",
    paperFormat: "a4",
    withPhotos: true,
  },
  {
    name: "rtl-arabic",
    familyId: "atelier",
    model: rtlRestaurantMenu,
    treatment: "balanced",
    outputFormat: "single-sheet",
    paperFormat: "a5",
  },
  {
    name: "rtl-hebrew",
    familyId: "maison",
    model: hebrewRestaurantMenu,
    treatment: "balanced",
    outputFormat: "single-sheet",
    paperFormat: "a4",
  },
  {
    name: "cjk-japanese",
    familyId: "gallery",
    model: cjkRestaurantMenu,
    treatment: "balanced",
    outputFormat: "single-sheet",
    paperFormat: "a4",
  },
];

function deterministicImageSvg(url: string): string {
  let hash = 0;
  for (const character of url)
    hash = (hash * 31 + character.charCodeAt(0)) >>> 0;
  const hue = hash % 360;
  const secondaryHue = (hue + 48) % 360;
  return `<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 1600 1200">
    <defs><linearGradient id="g" x1="0" y1="0" x2="1" y2="1"><stop stop-color="hsl(${hue} 42% 32%)"/><stop offset="1" stop-color="hsl(${secondaryHue} 64% 68%)"/></linearGradient></defs>
    <rect width="1600" height="1200" fill="url(#g)"/>
    <ellipse cx="760" cy="650" rx="520" ry="330" fill="hsl(${secondaryHue} 32% 88% / .72)"/>
    <circle cx="690" cy="610" r="210" fill="hsl(${hue} 55% 42% / .75)"/>
    <path d="M420 790c230-250 510-290 790-60-190 210-530 270-790 60Z" fill="hsl(${secondaryHue} 65% 26% / .58)"/>
  </svg>`;
}

async function installAssetRoutes(page: Page): Promise<void> {
  await page.route(`${ASSET_ORIGIN}/**`, async (route) => {
    const requestUrl = new URL(route.request().url());
    if (requestUrl.pathname.startsWith("/fonts/")) {
      const publicPath = path.resolve(
        process.cwd(),
        "public",
        requestUrl.pathname.slice(1),
      );
      if (!publicPath.startsWith(path.resolve(process.cwd(), "public"))) {
        await route.abort();
        return;
      }
      await route.fulfill({
        status: 200,
        contentType: "font/woff2",
        headers: { "access-control-allow-origin": "*" },
        body: await readFile(publicPath),
      });
      return;
    }
    await route.fulfill({
      status: 200,
      contentType: "image/svg+xml",
      headers: { "access-control-allow-origin": "*" },
      body: deterministicImageSvg(requestUrl.pathname),
    });
  });
  await page.route("https://images.payverge.test/**", async (route) => {
    await route.fulfill({
      status: 200,
      contentType: "image/svg+xml",
      headers: { "access-control-allow-origin": "*" },
      body: deterministicImageSvg(route.request().url()),
    });
  });
}

function imageSelectionsFor(
  model: PrintMenuModel,
  familyId: MenuDesignFamilyId,
): FeaturedImageSelection[] {
  const family = getMenuDesignFamily(familyId);
  const candidates = model.sections.flatMap((section) =>
    section.items.flatMap((item) =>
      (item.imageCandidates ?? [])
        .filter((url) => /^https:\/\//.test(url))
        .slice(0, 1)
        .map((url) => ({ itemId: item.id, url })),
    ),
  );
  return candidates
    .slice(0, Math.max(1, family.photography.maxFeatureImagesPerPage))
    .map((selection, index) => ({
      ...selection,
      role: family.photography.roles[index % family.photography.roles.length]!,
    }));
}

function directionFor(entry: VisualCase): MenuArtDirection {
  return makeDirection({
    familyId: entry.familyId,
    treatment: entry.treatment,
    outputFormat: entry.outputFormat,
    paperFormat: entry.paperFormat,
    imageSelections: entry.withPhotos
      ? imageSelectionsFor(entry.model, entry.familyId)
      : [],
    ornamentIntensity:
      entry.familyId === "street" || entry.familyId === "night-house"
        ? "expressive"
        : "restrained",
    coverMode:
      entry.withPhotos &&
      getMenuDesignFamily(entry.familyId).photography.roles.includes("cover")
        ? "photographic"
        : "none",
  });
}

async function waitForPrintAssets(
  page: Page,
  familyId: MenuDesignFamilyId,
  language: string,
): Promise<void> {
  const family = getMenuDesignFamily(familyId);
  const scriptFallback = getPrintScriptFontFallback(language);
  const fontRequirements = [
    ...new Set([
      family.typography.display,
      family.typography.body,
      ...(scriptFallback?.families ?? []),
    ]),
  ].map((name) => ({ name, weight: FONT_TEST_WEIGHT[name] }));
  const fontState = await page.evaluate(async (requirements) => {
    await document.fonts.ready;
    await Promise.all(
      [...document.images].map(async (image) => {
        if (!image.complete) {
          await new Promise<void>((resolve, reject) => {
            image.addEventListener("load", () => resolve(), { once: true });
            image.addEventListener(
              "error",
              () => reject(new Error(image.src)),
              {
                once: true,
              },
            );
          });
        }
        await image.decode();
      }),
    );
    const loaded = await Promise.all(
      requirements.map(async ({ name, weight }) => {
        const descriptor = `${weight} 16px "${name}"`;
        const faces = await document.fonts.load(descriptor, "Menu");
        return {
          name,
          faceCount: faces.length,
          ready: document.fonts.check(descriptor, "Menu"),
        };
      }),
    );
    await document.fonts.ready;
    return { status: document.fonts.status, loaded };
  }, fontRequirements);
  expect(fontState.status).toBe("loaded");
  for (const font of fontState.loaded) {
    expect(
      font.faceCount,
      `${font.name} must resolve to a self-hosted font face`,
    ).toBeGreaterThan(0);
    expect(
      font.ready,
      `${font.name} must be usable before the screenshot`,
    ).toBe(true);
  }
}

async function expectHealthyPrintLayout(
  page: Page,
  expectedFamily: MenuDesignFamilyId,
  expectedItemIds: string[],
): Promise<void> {
  const inspection = await page.evaluate(() => {
    const pages = [...document.querySelectorAll<HTMLElement>(".print-page")];
    const viewportWidth = document.documentElement.clientWidth;
    const bodyRect = document.body.getBoundingClientRect();
    const bodyStyle = getComputedStyle(document.body);
    const offViewportPages = pages
      .filter((pageNode) => {
        const rect = pageNode.getBoundingClientRect();
        return rect.left < -1 || rect.right > viewportWidth + 1;
      })
      .map((pageNode) => {
        const rect = pageNode.getBoundingClientRect();
        return `${pageNode.dataset.page ?? "unknown"} (${rect.left.toFixed(1)}..${rect.right.toFixed(1)} of ${viewportWidth}; body ${bodyRect.left.toFixed(1)}..${bodyRect.right.toFixed(1)} ${bodyStyle.display}/${bodyStyle.alignItems})`;
      });
    const blocks = [
      ...document.querySelectorAll<HTMLElement>(".planned-block"),
    ];
    const images = [
      ...document.querySelectorAll<HTMLImageElement>(
        ".planned-image, .cover-image, .restaurant-logo",
      ),
    ];
    const clippedBlocks = blocks
      .filter((block) => {
        const pageNode = block.closest<HTMLElement>(".print-page");
        if (!pageNode) return true;
        const blockRect = block.getBoundingClientRect();
        const pageRect = pageNode.getBoundingClientRect();
        const tolerance = 1;
        return (
          blockRect.left < pageRect.left - tolerance ||
          blockRect.top < pageRect.top - tolerance ||
          blockRect.right > pageRect.right + tolerance ||
          blockRect.bottom > pageRect.bottom + tolerance
        );
      })
      .map((block) => block.dataset.blockId ?? "unknown");
    const emptyImages = images
      .filter(
        (image) =>
          !image.complete ||
          image.naturalWidth === 0 ||
          image.naturalHeight === 0,
      )
      .map((image) => image.src);
    const stretchedImages = images
      .filter((image) => {
        const style = getComputedStyle(image);
        if (style.objectFit === "cover" || style.objectFit === "contain")
          return false;
        const displayedRatio =
          image.clientWidth / Math.max(1, image.clientHeight);
        const naturalRatio =
          image.naturalWidth / Math.max(1, image.naturalHeight);
        return Math.abs(displayedRatio - naturalRatio) > 0.08;
      })
      .map((image) => image.src);
    const emptyImageFrames = [
      ...document.querySelectorAll<HTMLElement>(".block-image"),
    ]
      .filter(
        (frame) =>
          !frame.querySelector<HTMLImageElement>(
            "img.planned-image[src], img.cover-image[src]",
          ),
      )
      .map((frame) => frame.dataset.blockId ?? "unknown");
    const tinyText = [
      ...document.querySelectorAll<HTMLElement>(
        ".restaurant-name, .category-name, .item-name, .item-description",
      ),
    ]
      .filter((node) => Number.parseFloat(getComputedStyle(node).fontSize) < 8)
      .map((node) => node.textContent?.trim() ?? "");
    const itemIds = [...document.querySelectorAll<HTMLElement>(".menu-item")]
      .map((node) => node.dataset.itemId)
      .filter((itemId): itemId is string => Boolean(itemId));
    const emptyPageNumbers = pages
      .filter(
        (pageNode) =>
          !pageNode.querySelector(
            '.planned-block:not([data-kind="footer"]):not([data-kind="folio"])',
          ),
      )
      .map((pageNode) => pageNode.dataset.page ?? "unknown");
    const semanticNodes = [
      ...document.querySelectorAll<HTMLElement>(
        ".restaurant-logo, .restaurant-name, .restaurant-tagline, .category-name, .category-description, .block-image, .menu-item",
      ),
    ].filter((node) => node.getClientRects().length > 0);
    const clippedOverflowNodes = [
      ...document.querySelectorAll<HTMLElement>(".planned-block, .menu-item"),
    ]
      .filter((node) => {
        const style = getComputedStyle(node);
        const clipsX =
          style.overflowX === "hidden" || style.overflowX === "clip";
        const clipsY =
          style.overflowY === "hidden" || style.overflowY === "clip";
        return (
          (clipsX && node.scrollWidth > node.clientWidth + 1) ||
          (clipsY && node.scrollHeight > node.clientHeight + 1)
        );
      })
      .map((node) => {
        const label =
          node.dataset.itemId ??
          node.dataset.blockId ??
          node.textContent?.trim().slice(0, 36) ??
          node.className;
        return `${label} (client ${node.clientWidth}px × ${node.clientHeight}px; scroll ${node.scrollWidth}px × ${node.scrollHeight}px; overflow ${Math.max(0, node.scrollWidth - node.clientWidth)}px × ${Math.max(0, node.scrollHeight - node.clientHeight)}px)`;
      });
    const semanticOverlaps: string[] = [];
    for (let leftIndex = 0; leftIndex < semanticNodes.length; leftIndex += 1) {
      const left = semanticNodes[leftIndex]!;
      const leftPage = left.closest<HTMLElement>(".print-page");
      const leftRect = left.getBoundingClientRect();
      for (
        let rightIndex = leftIndex + 1;
        rightIndex < semanticNodes.length;
        rightIndex += 1
      ) {
        const right = semanticNodes[rightIndex]!;
        if (leftPage !== right.closest<HTMLElement>(".print-page")) continue;
        if (left.contains(right) || right.contains(left)) continue;
        const rightRect = right.getBoundingClientRect();
        const intersectionWidth =
          Math.min(leftRect.right, rightRect.right) -
          Math.max(leftRect.left, rightRect.left);
        const intersectionHeight =
          Math.min(leftRect.bottom, rightRect.bottom) -
          Math.max(leftRect.top, rightRect.top);
        if (intersectionWidth > 1 && intersectionHeight > 1) {
          const label = (node: HTMLElement) =>
            node.dataset.itemId ??
            node.textContent?.trim().slice(0, 36) ??
            node.className;
          semanticOverlaps.push(
            `page ${leftPage?.dataset.page ?? "?"}: ${label(left)} [${leftRect.x.toFixed(1)},${leftRect.y.toFixed(1)},${leftRect.width.toFixed(1)},${leftRect.height.toFixed(1)}] <> ${label(right)} [${rightRect.x.toFixed(1)},${rightRect.y.toFixed(1)},${rightRect.width.toFixed(1)},${rightRect.height.toFixed(1)}]`,
          );
        }
      }
    }
    const isFolded =
      document.body.classList.contains("format-folded-booklet") ||
      document.body.classList.contains("format-takeaway-trifold");
    const miscenteredFoldedIdentities = isFolded
      ? [
          ...document.querySelectorAll<HTMLElement>(
            ".block-masthead .identity-lockup",
          ),
        ]
          .map((lockup) => {
            const masthead = lockup.closest<HTMLElement>(".block-masthead");
            if (!masthead) return null;
            const mastheadRect = masthead.getBoundingClientRect();
            const mastheadCenter = mastheadRect.top + mastheadRect.height / 2;
            const tolerancePx = 24;
            // Three-tier cover lockups pin the mark tier to the top edge and
            // the anchor tier to the bottom edge on purpose; the centering
            // contract applies to the title tier the 1fr/auto/1fr grid holds
            // in the middle, not to the full identity envelope.
            if (lockup.classList.contains("cover-lockup")) {
              const titleTier =
                lockup.querySelector<HTMLElement>(".cover-tier-title");
              if (!titleTier || titleTier.getClientRects().length === 0) {
                return `${masthead.dataset.blockId ?? "masthead"} cover lockup is missing its title tier`;
              }
              const titleRect = titleTier.getBoundingClientRect();
              const titleCenter = titleRect.top + titleRect.height / 2;
              const offset = titleCenter - mastheadCenter;
              if (Math.abs(offset) <= tolerancePx) return null;
              return `${masthead.dataset.blockId ?? "masthead"} cover title tier is ${offset.toFixed(1)}px from vertical center`;
            }
            const visibleIdentityNodes = [
              ...lockup.querySelectorAll<HTMLElement>(
                ".restaurant-logo, .restaurant-name, .restaurant-tagline, .cover-contact",
              ),
            ].filter((node) => node.getClientRects().length > 0);
            if (visibleIdentityNodes.length === 0) return null;
            const rects = visibleIdentityNodes.map((node) =>
              node.getBoundingClientRect(),
            );
            const contentTop = Math.min(...rects.map((rect) => rect.top));
            const contentBottom = Math.max(...rects.map((rect) => rect.bottom));
            const contentCenter = (contentTop + contentBottom) / 2;
            const offset = contentCenter - mastheadCenter;
            if (Math.abs(offset) <= tolerancePx) return null;
            return `${masthead.dataset.blockId ?? "masthead"} content is ${offset.toFixed(1)}px from vertical center`;
          })
          .filter((issue): issue is string => issue !== null)
      : [];
    const foldedCompositionIssues: string[] = [];
    if (document.body.classList.contains("format-takeaway-trifold")) {
      const outside = pages.find((node) => node.dataset.page === "1");
      if (
        outside?.querySelector(
          '.planned-block[data-kind="category-heading"], .planned-block[data-kind="item-list"], .planned-block[data-kind="item-feature"]',
        )
      ) {
        foldedCompositionIssues.push("trifold menu content escaped outside");
      }
      const inside = pages.find((node) => node.dataset.page === "2");
      const headingPanelXs = [
        ...(inside?.querySelectorAll<HTMLElement>(
          '.planned-block[data-kind="category-heading"]',
        ) ?? []),
      ].map((node) => Math.round(node.getBoundingClientRect().left));
      if (new Set(headingPanelXs).size !== headingPanelXs.length) {
        foldedCompositionIssues.push(
          "trifold categories do not begin on distinct panels",
        );
      }
    }
    if (document.body.classList.contains("format-folded-booklet")) {
      const outside = pages.find((node) => node.dataset.page === "1");
      const outsideMenu = [
        ...(outside?.querySelectorAll<HTMLElement>(
          '.planned-block[data-kind="category-heading"], .planned-block[data-kind="item-list"], .planned-block[data-kind="item-feature"]',
        ) ?? []),
      ];
      if (
        outsideMenu.some((node) => node.classList.contains("is-continuation"))
      )
        foldedCompositionIssues.push(
          "booklet back cover contains a category continuation",
        );
    }
    return {
      pageCount: pages.length,
      families: [...new Set(pages.map((node) => node.dataset.family))],
      clippedBlocks,
      emptyImages,
      emptyImageFrames,
      stretchedImages,
      clippedOverflowNodes,
      tinyText,
      itemCount: itemIds.length,
      itemIds,
      emptyPageNumbers,
      offViewportPages,
      semanticOverlaps,
      miscenteredFoldedIdentities,
      foldedCompositionIssues,
      visibleText: document.body.innerText,
      documentWidth: document.documentElement.scrollWidth,
      viewportWidth: document.documentElement.clientWidth,
    };
  });

  expect(inspection.pageCount).toBeGreaterThan(0);
  expect(inspection.families).toEqual([expectedFamily]);
  expect(inspection.clippedBlocks).toEqual([]);
  expect(inspection.emptyImages).toEqual([]);
  expect(inspection.emptyImageFrames).toEqual([]);
  expect(inspection.stretchedImages).toEqual([]);
  expect(inspection.clippedOverflowNodes).toEqual([]);
  expect(inspection.tinyText).toEqual([]);
  expect(inspection.itemCount).toBe(expectedItemIds.length);
  expect(inspection.itemIds?.sort()).toEqual([...expectedItemIds].sort());
  expect(inspection.emptyPageNumbers).toEqual([]);
  expect(inspection.offViewportPages).toEqual([]);
  expect(inspection.semanticOverlaps).toEqual([]);
  expect(inspection.miscenteredFoldedIdentities).toEqual([]);
  expect(inspection.foldedCompositionIssues).toEqual([]);
  expect(inspection.visibleText).not.toMatch(/payverge/i);
  expect(inspection.documentWidth).toBeLessThanOrEqual(
    inspection.viewportWidth,
  );
}

test.describe("planned restaurant menu visual matrix", () => {
  for (const entry of visualCases) {
    test(`${entry.name} renders a print-safe ${entry.familyId} document`, async ({
      page,
    }) => {
      await installAssetRoutes(page);
      const direction = directionFor(entry);
      const document = planMenuDocument({
        model: entry.model,
        audit: makeAudit(entry.model),
        direction,
      });
      const html = renderMenuPrintHtml(entry.model, document, {
        origin: ASSET_ORIGIN,
      });

      await page.setContent(html, { waitUntil: "load" });
      await page.addStyleTag({
        content:
          "html,body{width:100%;min-width:100%}body{display:flex;flex-direction:column;align-items:center;gap:24px;padding:24px;background:#e9e4dc}.print-page{flex:none;box-shadow:0 8px 28px rgb(31 27 24 / .16)}",
      });
      await waitForPrintAssets(page, entry.familyId, entry.model.language);
      await expectHealthyPrintLayout(
        page,
        entry.familyId,
        entry.model.sections.flatMap((section) =>
          section.items.map((item) => item.id),
        ),
      );

      await expect(page).toHaveScreenshot(`${entry.name}.png`, {
        animations: "disabled",
        fullPage: true,
        caret: "hide",
        scale: "css",
      });
    });
  }
});
