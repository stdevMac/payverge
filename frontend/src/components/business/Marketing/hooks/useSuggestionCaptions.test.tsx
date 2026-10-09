/** @jest-environment jsdom */
import React from "react";
import { act, renderHook, waitFor } from "@testing-library/react";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import {
  generateMarketingCaption,
  MarketingCaptionUnavailableError,
  updateMarketingSettings,
} from "@/api/marketing";
import type {
  CampaignSuggestion,
  MarketingCreativeProfile,
  MarketingSettings,
} from "@/api/marketing";
import type { Business } from "@/api/business";
import {
  businessGroundingFingerprint,
  canonicalMarketingRequestLocale,
  captionProfileFingerprint,
  captionRequestFingerprint,
  creativeProfileFingerprint,
  marketingCaptionQueryKey,
  resolveCaptionSubject,
  useSuggestionCaption,
} from "./useSuggestionCaptions";
import {
  marketingSettingsQueryKey,
  useMarketingSettings,
} from "./useMarketingSettings";

jest.mock("@/api/marketing", () => ({
  ...jest.requireActual("@/api/marketing"),
  generateMarketingCaption: jest.fn(),
  updateMarketingSettings: jest.fn(),
}));

const mockedGenerateCaption = generateMarketingCaption as jest.MockedFunction<
  typeof generateMarketingCaption
>;
const mockedUpdateSettings = updateMarketingSettings as jest.MockedFunction<
  typeof updateMarketingSettings
>;

const business = {
  id: 42,
  name: "Casa Sur",
  default_language: "en",
} as Business;

const suggestion = {
  id: "featured-7",
  play: "featured_dish",
  title: "Feature the empanadas",
  why_data: "Frequently ordered",
  source: "menu_engineering",
  copy_angle: "Show the dish simply",
  rank: 90,
  target_name: "Empanadas",
} as CampaignSuggestion;

const profile: MarketingCreativeProfile = {
  audience: "neighbours",
  voice: "welcoming",
  visual_mood: "natural",
  cta_style: "soft",
  hashtag_behavior: "light",
  avoid_phrases: ["best ever", "hurry"],
  default_language: "",
  default_tone: "warm",
};

function deferred<T>() {
  let resolve!: (value: T) => void;
  let reject!: (reason?: unknown) => void;
  const promise = new Promise<T>((res, rej) => {
    resolve = res;
    reject = rej;
  });
  return { promise, resolve, reject };
}

function testWrapper(client: QueryClient) {
  return function Wrapper({ children }: { children: React.ReactNode }) {
    return (
      <QueryClientProvider client={client}>{children}</QueryClientProvider>
    );
  };
}

function makeClient() {
  return new QueryClient({
    defaultOptions: { queries: { retry: false, gcTime: Infinity } },
  });
}

class IntersectionObserverMock implements IntersectionObserver {
  static instances: IntersectionObserverMock[] = [];

  readonly root = null;
  readonly rootMargin: string;
  readonly thresholds: readonly number[];
  private readonly callback: IntersectionObserverCallback;
  private target: Element | null = null;

  constructor(
    callback: IntersectionObserverCallback,
    options?: IntersectionObserverInit,
  ) {
    this.callback = callback;
    this.rootMargin = options?.rootMargin ?? "0px";
    this.thresholds = Array.isArray(options?.threshold)
      ? options?.threshold
      : [options?.threshold ?? 0];
    IntersectionObserverMock.instances.push(this);
  }

  observe = jest.fn((target: Element) => {
    this.target = target;
  });
  unobserve = jest.fn();
  disconnect = jest.fn();
  takeRecords = jest.fn(() => [] as IntersectionObserverEntry[]);

  trigger(isIntersecting: boolean) {
    if (!this.target) throw new Error("observer has no target");
    this.callback(
      [
        {
          isIntersecting,
          target: this.target,
          boundingClientRect: this.target.getBoundingClientRect(),
          intersectionRatio: isIntersecting ? 1 : 0,
          intersectionRect: this.target.getBoundingClientRect(),
          rootBounds: null,
          time: 0,
        } as IntersectionObserverEntry,
      ],
      this,
    );
  }
}

beforeEach(() => {
  jest.clearAllMocks();
  IntersectionObserverMock.instances = [];
  global.IntersectionObserver = IntersectionObserverMock;
});

afterEach(() => {
  delete (global as Partial<typeof globalThis>).IntersectionObserver;
});

