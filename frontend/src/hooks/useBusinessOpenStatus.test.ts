import { getBusinessOpenStatus } from "@/hooks/useBusinessOpenStatus";

describe("getBusinessOpenStatus", () => {
  it("uses the business timezone instead of the browser timezone", () => {
    const status = getBusinessOpenStatus(
      [
        {
          day_of_week: 6,
          open_time: "12:00",
          close_time: "22:00",
          is_closed: false,
        },
      ],
      "Asia/Dubai",
      new Date("2026-03-07T10:00:00.000Z"),
    );

    expect(status.isOpen).toBe(true);
    expect(status.businessDate).toBe("2026-03-07");
    expect(status.businessTime).toBe("14:00");
  });

  it("keeps overnight hours open after midnight from the previous day", () => {
    const status = getBusinessOpenStatus(
      [
        {
          day_of_week: 5,
          open_time: "18:00",
          close_time: "02:00",
          is_closed: false,
        },
        {
          day_of_week: 6,
          open_time: "00:00",
          close_time: "00:00",
          is_closed: true,
        },
      ],
      "UTC",
      new Date("2026-03-07T01:00:00.000Z"),
    );

    expect(status.isOpen).toBe(true);
    expect(status.businessTime).toBe("01:00");
  });

  it("treats split-shift lunch/dinner as open only inside a period", () => {
    const hours = [
      {
        day_of_week: 1,
        open_time: "11:00",
        close_time: "15:00",
        is_closed: false,
      },
      {
        day_of_week: 1,
        open_time: "19:00",
        close_time: "23:00",
        is_closed: false,
      },
    ];
    // Monday 2026-03-09 13:00 UTC = open (lunch)
    expect(
      getBusinessOpenStatus(hours, "UTC", new Date("2026-03-09T13:00:00.000Z"))
        .isOpen,
    ).toBe(true);
    // Monday 16:00 UTC = closed (between shifts)
    expect(
      getBusinessOpenStatus(hours, "UTC", new Date("2026-03-09T16:00:00.000Z"))
        .isOpen,
    ).toBe(false);
    // Monday 20:00 UTC = open (dinner)
    expect(
      getBusinessOpenStatus(hours, "UTC", new Date("2026-03-09T20:00:00.000Z"))
        .isOpen,
    ).toBe(true);
  });

  it("uses kitchen close for OPEN NOW even when the door stays open later", () => {
    const hours = [
      {
        day_of_week: 2,
        open_time: "11:00",
        close_time: "23:00",
        kitchen_close_time: "22:00",
        is_closed: false,
      },
    ];
    // Tuesday 2026-03-10 21:30 UTC — kitchen still open
    expect(
      getBusinessOpenStatus(hours, "UTC", new Date("2026-03-10T21:30:00.000Z"))
        .isOpen,
    ).toBe(true);
    // 22:30 UTC — door open until 23:00 but kitchen closed → not "OPEN NOW"
    expect(
      getBusinessOpenStatus(hours, "UTC", new Date("2026-03-10T22:30:00.000Z"))
        .isOpen,
    ).toBe(false);
  });

  it("honors a holiday closed exception over the weekly grid", () => {
    const hours = [
      {
        day_of_week: 3,
        open_time: "11:00",
        close_time: "23:00",
        is_closed: false,
      },
    ];
    expect(
      getBusinessOpenStatus(
        hours,
        "UTC",
        new Date("2026-03-11T15:00:00.000Z"),
        [{ exception_date: "2026-03-11", is_closed: true }],
      ).isOpen,
    ).toBe(false);
  });
});
