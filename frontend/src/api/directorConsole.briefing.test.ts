import { getDirectorBriefing, type BriefingResponse } from "@/api/directorConsole";
import { axiosInstance } from "@/api/tools/instance";

jest.mock("@/api/tools/instance", () => ({
  axiosInstance: {
    get: jest.fn(),
  },
}));

function fixture(): BriefingResponse {
  return {
    state: "active",
    pulse: {
      revenue: 4200,
      projected: 4700,
      typical_day: 4200,
      pace_pct: 12,
      orders: 84,
      avg_ticket: 37,
      food_cost_pct: 0.28,
      labor_cost_pct: 0.24,
      open_bills: 3,
    },
    insights: [],
    play: {
      kind: "promote",
      tab: "menu",
      item_name: "Carbonara",
      current_price: null,
      suggested_price: null,
      monthly_impact: 240,
    },
    win: null,
  };
}

describe("getDirectorBriefing", () => {
  beforeEach(() => {
    jest.clearAllMocks();
    (axiosInstance.get as jest.Mock).mockReset();
  });

  it("GETs the owner-gated briefing route for the business id", async () => {
    (axiosInstance.get as jest.Mock).mockResolvedValue({ data: fixture() });

    const res = await getDirectorBriefing(42);

    expect(axiosInstance.get).toHaveBeenCalledWith(
      "/inside/businesses/42/director-console/briefing",
    );
    expect(res.state).toBe("active");
    expect(res.pulse.orders).toBe(84);
    expect(res.play?.item_name).toBe("Carbonara");
  });

  it("passes the nullable pulse pointers through unchanged (no coercion of null)", async () => {
    const learning: BriefingResponse = {
      state: "learning",
      pulse: {
        revenue: 0,
        projected: null,
        typical_day: null,
        pace_pct: null,
        orders: 0,
        avg_ticket: 0,
        food_cost_pct: null,
        labor_cost_pct: null,
        open_bills: 0,
      },
      insights: [],
      play: null,
      win: null,
    };
    (axiosInstance.get as jest.Mock).mockResolvedValue({ data: learning });

    const res = await getDirectorBriefing(7);

    expect(res.pulse.pace_pct).toBeNull();
    expect(res.pulse.food_cost_pct).toBeNull();
    expect(res.play).toBeNull();
    expect(res.win).toBeNull();
  });
});