it("returns a localized deterministic fallback immediately while the top card fetches", async () => {
  const request = deferred<string>();
  mockedGenerateCaption.mockReturnValue(request.promise);
  const client = makeClient();

  const { result } = renderHook(
    () =>
      useSuggestionCaption({
        businessId: 42,
        business,
        suggestion,
        locale: "es-ar",
        creativeProfile: profile,
        enabled: true,
        priority: true,
      }),
    { wrapper: testWrapper(client) },
  );

  expect(result.current.caption).toBe("Conocé Empanadas en Casa Sur.");
  expect(result.current.source).toBe("fallback");
  await waitFor(() => expect(mockedGenerateCaption).toHaveBeenCalledTimes(1));
  expect(mockedGenerateCaption).toHaveBeenCalledWith(
    "42",
    expect.objectContaining({ language: "es-AR", tone: "warm" }),
    expect.any(AbortSignal),
  );

  act(() => request.resolve("Empanadas, hechas para compartir."));
  await waitFor(() => expect(result.current.source).toBe("ai"));
  expect(result.current.caption).toBe("Empanadas, hechas para compartir.");
});

it("does not sell an 86'd steak via fallback when caption generation refuses", async () => {
  mockedGenerateCaption.mockRejectedValue(
    new MarketingCaptionUnavailableError(),
  );
  const client = makeClient();
  const steak = {
    ...suggestion,
    id: "1:featured_dish:demo-steak",
    play: "featured_dish" as const,
    title: "Feature your star: Steak Plate",
    target_name: "Steak Plate",
    copy_angle: "tonight's hero steak, charred, chimichurri",
  };

  const { result } = renderHook(
    () =>
      useSuggestionCaption({
        businessId: 86,
        business: {
          ...business,
          name: "Payverge AI Pro Demo Lounge",
        } as Business,
        suggestion: steak,
        locale: "en",
        creativeProfile: profile,
        enabled: true,
        priority: true,
      }),
    { wrapper: testWrapper(client) },
  );

  await waitFor(() => expect(result.current.source).toBe("unavailable"));
  expect(result.current.caption).toBe("");
  expect(result.current.caption).not.toMatch(/steak|payverge/i);
});

it("does not sell Date Night fallback copy in Spanish when the combo is 86'd", async () => {
  mockedGenerateCaption.mockRejectedValue(
    new MarketingCaptionUnavailableError(),
  );
  const client = makeClient();
  const dateNight = {
    ...suggestion,
    id: "1:combo_deal:bundle:1",
    play: "combo_deal" as const,
    title: "Promote Date Night for Two",
    target_name: "Date Night for Two",
  };

  const { result } = renderHook(
    () =>
      useSuggestionCaption({
        businessId: 86,
        business,
        suggestion: dateNight,
        locale: "es",
        creativeProfile: profile,
        enabled: true,
        priority: true,
      }),
    { wrapper: testWrapper(client) },
  );

  await waitFor(() => expect(result.current.source).toBe("unavailable"));
  expect(result.current.caption).toBe("");
  expect(result.current.caption).not.toMatch(/date night|steak/i);
});

it("fetches only the priority card before later cards approach the viewport", async () => {
  mockedGenerateCaption.mockResolvedValue("AI caption");
  const client = makeClient();

  renderHook(
    () => {
      useSuggestionCaption({
        businessId: 42,
        business,
        suggestion,
        locale: "en",
        creativeProfile: profile,
        enabled: true,
        priority: true,
      });
      useSuggestionCaption({
        businessId: 42,
        business,
        suggestion: {
          ...suggestion,
          id: "featured-8",
          target_name: "Milanesa",
        },
        locale: "en",
        creativeProfile: profile,
        enabled: true,
        priority: false,
      });
    },
    { wrapper: testWrapper(client) },
  );

  await waitFor(() => expect(mockedGenerateCaption).toHaveBeenCalledTimes(1));
  expect(mockedGenerateCaption.mock.calls[0][1].item_name).toBe("Empanadas");
});

it("fetches a near-viewport card once through a one-shot observer", async () => {
  mockedGenerateCaption.mockResolvedValue("AI caption");
  const client = makeClient();
  const { result } = renderHook(
    () =>
      useSuggestionCaption({
        businessId: 42,
        business,
        suggestion,
        locale: "en",
        creativeProfile: profile,
        enabled: true,
        priority: false,
      }),
    { wrapper: testWrapper(client) },
  );

  const card = document.createElement("article");
  act(() => result.current.observeRef(card));
  await waitFor(() =>
    expect(IntersectionObserverMock.instances).toHaveLength(1),
  );
  expect(IntersectionObserverMock.instances[0].rootMargin).toBe("320px 0px");

  act(() => IntersectionObserverMock.instances[0].trigger(true));
  await waitFor(() => expect(mockedGenerateCaption).toHaveBeenCalledTimes(1));
  act(() => IntersectionObserverMock.instances[0].trigger(true));
  expect(mockedGenerateCaption).toHaveBeenCalledTimes(1);
  expect(IntersectionObserverMock.instances[0].disconnect).toHaveBeenCalled();
});

