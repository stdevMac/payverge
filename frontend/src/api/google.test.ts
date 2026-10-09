import {
  removeBusinessGoogleInfo,
  searchGoogleBusinesses,
  updateBusinessGoogleInfo,
} from "@/api/google";
import { axiosInstance } from "@/api/tools/instance";

jest.mock("@/api/tools/instance", () => ({
  axiosInstance: {
    delete: jest.fn(),
    post: jest.fn(),
    put: jest.fn(),
  },
}));

describe("google api", () => {
  beforeEach(() => {
    jest.clearAllMocks();
  });

  it("searches and updates Google business metadata through canonical routes", async () => {
    (axiosInstance.post as jest.Mock).mockResolvedValue({
      data: { results: [], count: 0 },
    });
    (axiosInstance.put as jest.Mock).mockResolvedValue({
      data: {
        message: "saved",
        google_place_id: "place-1",
        google_business_name: "Cafe",
        google_review_link: "review",
        google_business_url: "maps",
      },
    });
    (axiosInstance.delete as jest.Mock).mockResolvedValue({
      data: { message: "removed" },
    });

    await searchGoogleBusinesses("cafe palermo");
    await updateBusinessGoogleInfo("42", {
      place_id: "place-1",
      business_name: "Cafe",
    });
    await removeBusinessGoogleInfo("42");

    expect(axiosInstance.post).toHaveBeenCalledWith(
      "/inside/google/businesses/search",
      { query: "cafe palermo" },
    );
    expect(axiosInstance.put).toHaveBeenCalledWith(
      "/inside/businesses/42/google",
      { place_id: "place-1", business_name: "Cafe" },
    );
    expect(axiosInstance.delete).toHaveBeenCalledWith(
      "/inside/businesses/42/google",
    );
  });
});
