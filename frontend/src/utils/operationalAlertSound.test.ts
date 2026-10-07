import {
  createOperationalAlertSoundEngine,
  getLocalAlertSoundOverrides,
  saveLocalAlertSoundOverrides,
} from "./operationalAlertSound";

const originalAudioContext = (globalThis as any).AudioContext;
const originalWebkitAudioContext = (globalThis as any).webkitAudioContext;
const originalLocalStorage = (globalThis as any).localStorage;

const installMockLocalStorage = () => {
  const storage = new Map<string, string>();
  Object.defineProperty(globalThis, "localStorage", {
    configurable: true,
    value: {
      getItem: jest.fn((key: string) => storage.get(key) ?? null),
      setItem: jest.fn((key: string, value: string) => {
        storage.set(key, value);
      }),
      removeItem: jest.fn((key: string) => {
        storage.delete(key);
      }),
      clear: jest.fn(() => {
        storage.clear();
      }),
    },
  });
};

const installMockAudioContext = () => {
  const contexts: any[] = [];

  class MockAudioContext {
    currentTime = 0;
    destination = {};
    gainNodes: any[] = [];
    oscillators: any[] = [];
    resume = jest.fn().mockResolvedValue(undefined);

    constructor() {
      contexts.push(this);
    }

    createGain() {
      const gainNode = {
        connect: jest.fn(),
        gain: {
          setValueAtTime: jest.fn(),
          linearRampToValueAtTime: jest.fn(),
          exponentialRampToValueAtTime: jest.fn(),
        },
      };
      this.gainNodes.push(gainNode);
      return gainNode;
    }

    createOscillator() {
      const oscillator = {
        connect: jest.fn(),
        frequency: {
          setValueAtTime: jest.fn(),
          exponentialRampToValueAtTime: jest.fn(),
        },
        start: jest.fn(),
        stop: jest.fn(),
      };
      this.oscillators.push(oscillator);
      return oscillator;
    }
  }

  Object.defineProperty(globalThis, "AudioContext", {
    configurable: true,
    value: MockAudioContext,
  });
  delete (globalThis as any).webkitAudioContext;

  return contexts;
};

beforeEach(() => {
  installMockLocalStorage();
  delete (globalThis as any).AudioContext;
  delete (globalThis as any).webkitAudioContext;
});

afterEach(() => {
  jest.useRealTimers();
  jest.clearAllMocks();
  if (originalAudioContext) {
    Object.defineProperty(globalThis, "AudioContext", {
      configurable: true,
      value: originalAudioContext,
    });
  } else {
    delete (globalThis as any).AudioContext;
  }
  if (originalWebkitAudioContext) {
    Object.defineProperty(globalThis, "webkitAudioContext", {
      configurable: true,
      value: originalWebkitAudioContext,
    });
  } else {
    delete (globalThis as any).webkitAudioContext;
  }
  if (originalLocalStorage) {
    Object.defineProperty(globalThis, "localStorage", {
      configurable: true,
      value: originalLocalStorage,
    });
  } else {
    delete (globalThis as any).localStorage;
  }
});

describe("operational alert sound overrides", () => {
  it("persists localStorage overrides with merges", () => {
    expect(getLocalAlertSoundOverrides()).toEqual({
      muted: false,
      volume: null,
    });

    saveLocalAlertSoundOverrides({ muted: true, volume: 0.8 });
    expect(getLocalAlertSoundOverrides()).toEqual({
      muted: true,
      volume: 0.8,
    });

    saveLocalAlertSoundOverrides({ volume: null });
    expect(getLocalAlertSoundOverrides()).toEqual({
      muted: true,
      volume: null,
    });
  });
});

