/** @jest-environment jsdom */
import {
  formatDiscountLabel,
  composeStarterCaption,
  buildSlots,
  normalizeHandle,
  defaultTemplateForPlay,
  playActionTip,
  parseSocialLinks,
  instagramProfileUrl,
  crmLapsedSegmentsHref,
  composeWinBackOutreach,
  composeLocalizedFallbackCaption,
  publicMarketingVenueName,
  happyHourLiveOfferName,
  isHonestHappyHourSuggestion,
  shareOrDownload,
  shareOrDownloadPostPack,
  downloadBlob,
  downloadPostPack,
  downloadPostVideo,
  DOWNLOAD_OBJECT_URL_TTL_MS,
} from "./postContent";
import { renderPostToBlob } from "./templates/renderPost";
import { EDITORIAL } from "./templates/templates";
import type { CampaignSuggestion } from "@/api/marketing";
import type { Business } from "@/api/business";

jest.mock("./templates/renderPost", () => ({
  ...jest.requireActual("./templates/renderPost"),
  renderPostToBlob: jest.fn(),
}));

const business = {
  id: 1,
  name: "Trattoria",
  social_media: "trattoria",
  default_currency: "USD",
  default_language: "en",
} as unknown as Business;

it("formats a percentage discount", () => {
  expect(formatDiscountLabel("percentage", 20, "USD", "en-US")).toBe("20% OFF");
});

it("formats a fixed discount with currency", () => {
  expect(formatDiscountLabel("fixed", 5, "USD", "en-US")).toMatch(/5.*OFF/);
});

it("normalizes a handle", () => {
  expect(normalizeHandle("https://instagram.com/trattoria/")).toBe(
    "@trattoria",
  );
  expect(normalizeHandle("")).toBe("");
});

it("extracts a handle from a social-links JSON blob (business.social_media shape)", () => {
  const json =
    '{"instagram":"https://instagram.com/payverge","facebook":"https://facebook.com/payverge-page"}';
  expect(normalizeHandle(json)).toBe("@payverge");
});

it("falls back through the JSON blob when instagram is absent, and never emits raw JSON", () => {
  expect(normalizeHandle('{"facebook":"https://facebook.com/trattoria"}')).toBe(
    "@trattoria",
  );
  expect(normalizeHandle('{"website":"https://payverge.io"}')).toBe("");
  expect(normalizeHandle("{not json")).toBe("");
  expect(normalizeHandle("{}")).toBe("");
});

it("composes a starter caption, interpolating name/discount/handle", () => {
  const s = {
    play: "offer",
    target_name: "Summer Sale",
    discount_type: "percentage",
    discount_value: 20,
  } as CampaignSuggestion;
  const out = composeStarterCaption({
    suggestion: s,
    business,
    currency: "USD",
    intlLocale: "en-US",
    template: "{discount} — {name} is on now. {handle}",
  });
  expect(out).toBe("20% OFF — Summer Sale is on now. @trattoria");
});

