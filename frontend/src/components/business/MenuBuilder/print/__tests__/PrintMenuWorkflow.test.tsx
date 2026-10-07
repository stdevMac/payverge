/** @jest-environment jsdom */

import React from "react";
import { fireEvent, render, screen, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";

import {
  makeAudit,
  makeDirection,
} from "@/lib/menuPrint/__fixtures__/plannedDocuments";
import { photoRichRestaurantMenu } from "@/lib/menuPrint/__fixtures__/restaurantMenus";
import { getMenuDesignFamily } from "@/lib/menuPrint/families";
import type {
  FeaturedImageSelection,
  MenuTreatment,
  PrintMenuModel,
} from "@/lib/menuPrint/types";

import { PrintIdentityControls } from "../PrintIdentityControls";
import { PrintPhotoControls } from "../PrintPhotoControls";
import { PrintShapeControls } from "../PrintShapeControls";

const tString = (key: string) => key;

const measurements = {
  "harvest.jpg": {
    url: "harvest.jpg",
    status: "ready" as const,
    width: 1600,
    height: 1200,
  },
};

describe("PrintShapeControls", () => {
  it("shows all formats and emits supported format, paper, language, and category choices", async () => {
    const user = userEvent.setup();
    const onFormatChange = jest.fn();
    const onPaperChange = jest.fn();
    const onLanguageChange = jest.fn();
    const onCategoryChange = jest.fn();

    render(
      <PrintShapeControls
        family={getMenuDesignFamily("atelier")}
        outputFormat="single-sheet"
        paperFormat="a4"
        language="en"
        categories={photoRichRestaurantMenu.sections}
        selectedCategoryIds="all"
        pageEstimates={{
          "single-sheet": 2,
          "two-page-spread": 2,
          "folded-booklet": 4,
          "drinks-card": 2,
        }}
        tString={tString}
        onFormatChange={onFormatChange}
        onPaperChange={onPaperChange}
        onLanguageChange={onLanguageChange}
        onCategoryChange={onCategoryChange}
      />,
    );

    expect(screen.getAllByTestId("print-format-card")).toHaveLength(6);
    expect(
      screen.getByRole("button", { name: "print.format.counterMenu" }),
    ).toBeDisabled();
    expect(
      screen.getByText("print.compatibility.formatUnavailable"),
    ).toBeVisible();
    expect(screen.getByText("4 print.pageEstimate.label")).toBeVisible();

    await user.click(
      screen.getByRole("button", { name: "print.format.foldedBooklet" }),
    );
    await user.click(
      screen.getByRole("button", { name: "print.paper.letter" }),
    );
    await user.click(screen.getByRole("button", { name: "print.language.es" }));
    await user.click(screen.getByRole("checkbox", { name: "Garden" }));

    expect(onFormatChange).toHaveBeenCalledWith("folded-booklet");
    expect(onPaperChange).toHaveBeenCalledWith("letter");
    expect(onLanguageChange).toHaveBeenCalledWith("es");
    expect(onCategoryChange).toHaveBeenCalledWith(["coast"]);
  });

  it("keeps every geometry-supported paper visible when the format changes", () => {
    render(
      <PrintShapeControls
        family={getMenuDesignFamily("atelier")}
        outputFormat="drinks-card"
        paperFormat="a4"
        language="en"
        categories={photoRichRestaurantMenu.sections}
        selectedCategoryIds="all"
        pageEstimates={{}}
        tString={tString}
        onFormatChange={jest.fn()}
        onPaperChange={jest.fn()}
        onLanguageChange={jest.fn()}
        onCategoryChange={jest.fn()}
      />,
    );

    expect(
      screen.getByRole("button", { name: "print.paper.a4" }),
    ).toHaveAttribute("aria-pressed", "true");
    expect(
      screen.getByRole("button", { name: "print.paper.letter" }),
    ).toBeVisible();
    expect(
      screen.getByRole("button", { name: "print.paper.halfLetter" }),
    ).toBeVisible();
    expect(
      screen.getByRole("button", { name: "print.paper.a5" }),
    ).toBeVisible();
  });

  it("keeps at least one category selected", async () => {
    const user = userEvent.setup();
    const onCategoryChange = jest.fn();
    const category = photoRichRestaurantMenu.sections[0];

    render(
      <PrintShapeControls
        family={getMenuDesignFamily("atelier")}
        outputFormat="single-sheet"
        paperFormat="a4"
        language="en"
        categories={photoRichRestaurantMenu.sections}
        selectedCategoryIds={[category.id]}
        pageEstimates={{}}
        tString={tString}
        onFormatChange={jest.fn()}
        onPaperChange={jest.fn()}
        onLanguageChange={jest.fn()}
        onCategoryChange={onCategoryChange}
      />,
    );

    const lastSelected = screen.getByRole("checkbox", {
      name: category.name,
    });
    expect(lastSelected).toBeDisabled();
    await user.click(lastSelected);
    expect(onCategoryChange).not.toHaveBeenCalled();
  });
});

describe("PrintPhotoControls", () => {
  it("puts treatment first and emits treatment, selection, and focal choices", async () => {
    const user = userEvent.setup();
    const onTreatmentChange = jest.fn();
    const onSelectionChange = jest.fn();
    const onFocalPointChange = jest.fn();

    render(
      <PrintPhotoControls
        family={getMenuDesignFamily("atelier")}
        treatment="balanced"
        model={photoRichRestaurantMenu}
        audit={makeAudit({ photoCoverage: 0.8 })}
        measurements={measurements}
        selections={[]}
        focalPoints={{}}
        tString={tString}
        onTreatmentChange={onTreatmentChange}
        onSelectionChange={onSelectionChange}
        onFocalPointChange={onFocalPointChange}
      />,
    );

    const treatmentGroup = screen.getByRole("group", {
      name: "print.treatment.label",
    });
    const firstPhoto = screen.getByText("Harvest Bowl");
    expect(
      treatmentGroup.compareDocumentPosition(firstPhoto) &
        Node.DOCUMENT_POSITION_FOLLOWING,
    ).toBeTruthy();

    await user.click(
      screen.getByRole("button", { name: "print.treatment.photoLed" }),
    );
    await user.click(screen.getByRole("checkbox", { name: "Harvest Bowl" }));
    await user.click(
      screen.getByRole("button", { name: "print.focal.center" }),
    );

    expect(onTreatmentChange).toHaveBeenCalledWith("photo-led");
    expect(onSelectionChange).toHaveBeenCalledWith([
      expect.objectContaining({ itemId: "harvest-bowl", url: "harvest.jpg" }),
    ]);
    expect(onFocalPointChange).toHaveBeenCalledWith("harvest.jpg", {
      x: 0.5,
      y: 0.5,
    });
  });

  it("keeps unsupported treatments visible with an explanation", () => {
    render(
      <PrintPhotoControls
        family={getMenuDesignFamily("gallery")}
        treatment="balanced"
        model={photoRichRestaurantMenu}
        audit={makeAudit({ photoCoverage: 0.8 })}
        measurements={measurements}
        selections={[]}
        focalPoints={{}}
        tString={tString}
        onTreatmentChange={jest.fn()}
        onSelectionChange={jest.fn()}
        onFocalPointChange={jest.fn()}
      />,
    );

    expect(
      screen.getByRole("button", { name: "print.treatment.compact" }),
    ).toBeDisabled();
    expect(
      screen.getByText("print.compatibility.galleryCompactUnavailable"),
    ).toBeVisible();
  });

  it("disables failed photos and limits low-resolution photos to compact tiles", () => {
    const qualityModel: PrintMenuModel = {
      ...photoRichRestaurantMenu,
      sections: [
        {
          id: "quality",
          name: "Quality",
          items: [
            {
              id: "ready",
              name: "Ready Dish",
              price: 20,
              imageCandidates: ["ready.jpg"],
              dietaryTags: [],
              allergens: [],
            },
            {
              id: "small",
              name: "Small Dish",
              price: 18,
              imageCandidates: ["small.jpg"],
              dietaryTags: [],
              allergens: [],
            },
            {
              id: "failed",
              name: "Failed Dish",
              price: 16,
              imageCandidates: ["failed.jpg"],
              dietaryTags: [],
              allergens: [],
            },
          ],
        },
      ],
    };

    render(
      <PrintPhotoControls
        family={getMenuDesignFamily("osteria")}
        treatment="balanced"
        model={qualityModel}
        audit={makeAudit(qualityModel, {
          lowResolutionUrls: ["small.jpg"],
          failedUrls: ["failed.jpg"],
        })}
        measurements={{
          "ready.jpg": { status: "ready", width: 1600, height: 1200 },
          "small.jpg": { status: "ready", width: 420, height: 320 },
          "failed.jpg": { status: "failed" },
        }}
        selections={[]}
        focalPoints={{}}
        tString={tString}
        onTreatmentChange={jest.fn()}
        onSelectionChange={jest.fn()}
        onFocalPointChange={jest.fn()}
      />,
    );

    expect(
      screen.getByRole("checkbox", { name: "Failed Dish" }),
    ).toBeDisabled();
    const smallCard = screen.getByText("Small Dish").closest("article");
    expect(smallCard).not.toBeNull();
    expect(
      within(smallCard!).getByText("print.imageRole.compactTile"),
    ).toBeVisible();
    expect(
      within(smallCard!).getByText("print.photos.lowResolution"),
    ).toBeVisible();
    expect(screen.getByRole("checkbox", { name: "Small Dish" })).toBeEnabled();
  });

  it("keeps timeout distinct from failure and blocks unresolved selection", () => {
    const model: PrintMenuModel = {
      ...photoRichRestaurantMenu,
      sections: [
        {
          id: "quality",
          name: "Quality",
          items: [
            {
              id: "loading",
              name: "Loading Dish",
              price: 20,
              imageCandidates: ["loading.jpg"],
              dietaryTags: [],
              allergens: [],
            },
            {
              id: "timeout",
              name: "Timed Out Dish",
              price: 18,
              imageCandidates: ["timeout.jpg"],
              dietaryTags: [],
              allergens: [],
            },
          ],
        },
      ],
    };

    render(
      <PrintPhotoControls
        family={getMenuDesignFamily("osteria")}
        treatment="balanced"
        model={model}
        audit={makeAudit(model)}
        measurements={{
          "loading.jpg": { status: "loading" },
          "timeout.jpg": { status: "timeout" },
        }}
        selections={[]}
        focalPoints={{}}
        tString={tString}
        onTreatmentChange={jest.fn()}
        onSelectionChange={jest.fn()}
        onFocalPointChange={jest.fn()}
      />,
    );

    expect(
      screen.getByRole("checkbox", { name: "Loading Dish" }),
    ).toBeDisabled();
    expect(
      screen.getByRole("checkbox", { name: "Timed Out Dish" }),
    ).toBeDisabled();
    expect(screen.getAllByText("print.photos.timeout")).toHaveLength(2);
    expect(screen.queryAllByText("print.photos.failed")).toHaveLength(0);
  });

  it("preserves a selected image while measuring and normalizes it when ready", async () => {
    const user = userEvent.setup();
    const selection: FeaturedImageSelection = {
      itemId: "harvest-bowl",
      url: "harvest.jpg",
      role: "category-opener",
    };

    function ControlledPhotos() {
      const [ready, setReady] = React.useState(false);
      const [selections, setSelections] = React.useState<
        FeaturedImageSelection[]
      >([selection]);
      return (
        <>
          <button type="button" onClick={() => setReady(true)}>
            Complete measurement
          </button>
          <output data-testid="controlled-selections">
            {JSON.stringify(selections)}
          </output>
          <PrintPhotoControls
            family={getMenuDesignFamily("osteria")}
            treatment="balanced"
            model={photoRichRestaurantMenu}
            audit={makeAudit(
              photoRichRestaurantMenu,
              ready ? { lowResolutionUrls: ["harvest.jpg"] } : {},
            )}
            measurements={{
              "harvest.jpg": ready
                ? { status: "ready", width: 420, height: 320 }
                : { status: "loading" },
            }}
            selections={selections}
            focalPoints={{}}
            tString={tString}
            onTreatmentChange={jest.fn()}
            onSelectionChange={setSelections}
            onFocalPointChange={jest.fn()}
          />
        </>
      );
    }

    render(<ControlledPhotos />);

    expect(
      screen.getByRole("checkbox", { name: "Harvest Bowl" }),
    ).toBeDisabled();
    expect(
      screen.getByRole("checkbox", { name: "Harvest Bowl" }),
    ).toBeChecked();
    expect(screen.getByTestId("controlled-selections")).toHaveTextContent(
      '"role":"category-opener"',
    );

    await user.click(
      screen.getByRole("button", { name: "Complete measurement" }),
    );

    expect(
      screen.getByRole("checkbox", { name: "Harvest Bowl" }),
    ).toBeChecked();
    expect(screen.getByTestId("controlled-selections")).toHaveTextContent(
      '"role":"compact-tile"',
    );
  });

  it("clears controlled photo selections when switching to type-led", async () => {
    const user = userEvent.setup();
    const selection: FeaturedImageSelection = {
      itemId: "harvest-bowl",
      url: "harvest.jpg",
      role: "full-width",
    };

    function ControlledTreatment() {
      const [treatment, setTreatment] =
        React.useState<MenuTreatment>("balanced");
      const [selections, setSelections] = React.useState<
        FeaturedImageSelection[]
      >([selection]);
      return (
        <>
          <output data-testid="controlled-treatment-state">
            {treatment}:{selections.length}
          </output>
          <PrintPhotoControls
            family={getMenuDesignFamily("atelier")}
            treatment={treatment}
            model={photoRichRestaurantMenu}
            audit={makeAudit(photoRichRestaurantMenu)}
            measurements={measurements}
            selections={selections}
            focalPoints={{}}
            tString={tString}
            onTreatmentChange={setTreatment}
            onSelectionChange={setSelections}
            onFocalPointChange={jest.fn()}
          />
        </>
      );
    }

    render(<ControlledTreatment />);
    expect(screen.getByTestId("controlled-treatment-state")).toHaveTextContent(
      "balanced:1",
    );

    await user.click(
      screen.getByRole("button", { name: "print.treatment.typeLed" }),
    );

    expect(screen.getByTestId("controlled-treatment-state")).toHaveTextContent(
      "type-led:0",
    );
    expect(screen.getByText("print.photos.typeLedNoSelection")).toBeVisible();
  });

  it("replaces a same-section photo and keeps distinct-section selections within capacity", async () => {
    const user = userEvent.setup();
    const onSelectionChange = jest.fn();
    const first: FeaturedImageSelection = {
      itemId: "harvest-bowl",
      url: "harvest.jpg",
      role: "full-width",
    };
    const props = {
      family: getMenuDesignFamily("atelier"),
      treatment: "balanced" as const,
      model: photoRichRestaurantMenu,
      audit: makeAudit(photoRichRestaurantMenu),
      measurements: {
        "harvest.jpg": { status: "ready" as const, width: 1600, height: 1200 },
        "tiny.jpg": { status: "ready" as const, width: 1400, height: 1000 },
        "https://images.payverge.test/lemon-trout-1800x1200.jpg": {
          status: "ready" as const,
          width: 1800,
          height: 1200,
        },
      },
      focalPoints: {},
      tString,
      onTreatmentChange: jest.fn(),
      onSelectionChange,
      onFocalPointChange: jest.fn(),
    };

    const { rerender } = render(
      <PrintPhotoControls {...props} selections={[first]} />,
    );
    await user.click(screen.getByRole("checkbox", { name: "Charred Carrots" }));
    expect(onSelectionChange).toHaveBeenLastCalledWith([
      expect.objectContaining({ itemId: "charred-carrots" }),
    ]);

    const lemon: FeaturedImageSelection = {
      itemId: "lemon-trout",
      url: "https://images.payverge.test/lemon-trout-1800x1200.jpg",
      role: "editorial-crop",
    };
    onSelectionChange.mockClear();
    rerender(<PrintPhotoControls {...props} selections={[first]} />);
    await user.click(
      screen.getByRole("checkbox", { name: "Lemon-Roasted Trout" }),
    );
    expect(onSelectionChange).toHaveBeenLastCalledWith([
      first,
      expect.objectContaining({ itemId: lemon.itemId, url: lemon.url }),
    ]);
  });

  it("renders a useful empty state when the menu has no photos", () => {
    const model: PrintMenuModel = {
      ...photoRichRestaurantMenu,
      sections: photoRichRestaurantMenu.sections.map((section) => ({
        ...section,
        items: section.items.map((item) => ({
          ...item,
          imageUrl: undefined,
          imageCandidates: [],
        })),
      })),
    };

    render(
      <PrintPhotoControls
        family={getMenuDesignFamily("atelier")}
        treatment="photo-led"
        model={model}
        audit={makeAudit(model, { photoCoverage: 0 })}
        measurements={{}}
        selections={[]}
        focalPoints={{}}
        tString={tString}
        onTreatmentChange={jest.fn()}
        onSelectionChange={jest.fn()}
        onFocalPointChange={jest.fn()}
      />,
    );

    expect(screen.getByText("print.photos.empty")).toBeVisible();
  });

  it("maps all nine focal positions and supports arrow-key movement", async () => {
    const user = userEvent.setup();
    const onFocalPointChange = jest.fn();
    const selections: FeaturedImageSelection[] = [
      {
        itemId: "harvest-bowl",
        url: "harvest.jpg",
        role: "full-width",
      },
    ];

    render(
      <PrintPhotoControls
        family={getMenuDesignFamily("atelier")}
        treatment="balanced"
        model={photoRichRestaurantMenu}
        audit={makeAudit({ photoCoverage: 0.8 })}
        measurements={measurements}
        selections={selections}
        focalPoints={{ "harvest.jpg": { x: 0, y: 0 } }}
        tString={tString}
        onTreatmentChange={jest.fn()}
        onSelectionChange={jest.fn()}
        onFocalPointChange={onFocalPointChange}
      />,
    );

    const expected = [
      ["topLeft", { x: 0, y: 0 }],
      ["top", { x: 0.5, y: 0 }],
      ["topRight", { x: 1, y: 0 }],
      ["left", { x: 0, y: 0.5 }],
      ["center", { x: 0.5, y: 0.5 }],
      ["right", { x: 1, y: 0.5 }],
      ["bottomLeft", { x: 0, y: 1 }],
      ["bottom", { x: 0.5, y: 1 }],
      ["bottomRight", { x: 1, y: 1 }],
    ] as const;

    for (const [key, point] of expected) {
      await user.click(
        screen.getByRole("button", { name: `print.focal.${key}` }),
      );
      expect(onFocalPointChange).toHaveBeenLastCalledWith("harvest.jpg", point);
    }

    const topLeft = screen.getByRole("button", { name: "print.focal.topLeft" });
    topLeft.focus();
    await user.keyboard("{ArrowRight}");
    expect(
      screen.getByRole("button", { name: "print.focal.top" }),
    ).toHaveFocus();
    expect(onFocalPointChange).toHaveBeenLastCalledWith("harvest.jpg", {
      x: 0.5,
      y: 0,
    });
  });

  it("preserves controlled image selection on rerender", () => {
    const props = {
      family: getMenuDesignFamily("atelier"),
      treatment: "balanced" as const,
      model: photoRichRestaurantMenu,
      audit: makeAudit({ photoCoverage: 0.8 }),
      measurements,
      focalPoints: {},
      tString,
      onTreatmentChange: jest.fn(),
      onSelectionChange: jest.fn(),
      onFocalPointChange: jest.fn(),
    };
    const selection: FeaturedImageSelection = {
      itemId: "harvest-bowl",
      url: "harvest.jpg",
      role: "full-width",
    };
    const { rerender } = render(
      <PrintPhotoControls {...props} selections={[selection]} />,
    );

    expect(
      screen.getByRole("checkbox", { name: "Harvest Bowl" }),
    ).toBeChecked();
    rerender(<PrintPhotoControls {...props} selections={[selection]} />);
    expect(
      screen.getByRole("checkbox", { name: "Harvest Bowl" }),
    ).toBeChecked();
  });
});

describe("PrintIdentityControls", () => {
  it("shows resolved identity options and emits every identity callback", async () => {
    const user = userEvent.setup();
    const onLogoTreatmentChange = jest.fn();
    const onPaletteChange = jest.fn();
    const onTypographyChange = jest.fn();
    const onOrnamentChange = jest.fn();
    const onCoverChange = jest.fn();
    const onQrChange = jest.fn();
    const onContactPlacementChange = jest.fn();
    const direction = makeDirection({
      familyId: "atelier",
      treatment: "balanced",
    });

    render(
      <PrintIdentityControls
        family={getMenuDesignFamily("atelier")}
        direction={direction}
        paletteSource="restaurant"
        includeQr={false}
        canIncludeQr
        tString={tString}
        onLogoTreatmentChange={onLogoTreatmentChange}
        onPaletteChange={onPaletteChange}
        onTypographyChange={onTypographyChange}
        onOrnamentChange={onOrnamentChange}
        onCoverChange={onCoverChange}
        onQrChange={onQrChange}
        onContactPlacementChange={onContactPlacementChange}
      />,
    );

    expect(screen.getAllByTestId("print-palette-swatch")).toHaveLength(4);
    expect(screen.getByText("print.palette.restaurant")).toBeVisible();

    await user.click(
      screen.getByRole("button", { name: "print.logo.wordmark" }),
    );
    await user.click(
      screen.getByRole("button", { name: "print.typography.refined" }),
    );
    await user.click(
      screen.getByRole("button", { name: "print.ornament.expressive" }),
    );
    await user.click(
      screen.getByRole("button", { name: "print.cover.typographic" }),
    );
    await user.click(
      screen.getByRole("checkbox", { name: "print.qr.include" }),
    );
    await user.click(
      screen.getByRole("button", { name: "print.contact.cover" }),
    );
    fireEvent.change(screen.getByLabelText("print.palette.override"), {
      target: { value: "#145554" },
    });

    expect(onLogoTreatmentChange).toHaveBeenCalledWith("wordmark");
    expect(onTypographyChange).toHaveBeenCalledWith("refined");
    expect(onOrnamentChange).toHaveBeenCalledWith("expressive");
    expect(onCoverChange).toHaveBeenCalledWith("typographic");
    expect(onQrChange).toHaveBeenCalledWith(true);
    expect(onContactPlacementChange).toHaveBeenCalledWith("cover");
    expect(onPaletteChange).toHaveBeenCalledWith({
      ...direction.palette,
      accent: "#145554",
      source: "restaurant",
    });
  });

  it("explains family palette fallback and unavailable QR", () => {
    render(
      <PrintIdentityControls
        family={getMenuDesignFamily("gallery")}
        direction={makeDirection({ familyId: "gallery" })}
        paletteSource="family-fallback"
        canIncludeQr={false}
        tString={tString}
        onLogoTreatmentChange={jest.fn()}
        onPaletteChange={jest.fn()}
        onTypographyChange={jest.fn()}
        onOrnamentChange={jest.fn()}
        onCoverChange={jest.fn()}
        onQrChange={jest.fn()}
        onContactPlacementChange={jest.fn()}
      />,
    );

    expect(screen.getByText("print.palette.familyFallback")).toBeVisible();
    expect(
      screen.getByRole("checkbox", { name: "print.qr.include" }),
    ).toBeDisabled();
    expect(screen.getByText("print.qr.unavailable")).toBeVisible();
  });
});