it("requests an alternate tone only when that tone is selected", async () => {
  mockedGenerateCaption
    .mockResolvedValueOnce("Warm caption")
    .mockResolvedValueOnce("Punchy caption");
  const client = makeClient();
  const { result } = renderHook(
    () =>
      useSuggestionCaption({
        businessId: 42,
        business,
        suggestion,
        locale: "en",
        creativeProfile: profile,
        enabled: true,
        priority: true,
      }),
    { wrapper: testWrapper(client) },
  );

  await waitFor(() => expect(result.current.caption).toBe("Warm caption"));
  expect(mockedGenerateCaption).toHaveBeenCalledTimes(1);

  act(() => result.current.selectTone("punchy"));
  expect(result.current.caption).toBe("Discover Empanadas at Casa Sur.");
  await waitFor(() => expect(result.current.caption).toBe("Punchy caption"));
  expect(mockedGenerateCaption).toHaveBeenCalledTimes(2);
  expect(mockedGenerateCaption.mock.calls[1][1]).toEqual(
    expect.objectContaining({ language: "en", tone: "punchy" }),
  );
});

it("preserves the fallback after failure and exposes a bounded retry", async () => {
  mockedGenerateCaption
    .mockRejectedValueOnce(new Error("caption unavailable"))
    .mockResolvedValueOnce("Recovered caption");
  const client = makeClient();
  const { result } = renderHook(
    () =>
      useSuggestionCaption({
        businessId: 42,
        business,
        suggestion,
        locale: "en",
        creativeProfile: profile,
        enabled: true,
        priority: true,
      }),
    { wrapper: testWrapper(client) },
  );

  await waitFor(() => expect(result.current.error).toBeInstanceOf(Error));
  expect(result.current.caption).toBe("Discover Empanadas at Casa Sur.");
  expect(result.current.source).toBe("fallback");
  expect(mockedGenerateCaption).toHaveBeenCalledTimes(1);

  act(() => result.current.retry());
  await waitFor(() => expect(result.current.caption).toBe("Recovered caption"));
  expect(mockedGenerateCaption).toHaveBeenCalledTimes(2);
});

it("deduplicates the full identity and invalidates when the creative profile changes", async () => {
  mockedGenerateCaption.mockResolvedValue("AI caption");
  const client = makeClient();
  const { rerender } = renderHook(
    ({ voice }: { voice: string }) => {
      const creativeProfile = { ...profile, voice };
      return [
        useSuggestionCaption({
          businessId: 42,
          business,
          suggestion,
          locale: "ES-ar",
          creativeProfile,
          enabled: true,
          priority: true,
        }),
        useSuggestionCaption({
          businessId: "42",
          business,
          suggestion,
          locale: "es-AR",
          creativeProfile,
          enabled: true,
          priority: true,
        }),
      ];
    },
    { initialProps: { voice: "welcoming" }, wrapper: testWrapper(client) },
  );

  await waitFor(() => expect(mockedGenerateCaption).toHaveBeenCalledTimes(1));
  expect(client.getQueryCache().getAll()[0].queryKey).toEqual(
    marketingCaptionQueryKey(
      "42",
      suggestion.id,
      "es-AR",
      "warm",
      profile,
      captionRequestFingerprint(suggestion, business),
      businessGroundingFingerprint(business),
    ),
  );

  rerender({ voice: "plainspoken" });
  await waitFor(() => expect(mockedGenerateCaption).toHaveBeenCalledTimes(2));
  expect(client.getQueryCache().getAll()).toHaveLength(2);
});

it("fingerprints every creative-profile field in a stable order", () => {
  expect(creativeProfileFingerprint(profile)).toBe(
    creativeProfileFingerprint({
      ...profile,
      avoid_phrases: [...profile.avoid_phrases],
    }),
  );
  expect(
    creativeProfileFingerprint({ ...profile, audience: "tourists" }),
  ).not.toBe(creativeProfileFingerprint(profile));
  expect(
    creativeProfileFingerprint({ ...profile, default_tone: "elegant" }),
  ).not.toBe(creativeProfileFingerprint(profile));
  expect(
    creativeProfileFingerprint({ ...profile, avoid_phrases: ["cheap"] }),
  ).not.toBe(creativeProfileFingerprint(profile));
  expect(
    creativeProfileFingerprint({ ...profile, default_language: "PT" }),
  ).toBe(creativeProfileFingerprint({ ...profile, default_language: "pt" }));
});

