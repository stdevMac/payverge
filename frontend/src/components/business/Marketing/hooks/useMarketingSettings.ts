import * as React from "react";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import {
  getMarketingSettings,
  updateMarketingSettings,
  type MarketingCreativeProfile,
  type MarketingSettings,
} from "@/api/marketing";

export const marketingSettingsQueryKey = (businessId: string) =>
  ["business", businessId, "marketing", "settings"] as const;

const emptyCreativeProfile = (): MarketingCreativeProfile => ({
  audience: "",
  voice: "",
  visual_mood: "",
  cta_style: "",
  hashtag_behavior: "",
  avoid_phrases: [],
  default_language: "",
  default_tone: "",
});

const defaultMarketingSettings = (): MarketingSettings => ({
  enabled: true,
  disabled_plays: [],
  creative_profile: emptyCreativeProfile(),
});

export function normalizeMarketingSettings(
  settings?: MarketingSettings,
): MarketingSettings {
  const profile = settings?.creative_profile;
  return {
    enabled: settings?.enabled ?? true,
    disabled_plays: [...(settings?.disabled_plays ?? [])],
    creative_profile: {
      audience: profile?.audience ?? "",
      voice: profile?.voice ?? "",
      visual_mood: profile?.visual_mood ?? "",
      cta_style: profile?.cta_style ?? "",
      hashtag_behavior: profile?.hashtag_behavior ?? "",
      avoid_phrases: [...(profile?.avoid_phrases ?? [])],
      default_language: profile?.default_language ?? "",
      default_tone: profile?.default_tone ?? "",
    },
  };
}

function sameSettings(
  left: MarketingSettings,
  right: MarketingSettings,
): boolean {
  return (
    JSON.stringify(normalizeMarketingSettings(left)) ===
    JSON.stringify(normalizeMarketingSettings(right))
  );
}

function errorMessage(error: unknown): string | null {
  if (error instanceof Error) return error.message;
  return error ? String(error) : null;
}

interface SettingsMutationVariables {
  next: MarketingSettings;
  previous: MarketingSettings;
}

export function useMarketingSettings(
  businessId: number | string | undefined,
  enabled = true,
) {
  const id = businessId == null ? "" : String(businessId);
  const qc = useQueryClient();
  const key = marketingSettingsQueryKey(id);
  const [saved, setSaved] = React.useState(false);
  const lastFailed = React.useRef<MarketingSettings | null>(null);
  const [rollbackOccurred, setRollbackOccurred] = React.useState(false);
  const query = useQuery({
    queryKey: key,
    queryFn: () => getMarketingSettings(id),
    enabled: !!id && enabled,
    staleTime: 60_000,
  });
  const update = useMutation({
    mutationFn: ({ next }: SettingsMutationVariables) =>
      updateMarketingSettings(id, next),
    onMutate: async ({ next, previous }: SettingsMutationVariables) => {
      setSaved(false);
      setRollbackOccurred(false);
      lastFailed.current = null;
      await qc.cancelQueries({ queryKey: key });
      qc.setQueryData(key, normalizeMarketingSettings(next));
      return { previous };
    },
    onError: (_error, { next }, context) => {
      lastFailed.current = next;
      if (context?.previous) {
        qc.setQueryData(key, context.previous);
        setRollbackOccurred(true);
      }
    },
    onSuccess: (stored) => {
      lastFailed.current = null;
      setRollbackOccurred(false);
      qc.setQueryData(key, normalizeMarketingSettings(stored));
      setSaved(true);
      // Re-enabling/disabling or toggling a play changes the live feed.
      void qc.invalidateQueries({
        queryKey: ["business", id, "marketing", "suggestions"],
      });
    },
  });

  const save = React.useCallback(
    (next: MarketingSettings) => {
      if (!id) return;
      const normalizedNext = normalizeMarketingSettings(next);
      const current = qc.getQueryData<MarketingSettings>(key) ?? query.data;
      if (!current) return;
      const previous = normalizeMarketingSettings(current);
      if (sameSettings(previous, normalizedNext)) return;
      update.mutate({ next: normalizedNext, previous });
    },
    [id, key, qc, query.data, update],
  );

  // React Query hands back a fresh `query` object every render, so binding the
  // retry to it churned its identity on every render and made it unusable as
  // an effect dependency. Keep the callback stable and read the latest
  // refetch through a ref.
  const refetchRef = React.useRef(query.refetch);
  refetchRef.current = query.refetch;
  const retryLoad = React.useCallback(() => {
    void refetchRef.current();
  }, []);

  const retrySave = React.useCallback(() => {
    const next = lastFailed.current;
    const current = qc.getQueryData<MarketingSettings>(key) ?? query.data;
    if (!next || !current) return;
    update.mutate({
      next: normalizeMarketingSettings(next),
      previous: normalizeMarketingSettings(current),
    });
  }, [key, qc, query.data, update]);

  const hasSnapshot = query.data !== undefined;
  const [stickyLoadError, setStickyLoadError] = React.useState<string | null>(
    null,
  );
  React.useEffect(() => {
    if (query.error) {
      setStickyLoadError(errorMessage(query.error));
      return;
    }
    if (hasSnapshot) {
      setStickyLoadError(null);
    }
  }, [query.error, hasSnapshot]);
  const loadError = hasSnapshot ? null : stickyLoadError;
  const retryingLoad = Boolean(loadError) && !hasSnapshot && query.isFetching;

  return {
    settings: normalizeMarketingSettings(
      query.data ?? defaultMarketingSettings(),
    ),
    hasSnapshot,
    loading:
      (query.isLoading || (!hasSnapshot && query.isFetching)) && !loadError,
    retryingLoad,
    saving: update.isPending,
    loadError,
    saveError: errorMessage(update.error),
    rollbackOccurred,
    saved,
    save,
    retryLoad,
    retrySave,
  };
}
