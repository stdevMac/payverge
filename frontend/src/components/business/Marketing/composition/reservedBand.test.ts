import { FORMATS } from "../formats/formats";
import { COMPOSITIONS } from "./compositions";
import { reservedBandFor } from "./reservedBand";

describe("reservedBandFor", () => {
  it("reports the band a bottom-anchored family reserves", () => {
    expect(reservedBandFor(COMPOSITIONS.photoBottomStack, FORMATS["4:5"])).toBe("bottom");
    expect(reservedBandFor(COMPOSITIONS.badgeHero, FORMATS["4:5"])).toBe("bottom");
    expect(reservedBandFor(COMPOSITIONS.cornerCard, FORMATS["4:5"])).toBe("bottom");
  });

  it("reports the band a top-anchored family reserves", () => {
    expect(reservedBandFor(COMPOSITIONS.photoTopStack, FORMATS["4:5"])).toBe("top");
  });

  it("reports a centred poster as centre", () => {
    expect(reservedBandFor(COMPOSITIONS.posterStack, FORMATS["4:5"])).toBe("center");
  });

  // splitPanel's photo panel stops at 0.58 and its text starts at 0.59, so the
  // type never sits over the photograph at all: nothing is reserved, and the
  // generator should be free to use the whole panel.
  it("reserves nothing when the type never lands on the photo", () => {
    expect(reservedBandFor(COMPOSITIONS.splitPanel, FORMATS["1:1"])).toBe("none");
    expect(reservedBandFor(COMPOSITIONS.splitPanel, FORMATS["4:5"])).toBe("none");
    expect(reservedBandFor(COMPOSITIONS.splitPanel, FORMATS["9:16"])).toBe("none");
  });

  it("answers for every composition at every aspect", () => {
    const allowed = ["top", "bottom", "center", "left", "right", "none"];
    (Object.keys(COMPOSITIONS) as Array<keyof typeof COMPOSITIONS>).forEach(
      (id) => {
        (["1:1", "4:5", "9:16"] as const).forEach((aspect) => {
          expect(allowed).toContain(reservedBandFor(COMPOSITIONS[id], FORMATS[aspect]));
        });
      },
    );
  });
});