it("never calls the write-protected caption route for a read-only card", () => {
  const client = makeClient();
  const { result } = renderHook(
    () =>
      useSuggestionCaption({
        businessId: 42,
        business,
        suggestion,
        locale: "es",
        creativeProfile: profile,
        enabled: false,
        priority: true,
      }),
    { wrapper: testWrapper(client) },
  );

  expect(result.current.caption).toBe("Conoce Empanadas en Casa Sur.");
  expect(result.current.source).toBe("fallback");
  expect(mockedGenerateCaption).not.toHaveBeenCalled();
});

it("keeps each card independent when one errors while another is generating", async () => {
  const pending = deferred<string>();
  mockedGenerateCaption.mockImplementation((_id, request) => {
    if (request.item_name === "Empanadas")
      return Promise.reject(new Error("failed"));
    return pending.promise;
  });
  const client = makeClient();
  const { result } = renderHook(
    () => [
      useSuggestionCaption({
        businessId: 42,
        business,
        suggestion,
        locale: "en",
        creativeProfile: profile,
        enabled: true,
        priority: true,
      }),
      useSuggestionCaption({
        businessId: 42,
        business,
        suggestion: {
          ...suggestion,
          id: "featured-8",
          target_name: "Milanesa",
        },
        locale: "en",
        creativeProfile: profile,
        enabled: true,
        priority: true,
      }),
    ],
    { wrapper: testWrapper(client) },
  );

  await waitFor(() => expect(result.current[0].error).toBeInstanceOf(Error));
  expect(result.current[0].caption).toBe("Discover Empanadas at Casa Sur.");
  expect(result.current[1].caption).toBe("Discover Milanesa at Casa Sur.");
  expect(result.current[1].generating).toBe(true);
});

it("keys every normalized request fact and invalidates only meaningful changes", () => {
  const normalized = {
    ...suggestion,
    target_name: "  Empanadas  ",
    copy_angle: "  Show the dish simply  ",
    why_data: "  Frequently ordered  ",
  };
  const baseFingerprint = captionRequestFingerprint(normalized, business);
  const baseKey = marketingCaptionQueryKey(
    "42",
    suggestion.id,
    "en",
    "warm",
    profile,
    baseFingerprint,
  );

  expect(
    captionRequestFingerprint(
      {
        ...normalized,
        target_name: "Empanadas",
        copy_angle: "Show the dish simply",
        why_data: "Frequently ordered",
      },
      business,
    ),
  ).toBe(baseFingerprint);

  const changedFacts: CampaignSuggestion[] = [
    { ...normalized, target_name: "Milanesa" },
    { ...normalized, play: "move_item" },
    { ...normalized, copy_angle: "Lead with texture" },
    { ...normalized, why_data: "A menu item" },
  ];
  changedFacts.forEach((changed) => {
    expect(
      marketingCaptionQueryKey(
        "42",
        suggestion.id,
        "en",
        "warm",
        profile,
        captionRequestFingerprint(changed, business),
      ),
    ).not.toEqual(baseKey);
  });

  const nameless = { ...normalized, target_name: "" };
  expect(captionRequestFingerprint(nameless, business)).not.toBe(
    captionRequestFingerprint(nameless, { ...business, name: "Casa Norte" }),
  );
  expect(captionRequestFingerprint(normalized, business)).toBe(
    captionRequestFingerprint(normalized, { ...business, name: "Casa Norte" }),
  );
});

it("keys every normalized business grounding fact and ignores irrelevant fields", () => {
  const groundedBusiness = {
    ...business,
    name: "  Casa Sur  ",
    business_type: " restaurant ",
    description: " Seasonal neighbourhood cooking ",
    address: { city: " Buenos Aires " },
    social_media: '{"instagram":"@casasur","tiktok":"@backup"}',
    logo: "first-logo.png",
  } as Business;
  const baseFingerprint = businessGroundingFingerprint(groundedBusiness);

  expect(
    businessGroundingFingerprint({
      ...groundedBusiness,
      name: "Casa Sur",
      business_type: "restaurant",
      description: "Seasonal neighbourhood cooking",
      address: { ...groundedBusiness.address, city: "Buenos Aires" },
      social_media:
        '{ "tiktok": "@changed-backup", "instagram": "https://instagram.com/casasur/" }',
      logo: "unrelated-logo.png",
    }),
  ).toBe(baseFingerprint);

  const changes: Business[] = [
    { ...groundedBusiness, name: "Casa Norte" },
    { ...groundedBusiness, business_type: "cafe" },
    { ...groundedBusiness, description: "All-day cafe" },
    {
      ...groundedBusiness,
      address: { ...groundedBusiness.address, city: "Mendoza" },
    },
    {
      ...groundedBusiness,
      social_media: '{"instagram":"@casa-norte","tiktok":"@backup"}',
    },
  ];
  changes.forEach((changed) => {
    expect(businessGroundingFingerprint(changed)).not.toBe(baseFingerprint);
  });
});