describe("composeLocalizedFallbackCaption", () => {
  it("is immediate, deterministic, and localized for operator locales", () => {
    const input = {
      suggestion: {
        ...({} as CampaignSuggestion),
        play: "featured_dish" as const,
        target_name: "Empanadas",
      },
      business,
    };

    expect(composeLocalizedFallbackCaption({ ...input, locale: "en" })).toBe(
      "Discover Empanadas at Trattoria.",
    );
    expect(composeLocalizedFallbackCaption({ ...input, locale: "es" })).toBe(
      "Conoce Empanadas en Trattoria.",
    );
    expect(composeLocalizedFallbackCaption({ ...input, locale: "es-ar" })).toBe(
      "Conocé Empanadas en Trattoria.",
    );
    expect(composeLocalizedFallbackCaption({ ...input, locale: "ES-AR" })).toBe(
      "Conocé Empanadas en Trattoria.",
    );
  });

  it("uses only supplied facts and never leaks a raw play key or invented promotion", () => {
    const output = composeLocalizedFallbackCaption({
      suggestion: {
        ...({} as CampaignSuggestion),
        play: "happy_hour",
        title: "happy_hour",
        target_name: undefined,
      },
      business,
      locale: "es-AR",
    });

    expect(output).toBe("Descubrí Trattoria.");
    expect(output).not.toMatch(/happy_hour|descuento|%|hoy|ahora/i);
  });

  it("does not caption an OOS steak as a happy hour when no live offer is attached", () => {
    const output = composeLocalizedFallbackCaption({
      suggestion: {
        ...({} as CampaignSuggestion),
        play: "happy_hour",
        title: "Fill Tue 5–7pm with a happy hour",
        target_name: "Steak Plate",
      },
      business,
      locale: "en",
    });
    expect(output).toBe("Discover Trattoria.");
    expect(output).not.toMatch(/steak/i);
  });

  it("captions a happy hour from the attached offer name, not a leftover dish", () => {
    const output = composeLocalizedFallbackCaption({
      suggestion: {
        ...({} as CampaignSuggestion),
        play: "happy_hour",
        target_name: "Steak Plate",
        discount_type: "percentage",
        discount_value: 15,
        metrics: { suggested_offer: "Weekday Lunch 15% Off" },
      },
      business,
      locale: "en",
    });
    expect(output).toBe("Discover Weekday Lunch 15% Off at Trattoria.");
    expect(output).not.toMatch(/steak/i);
  });

  it("preserves factual punctuation and caps multibyte output at 280 Unicode runes", () => {
    expect(
      composeLocalizedFallbackCaption({
        suggestion: {
          ...({} as CampaignSuggestion),
          play: "featured_dish",
          target_name: "Chef’s pick…",
        },
        business: { ...business, name: "Café & Bar" },
        locale: "en",
      }),
    ).toBe("Discover Chef’s pick… at Café & Bar.");

    const output = composeLocalizedFallbackCaption({
      suggestion: {
        ...({} as CampaignSuggestion),
        play: "featured_dish",
        target_name: `Ñ${"🍜".repeat(400)}!`,
      },
      business,
      locale: "en",
    });
    expect(Array.from(output)).toHaveLength(280);
    expect(output.endsWith("…")).toBe(true);
    expect(output).not.toContain("�");
  });

  it("omits demo venue names and #Payverge from fallback copy", () => {
    const demo = {
      ...business,
      name: "Payverge AI Pro Demo Lounge",
    } as Business;
    expect(
      composeLocalizedFallbackCaption({
        suggestion: {
          ...({} as CampaignSuggestion),
          play: "featured_dish",
          target_name: "Harvest Bowl",
        },
        business: demo,
        locale: "en",
      }),
    ).toBe("Discover Harvest Bowl.");
    expect(
      composeLocalizedFallbackCaption({
        suggestion: {
          ...({} as CampaignSuggestion),
          play: "featured_dish",
          target_name: "Harvest Bowl",
        },
        business: demo,
        locale: "es",
      }),
    ).toBe("Conoce Harvest Bowl.");
    expect(publicMarketingVenueName(demo)).toBe("");
  });
});

it("offer slots put the discount in the badge and the name in the headline", () => {
  const s = {
    play: "offer",
    target_name: "Summer Sale",
    discount_type: "percentage",
    discount_value: 20,
  } as CampaignSuggestion;
  const slots = buildSlots({
    suggestion: s,
    business,
    currency: "USD",
    intlLocale: "en-US",
    ctaLabel: "ORDER NOW",
    badgeLabel: "DEAL",
  });
  expect(slots.badge).toBe("20% OFF");
  expect(slots.dishName).toBe("Summer Sale");
  expect(slots.cta).toBe("ORDER NOW");
  expect(slots.handle).toBe("@trattoria");
  expect(slots.price).toBeUndefined(); // offers have no price slot
});

it("omits the price slot when the price metric is zero or negative — never $0.00 on an asset", () => {
  const zero = {
    play: "featured_dish",
    target_name: "Harvest Bowl",
    metrics: { price: 0 },
  } as unknown as CampaignSuggestion;
  const slots = buildSlots({
    suggestion: zero,
    business,
    currency: "USD",
    intlLocale: "en-US",
    ctaLabel: "ORDER NOW",
    badgeLabel: "CHEF'S PICK",
  });
  expect(slots.price).toBe("");
});

