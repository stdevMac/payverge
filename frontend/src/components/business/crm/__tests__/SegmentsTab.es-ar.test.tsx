/** @jest-environment jsdom */
import { render, screen } from "@testing-library/react";
import SegmentsTab from "../SegmentsTab";
import { getSegments } from "@/api/crm";

jest.mock("@/api/crm", () => ({
  getSegments: jest.fn(),
}));

jest.mock("@/i18n/SimpleTranslationProvider", () => ({
  useSimpleLocale: () => ({ locale: "es-ar" }),
  getTranslation: (key: string) => {
    const copy: Record<string, string> = {
      "businessDashboard.crm.segments.vip.title": "VIP",
      "businessDashboard.crm.segments.vip.description":
        "5+ visitas y gasto de por vida por encima del promedio",
      "businessDashboard.crm.segments.lapsed.title": "Inactivos",
      "businessDashboard.crm.segments.lapsed.description": "Sin visitas",
      "businessDashboard.crm.segments.new.title": "Nuevos",
      "businessDashboard.crm.segments.new.description": "Primera visita",
      "businessDashboard.crm.segments.atRisk.title": "En Riesgo",
      "businessDashboard.crm.segments.atRisk.description": "Visitas cayeron",
      "businessDashboard.crm.segments.viewCustomers": "Ver clientes",
    };
    return copy[key] ?? key;
  },
}));

const getSegmentsMock = getSegments as jest.Mock;

describe("SegmentsTab es-AR", () => {
  beforeEach(() => {
    getSegmentsMock.mockResolvedValue({ lapsed: 1, vip: 3, new: 2, atRisk: 1 });
  });

  it("labels VIP as above-average spend in es-AR, not a top 10% cut (#695)", async () => {
    render(<SegmentsTab businessId={1} onJumpToCustomers={jest.fn()} />);
    expect(await screen.findByText("VIP")).toBeInTheDocument();
    expect(screen.queryByText(/10%/)).not.toBeInTheDocument();
    expect(screen.queryByText(/mejores por gasto/i)).not.toBeInTheDocument();
    expect(
      screen.getByText(/gasto de por vida por encima del promedio/i),
    ).toBeInTheDocument();
  });
});