it("matches the backend social-handle normalization and platform priority", () => {
  const fingerprint = (social_media: string) =>
    businessGroundingFingerprint({ ...business, social_media });
  const selectedInstagram = fingerprint("@cafe.sur_1");

  expect(fingerprint("https://instagram.com/cafe.sur_1/extra/path")).toBe(
    selectedInstagram,
  );
  expect(
    fingerprint(
      '{"instagram":"https://www.instagram.com/cafe.sur_1/","tiktok":"@ignored"}',
    ),
  ).toBe(selectedInstagram);
  expect(
    fingerprint(
      '{"instagram":"@cafe.sur_1","tiktok":"@changed","twitter":"@also-changed"}',
    ),
  ).toBe(selectedInstagram);
  expect(fingerprint('{"instagram":"@other","tiktok":"@ignored"}')).not.toBe(
    selectedInstagram,
  );

  const selectedTikTok = fingerprint(
    '{"tiktok":"https://www.tiktok.com/@cafe_sur/videos","twitter":"@ignored"}',
  );
  expect(fingerprint('{"tiktok":"@cafe_sur","twitter":"@changed"}')).toBe(
    selectedTikTok,
  );
  expect(fingerprint('{"tiktok":"@other","twitter":"@ignored"}')).not.toBe(
    selectedTikTok,
  );
});

it.each([
  ["backslash authority separator", "https://instagram.com\\evil.com/foo"],
  ["backslash in path", "https://instagram.com/foo\\bar"],
  ["tab in authority", "https://insta\tgram.com/foo"],
  ["newline in path", "https://instagram.com/\nfoo"],
  ["carriage return in path", "https://instagram.com/\rfoo"],
  ["NUL in path", "https://instagram.com/\0foo"],
  ["empty authority with four slashes", "https:////instagram.com/foo"],
  ["empty authority with three slashes", "https:///instagram.com/foo"],
] as const)(
  "rejects backend-invalid social URL syntax: %s",
  (_label, social_media) => {
    const emptyHandle = businessGroundingFingerprint({
      ...business,
      social_media: "",
    });
    expect(businessGroundingFingerprint({ ...business, social_media })).toBe(
      emptyHandle,
    );
  },
);

it("does not fall through to a lower-priority platform after selecting a malformed handle", () => {
  const malformedInstagram = JSON.stringify({
    instagram: "https://instagram.com\\evil.com/foo",
    tiktok: "@valid_backup",
  });
  expect(
    businessGroundingFingerprint({
      ...business,
      social_media: malformedInstagram,
    }),
  ).toBe(
    businessGroundingFingerprint({
      ...business,
      social_media: "",
    }),
  );
});

it("waits for persisted profile settings before generating and cannot cache a late old response under the new profile", async () => {
  const oldSettings: MarketingSettings = {
    enabled: true,
    disabled_plays: [],
    creative_profile: profile,
  };
  const newSettings: MarketingSettings = {
    ...oldSettings,
    creative_profile: { ...profile, voice: "plainspoken" },
  };
  const saveRequest = deferred<MarketingSettings>();
  const oldCaption = deferred<string>();
  const newCaption = deferred<string>();
  mockedUpdateSettings.mockReturnValue(saveRequest.promise);
  mockedGenerateCaption
    .mockReturnValueOnce(oldCaption.promise)
    .mockReturnValueOnce(newCaption.promise);
  const client = makeClient();
  client.setQueryData(marketingSettingsQueryKey("42"), oldSettings);

  const { result } = renderHook(
    () => {
      const settings = useMarketingSettings(42, false);
      const caption = useSuggestionCaption({
        businessId: 42,
        business,
        suggestion,
        locale: "en",
        creativeProfile: settings.settings.creative_profile,
        enabled: !settings.saving,
        priority: true,
      });
      return { settings, caption };
    },
    { wrapper: testWrapper(client) },
  );

  await waitFor(() => expect(mockedGenerateCaption).toHaveBeenCalledTimes(1));
  act(() => result.current.settings.save(newSettings));
  await waitFor(() => expect(result.current.settings.saving).toBe(true));
  expect(result.current.settings.settings.creative_profile?.voice).toBe(
    "plainspoken",
  );
  expect(mockedGenerateCaption).toHaveBeenCalledTimes(1);

  act(() => saveRequest.resolve(newSettings));
  await waitFor(() => expect(mockedGenerateCaption).toHaveBeenCalledTimes(2));
  act(() => newCaption.resolve("New profile caption"));
  await waitFor(() =>
    expect(result.current.caption.caption).toBe("New profile caption"),
  );

  act(() => oldCaption.resolve("Late old profile caption"));
  await waitFor(() => expect(result.current.settings.saving).toBe(false));
  expect(result.current.caption.caption).toBe("New profile caption");
});

