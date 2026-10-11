import ar from "../guest-messages/ar.json";
import da from "../guest-messages/da.json";
import de from "../guest-messages/de.json";
import en from "../guest-messages/en.json";
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
  ar, da, de, en, es, "es-AR": esAR, fr, hi, it: itMessages, ja, ko, nl, no, pl, pt, ru, sv, th, tr, vi, zh,
};

const REQUIRED = [
  "paymentProcessor.connectedWallet",
  "paymentProcessor.network",
  "crossChain.usdcOnBase",
  "bill.transactionId",
];

function get(obj: unknown, path: string): unknown {
  return path.split(".").reduce<unknown>((acc, part) => {
    if (acc && typeof acc === "object") {
      return (acc as Record<string, unknown>)[part];
    }
    return undefined;
  }, obj);
}

describe("guest payment i18n keys", () => {
  for (const [locale, messages] of Object.entries(LOCALES)) {
    for (const key of REQUIRED) {
      it(`${locale} defines ${key}`, () => {
        expect(typeof get(messages, key)).toBe("string");
      });
    }
  }
});