it("featured slots use the menu price and the localized badge", () => {
  const s = {
    play: "featured_dish",
    target_name: "Carbonara",
    metrics: { price: 12.99 },
  } as unknown as CampaignSuggestion;
  const slots = buildSlots({
    suggestion: s,
    business,
    currency: "USD",
    intlLocale: "en-US",
    ctaLabel: "ORDER NOW",
    badgeLabel: "CHEF'S PICK",
  });
  expect(slots.dishName).toBe("Carbonara");
  expect(slots.badge).toBe("CHEF'S PICK");
  expect(slots.price).toMatch(/12\.99/);
});

it("treats a dish-only happy hour as dishonest and an attached offer as honest", () => {
  const steakOnly = {
    play: "happy_hour",
    target_name: "Steak Plate",
  } as CampaignSuggestion;
  expect(isHonestHappyHourSuggestion(steakOnly)).toBe(false);
  expect(happyHourLiveOfferName(steakOnly)).toBe("");

  const live = {
    play: "happy_hour",
    target_name: "Steak Plate",
    discount_type: "percentage",
    discount_value: 15,
    metrics: { suggested_offer: "Weekday Lunch 15% Off" },
  } as unknown as CampaignSuggestion;
  expect(isHonestHappyHourSuggestion(live)).toBe(true);
  expect(happyHourLiveOfferName(live)).toBe("Weekday Lunch 15% Off");
  expect(
    isHonestHappyHourSuggestion({
      play: "featured_dish",
    } as CampaignSuggestion),
  ).toBe(true);
});

it("picks a bold default template for offer and happy_hour plays", () => {
  expect(defaultTemplateForPlay("offer")).toBe("bold");
  expect(defaultTemplateForPlay("happy_hour")).toBe("bold");
  expect(defaultTemplateForPlay("win_back")).toBe("minimal");
  expect(defaultTemplateForPlay("featured_dish")).toBe("editorial");
});

it("builds play action tips from metrics", () => {
  expect(
    playActionTip({
      play: "happy_hour",
      metrics: { weakest_window: "Tue 6–8pm", pct_below_mean: 0.25 },
    } as unknown as CampaignSuggestion),
  ).toEqual({
    key: "card.tips.happyHourWithPct",
    params: { window: "Tue 6–8pm", pct: 25 },
  });
  expect(
    playActionTip({
      play: "win_back",
      metrics: { marketable_lapsed: 42 },
    } as unknown as CampaignSuggestion),
  ).toEqual({ key: "card.tips.winBack", params: { count: 42 } });
  // Singular pluralization: count/qty of 1 selects the *One variants.
  expect(
    playActionTip({
      play: "win_back",
      metrics: { marketable_lapsed: 1 },
    } as unknown as CampaignSuggestion),
  ).toEqual({ key: "card.tips.winBackOne", params: { count: 1 } });
  expect(
    playActionTip({
      play: "move_item",
      metrics: { qty_sold: 1 },
    } as unknown as CampaignSuggestion),
  ).toEqual({ key: "card.tips.moveItemOne", params: { qty: 1 } });
  expect(
    playActionTip({
      play: "move_item",
      metrics: { qty_sold: 5 },
    } as unknown as CampaignSuggestion),
  ).toEqual({ key: "card.tips.moveItem", params: { qty: 5 } });
});

it("parses instagram from social_media JSON", () => {
  const links = parseSocialLinks({
    social_media: '{"instagram":"https://instagram.com/cafe"}',
  } as Business);
  expect(instagramProfileUrl(links)).toBe("https://instagram.com/cafe");
});

it("builds CRM lapsed segment deep link", () => {
  expect(crmLapsedSegmentsHref(42)).toBe(
    "/business/42/dashboard?tab=crm&sub=segments&focus=lapsed",
  );
});

it("composes win-back outreach from caption template", () => {
  const draft = composeWinBackOutreach({
    businessName: "Trattoria",
    caption: "We miss your pasta nights!",
    guestCount: 12,
    template: "Hi from {business}! {caption} ({count} guests)",
  });
  expect(draft).toContain("Trattoria");
  expect(draft).toContain("We miss your pasta nights!");
  expect(draft).toContain("12");
});