it("returns to the confirmed caption profile after a failed optimistic save", async () => {
  const oldSettings: MarketingSettings = {
    enabled: true,
    disabled_plays: [],
    creative_profile: profile,
  };
  const newSettings: MarketingSettings = {
    ...oldSettings,
    creative_profile: { ...profile, audience: "tourists" },
  };
  const saveRequest = deferred<MarketingSettings>();
  mockedUpdateSettings.mockReturnValue(saveRequest.promise);
  mockedGenerateCaption.mockResolvedValue("Confirmed profile caption");
  const client = makeClient();
  client.setQueryData(marketingSettingsQueryKey("42"), oldSettings);

  const { result } = renderHook(
    () => {
      const settings = useMarketingSettings(42, false);
      const caption = useSuggestionCaption({
        businessId: 42,
        business,
        suggestion,
        locale: "en",
        creativeProfile: settings.settings.creative_profile,
        enabled: !settings.saving,
        priority: true,
      });
      return { settings, caption };
    },
    { wrapper: testWrapper(client) },
  );

  await waitFor(() =>
    expect(result.current.caption.caption).toBe("Confirmed profile caption"),
  );
  act(() => result.current.settings.save(newSettings));
  await waitFor(() => expect(result.current.settings.saving).toBe(true));
  expect(mockedGenerateCaption).toHaveBeenCalledTimes(1);

  act(() => saveRequest.reject(new Error("save failed")));
  await waitFor(() =>
    expect(result.current.settings.rollbackOccurred).toBe(true),
  );
  expect(result.current.settings.settings.creative_profile?.audience).toBe(
    profile.audience,
  );
  expect(result.current.caption.caption).toBe("Confirmed profile caption");
  expect(mockedGenerateCaption).toHaveBeenCalledTimes(1);
});

it("bounds factual subjects to 160 Unicode runes before request and fingerprint", async () => {
  mockedGenerateCaption.mockImplementation((_businessId, request) => {
    if (Array.from(request.item_name).length > 160) {
      return Promise.reject(new Error("400 invalid_marketing_caption"));
    }
    return Promise.resolve("AI caption");
  });
  const exact = "🍜".repeat(160);
  const overlong = "🍜".repeat(161);

  expect(
    resolveCaptionSubject({ ...suggestion, target_name: exact }, business),
  ).toBe(exact);
  const bounded = resolveCaptionSubject(
    { ...suggestion, target_name: overlong },
    business,
  );
  expect(Array.from(bounded)).toHaveLength(160);
  expect(bounded).toBe(`${"🍜".repeat(159)}…`);
  expect(
    captionRequestFingerprint(
      { ...suggestion, target_name: overlong },
      business,
    ),
  ).toBe(
    captionRequestFingerprint(
      { ...suggestion, target_name: bounded },
      business,
    ),
  );

  const client = makeClient();
  const { result } = renderHook(
    () =>
      useSuggestionCaption({
        businessId: 42,
        business,
        suggestion: { ...suggestion, target_name: overlong },
        locale: "en",
        creativeProfile: profile,
        enabled: true,
        priority: true,
      }),
    { wrapper: testWrapper(client) },
  );

  await waitFor(() => expect(mockedGenerateCaption).toHaveBeenCalledTimes(1));
  expect(mockedGenerateCaption.mock.calls[0][1].item_name).toBe(bounded);
  await waitFor(() => expect(result.current.source).toBe("ai"));
  expect(result.current.error).toBeNull();
});

