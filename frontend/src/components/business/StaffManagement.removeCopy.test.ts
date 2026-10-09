/**
 * L5-24 Path A: staff soft-remove copy must not claim permanent destroy.
 * BE SoftDelete is reversible via re-invite; the prior es/en subtitle said
 * "cannot be undone" / "no se puede deshacer".
 */
import en from "@/i18n/messages/en/businessDashboard.json";
import es from "@/i18n/messages/es/businessDashboard.json";

type Dash = typeof en;

function removeModal(locale: Dash) {
  return locale.dashboard.staffManagement.removeModal;
}

describe("StaffManagement removeModal soft-delete copy (L5-24)", () => {
  it("en: does not claim irreversible destroy; states re-invite / preserve", () => {
    const m = removeModal(en);
    const blob = `${m.title} ${m.subtitle} ${m.description}`.toLowerCase();
    expect(blob).not.toMatch(/cannot be undone|can't be undone|permanent destroy/);
    expect(blob).toMatch(/re-invite|reactivat|soft|preserv/);
  });

  it("es: does not claim irreversible destroy; states re-invite / preserve", () => {
    const m = removeModal(es);
    const blob = `${m.title} ${m.subtitle} ${m.description}`.toLowerCase();
    expect(blob).not.toMatch(/no se puede deshacer|irreversible|destruye de forma permanente/);
    expect(blob).toMatch(/invitar|reactiv|suave|conserv/);
  });
});
