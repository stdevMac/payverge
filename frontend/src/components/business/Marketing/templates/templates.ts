import {
  PLATFORM_CONTENT_BOUNDS,
  type AspectRatio,
  type NormalizedRect,
  type SlotDef,
  type SlotKey,
  type TemplateDef,
  type TemplateLayout,
  type TemplateStyle,
} from "./types";

const fullBleed: NormalizedRect = { x: 0, y: 0, w: 1, h: 1 };

function slot(rect: NormalizedRect, options: Omit<SlotDef, "rect">): SlotDef {
  return { rect, ...options };
}

function layout(
  aspect: AspectRatio,
  options: Omit<TemplateLayout, "contentBounds" | "imageArea"> & {
    imageArea?: NormalizedRect;
  },
): TemplateLayout {
  return {
    contentBounds: PLATFORM_CONTENT_BOUNDS[aspect],
    imageArea: options.imageArea ?? fullBleed,
    logoAnchor: options.logoAnchor,
    slots: options.slots,
    scrim: options.scrim,
  };
}

const darkBottom = (region: NormalizedRect) => ({
  region,
  from: "rgba(28,25,23,0)",
  to: "rgba(28,25,23,0.78)",
  adaptive: true,
  luminanceThreshold: 155,
});

const darkFull = (region: NormalizedRect) => ({
  region,
  from: "rgba(28,25,23,0.1)",
  to: "rgba(28,25,23,0.55)",
  adaptive: true,
  luminanceThreshold: 155,
});

const lightBottom = (region: NormalizedRect) => ({
  region,
  from: "rgba(250,249,246,0)",
  to: "rgba(250,249,246,0.94)",
  adaptive: false,
  luminanceThreshold: 155,
});

const editorialSlots = {
  square: {
    badge: slot(
      { x: 0.07, y: 0.07, w: 0.5, h: 0.06 },
      {
        font: "sans",
        weight: 600,
        sizePct: 0.026,
        align: "left",
        color: "onPhoto",
        maxLines: 1,
        lineHeight: 1.1,
        uppercase: true,
        pill: { color: "primary", padX: 0.022, padY: 0.012 },
      },
    ),
    dishName: slot(
      { x: 0.07, y: 0.66, w: 0.86, h: 0.17 },
      {
        font: "serif",
        weight: 400,
        sizePct: 0.075,
        align: "left",
        color: "onPhoto",
        maxLines: 2,
        lineHeight: 1.05,
      },
    ),
    price: slot(
      { x: 0.07, y: 0.85, w: 0.38, h: 0.06 },
      {
        font: "sans",
        weight: 700,
        sizePct: 0.048,
        align: "left",
        color: "onPhoto",
        maxLines: 1,
        lineHeight: 1.1,
      },
    ),
    cta: slot(
      { x: 0.5, y: 0.855, w: 0.43, h: 0.05 },
      {
        font: "sans",
        weight: 600,
        sizePct: 0.029,
        align: "right",
        color: "onPhoto",
        maxLines: 1,
        lineHeight: 1.1,
        uppercase: true,
      },
    ),
    handle: slot(
      { x: 0.07, y: 0.915, w: 0.86, h: 0.025 },
      {
        font: "sans",
        weight: 500,
        sizePct: 0.021,
        align: "left",
        color: "onPhotoMuted",
        maxLines: 1,
        lineHeight: 1.1,
      },
    ),
  },
  feed: {
    badge: slot(
      { x: 0.07, y: 0.075, w: 0.5, h: 0.055 },
      {
        font: "sans",
        weight: 600,
        sizePct: 0.026,
        align: "left",
        color: "onPhoto",
        maxLines: 1,
        lineHeight: 1.1,
        uppercase: true,
        pill: { color: "primary", padX: 0.022, padY: 0.011 },
      },
    ),
    dishName: slot(
      { x: 0.07, y: 0.65, w: 0.86, h: 0.14 },
      {
        font: "serif",
        weight: 400,
        sizePct: 0.075,
        align: "left",
        color: "onPhoto",
        maxLines: 2,
        lineHeight: 1.05,
      },
    ),
    price: slot(
      { x: 0.07, y: 0.82, w: 0.38, h: 0.052 },
      {
        font: "sans",
        weight: 700,
        sizePct: 0.048,
        align: "left",
        color: "onPhoto",
        maxLines: 1,
        lineHeight: 1.1,
      },
    ),
    cta: slot(
      { x: 0.5, y: 0.825, w: 0.43, h: 0.045 },
      {
        font: "sans",
        weight: 600,
        sizePct: 0.029,
        align: "right",
        color: "onPhoto",
        maxLines: 1,
        lineHeight: 1.1,
        uppercase: true,
      },
    ),
    handle: slot(
      { x: 0.07, y: 0.89, w: 0.86, h: 0.025 },
      {
        font: "sans",
        weight: 500,
        sizePct: 0.021,
        align: "left",
        color: "onPhotoMuted",
        maxLines: 1,
        lineHeight: 1.1,
      },
    ),
  },
  story: {
    badge: slot(
      { x: 0.07, y: 0.15, w: 0.5, h: 0.05 },
      {
        font: "sans",
        weight: 600,
        sizePct: 0.026,
        align: "left",
        color: "onPhoto",
        maxLines: 1,
        lineHeight: 1.1,
        uppercase: true,
        pill: { color: "primary", padX: 0.022, padY: 0.009 },
      },
    ),
    dishName: slot(
      { x: 0.07, y: 0.53, w: 0.86, h: 0.14 },
      {
        font: "serif",
        weight: 400,
        sizePct: 0.075,
        align: "left",
        color: "onPhoto",
        maxLines: 2,
        lineHeight: 1.05,
      },
    ),
    price: slot(
      { x: 0.07, y: 0.7, w: 0.38, h: 0.045 },
      {
        font: "sans",
        weight: 700,
        sizePct: 0.048,
        align: "left",
        color: "onPhoto",
        maxLines: 1,
        lineHeight: 1.1,
      },
    ),
    cta: slot(
      { x: 0.5, y: 0.7, w: 0.43, h: 0.045 },
      {
        font: "sans",
        weight: 600,
        sizePct: 0.029,
        align: "right",
        color: "onPhoto",
        maxLines: 1,
        lineHeight: 1.1,
        uppercase: true,
      },
    ),
    handle: slot(
      { x: 0.07, y: 0.785, w: 0.86, h: 0.025 },
      {
        font: "sans",
        weight: 500,
        sizePct: 0.021,
        align: "left",
        color: "onPhotoMuted",
        maxLines: 1,
        lineHeight: 1.1,
      },
    ),
  },
};

