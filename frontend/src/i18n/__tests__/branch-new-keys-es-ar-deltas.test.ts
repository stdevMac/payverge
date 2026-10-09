import { getTranslation } from "../SimpleTranslationProvider";

// Operator copy added on this branch shipped en + es only. es-AR is a delta
// layer, so a new tuteo string with no Argentine override silently reaches the
// primary market in Peninsular Spanish. These pin the specific keys the review
// called out, plus the register consistency of the es source they layer over.
//
// The global voseo gate (operator-es-ar-voseo.test.ts) catches present-
// indicative leaks like "puedes"; it cannot see a mid-sentence tú imperative
// ("… o elige una imagen"), which is why image_not_hosted needs its own pin.

const es = (key: string) => getTranslation(key, "es") as string;
const ar = (key: string) => getTranslation(key, "es-AR") as string;

describe("es-AR deltas for the operator copy this branch added", () => {
  describe("aiWaiterDashboard.monitor.servicePausedBanner", () => {
    const KEY = "aiWaiterDashboard.monitor.servicePausedBanner";

    it("es keeps ONE register — tú throughout, no usted subjunctive", () => {
      const value = es(KEY);
      expect(value).toContain("Puedes");
      // "hasta que reactive el servicio" is usted; with "Puedes" it mixes two
      // registers in one sentence.
      expect(value).toContain("reactives");
      expect(value).not.toMatch(/hasta que reactive\b/);
    });

    it("es-AR says it in voseo", () => {
      const value = ar(KEY);
      expect(value).not.toBe(es(KEY));
      expect(value).toContain("Podés");
      expect(value).not.toMatch(/\bPuedes\b/);
      // Subjunctive stays the tuteo form in Rioplatense.
      expect(value).toContain("reactives");
    });
  });

  describe("marketingDashboard.errors.image_not_hosted", () => {
    const KEY = "marketingDashboard.errors.image_not_hosted";

    it("has an Argentine override like its sibling error keys", () => {
      expect(ar(KEY)).not.toBe(es(KEY));
    });

    it("uses voseo imperatives, not the Peninsular tú forms", () => {
      const value = ar(KEY);
      expect(value).toContain("Subila");
      expect(value).toContain("elegí");
      expect(value).not.toMatch(/\bSúbela\b/);
      expect(value).not.toMatch(/\belige\b/);
    });

    it("keeps the meaning of the en source", () => {
      // Still names the gallery and the two recovery paths.
      expect(ar(KEY)).toContain("galería");
      expect(ar(KEY)).toContain("mejorar");
    });
  });

  it("sibling error keys already overridden stay overridden (no regression)", () => {
    for (const key of [
      "marketingDashboard.errors.caption_failed",
      "marketingDashboard.errors.image_failed",
      "aiWaiterDashboard.monitor.emptyBody",
    ]) {
      expect(ar(key)).not.toBe(es(key));
    }
  });
});
