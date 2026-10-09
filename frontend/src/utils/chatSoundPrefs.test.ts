/** @jest-environment jsdom */
import { getChatSoundPrefs, saveChatSoundPrefs } from "@/utils/chatSoundPrefs";

describe("chatSoundPrefs", () => {
  test("defaults: announcements on, messages off", () => {
    localStorage.clear();
    expect(getChatSoundPrefs()).toEqual({
      announcementSound: true,
      messageSound: false,
    });
  });

  test("round-trips a saved override", () => {
    localStorage.clear();
    saveChatSoundPrefs({ messageSound: true });
    expect(getChatSoundPrefs().messageSound).toBe(true);
    expect(getChatSoundPrefs().announcementSound).toBe(true);
  });

  test("corrupted storage falls back to defaults", () => {
    localStorage.setItem("payverge_chat_sound_prefs", "{nope");
    expect(getChatSoundPrefs()).toEqual({
      announcementSound: true,
      messageSound: false,
    });
  });
});
