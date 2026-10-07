/** @jest-environment jsdom */
/**
 * Issue #693 — "Creative settings drawer never loads; retry closes the drawer."
 *
 * The retry-closes-the-drawer and the 1024px overflow halves are covered in
 * MarketingSettingsDrawer.test.tsx. What was left is the recover path: the
 * settings query is mounted by the dashboard, not by the drawer, so once the
 * load failed the error card latched for the lifetime of the page. Closing and
 * reopening the drawer never re-drove the fetch, leaving creative settings
 * unreachable.
 */
import React from "react";
import { render, screen, waitFor } from "@testing-library/react";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import type { MarketingSettings } from "@/api/marketing";
import { getMarketingSettings } from "@/api/marketing";
import messages from "@/i18n/messages/en/marketingDashboard.json";
import { MarketingSettingsDrawer } from "./MarketingSettingsDrawer";
import { useMarketingSettings } from "./hooks/useMarketingSettings";

jest.mock("@/api/marketing", () => ({
  ...jest.requireActual("@/api/marketing"),
  getMarketingSettings: jest.fn(),
  updateMarketingSettings: jest.fn(),
}));

const mockGetSettings = getMarketingSettings as jest.MockedFunction<
  typeof getMarketingSettings
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

const drawerProps = {
  onClose: jest.fn(),
  settings: blankSettings,
  hasSnapshot: false,
  loading: false,
  canEdit: true,
  saving: false,
  loadError: "Request failed with status code 500",
  saveError: null,
  rollbackOccurred: false,
  saved: false,
  onSave: jest.fn(),
  onRetryLoad: jest.fn(),
  onRetrySave: jest.fn(),
  t,
};

beforeEach(() => {
  jest.clearAllMocks();
});

it("re-drives a failed settings load when the drawer is reopened", () => {
  const onRetryLoad = jest.fn();
  const { rerender } = render(
    <MarketingSettingsDrawer
      {...drawerProps}
      isOpen={false}
      onRetryLoad={onRetryLoad}
    />,
  );
  expect(onRetryLoad).not.toHaveBeenCalled();

  rerender(
    <MarketingSettingsDrawer
      {...drawerProps}
      isOpen
      onRetryLoad={onRetryLoad}
    />,
  );

  expect(onRetryLoad).toHaveBeenCalledTimes(1);
});

it("does not re-drive the load when the drawer already has a snapshot", () => {
  const onRetryLoad = jest.fn();
  const { rerender } = render(
    <MarketingSettingsDrawer
      {...drawerProps}
      hasSnapshot
      loadError={null}
      isOpen={false}
      onRetryLoad={onRetryLoad}
    />,
  );
  rerender(
    <MarketingSettingsDrawer
      {...drawerProps}
      hasSnapshot
      loadError={null}
      isOpen
      onRetryLoad={onRetryLoad}
    />,
  );
  expect(onRetryLoad).not.toHaveBeenCalled();
});

function Harness({ open }: { open: boolean }) {
  const {
    settings,
    hasSnapshot,
    loading,
    retryingLoad,
    saving,
    loadError,
    saveError,
    rollbackOccurred,
    saved,
    save,
    retryLoad,
    retrySave,
  } = useMarketingSettings(8, true);
  return (
    <MarketingSettingsDrawer
      isOpen={open}
      onClose={jest.fn()}
      settings={settings}
      hasSnapshot={hasSnapshot}
      loading={loading}
      canEdit
      saving={saving}
      loadError={loadError}
      saveError={saveError}
      rollbackOccurred={rollbackOccurred}
      saved={saved}
      retryingLoad={retryingLoad}
      onSave={save}
      onRetryLoad={retryLoad}
      onRetrySave={retrySave}
      t={t}
    />
  );
}

it("recovers creative settings on reopen after the first load failed", async () => {
  mockGetSettings
    .mockRejectedValueOnce(new Error("Request failed with status code 500"))
    .mockResolvedValue(blankSettings);

  const client = new QueryClient({
    defaultOptions: { queries: { retry: false }, mutations: { retry: false } },
  });
  const { rerender } = render(
    <QueryClientProvider client={client}>
      <Harness open />
    </QueryClientProvider>,
  );

  expect(
    await screen.findByRole("alert", { name: t("settings.loadError.title") }),
  ).toBeInTheDocument();
  expect(mockGetSettings).toHaveBeenCalledTimes(1);

  // Operator closes the drawer and opens it again.
  rerender(
    <QueryClientProvider client={client}>
      <Harness open={false} />
    </QueryClientProvider>,
  );
  rerender(
    <QueryClientProvider client={client}>
      <Harness open />
    </QueryClientProvider>,
  );

  await waitFor(() =>
    expect(
      screen.getByLabelText(t("settings.fields.audience.label")),
    ).toBeInTheDocument(),
  );
  expect(
    screen.queryByRole("alert", { name: t("settings.loadError.title") }),
  ).not.toBeInTheDocument();
  expect(mockGetSettings).toHaveBeenCalledTimes(2);
});
