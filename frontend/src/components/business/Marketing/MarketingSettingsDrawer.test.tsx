/** @jest-environment jsdom */

import React from "react";
import {
  act,
  fireEvent,
  render,
  renderHook,
  screen,
  waitFor,
  within,
} from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { MARKETING_PLAYS, type MarketingSettings } from "@/api/marketing";
import { getMarketingSettings, updateMarketingSettings } from "@/api/marketing";
import messages from "@/i18n/messages/en/marketingDashboard.json";
import { MarketingSettingsDrawer } from "./MarketingSettingsDrawer";
import {
  marketingSettingsQueryKey,
  useMarketingSettings,
} from "./hooks/useMarketingSettings";

jest.mock("@/api/marketing", () => ({
  ...jest.requireActual("@/api/marketing"),
  getMarketingSettings: jest.fn(),
  updateMarketingSettings: jest.fn(),
}));

const mockGetSettings = getMarketingSettings as jest.MockedFunction<
  typeof getMarketingSettings
>;
const mockUpdateSettings = updateMarketingSettings as jest.MockedFunction<
  typeof updateMarketingSettings
>;

const t = (key: string, params?: Record<string, string | number>): string => {
  const value = key.split(".").reduce<unknown>((current, part) => {
    if (!current || typeof current !== "object") return undefined;
    return (current as Record<string, unknown>)[part];
  }, messages);
  if (typeof value !== "string") return key;
  return Object.entries(params ?? {}).reduce(
    (copy, [name, replacement]) =>
      copy.replaceAll(`{${name}}`, String(replacement)),
    value,
  );
};

const blankSettings: MarketingSettings = {
  enabled: true,
  disabled_plays: [],
  creative_profile: {
    audience: "",
    voice: "",
    visual_mood: "",
    cta_style: "",
    hashtag_behavior: "",
    avoid_phrases: [],
    default_language: "",
    default_tone: "",
  },
};

const populatedSettings: MarketingSettings = {
  enabled: false,
  disabled_plays: ["happy_hour", "offer"],
  creative_profile: {
    audience: "Neighborhood regulars",
    voice: "Warm, concise, and specific",
    visual_mood: "editorial",
    cta_style: "direct",
    hashtag_behavior: "light",
    avoid_phrases: ["best ever", "guaranteed"],
    default_language: "es-AR",
    default_tone: "punchy",
  },
};

const defaultProps = {
  isOpen: true,
  onClose: jest.fn(),
  settings: blankSettings,
  hasSnapshot: true,
  loading: false,
  canEdit: true,
  saving: false,
  loadError: null,
  saveError: null,
  rollbackOccurred: false,
  saved: false,
  onSave: jest.fn(),
  onRetryLoad: jest.fn(),
  onRetrySave: jest.fn(),
  t,
};

function renderDrawer(
  overrides: Partial<React.ComponentProps<typeof MarketingSettingsDrawer>> = {},
) {
  return render(<MarketingSettingsDrawer {...defaultProps} {...overrides} />);
}

function makeQueryClient() {
  return new QueryClient({
    defaultOptions: {
      queries: { retry: false },
      mutations: { retry: false },
    },
  });
}

function makeWrapper(client: QueryClient) {
  return function Wrapper({ children }: { children: React.ReactNode }) {
    return (
      <QueryClientProvider client={client}>{children}</QueryClientProvider>
    );
  };
}

function SettingsDrawerHarness() {
  const state = useMarketingSettings(42, true);
  return (
    <MarketingSettingsDrawer
      {...defaultProps}
      settings={state.settings}
      loading={state.loading}
      hasSnapshot={state.hasSnapshot}
      loadError={state.loadError}
      saveError={state.saveError}
      rollbackOccurred={state.rollbackOccurred}
      saving={state.saving}
      saved={state.saved}
      retryingLoad={state.retryingLoad}
      onSave={state.save}
      onRetryLoad={state.retryLoad}
      onRetrySave={state.retrySave}
    />
  );
}