it("uses the same 160-rune bound when the business name is the subject fallback", () => {
  const bounded = resolveCaptionSubject(
    { ...suggestion, target_name: "" },
    { ...business, name: `C${"é".repeat(160)}` },
  );
  expect(Array.from(bounded)).toHaveLength(160);
  expect(bounded.endsWith("…")).toBe(true);
});

it("fingerprints caption-affecting profile fields but not effective key dimensions", () => {
  const baseFingerprint = captionProfileFingerprint(profile);
  expect(
    captionProfileFingerprint({
      ...profile,
      default_tone: "elegant",
      default_language: "fr",
      visual_mood: "moody",
    }),
  ).toBe(baseFingerprint);

  const changes: MarketingCreativeProfile[] = [
    { ...profile, audience: "tourists" },
    { ...profile, voice: "plainspoken" },
    { ...profile, cta_style: "direct" },
    { ...profile, hashtag_behavior: "standard" },
    { ...profile, avoid_phrases: ["cheap"] },
  ];
  changes.forEach((changed) => {
    expect(captionProfileFingerprint(changed)).not.toBe(baseFingerprint);
  });
});

it("does not send an OOS steak name as a happy-hour caption subject", async () => {
  mockedGenerateCaption.mockResolvedValue("AI caption");
  const client = makeClient();
  renderHook(
    () =>
      useSuggestionCaption({
        businessId: 42,
        business,
        suggestion: {
          ...suggestion,
          play: "happy_hour",
          target_name: "Steak Plate",
          title: "Fill Tue 5–7pm with a happy hour",
        },
        locale: "en",
        creativeProfile: profile,
        enabled: true,
        priority: true,
      }),
    { wrapper: testWrapper(client) },
  );

  await waitFor(() => expect(mockedGenerateCaption).toHaveBeenCalledTimes(1));
  expect(mockedGenerateCaption.mock.calls[0][1].item_name).toBe("Casa Sur");
  expect(mockedGenerateCaption.mock.calls[0][1].item_name).not.toMatch(
    /steak/i,
  );
});

it("uses the attached happy-hour offer as the caption subject", async () => {
  mockedGenerateCaption.mockResolvedValue("AI caption");
  const client = makeClient();
  renderHook(
    () =>
      useSuggestionCaption({
        businessId: 42,
        business,
        suggestion: {
          ...suggestion,
          play: "happy_hour",
          target_name: "Steak Plate",
          discount_type: "percentage",
          discount_value: 15,
          metrics: { suggested_offer: "Weekday Lunch 15% Off" },
        },
        locale: "en",
        creativeProfile: profile,
        enabled: true,
        priority: true,
      }),
    { wrapper: testWrapper(client) },
  );

  await waitFor(() => expect(mockedGenerateCaption).toHaveBeenCalledTimes(1));
  expect(mockedGenerateCaption.mock.calls[0][1].item_name).toBe(
    "Weekday Lunch 15% Off",
  );
});

it.each(["happy_hour", "win_back"] as const)(
  "uses the business name as the factual subject for a nameless %s suggestion",
  async (play) => {
    mockedGenerateCaption.mockResolvedValue("AI caption");
    const client = makeClient();
    renderHook(
      () =>
        useSuggestionCaption({
          businessId: 42,
          business,
          suggestion: { ...suggestion, play, target_name: "" },
          locale: "en",
          creativeProfile: profile,
          enabled: true,
          priority: true,
        }),
      { wrapper: testWrapper(client) },
    );

    await waitFor(() => expect(mockedGenerateCaption).toHaveBeenCalledTimes(1));
    expect(mockedGenerateCaption.mock.calls[0][1].item_name).toBe("Casa Sur");
  },
);

it("keeps fallback-only state when neither suggestion nor business has a factual subject", () => {
  const client = makeClient();
  const blankBusiness = { ...business, name: "   " };
  const blankSuggestion = {
    ...suggestion,
    play: "happy_hour" as const,
    target_name: "   ",
    title: "Limited-time deal",
  };
  const { result } = renderHook(
    () =>
      useSuggestionCaption({
        businessId: 42,
        business: blankBusiness,
        suggestion: blankSuggestion,
        locale: "en",
        creativeProfile: profile,
        enabled: true,
        priority: true,
      }),
    { wrapper: testWrapper(client) },
  );

  expect(resolveCaptionSubject(blankSuggestion, blankBusiness)).toBe("");
  expect(result.current.caption).toBe("Discover an idea from your restaurant.");
  expect(result.current.error).toBeNull();
  expect(result.current.generating).toBe(false);
  act(() => result.current.retry());
  expect(mockedGenerateCaption).not.toHaveBeenCalled();
});