describe("operational alert sound engine", () => {
  it("plays one-shot alerts without entering repeating state", async () => {
    const contexts = installMockAudioContext();
    const engine = createOperationalAlertSoundEngine();

    await engine.playOnce({
      alertTypes: ["payment_received"],
      volume: 0.7,
    });

    expect(engine.isRepeating()).toBe(false);
    expect(engine.getState()).toEqual({
      unlocked: true,
      blocked: false,
      repeating: false,
    });
    expect(contexts[0].resume).toHaveBeenCalled();
    expect(contexts[0].oscillators).toHaveLength(2);
  });

  it("keeps repeating until stopped", async () => {
    jest.useFakeTimers();
    const contexts = installMockAudioContext();
    const engine = createOperationalAlertSoundEngine();

    await engine.startRepeating({
      alertTypes: ["order_new"],
      volume: 0.8,
      repeatIntervalSeconds: 30,
    });

    expect(engine.isRepeating()).toBe(true);
    expect(engine.getState()).toEqual({
      unlocked: true,
      blocked: false,
      repeating: true,
    });
    expect(contexts[0].resume).toHaveBeenCalled();

    jest.advanceTimersByTime(30_000);
    expect(engine.isRepeating()).toBe(true);

    engine.stopRepeating();
    expect(engine.isRepeating()).toBe(false);
  });

  it("gives service_call a distinct falling interval that outranks other types", async () => {
    const contexts = installMockAudioContext();
    const engine = createOperationalAlertSoundEngine();

    // A raised hand outranks everything — even mixed with the previous
    // highest-priority type, the service_call identity wins.
    await engine.playOnce({
      alertTypes: ["kitchen_order_ready", "service_call"],
      volume: 0.7,
    });

    const [first, second] = contexts[0].oscillators;
    expect(first.frequency.setValueAtTime).toHaveBeenCalledWith(
      980,
      expect.any(Number),
    );
    expect(second.frequency.setValueAtTime).toHaveBeenCalledWith(
      660,
      expect.any(Number),
    );
  });

  it("gives ai_takeover its own rising interval, distinct from other types", async () => {
    const contexts = installMockAudioContext();
    const engine = createOperationalAlertSoundEngine();

    // A guest waiting on a human outranks the kitchen chime, but yields to
    // service_call (which is matched first).
    await engine.playOnce({
      alertTypes: ["kitchen_order_ready", "ai_takeover"],
      volume: 0.7,
    });

    const [first, second] = contexts[0].oscillators;
    expect(first.frequency.setValueAtTime).toHaveBeenCalledWith(
      840,
      expect.any(Number),
    );
    expect(second.frequency.setValueAtTime).toHaveBeenCalledWith(
      1050,
      expect.any(Number),
    );
  });

  it("reports blocked when AudioContext constructors are missing", async () => {
    const engine = createOperationalAlertSoundEngine();

    await expect(engine.unlock()).resolves.toEqual({
      unlocked: false,
      blocked: true,
      repeating: false,
    });
  });

  it("playOnce with emphasize schedules a second pattern", async () => {
    jest.useFakeTimers();
    const contexts = installMockAudioContext();
    const engine = createOperationalAlertSoundEngine();

    await engine.playOnce({
      alertTypes: ["reservation_approval"],
      volume: 0.7,
      emphasize: true,
    });

    // First pattern plays immediately (2 oscillators).
    expect(contexts[0].oscillators).toHaveLength(2);

    jest.advanceTimersByTime(350);

    // Second pattern plays after the 350ms emphasize delay (4 total).
    expect(contexts[0].oscillators).toHaveLength(4);
  });

  it("playOnce without emphasize does not schedule a second pattern", async () => {
    jest.useFakeTimers();
    const contexts = installMockAudioContext();
    const engine = createOperationalAlertSoundEngine();

    await engine.playOnce({
      alertTypes: ["reservation_approval"],
      volume: 0.7,
    });

    expect(contexts[0].oscillators).toHaveLength(2);

    jest.advanceTimersByTime(350);

    expect(contexts[0].oscillators).toHaveLength(2);
  });
});
