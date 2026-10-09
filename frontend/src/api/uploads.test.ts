import { uploadFile, type UploadResponse } from "@/api/uploads";
import { axiosInstance } from "@/api/tools/instance";

jest.mock("@/api/tools/instance", () => ({
  axiosInstance: {
    post: jest.fn(),
  },
}));

describe("uploads api", () => {
  beforeEach(() => {
    jest.clearAllMocks();
  });

  it("uploads under the canonical business route", async () => {
    const file = new File(["logo"], "logo.png", { type: "image/png" });
    const uploadResponse: UploadResponse = {
      location: "https://cdn.example/businesses/42/business-logo/logo.png",
      filename: "logo.png",
      folder: "businesses/42/business-logo",
    };
    (axiosInstance.post as jest.Mock).mockResolvedValue({ data: uploadResponse });

    const result = await uploadFile(file, "business-logo", 42);

    expect(result).toEqual(uploadResponse);
    expect(axiosInstance.post).toHaveBeenCalledWith(
      "/inside/businesses/42/uploads",
      expect.any(FormData),
      { headers: { "Content-Type": "multipart/form-data" } },
    );
  });
});
