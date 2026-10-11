import type OperationalAlertType from "@/api/operationalAlerts";

const STORAGE_KEY = "payverge_operational_alert_sound_overrides";

export interface LocalAlertSoundOverrides {
  muted: boolean;
  volume: number | null;
}

export interface RepeatingAlarmConfig {
  alertTypes: OperationalAlertType[];
  volume: number;
  repeatIntervalSeconds: number;
}

export interface OneShotAlarmConfig {
  alertTypes: OperationalAlertType[];
  volume: number;
  /** Urgent alerts play the two-beep pattern twice (350ms apart). */
  emphasize?: boolean;
}

const EMPHASIZE_REPEAT_DELAY_MS = 350;

export interface SoundEngineState {
  unlocked: boolean;
  blocked: boolean;
  repeating: boolean;
}

type GainNodeLike = {
  connect: (destination: unknown) => unknown;
  gain: {
    setValueAtTime: (value: number, startTime: number) => unknown;
    linearRampToValueAtTime?: (value: number, endTime: number) => unknown;
    exponentialRampToValueAtTime?: (value: number, endTime: number) => unknown;
  };
};

type OscillatorNodeLike = {
  connect: (destination: unknown) => unknown;
  frequency: {
    setValueAtTime: (value: number, startTime: number) => unknown;
    exponentialRampToValueAtTime?: (value: number, endTime: number) => unknown;
  };
  start: (when?: number) => unknown;
  stop: (when?: number) => unknown;
};

type AudioContextLike = {
  currentTime: number;
  destination: unknown;
  resume?: () => Promise<void>;
  createGain: () => GainNodeLike;
  createOscillator: () => OscillatorNodeLike;
};

type AudioContextConstructor = new () => AudioContextLike;

const DEFAULT_OVERRIDES: LocalAlertSoundOverrides = {
  muted: false,
  volume: null,
};

const clampVolume = (volume: number): number => {
  if (!Number.isFinite(volume)) return 0;
  return Math.min(1, Math.max(0, volume));
};

const getStorage = (): Storage | null => {
  try {
    return globalThis.localStorage ?? null;
  } catch {
    return null;
  }
};

const normalizeOverrides = (
  overrides: Partial<LocalAlertSoundOverrides>,
): LocalAlertSoundOverrides => ({
  muted:
    typeof overrides.muted === "boolean"
      ? overrides.muted
      : DEFAULT_OVERRIDES.muted,
  volume:
    overrides.volume === null
      ? null
      : typeof overrides.volume === "number"
        ? clampVolume(overrides.volume)
        : DEFAULT_OVERRIDES.volume,
});

export function getLocalAlertSoundOverrides(): LocalAlertSoundOverrides {
  const storage = getStorage();
  if (!storage) return { ...DEFAULT_OVERRIDES };

  try {
    const stored = storage.getItem(STORAGE_KEY);
    if (!stored) return { ...DEFAULT_OVERRIDES };
    return normalizeOverrides({
      ...DEFAULT_OVERRIDES,
      ...(JSON.parse(stored) as Partial<LocalAlertSoundOverrides>),
    });
  } catch {
    return { ...DEFAULT_OVERRIDES };
  }
}

export function saveLocalAlertSoundOverrides(
  overrides: Partial<LocalAlertSoundOverrides>,
): void {
  const storage = getStorage();
  if (!storage) return;

  const next = normalizeOverrides({
    ...getLocalAlertSoundOverrides(),
    ...overrides,
  });
  storage.setItem(STORAGE_KEY, JSON.stringify(next));
}

const getAudioContextConstructor = (): AudioContextConstructor | null => {
  const audioGlobal = globalThis as typeof globalThis & {
    AudioContext?: AudioContextConstructor;
    webkitAudioContext?: AudioContextConstructor;
  };

  return audioGlobal.AudioContext ?? audioGlobal.webkitAudioContext ?? null;
};

const alertFrequencies = (
  alertTypes: OperationalAlertType[],
): [number, number] => {
  if (alertTypes.includes("service_call")) {
    return [980, 660]; // falling interval, distinct from every other alert
  }
  if (alertTypes.includes("ai_takeover")) {
    return [840, 1050]; // rising interval — a guest is waiting on a human
  }
  if (alertTypes.includes("kitchen_order_ready")) {
    return [740, 980];
  }
  if (alertTypes.includes("delivery_new")) {
    return [520, 720];
  }
  if (alertTypes.includes("payment_received")) {
    return [880, 1120];
  }
  if (
    alertTypes.includes("reservation_new") ||
    alertTypes.includes("reservation_approval")
  ) {
    return [560, 760];
  }
  if (alertTypes.includes("bill_new")) {
    return [680, 860];
  }
  return [620, 820];
};

