const STORAGE_KEY = "payverge_chat_sound_prefs";

export interface ChatSoundPrefs {
  /** Manager broadcasts — rare and important. Default ON. */
  announcementSound: boolean;
  /** Regular channel/DM traffic — noisy during service. Default OFF. */
  messageSound: boolean;
}

const DEFAULTS: ChatSoundPrefs = {
  announcementSound: true,
  messageSound: false,
};

const getStorage = (): Storage | null => {
  try {
    return globalThis.localStorage ?? null;
  } catch {
    return null;
  }
};

const normalizePrefs = (prefs: Partial<ChatSoundPrefs>): ChatSoundPrefs => ({
  announcementSound:
    typeof prefs.announcementSound === "boolean"
      ? prefs.announcementSound
      : DEFAULTS.announcementSound,
  messageSound:
    typeof prefs.messageSound === "boolean"
      ? prefs.messageSound
      : DEFAULTS.messageSound,
});

export function getChatSoundPrefs(): ChatSoundPrefs {
  const storage = getStorage();
  if (!storage) return { ...DEFAULTS };

  try {
    const stored = storage.getItem(STORAGE_KEY);
    if (!stored) return { ...DEFAULTS };
    return normalizePrefs({
      ...DEFAULTS,
      ...(JSON.parse(stored) as Partial<ChatSoundPrefs>),
    });
  } catch {
    return { ...DEFAULTS };
  }
}

export function saveChatSoundPrefs(next: Partial<ChatSoundPrefs>): void {
  const storage = getStorage();
  if (!storage) return;

  const merged = normalizePrefs({
    ...getChatSoundPrefs(),
    ...next,
  });
  storage.setItem(STORAGE_KEY, JSON.stringify(merged));
}
