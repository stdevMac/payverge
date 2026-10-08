import {
  cashRegisterApi,
  type CashRegisterCurrentResponse,
  type CashRegisterMovement,
  type CashRegisterSession,
  type CashRegisterSessionsResponse,
  type CashRegisterUnassignedResponse,
} from "@/api/cashRegister";
import { axiosInstance } from "@/api/tools/instance";
import { asDollars } from "@/types/money";

jest.mock("@/api/tools/instance", () => ({
  axiosInstance: {
    get: jest.fn(),
    post: jest.fn(),
  },
}));

const openSession: CashRegisterSession = {
  id: 7,
  business_id: 42,
  status: "open",
  opening_float: asDollars(100),
  opening_note: "Morning shift",
  opened_by_user_id: 3,
  opened_by_staff_id: null,
  opened_by_label: "Owner",
  opened_at: "2026-06-27T12:00:00Z",
  cash_sales: asDollars(80),
  cash_refunds: asDollars(5),
  cash_in: asDollars(25),
  cash_out: asDollars(10),
  closing_note: "",
  closed_by_user_id: null,
  closed_by_staff_id: null,
  closed_by_label: "",
  closed_at: null,
  created_at: "2026-06-27T12:00:01Z",
  updated_at: "2026-06-27T12:00:01Z",
};

const closedSession: CashRegisterSession = {
  ...openSession,
  status: "closed",
  expected_cash: asDollars(190),
  counted_cash: asDollars(175.25),
  variance: asDollars(-14.75),
  closing_note: "Balanced",
  closed_by_user_id: 3,
  closed_by_staff_id: null,
  closed_by_label: "Owner",
  closed_at: "2026-06-27T20:00:00Z",
  updated_at: "2026-06-27T20:00:00Z",
};

const movement: CashRegisterMovement = {
  id: 11,
  business_id: 42,
  session_id: 7,
  movement_type: "cash_in",
  amount: asDollars(25),
  reason: "change_bank",
  note: "Added small bills",
  alternative_payment_id: null,
  bill_id: null,
  actor_user_id: 3,
  actor_staff_id: null,
  actor_label: "Owner",
  occurred_at: "2026-06-27T13:00:00Z",
  created_at: "2026-06-27T13:00:01Z",
};

describe("cashRegisterApi", () => {
  beforeEach(() => {
    jest.clearAllMocks();
    (axiosInstance.get as jest.Mock).mockReset();
    (axiosInstance.post as jest.Mock).mockReset();
  });

  it("fetches the current caja session", async () => {
    const data: CashRegisterCurrentResponse = {
      session: null,
      unassigned_cash_total: asDollars(12.5),
      unassigned_cash_count: 2,
      suggested_opening_float: asDollars(200),
    };
    (axiosInstance.get as jest.Mock).mockResolvedValue({ data });

    const result = await cashRegisterApi.getCurrent("42");

    expect(axiosInstance.get).toHaveBeenCalledWith("/inside/businesses/42/cash-register/current");
    expect(result).toBe(data);
  });

  it("opens a caja session with opening float", async () => {
    const payload = {
      opening_float: asDollars(100),
      opening_note: "Morning shift",
    };
    const data: CashRegisterSession = openSession;
    (axiosInstance.post as jest.Mock).mockResolvedValue({ data });

    const result = await cashRegisterApi.openSession("42", payload);

    expect(axiosInstance.post).toHaveBeenCalledWith("/inside/businesses/42/cash-register/sessions", payload);
    expect(result).toBe(data);
  });

  it("creates a manual cash in movement", async () => {
    const payload = {
      movement_type: "cash_in" as const,
      amount: asDollars(25),
      reason: "change_bank",
      note: "Added small bills",
    };
    const data: { movement: CashRegisterMovement; session: CashRegisterSession } = {
      movement,
      session: openSession,
    };
    (axiosInstance.post as jest.Mock).mockResolvedValue({ data });

    const result = await cashRegisterApi.createMovement("42", 7, payload);

    expect(axiosInstance.post).toHaveBeenCalledWith(
      "/inside/businesses/42/cash-register/sessions/7/movements",
      payload,
    );
    expect(result).toBe(data);
  });

  it("closes a caja session with counted cash", async () => {
    const payload = {
      counted_cash: asDollars(175.25),
      closing_note: "Balanced",
    };
    const data: CashRegisterSession = closedSession;
    (axiosInstance.post as jest.Mock).mockResolvedValue({ data });

    const result = await cashRegisterApi.closeSession("42", 7, payload);

    expect(axiosInstance.post).toHaveBeenCalledWith(
      "/inside/businesses/42/cash-register/sessions/7/close",
      payload,
    );
    expect(result).toBe(data);
  });

  it("lists historical sessions with pagination", async () => {
    const data: CashRegisterSessionsResponse = {
      sessions: [closedSession],
      total: 1,
    };
    (axiosInstance.get as jest.Mock).mockResolvedValue({ data });

    const result = await cashRegisterApi.listSessions("42", { limit: 20, offset: 40 });

    expect(axiosInstance.get).toHaveBeenCalledWith(
      "/inside/businesses/42/cash-register/sessions",
      { params: { limit: 20, offset: 40 } },
    );
    expect(result).toBe(data);
  });

  it("fetches one session with movements", async () => {
    const data: CashRegisterSession = {
      ...openSession,
      movements: [movement],
    };
    (axiosInstance.get as jest.Mock).mockResolvedValue({ data });

    const result = await cashRegisterApi.getSession("42", 7);

    expect(axiosInstance.get).toHaveBeenCalledWith("/inside/businesses/42/cash-register/sessions/7");
    expect(result).toBe(data);
  });

  it("fetches unassigned cash summary", async () => {
    const data: CashRegisterUnassignedResponse = {
      total: asDollars(48.75),
      count: 3,
    };
    (axiosInstance.get as jest.Mock).mockResolvedValue({ data });

    const result = await cashRegisterApi.getUnassigned("42");

    expect(axiosInstance.get).toHaveBeenCalledWith("/inside/businesses/42/cash-register/unassigned");
    expect(result).toBe(data);
  });
});
