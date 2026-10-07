/** @jest-environment jsdom */

import React from "react";
import { act, render, screen, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { makeAudit } from "@/lib/menuPrint/__fixtures__/plannedDocuments";
import { photoRichRestaurantMenu } from "@/lib/menuPrint/__fixtures__/restaurantMenus";
import * as planner from "@/lib/menuPrint/planner/planDocument";
import { recommendMenuDirections } from "@/lib/menuPrint/recommend";
import * as renderer from "@/lib/menuPrint/renderPlannedHtml";
import type {
  MenuDesignFamilyId,
  PrintMenuModel,
} from "@/lib/menuPrint/types";
import { PrintLookPicker, type PrintLookPickerProps } from "./PrintLookPicker";

const CSS_PX_PER_MM = 96 / 25.4;
const A4_WIDTH_PX = 210 * CSS_PX_PER_MM;
const A4_HEIGHT_PX = 297 * CSS_PX_PER_MM;
const OriginalResizeObserver = globalThis.ResizeObserver;

class MockResizeObserver {
  static instances: MockResizeObserver[] = [];

  readonly observe = jest.fn();
  readonly unobserve = jest.fn();
  readonly disconnect = jest.fn();

  constructor(private readonly callback: ResizeObserverCallback) {
    MockResizeObserver.instances.push(this);
  }

  resize(width: number, height: number): void {
    const target = this.observe.mock.calls[0]?.[0] as Element;
    this.callback(
      [
        {
          target,
          contentRect: {
            x: 0,
            y: 0,
            top: 0,
            right: width,
            bottom: height,
            left: 0,
            width,
            height,
            toJSON: () => ({}),
          },
          borderBoxSize: [],
          contentBoxSize: [],
          devicePixelContentBoxSize: [],
        },
      ],
      this as unknown as ResizeObserver,
    );
  }
}

beforeEach(() => {
  MockResizeObserver.instances = [];
  globalThis.ResizeObserver =
    MockResizeObserver as unknown as typeof ResizeObserver;
});

afterAll(() => {
  if (OriginalResizeObserver) {
    globalThis.ResizeObserver = OriginalResizeObserver;
  } else {
    Reflect.deleteProperty(globalThis, "ResizeObserver");
  }
});

const model: PrintMenuModel = {
  ...photoRichRestaurantMenu,
  business: {
    ...photoRichRestaurantMenu.business,
    name: "Casa Bruma",
    logoUrl: undefined,
  },
};

/** The shortlist the picker shows before the quiet "see the rest" disclosure. */
const SHORTLIST_SIZE = 3;
const ALL_FAMILY_COUNT = 8;

function printReadyMeasurements() {
  return Object.fromEntries(
    model.sections.flatMap((section) =>
      section.items.flatMap((item) =>
        (item.imageCandidates ?? []).map((url) => [
          url,
          { url, status: "ready" as const, width: 1800, height: 1400 },
        ]),
      ),
    ),
  );
}

function makeProps(
  overrides: Partial<PrintLookPickerProps> = {},
): PrintLookPickerProps {
  const audit = makeAudit(model, {
    photoCoverage: 0.8,
    measurements: printReadyMeasurements(),
  });
  return {
    model,
    audit,
    selectedFamilyId: "atelier",
    recommendations: recommendMenuDirections({
      audit,
      businessType: "cafe",
    }),
    paperFormat: "a4",
    tString: (key) => key,
    onSelect: jest.fn(),
    ...overrides,
  };
}

function visibleFamilyNames(): (string | null)[] {
  return screen
    .getAllByRole("radio")
    .map((radio) => radio.getAttribute("aria-label"));
}

describe("PrintLookPicker", () => {
  it("shows a short list of restaurant-specific planned previews", () => {
    const props = makeProps();
    const { container } = render(<PrintLookPicker {...props} />);

    expect(screen.queryByText("Aa")).not.toBeInTheDocument();
    expect(screen.getByText("Casa Bruma")).toBeInTheDocument();
    expect(screen.getAllByRole("radio")).toHaveLength(SHORTLIST_SIZE);
    expect(screen.getAllByTitle("print.look.previewTitle")).toHaveLength(
      SHORTLIST_SIZE,
    );
    expect(container.textContent).not.toMatch(/payverge/i);

    // Every visible card explains itself with the recommendation's own reason.
    const shortlisted = props.recommendations.slice(0, SHORTLIST_SIZE);

    // Each card must plan and render its OWN recommended format/treatment,
    // not a shared hardcoded shape (regression coverage for a bug where every
    // preview silently rendered as single-sheet A4 regardless of the
    // recommendation behind it).
    const recommendationByFamily = new Map(
      shortlisted.map((recommendation) => [
        recommendation.familyId,
        recommendation,
      ]),
    );
    screen.getAllByRole<HTMLInputElement>("radio").forEach((radio) => {
      const recommendation = recommendationByFamily.get(
        radio.value as MenuDesignFamilyId,
      );
      expect(recommendation).toBeDefined();
      const frame = within(
        radio.closest("label") as HTMLLabelElement,
      ).getByTitle<HTMLIFrameElement>("print.look.previewTitle");

      expect(frame.srcdoc).toContain("<!doctype html>");
      expect(frame.srcdoc).toContain("Casa Bruma");
      expect(frame.srcdoc).toContain(`format-${recommendation!.outputFormat}`);
      expect(frame.srcdoc).toContain(`treatment-${recommendation!.treatment}`);
      expect(frame.srcdoc).toContain("harvest.jpg");
      expect(frame.srcdoc).not.toContain("Aa");
    });
    for (const reasonKey of new Set(
      shortlisted.map((recommendation) => recommendation.reasonKey),
    )) {
      expect(screen.getAllByText(reasonKey)).toHaveLength(
        shortlisted.filter(
          (recommendation) => recommendation.reasonKey === reasonKey,
        ).length,
      );
    }
  });

  it("marks the single best direction for this restaurant", () => {
    const props = makeProps();
    render(<PrintLookPicker {...props} />);

    const badges = screen.getAllByTestId("print-look-best-badge");
    expect(badges).toHaveLength(1);
    expect(badges[0]).toHaveTextContent("print.look.recommended");

    const bestCard = badges[0].closest("label") as HTMLLabelElement;
    expect(within(bestCard).getByRole("radio")).toHaveAttribute(
      "value",
      props.recommendations[0].familyId,
    );
    expect(visibleFamilyNames()[0]).toBe(
      `print.family.${props.recommendations[0].familyId}`,
    );
  });

  it("requests a family change without diverging from the controlled selection", async () => {
    const user = userEvent.setup();
    const props = makeProps();
    const { rerender } = render(<PrintLookPicker {...props} />);

    const atelier = screen.getByRole("radio", {
      name: "print.family.atelier",
    });
    const gallery = screen.getByRole("radio", {
      name: "print.family.gallery",
    });
    await user.click(gallery);

    expect(props.onSelect).toHaveBeenCalledWith(
      "gallery",
      "photo-led",
      "folded-booklet",
    );
    expect(atelier).toBeChecked();
    expect(gallery).not.toBeChecked();
    expect(
      within(gallery.closest("label") as HTMLLabelElement).queryByTestId(
        "print-look-selected-check",
      ),
    ).not.toBeInTheDocument();

    rerender(<PrintLookPicker {...props} selectedFamilyId="gallery" />);
    const selectedGallery = screen.getByRole("radio", {
      name: "print.family.gallery",
    });
    expect(selectedGallery).toBeChecked();
    expect(
      within(selectedGallery.closest("label") as HTMLLabelElement).getByTestId(
        "print-look-selected-check",
      ),
    ).toBeVisible();
  });

  it("uses native radios in a labelled radiogroup", () => {
    render(<PrintLookPicker {...makeProps()} />);

    expect(
      screen.getByRole("radiogroup", { name: "print.look.familyLabel" }),
    ).toBeInTheDocument();
    screen.getAllByRole<HTMLInputElement>("radio").forEach((radio) => {
      expect(radio).toHaveAttribute("type", "radio");
      expect(radio).toHaveAttribute("name", "print-menu-family");
    });
  });

  it("orders recommendations by score before families in registry order", async () => {
    const user = userEvent.setup();
    render(
      <PrintLookPicker
        {...makeProps({
          selectedFamilyId: "maison",
          recommendations: [
            {
              familyId: "gallery",
              treatment: "photo-led",
              outputFormat: "two-page-spread",
              reasonKey: "gallery-reason",
              score: 20,
            },
            {
              familyId: "street",
              treatment: "photo-led",
              outputFormat: "counter-menu",
              reasonKey: "street-reason",
              score: 95,
            },
            {
              familyId: "maison",
              treatment: "photo-led",
              outputFormat: "two-page-spread",
              reasonKey: "maison-reason",
              score: 95,
            },
          ],
        })}
      />,
    );

    expect(visibleFamilyNames()).toEqual([
      "print.family.maison",
      "print.family.street",
      "print.family.gallery",
    ]);

    await user.click(
      screen.getByRole("button", { name: "print.look.showAll" }),
    );

    expect(visibleFamilyNames()).toEqual([
      "print.family.maison",
      "print.family.street",
      "print.family.gallery",
      "print.family.atelier",
      "print.family.osteria",
      "print.family.nightHouse",
      "print.family.counter",
      "print.family.field",
    ]);
  });

  it("keeps the remaining looks one quiet disclosure away", async () => {
    const user = userEvent.setup();
    render(<PrintLookPicker {...makeProps()} />);

    expect(
      screen.queryByRole("button", { name: "print.look.showFewer" }),
    ).not.toBeInTheDocument();

    await user.click(
      screen.getByRole("button", { name: "print.look.showAll" }),
    );
    expect(screen.getAllByRole("radio")).toHaveLength(ALL_FAMILY_COUNT);

    await user.click(
      screen.getByRole("button", { name: "print.look.showFewer" }),
    );
    expect(screen.getAllByRole("radio")).toHaveLength(SHORTLIST_SIZE);
  });

  it("keeps a selection made outside the shortlist visible", () => {
    const props = makeProps({ selectedFamilyId: "street" });
    render(<PrintLookPicker {...props} />);

    const names = visibleFamilyNames();
    expect(names).toHaveLength(SHORTLIST_SIZE + 1);
    expect(names.at(-1)).toBe("print.family.street");
    expect(
      screen.getByRole("radio", { name: "print.family.street" }),
    ).toBeChecked();
    expect(props.onSelect).not.toHaveBeenCalled();
  });

  it("previews and selects the treatment the recommendation asks for", async () => {
    const user = userEvent.setup();
    const props = makeProps({
      selectedFamilyId: "atelier",
      recommendations: [
        {
          familyId: "maison",
          treatment: "type-led",
          outputFormat: "two-page-spread",
          reasonKey: "maison-reason",
          score: 95,
        },
      ],
    });
    render(<PrintLookPicker {...props} />);

    const maison = screen.getByRole("radio", { name: "print.family.maison" });
    const preview = within(
      maison.closest("label") as HTMLLabelElement,
    ).getByTitle<HTMLIFrameElement>("print.look.previewTitle");

    expect(preview.srcdoc).toContain("treatment-type-led");
    expect(preview.srcdoc).not.toContain('data-treatment="photo-led"');
    await user.click(maison);
    expect(props.onSelect).toHaveBeenLastCalledWith(
      "maison",
      "type-led",
      "two-page-spread",
    );
  });

  it("keeps preview iframes sandboxed, lazy, and outside interaction", () => {
    render(<PrintLookPicker {...makeProps()} />);

    screen
      .getAllByTitle<HTMLIFrameElement>("print.look.previewTitle")
      .forEach((frame) => {
        expect(frame).toHaveAttribute("sandbox", "allow-same-origin");
        expect(frame.getAttribute("sandbox")).not.toContain("allow-scripts");
        expect(frame).toHaveAttribute("tabindex", "-1");
        expect(frame).toHaveAttribute("aria-hidden", "true");
        expect(frame).toHaveAttribute("loading", "lazy");
        expect(frame).toHaveClass("pointer-events-none");
        expect(frame.srcdoc).toContain('src: url("/fonts/');
        expect(frame.srcdoc).not.toMatch(/<script(?:\s|>)/i);
        expect(
          frame.closest("label")?.querySelector('input[type="radio"]'),
        ).toHaveAccessibleName(expect.stringMatching(/^print\.family\./));
      });
  });

  it("skips failed and low-resolution candidates when choosing preview photography", () => {
    const firstUsableUrl =
      "https://images.payverge.test/harvest-bowl-detail-1000x1250.jpg";
    const audit = makeAudit(model, {
      photoCoverage: 0.8,
      measurements: {
        ...printReadyMeasurements(),
        [firstUsableUrl]: {
          url: firstUsableUrl,
          status: "ready",
          width: 1400,
          height: 1250,
        },
      },
      lowResolutionUrls: ["harvest.jpg"],
      failedUrls: ["https://images.payverge.test/harvest-bowl-1600x1200.jpg"],
    });

    render(<PrintLookPicker {...makeProps({ audit })} />);

    screen
      .getAllByTitle<HTMLIFrameElement>("print.look.previewTitle")
      .forEach((frame) => {
        expect(frame.srcdoc).toContain(firstUsableUrl);
        expect(frame.srcdoc).not.toContain('src="harvest.jpg"');
        expect(frame.srcdoc).not.toContain("harvest-bowl-1600x1200.jpg");
      });
  });

  it("fits the exact A4 iframe viewport inside its wrapper and cleans up resize observation", () => {
    const { unmount } = render(<PrintLookPicker {...makeProps()} />);
    const wrappers = screen.getAllByTestId("print-look-preview-frame");
    const frames = screen.getAllByTitle<HTMLIFrameElement>(
      "print.look.previewTitle",
    );
    const wrapper = wrappers[0];
    const frame = frames[0];
    const observer = MockResizeObserver.instances[0];

    expect(MockResizeObserver.instances).toHaveLength(SHORTLIST_SIZE);
    expect(observer.observe).toHaveBeenCalledWith(wrapper);
    expect(wrapper).toHaveClass("overflow-hidden");
    expect(frame.style.width).toBe(`${A4_WIDTH_PX}px`);
    expect(frame.style.height).toBe(`${A4_HEIGHT_PX}px`);
    expect(frame.style.transformOrigin).toBe("top left");
    expect(Number(frame.style.transform.match(/scale\(([^)]+)\)/)?.[1])).toBe(
      0.25,
    );

    act(() => observer.resize(400, 300));
    const expectedScale = Math.min(400 / A4_WIDTH_PX, 300 / A4_HEIGHT_PX);
    const scale = Number(frame.style.transform.match(/scale\(([^)]+)\)/)?.[1]);
    expect(scale).toBeCloseTo(expectedScale, 10);
    expect(A4_WIDTH_PX * scale).toBeLessThanOrEqual(400);
    expect(A4_HEIGHT_PX * scale).toBeLessThanOrEqual(300);

    act(() => observer.resize(0, 300));
    expect(
      Number(frame.style.transform.match(/scale\(([^)]+)\)/)?.[1]),
    ).toBeCloseTo(expectedScale, 10);

    const observers = [...MockResizeObserver.instances];
    unmount();
    observers.forEach((instance) => {
      expect(instance.disconnect).toHaveBeenCalledTimes(1);
    });
  });

  it("sizes a landscape look's preview frame to its own geometry instead of a portrait A4 box", () => {
    // Regression coverage: previews used to hardcode single-sheet/A4 for
    // every candidate, so a folded booklet (landscape, panelCount 2) would
    // render inside a portrait A4 iframe and look flat-sheet shaped. The
    // frame's pixel size must now come from the planned document's real
    // geometry, which swaps width/height for landscape formats.
    const props = makeProps({
      selectedFamilyId: "maison",
      recommendations: [
        {
          familyId: "maison",
          treatment: "type-led",
          outputFormat: "folded-booklet",
          reasonKey: "maison-booklet-reason",
          score: 95,
        },
      ],
    });
    render(<PrintLookPicker {...props} />);

    const maison = screen.getByRole("radio", { name: "print.family.maison" });
    const frame = within(
      maison.closest("label") as HTMLLabelElement,
    ).getByTitle<HTMLIFrameElement>("print.look.previewTitle");

    // a4 folded-booklet swaps to landscape: widthMm=297 (paper height),
    // heightMm=210 (paper width) — see geometry.ts resolvePrintGeometry.
    const expectedWidthPx = 297 * CSS_PX_PER_MM;
    const expectedHeightPx = 210 * CSS_PX_PER_MM;

    expect(frame.style.width).toBe(`${expectedWidthPx}px`);
    expect(frame.style.height).toBe(`${expectedHeightPx}px`);
    expect(frame.style.width).not.toBe(`${A4_WIDTH_PX}px`);
    expect(frame.style.height).not.toBe(`${A4_HEIGHT_PX}px`);
    expect(frame.srcdoc).toContain("format-folded-booklet");
    expect(frame.srcdoc).toContain("--page-width:297mm");
    expect(frame.srcdoc).toContain("--page-height:210mm");
  });

  it("re-plans previews when the paper format changes, keeping the cache scoped to it", () => {
    const planSpy = jest.spyOn(planner, "planMenuDocument");
    const props = makeProps();
    const { rerender } = render(<PrintLookPicker {...props} />);
    const callsBeforeChange = planSpy.mock.calls.length;
    expect(callsBeforeChange).toBe(SHORTLIST_SIZE);

    rerender(<PrintLookPicker {...props} paperFormat="letter" />);

    expect(planSpy.mock.calls.length).toBe(callsBeforeChange + SHORTLIST_SIZE);
    screen
      .getAllByTitle<HTMLIFrameElement>("print.look.previewTitle")
      .forEach((frame) => {
        // Letter stock in either orientation: portrait cards carry the
        // 215.9mm width, the landscape gallery booklet carries it as height.
        expect(frame.srcdoc).toMatch(/--page-(width|height):215\.9mm/);
        expect(frame.srcdoc).not.toMatch(/--page-(width|height):210mm/);
      });

    planSpy.mockRestore();
  });

  it("escalates a blocked inherited format to the family's spacious format", async () => {
    // Only recommended families carry their own right-sized format; the rest
    // inherit the top pick's. A fuller photo-led menu overflows maison's
    // generous single sheet, so its card must escalate to the family's
    // spacious format instead of previewing (and applying) a blocked plan
    // with half the menu omitted.
    const user = userEvent.setup();
    const imageUrl = (index: number) =>
      `https://images.payverge.test/dish-${index}.jpg`;
    const fullerModel: PrintMenuModel = {
      ...model,
      sections: [0, 1, 2].map((sectionIndex) => ({
        id: `section-${sectionIndex}`,
        name: `Section ${sectionIndex + 1}`,
        items: [0, 1, 2].map((itemIndex) => {
          const index = sectionIndex * 3 + itemIndex;
          return {
            id: `item-${index}`,
            name: `Dish ${index + 1}`,
            description:
              "Charred seasonal vegetables, whipped ricotta and toasted seeds.",
            price: 12 + index,
            imageCandidates: [imageUrl(index)],
            dietaryTags: [],
            allergens: [],
          };
        }),
      })),
    };
    const audit = makeAudit(fullerModel, {
      photoCoverage: 1,
      measurements: Object.fromEntries(
        Array.from({ length: 9 }, (_, index) => [
          imageUrl(index),
          {
            url: imageUrl(index),
            status: "ready" as const,
            width: 1800,
            height: 1400,
          },
        ]),
      ),
    });
    const props = makeProps({
      model: fullerModel,
      audit,
      recommendations: [
        {
          familyId: "atelier",
          treatment: "photo-led",
          outputFormat: "single-sheet",
          reasonKey: "atelier-reason",
          score: 95,
        },
      ],
    });
    render(<PrintLookPicker {...props} />);

    await user.click(
      screen.getByRole("button", { name: "print.look.showAll" }),
    );
    const maison = screen.getByRole("radio", { name: "print.family.maison" });
    const frame = within(
      maison.closest("label") as HTMLLabelElement,
    ).getByTitle<HTMLIFrameElement>("print.look.previewTitle");

    expect(frame.srcdoc).toContain("format-folded-booklet");
    expect(frame.srcdoc).not.toContain("format-single-sheet");
    await user.click(maison);
    expect(props.onSelect).toHaveBeenLastCalledWith(
      "maison",
      "photo-led",
      "folded-booklet",
    );
  });

  it("keeps hostile restaurant copy escaped inside the safe renderer", () => {
    const hostileName = '</title><script>alert("look")</script>';
    const hostileModel: PrintMenuModel = {
      ...model,
      business: { ...model.business, name: hostileName },
    };
    render(<PrintLookPicker {...makeProps({ model: hostileModel })} />);

    screen
      .getAllByTitle<HTMLIFrameElement>("print.look.previewTitle")
      .forEach((frame) => {
        expect(frame.srcdoc).not.toContain('<script>alert("look")</script>');
        expect(frame.srcdoc).toContain(
          "&lt;script&gt;alert(&quot;look&quot;)&lt;/script&gt;",
        );
      });
  });

  it("plans only the previews it shows and never re-plans the same menu", async () => {
    const user = userEvent.setup();
    const planSpy = jest.spyOn(planner, "planMenuDocument");
    const renderSpy = jest.spyOn(renderer, "renderPlannedMenuHtml");
    const props = makeProps();
    const { rerender } = render(<PrintLookPicker {...props} />);

    expect(planSpy).toHaveBeenCalledTimes(SHORTLIST_SIZE);
    expect(renderSpy).toHaveBeenCalledTimes(SHORTLIST_SIZE);
    rerender(<PrintLookPicker {...props} />);
    expect(planSpy).toHaveBeenCalledTimes(SHORTLIST_SIZE);
    expect(renderSpy).toHaveBeenCalledTimes(SHORTLIST_SIZE);

    await user.click(
      screen.getByRole("button", { name: "print.look.showAll" }),
    );
    expect(planSpy).toHaveBeenCalledTimes(ALL_FAMILY_COUNT);

    await user.click(
      screen.getByRole("button", { name: "print.look.showFewer" }),
    );
    await user.click(
      screen.getByRole("button", { name: "print.look.showAll" }),
    );
    expect(planSpy).toHaveBeenCalledTimes(ALL_FAMILY_COUNT);
    expect(renderSpy).toHaveBeenCalledTimes(ALL_FAMILY_COUNT);

    planSpy.mockRestore();
    renderSpy.mockRestore();
  });
});
