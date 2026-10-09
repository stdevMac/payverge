/**
 * @jest-environment node
 */
import {
  listReceiptDelivery,
  retryDeliveryTask,
} from "@/api/fiscal";

jest.mock("@/api/tools/instance", () => ({
  axiosInstance: {
    get: jest.fn(),
    post: jest.fn(),
  },
}));

import { axiosInstance } from "@/api/tools/instance";

const mocked = axiosInstance as jest.Mocked<typeof axiosInstance>;

describe("fiscal delivery API", () => {
  beforeEach(() => {
    jest.clearAllMocks();
  });

  it("lists delivery tasks for a receipt", async () => {
    mocked.get.mockResolvedValueOnce({
      data: {
        items: [
          {
            id: 1,
            receipt_id: 9,
            channel: "email",
            status: "dead",
            attempts: 3,
            max_attempts: 8,
            last_error_category: "transient",
            masked_recipient: "g***@example.com",
          },
        ],
      },
    });
    const items = await listReceiptDelivery(42, 9);
    expect(mocked.get).toHaveBeenCalledWith(
      "/inside/businesses/42/fiscal/receipts/9/delivery",
    );
    expect(items).toHaveLength(1);
    expect(items[0].channel).toBe("email");
    expect(items[0].masked_recipient).toBe("g***@example.com");
  });

  it("retries a delivery task (double-click safe empty body)", async () => {
    mocked.post.mockResolvedValueOnce({ data: { ok: true, business_id: 42 } });
    const res = await retryDeliveryTask(42, 7);
    expect(mocked.post).toHaveBeenCalledWith(
      "/inside/businesses/42/fiscal/delivery-tasks/7/retry",
      {},
    );
    expect(res.ok).toBe(true);
  });
});
