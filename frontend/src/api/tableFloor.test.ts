import {
  clearTable,
  mergeTable,
  seatTable,
  transferTable,
} from "./tableFloor";

jest.mock("./tools/instance", () => ({
  axiosInstance: {
    post: jest.fn(),
  },
}));

import { axiosInstance } from "./tools/instance";

const post = axiosInstance.post as jest.Mock;

describe("tableFloor API", () => {
  beforeEach(() => {
    post.mockReset();
    post.mockResolvedValue({ data: { ok: true } });
  });

  it("seats a table with optional covers + reservation", async () => {
    await seatTable(9, 3, { party_size: 4, reservation_id: 12 });
    expect(post).toHaveBeenCalledWith(
      "/inside/businesses/9/tables/3/seat",
      { party_size: 4, reservation_id: 12 },
    );
  });

  it("clears a table", async () => {
    await clearTable(9, 3);
    expect(post).toHaveBeenCalledWith("/inside/businesses/9/tables/3/clear");
  });

  it("transfers a table check", async () => {
    await transferTable(9, 3, { target_table_id: 7 });
    expect(post).toHaveBeenCalledWith(
      "/inside/businesses/9/tables/3/transfer",
      { target_table_id: 7 },
    );
  });

  it("merges a table check", async () => {
    await mergeTable(9, 3, { target_table_id: 7 });
    expect(post).toHaveBeenCalledWith(
      "/inside/businesses/9/tables/3/merge",
      { target_table_id: 7 },
    );
  });
});
