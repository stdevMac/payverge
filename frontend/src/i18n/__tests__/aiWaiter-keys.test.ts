import en from "../guest-messages/en.json";
import ar from "../guest-messages/ar.json";
import da from "../guest-messages/da.json";
import de from "../guest-messages/de.json";
import es from "../guest-messages/es.json";
import esAR from "../guest-messages/es-AR.json";
import fr from "../guest-messages/fr.json";
import hi from "../guest-messages/hi.json";
import itMessages from "../guest-messages/it.json";
import ja from "../guest-messages/ja.json";
import ko from "../guest-messages/ko.json";
import nl from "../guest-messages/nl.json";
import no from "../guest-messages/no.json";
import pl from "../guest-messages/pl.json";
import pt from "../guest-messages/pt.json";
import ru from "../guest-messages/ru.json";
import sv from "../guest-messages/sv.json";
import th from "../guest-messages/th.json";
import tr from "../guest-messages/tr.json";
import vi from "../guest-messages/vi.json";
import zh from "../guest-messages/zh.json";

const LOCALES: Record<string, Record<string, unknown>> = {
  ar,
  da,
  de,
  en,
  es,
  "es-AR": esAR,
  fr,
  hi,
  it: itMessages,
  ja,
  ko,
  nl,
  no,
  pl,
  pt,
  ru,
  sv,
  th,
  tr,
  vi,
  zh,
};

const REQUIRED = [
  "aiWaiter.disclosure",
  "aiWaiter.identity",
  "aiWaiter.modeConcierge",
  "aiWaiter.modeAiWaiter",
  "aiWaiter.placeholder",
  "aiWaiter.cartPrompt",
  "aiWaiter.resetChat",
  "aiWaiter.addedToast",
  "aiWaiter.addedChat",
  "aiWaiter.bundleNotFound",
  "aiWaiter.itemNotFound",
  "aiWaiter.fallbackHelp",
  "aiWaiter.errUnderstand",
  "aiWaiter.errConnect",
  "aiWaiter.retrySend",
  "aiWaiter.charCount",
  "aiWaiter.rateLimited",
  "aiWaiter.tooLong",
  "aiWaiter.overBudget",
  "aiWaiter.newConversationNotice",
  "aiWaiter.allergenDisclaimer",
  "aiWaiter.conciergeOrderingOnly",
  "aiWaiter.orderingUnavailable",
  "aiWaiter.nudgeMessage",
  "aiWaiter.nudgeDismiss",
  "aiWaiter.jumpToLatest",
  "aiWaiter.responseReady",
  "aiWaiter.sourcesUsed",
  "aiWaiter.sourceUsed",
  "aiWaiter.sourceDetails",
  "aiWaiter.sourceDetail",
  "aiWaiter.sourceVerified",
  "aiWaiter.externalSource",
  "aiWaiter.actions",
  "aiWaiter.steps",
  "aiWaiter.relatedItems",
  "aiWaiter.availability",
  "aiWaiter.availabilityAvailable",
  "aiWaiter.availabilityUnavailable",
  "aiWaiter.availabilityUnknown",
  "aiWaiter.followUps",
  "aiWaiter.notices",
  "aiWaiter.notice",
  "aiWaiter.statusComplete",
  "aiWaiter.statusNeedsClarification",
  "aiWaiter.statusBlocked",
  "aiWaiter.statusDegraded",
  "aiWaiter.workflowProgress",
  "aiWaiter.actionUnavailable",
  "aiWaiter.renderError",
  "aiWaiter.sendingMessage",
];

// D6: the five locales that shipped English clones for the chat chrome, and
// the prose keys that must never be byte-identical to en again. charCount is
// excluded (locale-invariant "{count}/{max}" numeric shape by design);
// modeConcierge is excluded ("Concierge" is a legitimate loanword in the
// Nordic languages and Polish).
const CLONE_GUARDED_LOCALES = ["da", "no", "pl", "sv", "vi"] as const;
const CLONE_GUARDED_KEYS = [
  "aiWaiter.addedChat",
  "aiWaiter.addedToast",
  "aiWaiter.bundleNotFound",
  "aiWaiter.cartPrompt",
  "aiWaiter.closeAssistant",
  "aiWaiter.errConnect",
  "aiWaiter.errUnderstand",
  "aiWaiter.fallbackHelp",
  "aiWaiter.inputLabel",
  "aiWaiter.itemNotFound",
  "aiWaiter.messagesRegionLabel",
  "aiWaiter.modeAiWaiter",
  "aiWaiter.openAssistant",
  "aiWaiter.resetChat",
  "aiWaiter.sendMessage",
  "aiWaiter.typingIndicator",
  "aiWaiter.jumpToLatest",
  "aiWaiter.responseReady",
  "aiWaiter.sourcesUsed",
  "aiWaiter.sourceUsed",
  "aiWaiter.sourceDetails",
  "aiWaiter.sourceDetail",
  "aiWaiter.sourceVerified",
  "aiWaiter.externalSource",
  "aiWaiter.actions",
  "aiWaiter.steps",
  "aiWaiter.relatedItems",
  "aiWaiter.availability",
  "aiWaiter.availabilityAvailable",
  "aiWaiter.availabilityUnavailable",
  "aiWaiter.availabilityUnknown",
  "aiWaiter.followUps",
  "aiWaiter.notices",
  "aiWaiter.notice",
  "aiWaiter.statusComplete",
  "aiWaiter.statusNeedsClarification",
  "aiWaiter.statusBlocked",
  "aiWaiter.statusDegraded",
  "aiWaiter.workflowProgress",
  "aiWaiter.actionUnavailable",
  "aiWaiter.renderError",
  "aiWaiter.sendingMessage",
];

function get(obj: Record<string, unknown>, path: string): unknown {
  return path
    .split(".")
    .reduce<unknown>(
      (acc, k) =>
        acc && typeof acc === "object"
          ? (acc as Record<string, unknown>)[k]
          : undefined,
      obj,
    );
}

describe("aiWaiter guest keys — all 21 locales", () => {
  for (const locale of Object.keys(LOCALES)) {
    it(`${locale} defines every required aiWaiter string key`, () => {
      for (const k of REQUIRED) {
        expect(typeof get(LOCALES[locale], k)).toBe("string");
      }
    });

    it(`${locale} defines a starterQuestions array of >= 4 entries`, () => {
      const sq = get(LOCALES[locale], "aiWaiter.starterQuestions");
      expect(Array.isArray(sq)).toBe(true);
      expect((sq as unknown[]).length).toBeGreaterThanOrEqual(4);
    });
  }
});

describe("aiWaiter English-clone guard (D6)", () => {
  for (const locale of CLONE_GUARDED_LOCALES) {
    it(`${locale} does not ship byte-identical English for user-facing chat chrome`, () => {
      const clones = CLONE_GUARDED_KEYS.filter(
        (k) => get(LOCALES[locale], k) === get(LOCALES.en, k),
      );
      expect(clones).toEqual([]);
    });
  }
});
