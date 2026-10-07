import { getServerApiUrl } from "./serverApiUrl";

describe("getServerApiUrl", () => {
  const originalInternal = process.env.INTERNAL_API_URL;
  const originalPublic = process.env.API_URL;

  afterEach(() => {
    if (originalInternal === undefined) delete process.env.INTERNAL_API_URL;
    else process.env.INTERNAL_API_URL = originalInternal;

    if (originalPublic === undefined) delete process.env.API_URL;
    else process.env.API_URL = originalPublic;
  });

  it("prefers the container-reachable internal URL for server fetches", () => {
    process.env.INTERNAL_API_URL = "http://backend:8080/api/v1/";
    process.env.API_URL = "http://localhost:8080/api/v1";

    expect(getServerApiUrl()).toBe("http://backend:8080/api/v1");
  });

  it("falls back to the public API URL outside a private network", () => {
    delete process.env.INTERNAL_API_URL;
    process.env.API_URL = "https://api.example.test/api/v1/";

    expect(getServerApiUrl()).toBe("https://api.example.test/api/v1");
  });
});
