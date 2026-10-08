import en from "../en/dashboard.json";
import es from "../es/dashboard.json";

// Issue #795: the dashboard sign-in gate advertised "0% crypto payment fees"
// while every payment plugin on the venue was disabled. The gate is pre-auth
// and cannot know per-business rails, so its pitch must be the platform truth,
// never a live-rail claim. On a self-hosted install there is no hosted
// platform taking a cut either, so the gate states what the software does
// (payments go straight to the operator) instead of a SaaS pricing pitch.
describe("dashboard auth gate does not advertise a dead crypto fee", () => {
  it.each([
    ["en", en, /straight to you/i],
    ["es", es, /directo a ti/i],
  ])("%s value prop states the payment truth", (_name, bundle, truth) => {
    const copy = JSON.stringify(
      (bundle as { authentication?: { valueProp?: unknown } }).authentication
        ?.valueProp ?? {},
    );
    expect(copy).not.toMatch(/crypto payment fees/i);
    expect(copy).not.toMatch(/comisiones cripto/i);
    // No hosted-SaaS pricing pitch on a self-hosted operator screen.
    expect(copy).not.toMatch(/0%/);
    expect(copy).not.toMatch(/commission|comisi[oó]n/i);
    expect(copy).toMatch(truth);
  });
});