const beep = (
  context: AudioContextLike,
  startFrequency: number,
  endFrequency: number,
  volume: number,
  delaySeconds: number,
): void => {
  const oscillator = context.createOscillator();
  const gain = context.createGain();
  const start = context.currentTime + delaySeconds;
  const end = start + 0.18;
  const attackEnd = start + 0.025;
  const safeVolume = Math.max(0.0001, clampVolume(volume));

  oscillator.connect(gain);
  gain.connect(context.destination);

  oscillator.frequency.setValueAtTime(startFrequency, start);
  oscillator.frequency.exponentialRampToValueAtTime?.(
    Math.max(1, endFrequency),
    end,
  );

  gain.gain.setValueAtTime(0.0001, start);
  if (gain.gain.linearRampToValueAtTime) {
    gain.gain.linearRampToValueAtTime(safeVolume, attackEnd);
  } else {
    gain.gain.setValueAtTime(safeVolume, attackEnd);
  }
  gain.gain.exponentialRampToValueAtTime?.(0.0001, end);

  oscillator.start(start);
  oscillator.stop(end);
};

const playPattern = (
  context: AudioContextLike,
  config: RepeatingAlarmConfig,
): void => {
  const overrides = getLocalAlertSoundOverrides();
  if (overrides.muted) return;

  const volume = clampVolume(overrides.volume ?? config.volume);
  if (volume <= 0) return;

  const [firstFrequency, secondFrequency] = alertFrequencies(config.alertTypes);
  beep(context, firstFrequency, firstFrequency * 1.08, volume, 0);
  beep(context, secondFrequency, secondFrequency * 0.92, volume, 0.24);
};

export function createOperationalAlertSoundEngine() {
  let context: AudioContextLike | null = null;
  let intervalID: ReturnType<typeof setInterval> | null = null;
  let state: SoundEngineState = {
    unlocked: false,
    blocked: false,
    repeating: false,
  };

  const getState = (): SoundEngineState => ({ ...state });

  const unlock = async (): Promise<SoundEngineState> => {
    const AudioContextCtor = getAudioContextConstructor();
    if (!AudioContextCtor) {
      state = { ...state, unlocked: false, blocked: true };
      return getState();
    }

    try {
      context = context ?? new AudioContextCtor();
      await context.resume?.();
      state = { ...state, unlocked: true, blocked: false };
    } catch {
      state = { ...state, unlocked: false, blocked: true };
    }

    return getState();
  };

  const stopRepeating = (): void => {
    if (intervalID) {
      clearInterval(intervalID);
      intervalID = null;
    }
    state = { ...state, repeating: false };
  };

  const startRepeating = async (
    config: RepeatingAlarmConfig,
  ): Promise<void> => {
    if (intervalID) {
      clearInterval(intervalID);
      intervalID = null;
    }

    state = { ...state, repeating: true };
    const current = await unlock();
    if (current.unlocked && context) {
      playPattern(context, config);
    }

    const intervalSeconds = Math.max(30, config.repeatIntervalSeconds);
    intervalID = setInterval(() => {
      void unlock().then((updatedState) => {
        if (updatedState.unlocked && context && state.repeating) {
          playPattern(context, config);
        }
      });
    }, intervalSeconds * 1000);
  };

  const playOnce = async (config: OneShotAlarmConfig): Promise<void> => {
    const current = await unlock();
    if (current.unlocked && context) {
      playPattern(context, {
        ...config,
        repeatIntervalSeconds: 0,
      });

      if (config.emphasize) {
        const heldContext = context;
        setTimeout(() => {
          // Re-reads mute/volume overrides at fire time (via playPattern)
          // so the emphasized repeat honors any override change in between.
          playPattern(heldContext, {
            ...config,
            repeatIntervalSeconds: 0,
          });
        }, EMPHASIZE_REPEAT_DELAY_MS);
      }
    }
  };

  const isRepeating = (): boolean => state.repeating;

  return {
    unlock,
    playOnce,
    startRepeating,
    stopRepeating,
    isRepeating,
    getState,
  };
}
