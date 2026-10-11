/** @jest-environment jsdom */

import { act, renderHook } from "@testing-library/react";

import { makeAudit } from "@/lib/menuPrint/__fixtures__/plannedDocuments";
import { photoRichRestaurantMenu } from "@/lib/menuPrint/__fixtures__/restaurantMenus";
import { resolvePrintPalette } from "@/lib/menuPrint/palette";
import { planMenuDocument } from "@/lib/menuPrint/planner/planDocument";
import type {
  FeaturedImageSelection,
  MenuDirectionRecommendation,
  PaperFormat,
  ResolvedPrintPalette,
} from "@/lib/menuPrint/types";

import {
  PRINT_MENU_STAGES,
  type PrintMenuStage,
  useMenuPrintController,
} from "./useMenuPrintController";

const recommendations: MenuDirectionRecommendation[] = [
  {
    familyId: "atelier",
    treatment: "balanced",
    outputFormat: "two-page-spread",
    reasonKey: "print.recommendation.balanced",
    score: 100,
  },
];

describe("useMenuPrintController", () => {
  it("starts from the first valid recommendation and keeps compatible choices", () => {
    const { result } = renderHook(() =>
      useMenuPrintController({ recommendations, defaultPaper: "a4" }),
    );

    expect(result.current.state.stage).toBe("look");
    expect(result.current.state.direction).toMatchObject({
      ...recommendations[0],
      paperFormat: "a4",
      typographyPersonality: "editorial",
      contactPlacement: "footer",
    });

    act(() => result.current.actions.selectFamily("gallery"));

    expect(result.current.state.direction).toMatchObject({
      familyId: "gallery",
      treatment: "balanced",
      outputFormat: "two-page-spread",
    });
  });

  it("keeps typography and contact placement in the printable direction", () => {
    const { result } = renderHook(() =>
      useMenuPrintController({ recommendations, defaultPaper: "a4" }),
    );

    act(() => {
      result.current.actions.setTypographyPersonality("refined");
      result.current.actions.setContactPlacement("cover");
    });

    expect(result.current.state.direction).toMatchObject({
      typographyPersonality: "refined",
      contactPlacement: "cover",
    });
  });

  it("falls back deterministically when a new family does not support current choices", () => {
    const { result } = renderHook(() =>
      useMenuPrintController({ recommendations, defaultPaper: "a4" }),
    );

    act(() => {
      result.current.actions.selectTreatment("type-led");
      result.current.actions.selectFormat("drinks-card");
    });
    act(() => result.current.actions.selectFamily("gallery"));

    expect(result.current.state.direction).toMatchObject({
      familyId: "gallery",
      treatment: "balanced",
      outputFormat: "single-sheet",
    });
  });

  it("lands a candidate's own recommended format when selectFormat follows selectFamily in the same dispatch batch", () => {
    // Mirrors PrintMenuStudio's handleLookSelect: selecting a look card whose
    // family does not support the currently active format must still end up
    // on that card's own recommended format, not the family's first
    // supported fallback, as long as selectFormat is dispatched after
    // selectFamily.
    const { result } = renderHook(() =>
      useMenuPrintController({ recommendations, defaultPaper: "a4" }),
    );

    // Starting format is "two-page-spread" (atelier's recommendation), which
    // "street" does not support, so selectFamily alone would fall back to
    // "single-sheet" (street.supportedFormats[0]).
    expect(result.current.state.direction.outputFormat).toBe(
      "two-page-spread",
    );

    act(() => {
      result.current.actions.selectFamily("street");
      result.current.actions.selectFormat("counter-menu");
    });

    expect(result.current.state.direction).toMatchObject({
      familyId: "street",
      outputFormat: "counter-menu",
    });
  });

  it("rejects unsupported treatment and format actions", () => {
    const { result } = renderHook(() =>
      useMenuPrintController({
        recommendations: [
          {
            ...recommendations[0],
            familyId: "gallery",
            outputFormat: "single-sheet",
          },
        ],
        defaultPaper: "a4",
      }),
    );
    const initialDirection = result.current.state.direction;

    act(() => result.current.actions.selectTreatment("compact"));
    expect(result.current.state.direction).toBe(initialDirection);

    act(() => result.current.actions.selectFormat("counter-menu"));
    expect(result.current.state.direction).toBe(initialDirection);
  });

  it("navigates the two visible moments and derives navigation availability", () => {
    const { result } = renderHook(() =>
      useMenuPrintController({ recommendations, defaultPaper: "letter" }),
    );

    expect(PRINT_MENU_STAGES).toEqual(["look", "personalize"]);
    expect(result.current.state.stage).toBe("look");
    expect(result.current.canGoBack).toBe(false);
    expect(result.current.canGoNext).toBe(true);

    act(() => result.current.actions.goTo("personalize"));
    expect(result.current.state.stage).toBe("personalize");
    expect(result.current.canGoBack).toBe(true);
    expect(result.current.canGoNext).toBe(false);

    act(() => result.current.actions.goTo("look"));
    expect(result.current.state.stage).toBe("look");
    expect(result.current.canGoBack).toBe(false);
    expect(result.current.canGoNext).toBe(true);
    expect(result.current.state.direction.familyId).toBe(
      recommendations[0].familyId,
    );
  });

  it("updates categories, images, focal points, palette, and paper immutably", () => {
    const { result } = renderHook(() =>
      useMenuPrintController({ recommendations, defaultPaper: "a4" }),
    );
    const categoryIds = ["mains", "desserts"];
    const selections: FeaturedImageSelection[] = [
      { itemId: "dish-1", url: "dish.jpg", role: "full-width" },
    ];
    const point = { x: 0.72, y: 0.31 };
    const palette: ResolvedPrintPalette = {
      ground: "#ffffff",
      ink: "#111111",
      accent: "#1a6b6a",
      muted: "#555555",
      source: "restaurant",
    };
    const initialState = result.current.state;

    act(() => result.current.actions.setCategories(categoryIds));
    expect(result.current.state).not.toBe(initialState);
    expect(result.current.state.selectedCategoryIds).toEqual(categoryIds);
    expect(result.current.state.selectedCategoryIds).not.toBe(categoryIds);

    act(() => result.current.actions.setImages(selections));
    expect(result.current.state.direction.imageSelections).toEqual(selections);
    expect(result.current.state.direction.imageSelections).not.toBe(selections);
    expect(result.current.state.direction.imageSelections[0]).not.toBe(
      selections[0],
    );

    act(() => result.current.actions.setFocalPoint("dish.jpg", point));
    expect(result.current.state.direction.imageFocalPoints).toEqual({
      "dish.jpg": point,
    });
    expect(
      result.current.state.direction.imageFocalPoints["dish.jpg"],
    ).not.toBe(point);

    act(() => result.current.actions.setPalette(palette));
    expect(result.current.state.direction.palette).toMatchObject(palette);
    expect(result.current.state.direction.palette).not.toBe(palette);

    act(() => result.current.actions.selectPaper("half-letter"));
    expect(result.current.state.direction.paperFormat).toBe("half-letter");
  });

  it("canonicalizes category ids and ignores invalid workflow stages", () => {
    const { result } = renderHook(() =>
      useMenuPrintController({ recommendations, defaultPaper: "a4" }),
    );
    const categoryIds = [
      " mains ",
      "",
      "mains",
      " desserts ",
      "desserts",
      "   ",
    ];

    act(() => result.current.actions.setCategories(categoryIds));
    expect(result.current.state.selectedCategoryIds).toEqual([
      "mains",
      "desserts",
    ]);
    expect(categoryIds).toEqual([
      " mains ",
      "",
      "mains",
      " desserts ",
      "desserts",
      "   ",
    ]);

    const categoryState = result.current.state;
    act(() => result.current.actions.goTo("not-a-stage" as PrintMenuStage));
    expect(result.current.state).toBe(categoryState);
  });

  it("accepts only readable resolved palettes and preserves their provenance", () => {
    const { result } = renderHook(() =>
      useMenuPrintController({ recommendations, defaultPaper: "a4" }),
    );
    const safePalette = resolvePrintPalette({ familyId: "gallery" });

    act(() => result.current.actions.setPalette(safePalette));
    expect(result.current.state.direction.palette).toEqual(safePalette);
    expect(result.current.state.direction.palette).not.toBe(safePalette);
    expect(result.current.state.direction.palette.source).toBe(
      "family-fallback",
    );

    const safeState = result.current.state;
    const unsafePalette: ResolvedPrintPalette = {
      ground: "#ffffff",
      ink: "#ffffff",
      accent: "#ffffff",
      muted: "#ffffff",
      source: "restaurant",
    };
    act(() => result.current.actions.setPalette(unsafePalette));
    expect(result.current.state).toBe(safeState);
  });

  it("reselects the active family as an exact no-op", () => {
    const { result } = renderHook(() =>
      useMenuPrintController({ recommendations, defaultPaper: "a4" }),
    );
    const palette = resolvePrintPalette({ familyId: "gallery" });
    act(() => result.current.actions.setPalette(palette));
    const selectedState = result.current.state;

    act(() => result.current.actions.selectFamily("atelier"));

    expect(result.current.state).toBe(selectedState);
    expect(result.current.state.direction.palette).toEqual(palette);
  });

  it("ignores non-finite or out-of-range focal points", () => {
    const { result } = renderHook(() =>
      useMenuPrintController({ recommendations, defaultPaper: "a4" }),
    );
    act(() =>
      result.current.actions.setFocalPoint("dish.jpg", { x: 0.4, y: 0.6 }),
    );

    for (const point of [
      { x: Number.NaN, y: 0.5 },
      { x: 0.5, y: Number.POSITIVE_INFINITY },
      { x: -0.1, y: 0.5 },
      { x: 0.5, y: 1.1 },
    ]) {
      const validState = result.current.state;
      act(() => result.current.actions.setFocalPoint("dish.jpg", point));
      expect(result.current.state).toBe(validState);
    }

    expect(result.current.state.direction.imageFocalPoints).toEqual({
      "dish.jpg": { x: 0.4, y: 0.6 },
    });
  });

  it("deduplicates image selections and drops roles unsupported by the active family", () => {
    const { result } = renderHook(() =>
      useMenuPrintController({ recommendations, defaultPaper: "a4" }),
    );
    const selections = [
      { itemId: "one", url: "one.jpg", role: "full-width" },
      { itemId: "one", url: "duplicate-item.jpg", role: "paired" },
      { itemId: "two", url: "one.jpg", role: "editorial-crop" },
      { itemId: "three", url: "three.jpg", role: "cover" },
      { itemId: "four", url: "four.jpg", role: "editorial-crop" },
    ] as FeaturedImageSelection[];

    act(() => result.current.actions.setImages(selections));

    expect(result.current.state.direction.imageSelections).toEqual([
      { itemId: "one", url: "one.jpg", role: "full-width" },
      { itemId: "four", url: "four.jpg", role: "editorial-crop" },
    ]);
  });

  it("normalizes image roles on family change and remains planner-safe", () => {
    const { result } = renderHook(() =>
      useMenuPrintController({ recommendations, defaultPaper: "a4" }),
    );
    act(() =>
      result.current.actions.setImages([
        {
          itemId: "harvest-bowl",
          url: "harvest.jpg",
          role: "editorial-crop",
        },
        {
          itemId: "charred-carrots",
          url: "tiny.jpg",
          role: "paired",
        },
      ]),
    );

    act(() => result.current.actions.selectFamily("maison"));

    expect(result.current.state.direction.imageSelections).toEqual([
      {
        itemId: "harvest-bowl",
        url: "harvest.jpg",
        role: "half-page",
      },
    ]);
    expect(() =>
      planMenuDocument({
        model: photoRichRestaurantMenu,
        audit: makeAudit(photoRichRestaurantMenu),
        direction: result.current.state.direction,
      }),
    ).not.toThrow();
  });

  it("uses a safe fallback for empty or invalid recommendations", () => {
    const invalidRecommendations = [
      {
        ...recommendations[0],
        familyId: "unknown-family",
      },
      {
        ...recommendations[0],
        familyId: "gallery",
        treatment: "compact",
      },
    ] as unknown as MenuDirectionRecommendation[];
    const { result: invalidResult } = renderHook(() =>
      useMenuPrintController({
        recommendations: invalidRecommendations,
        defaultPaper: "letter",
      }),
    );
    const { result: emptyResult } = renderHook(() =>
      useMenuPrintController({ recommendations: [], defaultPaper: "a4" }),
    );

    expect(invalidResult.current.state.direction).toMatchObject({
      familyId: "atelier",
      treatment: "type-led",
      outputFormat: "single-sheet",
      paperFormat: "letter",
    });
    expect(emptyResult.current.state.direction).toMatchObject({
      familyId: "atelier",
      treatment: "type-led",
      outputFormat: "single-sheet",
      paperFormat: "a4",
    });
  });

  it("does not reset active work when external defaults change, but resets explicitly", () => {
    const replacement: MenuDirectionRecommendation[] = [
      {
        familyId: "street",
        treatment: "compact",
        outputFormat: "counter-menu",
        reasonKey: "print.recommendation.quickServiceStreet",
        score: 90,
      },
    ];
    const initialProps: {
      directions: MenuDirectionRecommendation[];
      paper: PaperFormat;
    } = {
      directions: recommendations,
      paper: "a4",
    };
    const { result, rerender } = renderHook(
      ({ directions, paper }) =>
        useMenuPrintController({
          recommendations: directions,
          defaultPaper: paper,
        }),
      {
        initialProps,
      },
    );
    const initialActions = result.current.actions;

    act(() => {
      result.current.actions.goTo("personalize");
      result.current.actions.selectFamily("gallery");
    });

    rerender({ directions: replacement, paper: "letter" });

    expect(result.current.actions).toBe(initialActions);
    expect(result.current.state).toMatchObject({
      stage: "personalize",
      direction: { familyId: "gallery", paperFormat: "a4" },
    });

    act(() => result.current.actions.reset());

    expect(result.current.state).toMatchObject({
      stage: "look",
      direction: {
        familyId: "street",
        treatment: "compact",
        outputFormat: "counter-menu",
        paperFormat: "letter",
      },
    });
  });
});
