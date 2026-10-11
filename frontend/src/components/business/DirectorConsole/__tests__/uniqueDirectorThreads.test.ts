import { uniqueDirectorThreads } from "../uniqueDirectorThreads";
import type { DirectorThread } from "@/api/directorConsole";

function thread(
  id: number,
  title: string,
  updatedAt: string,
): DirectorThread {
  return {
    id,
    title,
    created_at: updatedAt,
    updated_at: updatedAt,
  };
}

describe("uniqueDirectorThreads (#730)", () => {
  it("drops duplicate ids and keeps distinct ids that share a title", () => {
    const older = "2026-08-21T10:00:00.000Z";
    const newer = "2026-08-21T12:00:00.000Z";
    const got = uniqueDirectorThreads([
      thread(1, "what's our best seller", older),
      thread(2, "cierra la caja", older),
      thread(1, "what's our best seller", older),
      thread(3, "what's our best seller", newer),
      thread(4, "cierra la caja", newer),
      thread(5, "mandá un mozo", newer),
    ]);
    expect(got.map((t) => t.id)).toEqual([1, 2, 3, 4, 5]);
    expect(
      got.filter((t) => t.title === "what's our best seller").map((t) => t.id),
    ).toEqual([1, 3]);
  });

  it("keeps a second money-night conversation that reuses the same title", () => {
    const firstNight = "2026-08-20T22:00:00.000Z";
    const secondNight = "2026-08-21T22:00:00.000Z";
    const got = uniqueDirectorThreads([
      thread(10, "how much did we sell tonight", firstNight),
      thread(11, "how much did we sell tonight", secondNight),
    ]);
    expect(got.map((t) => t.id)).toEqual([10, 11]);
  });

  it("returns empty for missing input", () => {
    expect(uniqueDirectorThreads(undefined)).toEqual([]);
    expect(uniqueDirectorThreads([])).toEqual([]);
  });
});