export const EDITORIAL: TemplateDef = {
  id: "editorial",
  labelKey: "templates.editorial",
  layouts: {
    "1:1": layout("1:1", {
      logoAnchor: { x: 0.83, y: 0.065, size: 0.1 },
      slots: editorialSlots.square,
      scrim: darkBottom({ x: 0, y: 0.43, w: 1, h: 0.57 }),
    }),
    "4:5": layout("4:5", {
      logoAnchor: { x: 0.83, y: 0.075, size: 0.1 },
      slots: editorialSlots.feed,
      scrim: darkBottom({ x: 0, y: 0.42, w: 1, h: 0.58 }),
    }),
    "9:16": layout("9:16", {
      logoAnchor: { x: 0.84, y: 0.145, size: 0.09 },
      slots: editorialSlots.story,
      scrim: darkBottom({ x: 0, y: 0.42, w: 1, h: 0.4 }),
    }),
  },
};

function boldSlots(aspect: AspectRatio): Partial<Record<SlotKey, SlotDef>> {
  const story = aspect === "9:16";
  const feed = aspect === "4:5";
  return {
    badge: slot(
      {
        x: story ? 0.25 : 0.22,
        y: story ? 0.15 : feed ? 0.08 : 0.07,
        w: story ? 0.5 : 0.56,
        h: 0.05,
      },
      {
        font: "sans",
        weight: 600,
        sizePct: 0.023,
        align: "center",
        color: "ink",
        maxLines: 1,
        lineHeight: 1.1,
        uppercase: true,
        pill: { color: "onPhoto", padX: 0.022, padY: 0.01 },
      },
    ),
    dishName: slot(
      {
        x: 0.06,
        y: story ? 0.25 : feed ? 0.19 : 0.16,
        w: 0.88,
        h: story ? 0.22 : feed ? 0.2 : 0.23,
      },
      {
        font: "sans",
        weight: 700,
        sizePct: 0.082,
        align: "center",
        color: "onPhoto",
        maxLines: 3,
        lineHeight: 1.02,
        uppercase: true,
      },
    ),
    price: slot(
      {
        x: 0.06,
        y: story ? 0.61 : feed ? 0.73 : 0.75,
        w: 0.88,
        h: 0.06,
      },
      {
        font: "sans",
        weight: 700,
        sizePct: 0.055,
        align: "center",
        color: "onPhoto",
        maxLines: 1,
        lineHeight: 1.1,
      },
    ),
    cta: slot(
      {
        x: 0.2,
        y: story ? 0.69 : feed ? 0.82 : 0.84,
        w: 0.6,
        h: 0.06,
      },
      {
        font: "sans",
        weight: 600,
        sizePct: 0.03,
        align: "center",
        color: "ink",
        maxLines: 1,
        lineHeight: 1.1,
        uppercase: true,
        pill: { color: "onPhoto", padX: 0.03, padY: 0.012 },
      },
    ),
    handle: slot(
      {
        x: 0.06,
        y: story ? 0.79 : feed ? 0.895 : 0.92,
        w: 0.88,
        h: 0.025,
      },
      {
        font: "sans",
        weight: 500,
        sizePct: 0.02,
        align: "center",
        color: "onPhotoMuted",
        maxLines: 1,
        lineHeight: 1.1,
      },
    ),
  };
}