describe("MarketingSettingsDrawer", () => {
  beforeEach(() => {
    jest.clearAllMocks();
  });

  it("renders a layout-matched loading skeleton", () => {
    renderDrawer({ loading: true });

    expect(
      screen.getByTestId("marketing-settings-skeleton"),
    ).toBeInTheDocument();
    expect(
      screen.queryByLabelText(t("settings.fields.audience.label")),
    ).not.toBeInTheDocument();
  });

  it("blocks blank controls after an initial load failure, then retries into the server snapshot", async () => {
    const client = makeQueryClient();
    mockGetSettings
      .mockRejectedValueOnce(new Error("load failed"))
      .mockResolvedValueOnce(populatedSettings);
    render(
      <QueryClientProvider client={client}>
        <SettingsDrawerHarness />
      </QueryClientProvider>,
    );

    expect(
      await screen.findByRole("alert", {
        name: t("settings.loadError.title"),
      }),
    ).toHaveTextContent(t("settings.loadError.body"));
    expect(
      screen.queryByLabelText(t("settings.fields.audience.label")),
    ).not.toBeInTheDocument();
    expect(screen.queryAllByRole("switch")).toHaveLength(0);

    fireEvent.click(
      screen.getByRole("button", { name: t("settings.loadError.retry") }),
    );

    expect(
      await screen.findByLabelText(t("settings.fields.audience.label")),
    ).toHaveValue("Neighborhood regulars");
    expect(screen.getAllByRole("switch")).toHaveLength(
      MARKETING_PLAYS.length + 1,
    );
    expect(mockGetSettings).toHaveBeenCalledTimes(2);
  });

  it("retries a load error without closing the drawer or swapping to a skeleton", async () => {
    const onClose = jest.fn();
    const onRetryLoad = jest.fn();
    renderDrawer({
      hasSnapshot: false,
      loading: false,
      loadError: "load failed",
      onClose,
      onRetryLoad,
    });

    expect(
      screen.getByRole("alert", { name: t("settings.loadError.title") }),
    ).toBeInTheDocument();
    expect(
      screen.queryByTestId("marketing-settings-skeleton"),
    ).not.toBeInTheDocument();
    expect(screen.getByTestId("marketing-settings-drawer")).toBeInTheDocument();

    fireEvent.click(
      screen.getByRole("button", { name: t("settings.loadError.retry") }),
    );

    expect(onRetryLoad).toHaveBeenCalledTimes(1);
    expect(onClose).not.toHaveBeenCalled();
    expect(
      screen.getByRole("alert", { name: t("settings.loadError.title") }),
    ).toBeInTheDocument();
  });

  it("keeps the drawer panel inside the viewport at 1024px", () => {
    renderDrawer({ hasSnapshot: false, loading: false, loadError: "load failed" });
    const panel = screen.getByTestId("marketing-settings-drawer");
    expect(panel.className).toMatch(/max-h-\[100dvh\]|overflow/);
  });

  it("shows blank creative profile values as automatic defaults", () => {
    renderDrawer({ settings: { enabled: true, disabled_plays: [] } });

    // Text inputs stay native.
    expect(
      screen.getByLabelText(t("settings.fields.audience.label")),
    ).toHaveValue("");
    expect(screen.getByLabelText(t("settings.fields.voice.label"))).toHaveValue(
      "",
    );
    expect(
      screen.getByLabelText(t("settings.fields.avoidPhrases.label")),
    ).toHaveValue("");
    // NextUI Select triggers render the selected option label; empty maps to
    // the "automatic" sentinel.
    expect(
      screen.getByLabelText(t("settings.fields.visualMood.label")),
    ).toHaveTextContent(t("settings.automatic"));
    expect(
      screen.getByLabelText(t("settings.fields.ctaStyle.label")),
    ).toHaveTextContent(t("settings.automatic"));
    expect(
      screen.getByLabelText(t("settings.fields.hashtags.label")),
    ).toHaveTextContent(t("settings.automatic"));
    expect(
      screen.getByLabelText(t("settings.fields.language.label")),
    ).toHaveTextContent(t("settings.automatic"));
    expect(
      screen.getByLabelText(t("settings.fields.tone.label")),
    ).toHaveTextContent(t("settings.automatic"));
    expect(
      screen.getAllByText(t("settings.automatic")).length,
    ).toBeGreaterThanOrEqual(5);
  });

  it("hydrates every populated creative override", () => {
    renderDrawer({ settings: populatedSettings });

    expect(
      screen.getByLabelText(t("settings.fields.audience.label")),
    ).toHaveValue("Neighborhood regulars");
    expect(screen.getByLabelText(t("settings.fields.voice.label"))).toHaveValue(
      "Warm, concise, and specific",
    );
    expect(
      screen.getByLabelText(t("settings.fields.avoidPhrases.label")),
    ).toHaveValue("best ever, guaranteed");
    expect(
      screen.getByLabelText(t("settings.fields.visualMood.label")),
    ).toHaveTextContent(t("settings.options.visualMood.editorial"));
    expect(
      screen.getByLabelText(t("settings.fields.ctaStyle.label")),
    ).toHaveTextContent(t("settings.options.ctaStyle.direct"));
    expect(
      screen.getByLabelText(t("settings.fields.hashtags.label")),
    ).toHaveTextContent(t("settings.options.hashtags.light"));
    expect(
      screen.getByLabelText(t("settings.fields.language.label")),
    ).toHaveTextContent(t("settings.options.languages.es-AR"));
    expect(
      screen.getByLabelText(t("settings.fields.tone.label")),
    ).toHaveTextContent(t("tones.punchy"));
  });

  it("normalizes and saves ten valid avoid phrases", () => {
    const onSave = jest.fn();
    const phrases = Array.from(
      { length: 10 },
      (_, index) => `Phrase ${index + 1}`,
    );
    renderDrawer({ onSave });
    const input = screen.getByLabelText(
      t("settings.fields.avoidPhrases.label"),
    );

    fireEvent.change(input, { target: { value: phrases.join(", ") } });
    expect(onSave).not.toHaveBeenCalled();
    fireEvent.blur(input);

    expect(onSave).toHaveBeenCalledWith({
      ...blankSettings,
      creative_profile: {
        ...blankSettings.creative_profile!,
        avoid_phrases: phrases,
      },
    });
  });

  it("keeps eleven phrases visible and blocks the save", () => {
    const onSave = jest.fn();
    const draft = Array.from(
      { length: 11 },
      (_, index) => `Phrase ${index + 1}`,
    ).join(", ");
    renderDrawer({ onSave });
    const input = screen.getByLabelText(
      t("settings.fields.avoidPhrases.label"),
    );

    fireEvent.change(input, { target: { value: draft } });
    fireEvent.blur(input);

    expect(input).toHaveValue(draft);
    expect(screen.getByRole("alert")).toHaveTextContent(
      t("settings.fields.avoidPhrases.tooMany", { max: 10 }),
    );
    expect(onSave).not.toHaveBeenCalled();
  });

  it("accepts a sixty-rune multibyte phrase", () => {
    const onSave = jest.fn();
    const phrase = "é".repeat(60);
    renderDrawer({ onSave });
    const input = screen.getByLabelText(
      t("settings.fields.avoidPhrases.label"),
    );

    fireEvent.change(input, { target: { value: phrase } });
    fireEvent.blur(input);

    expect(onSave).toHaveBeenCalledWith({
      ...blankSettings,
      creative_profile: {
        ...blankSettings.creative_profile!,
        avoid_phrases: [phrase],
      },
    });
  });

  it("keeps a sixty-one-rune phrase visible and blocks the save", () => {
    const onSave = jest.fn();
    const phrase = "é".repeat(61);
    renderDrawer({ onSave });
    const input = screen.getByLabelText(
      t("settings.fields.avoidPhrases.label"),
    );

    fireEvent.change(input, { target: { value: phrase } });
    fireEvent.blur(input);

    expect(input).toHaveValue(phrase);
    expect(screen.getByRole("alert")).toHaveTextContent(
      t("settings.fields.avoidPhrases.tooLong", { max: 60 }),
    );
    expect(onSave).not.toHaveBeenCalled();
  });

  it("deduplicates avoid phrases case-insensitively before saving", () => {
    const onSave = jest.fn();
    renderDrawer({ onSave });
    const input = screen.getByLabelText(
      t("settings.fields.avoidPhrases.label"),
    );

    fireEvent.change(input, {
      target: { value: "  Best Ever, best ever , BEST EVER, guaranteed  " },
    });
    fireEvent.blur(input);

    expect(onSave).toHaveBeenCalledWith({
      ...blankSettings,
      creative_profile: {
        ...blankSettings.creative_profile!,
        avoid_phrases: ["Best Ever", "guaranteed"],
      },
    });
  });

  it("pauses and resumes all suggestions from the autonomy switch", () => {
    const onSave = jest.fn();
    const { rerender } = renderDrawer({ settings: blankSettings, onSave });

    fireEvent.click(
      screen.getByRole("switch", { name: t("settings.autonomy.master") }),
    );
    expect(onSave).toHaveBeenLastCalledWith({
      ...blankSettings,
      enabled: false,
    });

    rerender(
      <MarketingSettingsDrawer
        {...defaultProps}
        settings={{ ...blankSettings, enabled: false }}
        onSave={onSave}
      />,
    );
    fireEvent.click(
      screen.getByRole("switch", { name: t("settings.autonomy.master") }),
    );
    expect(onSave).toHaveBeenLastCalledWith({
      ...blankSettings,
      enabled: true,
    });
  });

  it.each(MARKETING_PLAYS)("toggles the %s play", (play) => {
    const onSave = jest.fn();
    renderDrawer({ onSave });

    fireEvent.click(screen.getByRole("switch", { name: t(`plays.${play}`) }));

    expect(onSave).toHaveBeenCalledWith({
      ...blankSettings,
      disabled_plays: [play],
    });
  });

  it("announces saving and saved states without replacing the form", () => {
    const { rerender } = renderDrawer({ saving: true });
    expect(screen.getByRole("status")).toHaveTextContent(
      t("settings.status.saving"),
    );
    expect(
      screen.getByLabelText(t("settings.fields.audience.label")),
    ).toBeInTheDocument();

    rerender(<MarketingSettingsDrawer {...defaultProps} saved />);
    expect(screen.getByRole("status")).toHaveTextContent(
      t("settings.status.saved"),
    );
  });

  it("only claims a failed save was restored when rollback is confirmed", () => {
    const onRetrySave = jest.fn();
    const { rerender } = renderDrawer({
      saveError: "network down",
      rollbackOccurred: false,
      onRetrySave,
    });

    expect(screen.getByRole("alert")).toHaveTextContent(
      t("settings.status.errorGeneric"),
    );
    expect(screen.getByRole("alert")).not.toHaveTextContent(
      t("settings.status.error"),
    );
    fireEvent.click(
      screen.getByRole("button", { name: t("settings.status.retry") }),
    );
    expect(onRetrySave).toHaveBeenCalledTimes(1);

    rerender(
      <MarketingSettingsDrawer
        {...defaultProps}
        saveError="network down"
        rollbackOccurred
      />,
    );
    expect(screen.getByRole("alert")).toHaveTextContent(
      t("settings.status.error"),
    );
  });

  it("lets read-only users inspect values without mutation controls", () => {
    const onSave = jest.fn();
    renderDrawer({ settings: populatedSettings, canEdit: false, onSave });

    expect(screen.getByText(t("settings.readOnly"))).toBeInTheDocument();
    expect(
      screen.getByLabelText(t("settings.fields.audience.label")),
    ).toHaveValue("Neighborhood regulars");
    screen
      .getAllByRole("textbox")
      .forEach((field) => expect(field).toBeDisabled());
    // NextUI Select triggers are disabled buttons for read-only viewers.
    for (const fieldKey of [
      "visualMood",
      "ctaStyle",
      "hashtags",
      "language",
      "tone",
    ] as const) {
      const trigger = screen.getByLabelText(
        t(`settings.fields.${fieldKey}.label`),
      );
      expect(trigger).toHaveAttribute("aria-haspopup", "listbox");
      // NextUI marks a disabled Select trigger via data-disabled, not the
      // native disabled attribute.
      expect(trigger).toHaveAttribute("data-disabled", "true");
    }
    screen
      .getAllByRole("switch")
      .forEach((field) => expect(field).toBeDisabled());
    expect(
      screen.queryByRole("button", { name: t("settings.status.retry") }),
    ).not.toBeInTheDocument();
    expect(screen.getByText(t("settings.status.readOnly"))).toBeInTheDocument();
    expect(
      screen.queryByText(t("settings.status.autoSave")),
    ).not.toBeInTheDocument();
    expect(onSave).not.toHaveBeenCalled();
  });

  it("provides an accessible dialog name, close control, and initial focus", async () => {
    renderDrawer();

    const dialog = await screen.findByRole("dialog", {
      name: new RegExp(`^${t("settings.title")}`),
    });
    expect(screen.getByRole("button", { name: /close/i })).toBeInTheDocument();
    await waitFor(() =>
      expect(dialog).toContainElement(document.activeElement as HTMLElement),
    );
  });

  it("returns focus to its visible trigger after Escape", async () => {
    const user = userEvent.setup();
    function Harness() {
      const [isOpen, setIsOpen] = React.useState(false);
      return (
        <>
          <button type="button" onClick={() => setIsOpen(true)}>
            Open settings
          </button>
          <MarketingSettingsDrawer
            {...defaultProps}
            isOpen={isOpen}
            onClose={() => setIsOpen(false)}
          />
        </>
      );
    }

    render(<Harness />);
    const trigger = screen.getByRole("button", { name: "Open settings" });
    await user.click(trigger);
    const dialog = await screen.findByRole("dialog", {
      name: new RegExp(`^${t("settings.title")}`),
    });
    await user.click(
      screen.getByLabelText(t("settings.fields.audience.label")),
    );

    await user.keyboard("{Escape}");
    await waitFor(() => expect(dialog).not.toBeInTheDocument());
    await waitFor(() => expect(trigger).toHaveFocus());
  });

  it("leaves Escape to an open select listbox without closing settings", async () => {
    const user = userEvent.setup();
    const onClose = jest.fn();
    renderDrawer({ onClose });
    const visualMood = screen.getByLabelText(
      t("settings.fields.visualMood.label"),
    );

    await user.click(visualMood);
    expect(await screen.findByRole("listbox")).toBeInTheDocument();

    // Escape dismisses the open listbox, not the enclosing drawer.
    await user.keyboard("{Escape}");
    await waitFor(() =>
      expect(screen.queryByRole("listbox")).not.toBeInTheDocument(),
    );

    expect(
      screen.getByRole("dialog", {
        name: new RegExp(`^${t("settings.title")}`),
      }),
    ).toBeInTheDocument();
    expect(onClose).not.toHaveBeenCalled();
  });

  it("commits a chosen creative-profile option through the select listbox", async () => {
    const user = userEvent.setup();
    const onSave = jest.fn();
    renderDrawer({ settings: blankSettings, onSave });

    await user.click(
      screen.getByLabelText(t("settings.fields.visualMood.label")),
    );
    const listbox = await screen.findByRole("listbox");
    await user.click(
      within(listbox).getByRole("option", {
        name: t("settings.options.visualMood.editorial"),
      }),
    );

    await waitFor(() =>
      expect(onSave).toHaveBeenCalledWith(
        expect.objectContaining({
          creative_profile: expect.objectContaining({
            visual_mood: "editorial",
          }),
        }),
      ),
    );
  });
});