it("reuses the first rendered blob when native share falls back to download", async () => {
  const renderBlob = renderPostToBlob as jest.MockedFunction<
    typeof renderPostToBlob
  >;
  renderBlob.mockResolvedValue(new Blob(["png"], { type: "image/png" }));
  const createObjectURL = jest.fn(() => "blob:post");
  const revokeObjectURL = jest.fn();
  Object.defineProperty(URL, "createObjectURL", {
    configurable: true,
    value: createObjectURL,
  });
  Object.defineProperty(URL, "revokeObjectURL", {
    configurable: true,
    value: revokeObjectURL,
  });
  Object.defineProperty(navigator, "share", {
    configurable: true,
    value: undefined,
  });
  const writeText = jest.fn(() => Promise.resolve());
  Object.defineProperty(navigator, "clipboard", {
    configurable: true,
    value: { writeText },
  });
  jest.spyOn(HTMLAnchorElement.prototype, "click").mockImplementation(() => {});
  const renderInput = {
    template: EDITORIAL,
    aspect: "4:5" as const,
    photoUrl: "https://cdn.example/post.jpg",
    slots: { dishName: "Ravioles" },
    palette: { primary: "brand", secondary: "ink" },
  };

  await expect(
    shareOrDownloadPostPack({
      renderInput,
      filename: "ravioles.png",
      caption: "Fresh ravioles",
    }),
  ).resolves.toBe("downloaded");

  expect(renderBlob).toHaveBeenCalledTimes(1);
  expect(renderBlob).toHaveBeenCalledWith(renderInput);
  expect(createObjectURL).toHaveBeenCalledTimes(2);
  expect(writeText).toHaveBeenCalledWith("Fresh ravioles");
});

describe("shareOrDownload", () => {
  const blob = new Blob(["png-bytes"], { type: "image/png" });
  let createObjectURL: jest.Mock;
  let clicked: string[];

  beforeEach(() => {
    clicked = [];
    createObjectURL = jest.fn(() => "blob:share");
    Object.defineProperty(URL, "createObjectURL", {
      configurable: true,
      value: createObjectURL,
    });
    Object.defineProperty(URL, "revokeObjectURL", {
      configurable: true,
      value: jest.fn(),
    });
    jest
      .spyOn(HTMLAnchorElement.prototype, "click")
      .mockImplementation(function (this: HTMLAnchorElement) {
        clicked.push(this.download);
      });
    Object.defineProperty(navigator, "clipboard", {
      configurable: true,
      value: { writeText: jest.fn(() => Promise.resolve()) },
    });
  });

  afterEach(() => {
    jest.restoreAllMocks();
    // Drop share stubs so other suites do not inherit them.
    try {
      Reflect.deleteProperty(navigator, "share");
      Reflect.deleteProperty(navigator, "canShare");
    } catch {
      Object.defineProperty(navigator, "share", {
        configurable: true,
        value: undefined,
      });
      Object.defineProperty(navigator, "canShare", {
        configurable: true,
        value: undefined,
      });
    }
  });

  it("uses native share when canShare accepts files", async () => {
    const share = jest.fn(() => Promise.resolve());
    const canShare = jest.fn(() => true);
    Object.defineProperty(navigator, "share", {
      configurable: true,
      value: share,
    });
    Object.defineProperty(navigator, "canShare", {
      configurable: true,
      value: canShare,
    });

    await expect(
      shareOrDownload({
        blob,
        filename: "dish.png",
        mime: "image/png",
        text: "Tonight only",
      }),
    ).resolves.toBe("shared");

    expect(canShare).toHaveBeenCalled();
    expect(share).toHaveBeenCalled();
    expect(clicked).toHaveLength(0);
  });

  it("falls back to download when navigator.share is missing", async () => {
    Object.defineProperty(navigator, "share", {
      configurable: true,
      value: undefined,
    });

    await expect(
      shareOrDownload({
        blob,
        filename: "dish.png",
        text: "Caption text",
      }),
    ).resolves.toBe("downloaded");

    expect(clicked).toEqual(
      expect.arrayContaining(["dish.png", "dish-caption.txt"]),
    );
    expect(navigator.clipboard.writeText).toHaveBeenCalledWith("Caption text");
  });

  it("falls back to download when canShare rejects files", async () => {
    Object.defineProperty(navigator, "share", {
      configurable: true,
      value: jest.fn(() => Promise.resolve()),
    });
    Object.defineProperty(navigator, "canShare", {
      configurable: true,
      value: jest.fn(() => false),
    });

    await expect(
      shareOrDownload({ blob, filename: "dish.png", text: "Hi" }),
    ).resolves.toBe("downloaded");
    expect(navigator.share).not.toHaveBeenCalled();
    expect(clicked).toContain("dish.png");
  });

  it("re-throws AbortError so a dismissed share sheet is not a download", async () => {
    Object.defineProperty(navigator, "share", {
      configurable: true,
      value: jest.fn(() =>
        Promise.reject(new DOMException("dismissed", "AbortError")),
      ),
    });
    Object.defineProperty(navigator, "canShare", {
      configurable: true,
      value: jest.fn(() => true),
    });

    await expect(
      shareOrDownload({ blob, filename: "dish.png" }),
    ).rejects.toMatchObject({ name: "AbortError" });
    expect(clicked).toHaveLength(0);
  });
});

