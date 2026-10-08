import { readFileSync } from "fs";
import { join } from "path";

// WS3/WS5: the Reservations first-load must render the purpose-built
// ReservationsSkeleton (previously orphaned, zero importers), not a bare
// <Spinner/> in a Card.
describe("ReservationManager first-load skeleton", () => {
  const source = readFileSync(
    join(__dirname, "..", "ReservationManager.tsx"),
    "utf8",
  );

  it("imports ReservationsSkeleton", () => {
    expect(source).toContain("ReservationsSkeleton");
    expect(source).toMatch(/import\s+\{\s*ReservationsSkeleton\s*\}\s+from/);
  });

  it("renders the skeleton on loading instead of a bare Spinner Card", () => {
    // The first-load ternary must render <ReservationsSkeleton /> directly,
    // not a <Card><CardBody ...><Spinner /></CardBody></Card> wrapper. The
    // ternary keys on the active view's first-load state (not the global
    // `loading`, which toggles on every background refresh).
    expect(source).toMatch(
      /\{shouldShowReservationsSkeleton\(activeSourceLoaded\) \? \(\s*<ReservationsSkeleton \/>/,
    );
    expect(source).not.toMatch(
      /\{loading \? \(\s*<Card>\s*<CardBody className="flex justify-center py-12">/,
    );
  });

  // F13: the insight KPI cards ("Covers Today", etc.) must not flash a
  // misleading 0 before the first complete-reservations load resolves. A
  // dedicated hasLoadedReservations flag (distinct from `loading`, which
  // toggles on every refresh) gates the card value behind a placeholder.
  it("gates insight KPI values behind a first-load flag so they don't flash 0", () => {
    expect(source).toMatch(
      /const \[hasLoadedReservations, setHasLoadedReservations\] = useState\(false\)/,
    );
    // Flag is flipped true once the complete-reservations load settles.
    expect(source).toMatch(/setHasLoadedReservations\(true\)/);
    // The card render shows a placeholder ("—") until the flag is true.
    expect(source).toMatch(/!hasLoadedReservations \? \(/);
  });
});
