/** @jest-environment node */

// SEO-0.5: the token-bearing guest delivery pages must never be indexed.
// Both routes are client components, so the robots directive lives in their
// server layouts.
import { metadata as trackMetadata } from "../track/layout";
import { metadata as payMetadata } from "../pay/layout";

describe("guest delivery route metadata — noindex (SEO-0.5)", () => {
  it("track layout: noindex + keeps the no-referrer token-leak guard", () => {
    expect(trackMetadata.robots).toEqual(
      expect.objectContaining({ index: false, follow: false }),
    );
    expect(trackMetadata.referrer).toBe("no-referrer");
    expect(trackMetadata.title).toBe("Track your delivery | Payverge");
  });

  it("pay layout: noindex with a generic title", () => {
    expect(payMetadata.robots).toEqual(
      expect.objectContaining({ index: false, follow: false }),
    );
    expect(payMetadata.title).toBe("Delivery payment | Payverge");
  });
});