describe("downloadPostVideo", () => {
  const renderInput = {
    kit: "editorial" as const,
    aspect: "4:5" as const,
    photoUrl: "https://cdn.example.com/dish.jpg",
    slots: { dishName: "Milanesa" },
    palette: { primary: "#1a6b6a", secondary: "#0f3d3c" },
  };

  let clicked: string[];

  beforeEach(() => {
    clicked = [];
    Object.defineProperty(URL, "createObjectURL", {
      value: jest.fn(() => "blob:fake"),
      configurable: true,
      writable: true,
    });
    Object.defineProperty(URL, "revokeObjectURL", {
      value: jest.fn(),
      configurable: true,
      writable: true,
    });
    // Spy on click, not createElement: a createElement mock that re-enters
    // itself via jest.spyOn stacks and overflows after the first test.
    jest
      .spyOn(HTMLAnchorElement.prototype, "click")
      .mockImplementation(function (this: HTMLAnchorElement) {
        clicked.push(this.download);
      });
  });

  afterEach(() => {
    jest.restoreAllMocks();
  });

  it("downloads the mp4, the poster and the caption in one gesture", async () => {
    await downloadPostVideo({
      renderInput,
      filename: "milanesa-4x5.mp4",
      caption: "Tonight only.",
      exportMotion: async () => ({
        video: new Blob(["mp4"], { type: "video/mp4" }),
        poster: new Blob(["png"], { type: "image/png" }),
      }),
      preset: "pushIn",
    });

    expect(clicked).toEqual([
      "milanesa-4x5.mp4",
      "milanesa-4x5-poster.png",
      "milanesa-4x5-caption.txt",
    ]);
  });

  it("skips the caption file when there is no caption", async () => {
    await downloadPostVideo({
      renderInput,
      filename: "milanesa-4x5.mp4",
      caption: "   ",
      exportMotion: async () => ({
        video: new Blob(["mp4"], { type: "video/mp4" }),
        poster: new Blob(["png"], { type: "image/png" }),
      }),
      preset: "pushIn",
    });

    expect(clicked).toEqual(["milanesa-4x5.mp4", "milanesa-4x5-poster.png"]);
  });

  it("forwards the preset, progress callback and signal to the exporter", async () => {
    const seen: Record<string, unknown> = {};
    const controller = new AbortController();
    const onProgress = jest.fn();
    await downloadPostVideo({
      renderInput,
      filename: "a.mp4",
      caption: "",
      preset: "grainDrift",
      onProgress,
      signal: controller.signal,
      exportMotion: async (args) => {
        Object.assign(seen, args);
        return {
          video: new Blob(["mp4"], { type: "video/mp4" }),
          poster: new Blob(["png"], { type: "image/png" }),
        };
      },
    });
    expect(seen.preset).toBe("grainDrift");
    expect(seen.onProgress).toBe(onProgress);
    expect(seen.signal).toBe(controller.signal);
  });

  it("rejects an empty video blob instead of clicking a no-op download", async () => {
    await expect(
      downloadPostVideo({
        renderInput,
        filename: "milanesa-4x5.mp4",
        caption: "Tonight",
        exportMotion: async () => ({
          video: new Blob([], { type: "video/mp4" }),
          poster: new Blob(["png"], { type: "image/png" }),
        }),
        preset: "pushIn",
      }),
    ).rejects.toThrow("empty_blob");
    expect(clicked).toHaveLength(0);
  });
});

