import {
  createBusiness,
  type Business,
  type CreateBusinessRequest,
} from "@/api/business";
import { axiosInstance } from "@/api/tools/instance";

jest.mock("@/api/tools/instance", () => ({
  axiosInstance: {
    post: jest.fn(),
  },
}));

jest.mock("@/utils/errorLogger", () => ({
  logError: jest.fn(),
}));

type IdempotentCreateBusiness = (
  request: CreateBusinessRequest,
  options: { idempotencyKey: string },
) => Promise<Business>;

const request: CreateBusinessRequest = {
  name: "Retry Safe Cafe",
  address: {
    street: "1 Main Street",
    city: "Dubai",
    state: "Dubai",
    postal_code: "00000",
    country: "AE",
  },
  settlement_address: "",
  tipping_address: "",
  tax_rate: 0,
  service_fee_rate: 0,
  tax_inclusive: false,
  service_inclusive: false,
};

const response: Business = {
  id: 42,
  business_id: "retry-safe-cafe",
  owner_address: "",
  name: request.name,
  logo: "",
  address: request.address,
  settlement_address: "",
  tipping_address: "",
  tax_rate: 0,
  service_fee_rate: 0,
  tax_inclusive: false,
  service_inclusive: false,
  is_active: true,
  created_at: "2026-07-31T00:00:00Z",
  updated_at: "2026-07-31T00:00:00Z",
};

describe("createBusiness idempotency contract", () => {
  beforeEach(() => {
    jest.clearAllMocks();
    (axiosInstance.post as jest.Mock).mockResolvedValue({ data: response });
  });

  it("sends the same explicit Idempotency-Key on a response-loss retry", async () => {
    const client = createBusiness as unknown as IdempotentCreateBusiness;
    const options = { idempotencyKey: "workspace-owner-42-attempt-1" };

    await client(request, options);
    await client(request, options);

    expect(axiosInstance.post).toHaveBeenNthCalledWith(
      1,
      "/inside/businesses",
      request,
      { headers: { "Idempotency-Key": options.idempotencyKey } },
    );
    expect(axiosInstance.post).toHaveBeenNthCalledWith(
      2,
      "/inside/businesses",
      request,
      { headers: { "Idempotency-Key": options.idempotencyKey } },
    );
  });
});
