/** @jest-environment jsdom */

import {
  OFFER_FOOTER_OVERLAP_1024x622,
  hitTestTopmost,
  pointInRect,
  resolvedPointerEvents,
} from "./offerModalHitTest";

describe("#743 offer footer hit-test", () => {
  it("treats the 1024×622 suggestions point as inside both rects", () => {
    const { point, suggestions, footer } = OFFER_FOOTER_OVERLAP_1024x622;
    expect(pointInRect(point.x, point.y, suggestions)).toBe(true);
    expect(pointInRect(point.x, point.y, footer)).toBe(true);
  });

  it("lets Show suggestions win when the footer chrome has pointer-events none", () => {
    const { point, suggestions, footer } = OFFER_FOOTER_OVERLAP_1024x622;
    const footerEl = document.createElement("footer");
    footerEl.style.pointerEvents = "none";
    document.body.appendChild(footerEl);
    const suggest = document.createElement("button");
    suggest.setAttribute("aria-label", "Show suggestions");
    document.body.appendChild(suggest);

    expect(
      hitTestTopmost(point.x, point.y, [
        { el: footerEl, rect: footer },
        { el: suggest, rect: suggestions },
      ]),
    ).toBe(suggest);
  });

  it("treats the Tailwind pointer-events-none class as none in jsdom", () => {
    const footerEl = document.createElement("footer");
    footerEl.className = "relative z-0 pointer-events-none";
    document.body.appendChild(footerEl);
    expect(resolvedPointerEvents(footerEl)).toBe("none");
  });
});
