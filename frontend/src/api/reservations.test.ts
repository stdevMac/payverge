import { guestReservationAPI, reservationAPI } from "@/api/reservations";
import { axiosInstance } from "@/api/tools/instance";

jest.mock("@/api/tools/instance", () => ({
  axiosInstance: {
    post: jest.fn(),
    get: jest.fn(),
    put: jest.fn(),
    delete: jest.fn(),
  },
}));

describe("reservationAPI", () => {
  beforeEach(() => {
    jest.clearAllMocks();
  });

  it("requests reservation availability with party size and date", async () => {
    (axiosInstance.get as jest.Mock).mockResolvedValue({ data: { available_slots: [] } });

    await guestReservationAPI.getAvailability("demo-bistro", "2026-03-09", 4);

    expect(axiosInstance.get).toHaveBeenCalledWith(
      "/business/demo-bistro/reservations/availability?date=2026-03-09&party_size=4",
    );
  });

  it("uses the check-in transition endpoint", async () => {
    (axiosInstance.post as jest.Mock).mockResolvedValue({ data: { id: 9, status: "seated" } });

    await reservationAPI.checkIn(42, 9, { notes: "Guest arrived" });

    expect(axiosInstance.post).toHaveBeenCalledWith(
      "/inside/businesses/42/reservations/9/check-in",
      { notes: "Guest arrived" },
    );
  });

  it("passes no-show status filtering to the reservation list endpoint", async () => {
    (axiosInstance.get as jest.Mock).mockResolvedValue({ data: { reservations: [], total: 0 } });

    await reservationAPI.getReservations(42, "2026-03-01", "2026-03-31", "no_show");

    expect(axiosInstance.get).toHaveBeenCalledWith(
      "/inside/businesses/42/reservations?start_date=2026-03-01&end_date=2026-03-31&status=no_show",
      { _useCache: false },
    );
  });

  it("passes pagination to the reservation list endpoint", async () => {
    (axiosInstance.get as jest.Mock).mockResolvedValue({
      data: { reservations: [], total: 0, page: 3, page_size: 10, total_pages: 1 },
    });

    await reservationAPI.getReservations(42, "2026-03-01", "2026-03-31", "no_show", {
      page: 3,
      pageSize: 10,
    });

    expect(axiosInstance.get).toHaveBeenCalledWith(
      "/inside/businesses/42/reservations?start_date=2026-03-01&end_date=2026-03-31&status=no_show&page=3&page_size=10",
      { _useCache: false },
    );
  });

  it("passes the optional q search param when set", async () => {
    (axiosInstance.get as jest.Mock).mockResolvedValue({
      data: { reservations: [], total: 0 },
    });

    await reservationAPI.getReservations(42, "2026-03-01", "2026-03-31", "all", {
      q: "  Alice  ",
      page: 1,
      pageSize: 25,
    });

    expect(axiosInstance.get).toHaveBeenCalledWith(
      "/inside/businesses/42/reservations?start_date=2026-03-01&end_date=2026-03-31&q=Alice&page=1&page_size=25",
      { _useCache: false },
    );
  });

  it("omits q when search is empty or whitespace (legacy shape)", async () => {
    (axiosInstance.get as jest.Mock).mockResolvedValue({
      data: { reservations: [], total: 0 },
    });

    await reservationAPI.getReservations(42, "2026-03-01", "2026-03-31", "all", {
      q: "   ",
    });

    expect(axiosInstance.get).toHaveBeenCalledWith(
      "/inside/businesses/42/reservations?start_date=2026-03-01&end_date=2026-03-31",
      { _useCache: false },
    );
  });

  it("uses the promote waitlist endpoint", async () => {
    (axiosInstance.post as jest.Mock).mockResolvedValue({ data: { id: 9, status: "confirmed" } });

    await reservationAPI.promoteWaitlist(42, 9, { notes: "Table opened" });

    expect(axiosInstance.post).toHaveBeenCalledWith(
      "/inside/businesses/42/reservations/9/promote-waitlist",
      { notes: "Table opened" },
    );
  });

  it("requests protected table options without using the response cache", async () => {
    (axiosInstance.get as jest.Mock).mockResolvedValue({
      data: { business_timezone: "America/New_York", near_term: true, tables: [] },
    });

    await reservationAPI.getTableOptions(42, {
      reservationTime: "2026-07-18T19:00:00.000Z",
      duration: 120,
      partySize: 2,
      excludeReservationId: 9,
    });

    expect(axiosInstance.get).toHaveBeenCalledWith(
      "/inside/businesses/42/reservations/table-options?reservation_time=2026-07-18T19%3A00%3A00.000Z&duration=120&party_size=2&exclude_reservation_id=9",
      { _useCache: false },
    );
  });

  it("cancels a reservation by confirmation code", async () => {
    (axiosInstance.post as jest.Mock).mockResolvedValue({ data: { message: "Reservation cancelled successfully" } });

    await guestReservationAPI.cancelReservation("abc123");

    expect(axiosInstance.post).toHaveBeenCalledWith(
      "/reservations/abc123/cancel",
    );
  });

  it("loads public reservation details by confirmation code", async () => {
    (axiosInstance.get as jest.Mock).mockResolvedValue({ data: { reservation: { id: 3 } } });

    await guestReservationAPI.getReservation("abc123");

    expect(axiosInstance.get).toHaveBeenCalledWith(
      "/reservations/abc123",
    );
  });
});