describe("downloadBlob", () => {
  const blob = new Blob(["png-bytes"], { type: "image/png" });
  let revokeObjectURL: jest.Mock;
  let clicked: string[];

  beforeEach(() => {
    clicked = [];
    revokeObjectURL = jest.fn();
    Object.defineProperty(URL, "createObjectURL", {
      configurable: true,
      value: jest.fn(() => "blob:download"),
    });
    Object.defineProperty(URL, "revokeObjectURL", {
      configurable: true,
      value: revokeObjectURL,
    });
    jest
      .spyOn(HTMLAnchorElement.prototype, "click")
      .mockImplementation(function (this: HTMLAnchorElement) {
        clicked.push(this.download);
      });
  });

  afterEach(() => {
    jest.useRealTimers();
    jest.restoreAllMocks();
  });

  it("clicks a named download and does not revoke the object URL in the same turn", () => {
    downloadBlob(blob, "milanesa-4x5.png");
    expect(clicked).toEqual(["milanesa-4x5.png"]);
    expect(revokeObjectURL).not.toHaveBeenCalled();
  });

  it("revokes the object URL after the browser has had time to start the download", () => {
    jest.useFakeTimers();
    downloadBlob(blob, "milanesa-4x5.png");
    expect(revokeObjectURL).not.toHaveBeenCalled();
    jest.advanceTimersByTime(DOWNLOAD_OBJECT_URL_TTL_MS - 1);
    expect(revokeObjectURL).not.toHaveBeenCalled();
    jest.advanceTimersByTime(1);
    expect(revokeObjectURL).toHaveBeenCalledWith("blob:download");
  });

  it("throws on an empty blob so callers can surface an error", () => {
    expect(() =>
      downloadBlob(new Blob([], { type: "image/png" }), "empty.png"),
    ).toThrow("empty_blob");
    expect(clicked).toHaveLength(0);
    expect(revokeObjectURL).not.toHaveBeenCalled();
  });
});

describe("downloadPostPack", () => {
  const renderInput = {
    kit: "editorial" as const,
    aspect: "4:5" as const,
    photoUrl: "https://cdn.example.com/dish.jpg",
    slots: { dishName: "Milanesa" },
    palette: { primary: "#1a6b6a", secondary: "#0f3d3c" },
  };
  let clicked: string[];

  beforeEach(() => {
    clicked = [];
    (renderPostToBlob as jest.MockedFunction<typeof renderPostToBlob>)
      .mockReset()
      .mockResolvedValue(new Blob(["png"], { type: "image/png" }));
    Object.defineProperty(URL, "createObjectURL", {
      configurable: true,
      value: jest.fn(() => "blob:pack"),
    });
    Object.defineProperty(URL, "revokeObjectURL", {
      configurable: true,
      value: jest.fn(),
    });
    jest
      .spyOn(HTMLAnchorElement.prototype, "click")
      .mockImplementation(function (this: HTMLAnchorElement) {
        clicked.push(this.download);
      });
  });

  afterEach(() => {
    jest.restoreAllMocks();
  });

  it("downloads the png and caption under the given filename", async () => {
    await downloadPostPack({
      renderInput,
      filename: "milanesa-4x5.png",
      caption: "Tonight only.",
    });
    expect(clicked).toEqual(["milanesa-4x5.png", "milanesa-4x5-caption.txt"]);
  });

  it("rejects a missing render blob instead of a silent no-op", async () => {
    (renderPostToBlob as jest.MockedFunction<typeof renderPostToBlob>)
      .mockResolvedValueOnce(new Blob([], { type: "image/png" }));
    await expect(
      downloadPostPack({
        renderInput,
        filename: "milanesa-4x5.png",
        caption: "Tonight only.",
      }),
    ).rejects.toThrow("empty_blob");
    expect(clicked).toHaveLength(0);
  });
});