export const BOLD: TemplateDef = {
  id: "bold",
  labelKey: "templates.bold",
  layouts: {
    "1:1": layout("1:1", {
      logoAnchor: { x: 0.82, y: 0.84, size: 0.1 },
      slots: boldSlots("1:1"),
      scrim: darkFull({ x: 0, y: 0, w: 1, h: 1 }),
    }),
    "4:5": layout("4:5", {
      logoAnchor: { x: 0.82, y: 0.075, size: 0.1 },
      slots: boldSlots("4:5"),
      scrim: darkFull({ x: 0, y: 0, w: 1, h: 1 }),
    }),
    "9:16": layout("9:16", {
      logoAnchor: { x: 0.84, y: 0.72, size: 0.09 },
      slots: boldSlots("9:16"),
      scrim: darkFull({ x: 0, y: 0.12, w: 1, h: 0.7 }),
    }),
  },
};

function minimalSlots(aspect: AspectRatio): Partial<Record<SlotKey, SlotDef>> {
  const story = aspect === "9:16";
  const feed = aspect === "4:5";
  return {
    dishName: slot(
      {
        x: 0.08,
        y: story ? 0.59 : feed ? 0.72 : 0.75,
        w: 0.84,
        h: story ? 0.13 : 0.12,
      },
      {
        font: "serif",
        weight: 400,
        sizePct: 0.058,
        align: "left",
        color: "ink",
        maxLines: 2,
        lineHeight: 1.05,
      },
    ),
    price: slot(
      {
        x: 0.08,
        y: story ? 0.75 : feed ? 0.86 : 0.88,
        w: 0.38,
        h: 0.05,
      },
      {
        font: "sans",
        weight: 600,
        sizePct: 0.034,
        align: "left",
        color: "primary",
        maxLines: 1,
        lineHeight: 1.1,
      },
    ),
    handle: slot(
      {
        x: 0.5,
        y: story ? 0.755 : feed ? 0.865 : 0.885,
        w: 0.42,
        h: 0.04,
      },
      {
        font: "sans",
        weight: 500,
        sizePct: 0.022,
        align: "right",
        color: "ink",
        maxLines: 1,
        lineHeight: 1.1,
      },
    ),
  };
}

export const MINIMAL: TemplateDef = {
  id: "minimal",
  labelKey: "templates.minimal",
  layouts: {
    "1:1": layout("1:1", {
      imageArea: { x: 0.04, y: 0.04, w: 0.92, h: 0.72 },
      logoAnchor: { x: 0.82, y: 0.065, size: 0.1 },
      slots: minimalSlots("1:1"),
      scrim: lightBottom({ x: 0, y: 0.66, w: 1, h: 0.34 }),
    }),
    "4:5": layout("4:5", {
      imageArea: { x: 0.04, y: 0.04, w: 0.92, h: 0.68 },
      logoAnchor: { x: 0.82, y: 0.065, size: 0.1 },
      slots: minimalSlots("4:5"),
      scrim: lightBottom({ x: 0, y: 0.66, w: 1, h: 0.34 }),
    }),
    "9:16": layout("9:16", {
      imageArea: { x: 0.04, y: 0.12, w: 0.92, h: 0.48 },
      logoAnchor: { x: 0.84, y: 0.145, size: 0.09 },
      slots: minimalSlots("9:16"),
      scrim: lightBottom({ x: 0, y: 0.54, w: 1, h: 0.28 }),
    }),
  },
};

export const TEMPLATES: Record<TemplateStyle, TemplateDef> = {
  editorial: EDITORIAL,
  bold: BOLD,
  minimal: MINIMAL,
};

export const TEMPLATE_ORDER: TemplateStyle[] = ["editorial", "bold", "minimal"];

/** Which slots each play prefills by default (others stay empty/operator-editable). */
export const PLAY_SLOTS: Record<string, SlotKey[]> = {
  featured_dish: ["badge", "dishName", "price", "cta", "handle"],
  happy_hour: ["badge", "dishName", "price", "cta", "handle"],
  move_item: ["badge", "dishName", "price", "cta", "handle"],
  win_back: ["badge", "dishName", "cta", "handle"],
  combo_deal: ["badge", "dishName", "price", "cta", "handle"],
  offer: ["badge", "dishName", "cta", "handle"],
  "": ["dishName", "price", "cta", "handle"],
};
