'use client';

import { useContext, useEffect, useState } from 'react';
import { axiosInstance } from '@/api/tools/instance';
import {
  parseInstanceInfo,
  productNameOf,
  readInstanceSeed,
  type InstanceFeatures,
  type InstanceInfo,
} from '@/lib/instance/instanceInfo';
import { InstanceContext } from '@/lib/instance/instanceContext';

/**
 * One source of truth for GET /api/v1/instance in client components.
 *
 * The root layout seeds `window.__PAYVERGE_INSTANCE__` from its own server
 * fetch, so most pages start with the answer and never flash hidden UI. The
 * cache is module-level (not React Query) so it also works outside any
 * QueryClientProvider; the payload only changes on a backend restart.
 */

let cached: InstanceInfo | null = null;
let seeded = false;
let inflight: Promise<InstanceInfo> | null = null;

/** The payload already known in this page session, if any. */
function peekInstance(): InstanceInfo | null {
  if (!cached && !seeded) {
    seeded = true;
    cached = readInstanceSeed();
  }
  return cached;
}

/** GET /instance once per page session; failures are not cached. */
function loadInstance(): Promise<InstanceInfo> {
  const known = peekInstance();
  if (known) return Promise.resolve(known);
  if (!inflight) {
    inflight = axiosInstance
      .get('/instance')
      .then((response) => {
        const info = parseInstanceInfo(response.data);
        if (!info) throw new Error('Invalid instance response');
        cached = info;
        return info;
      })
      .finally(() => {
        inflight = null;
      });
  }
  return inflight;
}

/** Test-only: forget the cached payload (and the window seed). */
export function resetInstanceCacheForTests(): void {
  cached = null;
  seeded = false;
  inflight = null;
}

/** Test-only: pretend the probe already answered. */
export function setInstanceForTests(info: InstanceInfo | null): void {
  cached = info;
  seeded = true;
  inflight = null;
}

export type FeatureName = keyof InstanceFeatures;

export interface UseInstanceResult {
  /** The payload, or null while loading / when the probe failed. */
  instance: InstanceInfo | null;
  loading: boolean;
  isError: boolean;
  /** Product name to display; the upstream default until known. */
  productName: string;
  /**
   * True only when the server confirmed the feature is OFF. Unknown (loading,
   * failed probe) is never "off", so a transient error cannot hide working UI
   * on a configured install; the backend still refuses what is missing.
   */
  isOff: (feature: FeatureName) => boolean;
  /** True only when the server confirmed the feature is ON. */
  isOn: (feature: FeatureName) => boolean;
}

/**
 * The payload this render can rely on: the module cache / window seed, else
 * the layout's server-fetched value from InstanceProvider. The provider is
 * what the SERVER render sees (no window there), so SSR and hydration agree.
 */
function knownInstance(fromServer: InstanceInfo | null): InstanceInfo | null {
  const known = peekInstance();
  if (known) return known;
  if (fromServer && typeof window !== 'undefined') cached = fromServer;
  return fromServer;
}

export function useInstance(): UseInstanceResult {
  const fromServer = useContext(InstanceContext);
  const [state, setState] = useState<{
    data: InstanceInfo | null;
    loading: boolean;
    isError: boolean;
  }>(() => {
    const known = knownInstance(fromServer);
    return { data: known, loading: known === null, isError: false };
  });

  useEffect(() => {
    const known = knownInstance(fromServer);
    if (known) {
      setState((prev) =>
        prev.data === known ? prev : { data: known, loading: false, isError: false },
      );
      return;
    }
    let alive = true;
    loadInstance().then(
      (data) => {
        if (alive) setState({ data, loading: false, isError: false });
      },
      () => {
        if (alive) setState({ data: null, loading: false, isError: true });
      },
    );
    return () => {
      alive = false;
    };
  }, [fromServer]);

  const data = state.data;
  return {
    instance: data,
    loading: state.loading,
    isError: state.isError,
    productName: productNameOf(data),
    isOff: (feature) => data !== null && data.features[feature] === false,
    isOn: (feature) => data !== null && data.features[feature] === true,
  };
}
