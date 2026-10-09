import { axiosInstance } from "@/api/tools/instance";
import { checkInCustomerToTable } from "./customerTable";

jest.mock("@/api/tools/instance", () => ({
  axiosInstance: {
    post: jest.fn(),
  },
}));

const mockedAxios = axiosInstance as jest.Mocked<typeof axiosInstance>;

describe("customerTable API", () => {
  beforeEach(() => {
    jest.clearAllMocks();
  });

  it("posts check-in without customer_id", async () => {
    mockedAxios.post.mockResolvedValueOnce({
      data: { customer_business: { id: 1 }, bill_attached: true, bill_id: 12 },
    });

    await checkInCustomerToTable("TABLE123");

    expect(mockedAxios.post).toHaveBeenCalledWith(
      "/customer/table/TABLE123/check-in",
      undefined,
    );
  });

  it("encodes table codes in the URL", async () => {
    mockedAxios.post.mockResolvedValueOnce({
      data: { customer_business: { id: 1 }, bill_attached: false },
    });

    await checkInCustomerToTable("TABLE 123");

    expect(mockedAxios.post).toHaveBeenCalledWith(
      "/customer/table/TABLE%20123/check-in",
      undefined,
    );
  });
});
