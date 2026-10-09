import { directoryCopy } from "../VenueDirectory";

describe("directoryCopy", () => {
  it("serves neutral tú to generic Spanish and voseo only to es-AR (R2-2)", () => {
    expect(directoryCopy("es").heading).toBe("Elige un local");
    expect(directoryCopy("es-AR").heading).toBe("Elegí un local");
    expect(directoryCopy("es-ar").heading).toBe("Elegí un local");
    expect(directoryCopy("es-MX").heading).toBe("Elige un local");
  });

  it("falls back to English", () => {
    expect(directoryCopy("fr").heading).toBe("Choose a location");
  });
});