it.each([
  ["PT", "pt", "Discover Empanadas at Casa Sur."],
  ["fr", "fr", "Discover Empanadas at Casa Sur."],
  ["es-ar", "es-AR", "Conocé Empanadas en Casa Sur."],
] as const)(
  "sends canonical backend locale %s without changing the fallback language bucket",
  async (inputLocale, requestLocale, expectedFallback) => {
    const request = deferred<string>();
    mockedGenerateCaption.mockReturnValue(request.promise);
    const client = makeClient();
    const { result } = renderHook(
      () =>
        useSuggestionCaption({
          businessId: 42,
          business,
          suggestion,
          locale: inputLocale,
          creativeProfile: { ...profile, default_language: inputLocale },
          enabled: true,
          priority: true,
        }),
      { wrapper: testWrapper(client) },
    );

    expect(canonicalMarketingRequestLocale(inputLocale)).toBe(requestLocale);
    expect(result.current.caption).toBe(expectedFallback);
    await waitFor(() => expect(mockedGenerateCaption).toHaveBeenCalledTimes(1));
    expect(mockedGenerateCaption.mock.calls[0][1].language).toBe(requestLocale);
  },
);

it("switches a changed profile default directly without requesting the obsolete tone", async () => {
  mockedGenerateCaption.mockResolvedValue("AI caption");
  const client = makeClient();
  const { rerender } = renderHook(
    ({
      defaultTone,
    }: {
      defaultTone: MarketingCreativeProfile["default_tone"];
    }) =>
      useSuggestionCaption({
        businessId: 42,
        business,
        suggestion,
        locale: "en",
        creativeProfile: { ...profile, default_tone: defaultTone },
        enabled: true,
        priority: true,
      }),
    {
      initialProps: {
        defaultTone: "warm" as MarketingCreativeProfile["default_tone"],
      },
      wrapper: testWrapper(client),
    },
  );

  await waitFor(() => expect(mockedGenerateCaption).toHaveBeenCalledTimes(1));
  rerender({ defaultTone: "elegant" });
  await waitFor(() => expect(mockedGenerateCaption).toHaveBeenCalledTimes(2));
  expect(mockedGenerateCaption.mock.calls.map((call) => call[1].tone)).toEqual([
    "warm",
    "elegant",
  ]);
});

it("keeps an explicit tone selected when the profile default changes", async () => {
  mockedGenerateCaption.mockResolvedValue("AI caption");
  const client = makeClient();
  const { result, rerender } = renderHook(
    ({
      defaultTone,
      audience,
    }: {
      defaultTone: MarketingCreativeProfile["default_tone"];
      audience: string;
    }) =>
      useSuggestionCaption({
        businessId: 42,
        business,
        suggestion,
        locale: "en",
        creativeProfile: { ...profile, default_tone: defaultTone, audience },
        enabled: true,
        priority: true,
      }),
    {
      initialProps: {
        defaultTone: "warm" as MarketingCreativeProfile["default_tone"],
        audience: profile.audience,
      },
      wrapper: testWrapper(client),
    },
  );

  await waitFor(() => expect(mockedGenerateCaption).toHaveBeenCalledTimes(1));
  act(() => result.current.selectTone("punchy"));
  await waitFor(() => expect(mockedGenerateCaption).toHaveBeenCalledTimes(2));
  rerender({ defaultTone: "elegant", audience: profile.audience });
  expect(result.current.tone).toBe("punchy");
  expect(client.getQueryCache().getAll()).toHaveLength(2);
  expect(mockedGenerateCaption.mock.calls.map((call) => call[1].tone)).toEqual([
    "warm",
    "punchy",
  ]);

  rerender({ defaultTone: "elegant", audience: "tourists" });
  await waitFor(() => expect(mockedGenerateCaption).toHaveBeenCalledTimes(3));
});

it("forwards React Query cancellation and aborts an unmounted caption request", async () => {
  let forwardedSignal: AbortSignal | undefined;
  mockedGenerateCaption.mockImplementation((_id, _request, signal) => {
    forwardedSignal = signal;
    return new Promise<string>(() => undefined);
  });
  const client = makeClient();
  const { unmount } = renderHook(
    () =>
      useSuggestionCaption({
        businessId: 42,
        business,
        suggestion,
        locale: "en",
        creativeProfile: profile,
        enabled: true,
        priority: true,
      }),
    { wrapper: testWrapper(client) },
  );

  await waitFor(() => expect(forwardedSignal).toBeDefined());
  expect(forwardedSignal?.aborted).toBe(false);
  unmount();
  expect(forwardedSignal?.aborted).toBe(true);
});