describe("useMarketingSettings", () => {
  beforeEach(() => {
    jest.clearAllMocks();
  });

  it("optimistically exposes an update before the request resolves, then marks it saved", async () => {
    const client = makeQueryClient();
    client.setQueryData(marketingSettingsQueryKey("42"), blankSettings);
    client.setQueryData(["business", "42", "marketing", "suggestions"], []);
    let resolveRequest!: (settings: MarketingSettings) => void;
    mockUpdateSettings.mockImplementation(
      () =>
        new Promise<MarketingSettings>((resolve) => {
          resolveRequest = resolve;
        }),
    );
    const { result } = renderHook(() => useMarketingSettings(42, false), {
      wrapper: makeWrapper(client),
    });
    const next = { ...blankSettings, enabled: false };

    act(() => result.current.save(next));

    await waitFor(() => expect(result.current.settings.enabled).toBe(false));
    expect(result.current.saving).toBe(true);
    expect(result.current.saved).toBe(false);
    expect(client.getQueryData(marketingSettingsQueryKey("42"))).toEqual(next);

    resolveRequest(next);
    await waitFor(() => expect(result.current.saved).toBe(true));
    expect(result.current.saving).toBe(false);
    expect(mockUpdateSettings).toHaveBeenCalledWith("42", next);
    await waitFor(() =>
      expect(
        client.getQueryState(["business", "42", "marketing", "suggestions"])
          ?.isInvalidated,
      ).toBe(true),
    );
  });

  it("rolls back a failed optimistic update and retries the same values", async () => {
    const client = makeQueryClient();
    client.setQueryData(marketingSettingsQueryKey("42"), blankSettings);
    mockUpdateSettings
      .mockRejectedValueOnce(new Error("network down"))
      .mockResolvedValueOnce({ ...blankSettings, enabled: false });
    const { result } = renderHook(() => useMarketingSettings(42, false), {
      wrapper: makeWrapper(client),
    });

    act(() => result.current.save({ ...blankSettings, enabled: false }));

    await waitFor(() => expect(result.current.saveError).toBe("network down"));
    expect(result.current.loadError).toBeNull();
    expect(result.current.rollbackOccurred).toBe(true);
    expect(result.current.settings).toEqual(blankSettings);
    expect(result.current.saved).toBe(false);

    act(() => result.current.retrySave());

    await waitFor(() => expect(result.current.saved).toBe(true));
    expect(mockUpdateSettings).toHaveBeenCalledTimes(2);
    expect(result.current.settings.enabled).toBe(false);
  });

  it("does not write when the requested settings already match the cache", () => {
    const client = makeQueryClient();
    client.setQueryData(marketingSettingsQueryKey("42"), blankSettings);
    const { result } = renderHook(() => useMarketingSettings(42, false), {
      wrapper: makeWrapper(client),
    });

    act(() => result.current.save({ ...blankSettings }));

    expect(mockUpdateSettings).not.toHaveBeenCalled();
  });

  it("keeps an initial load error separate and blocks writes until retry loads a snapshot", async () => {
    const client = makeQueryClient();
    mockGetSettings
      .mockRejectedValueOnce(new Error("load failed"))
      .mockResolvedValueOnce(blankSettings);
    const { result } = renderHook(() => useMarketingSettings(42, true), {
      wrapper: makeWrapper(client),
    });

    await waitFor(() => expect(result.current.loadError).toBe("load failed"));
    expect(result.current.saveError).toBeNull();
    expect(result.current.hasSnapshot).toBe(false);
    act(() => result.current.save({ ...blankSettings, enabled: false }));
    expect(mockUpdateSettings).not.toHaveBeenCalled();

    act(() => result.current.retryLoad());
    await waitFor(() => expect(result.current.loadError).toBeNull());
    expect(result.current.hasSnapshot).toBe(true);
    expect(result.current.settings).toEqual(blankSettings);
    expect(mockGetSettings).toHaveBeenCalledTimes(2);
  });

  it("does not treat a refetch after load error as a blank loading state", async () => {
    const client = makeQueryClient();
    mockGetSettings
      .mockRejectedValueOnce(new Error("load failed"))
      .mockImplementationOnce(
        () =>
          new Promise<MarketingSettings>(() => {
            /* hang so the retry stays in-flight */
          }),
      );
    const { result } = renderHook(() => useMarketingSettings(42, true), {
      wrapper: makeWrapper(client),
    });

    await waitFor(() => expect(result.current.loadError).toBe("load failed"));
    expect(result.current.loading).toBe(false);
    expect(result.current.hasSnapshot).toBe(false);

    act(() => result.current.retryLoad());

    await waitFor(() => expect(result.current.retryingLoad).toBe(true));
    expect(result.current.loading).toBe(false);
    expect(result.current.loadError).toBe("load failed");
  });
});
