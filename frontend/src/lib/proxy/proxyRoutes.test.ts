import * as apiRoute from "@/app/api/v1/[...path]/route";
import * as mediaRoute from "@/app/media/[...path]/route";

const saved = process.env.BACKEND_INTERNAL_URL;
afterEach(() => {
  if (saved === undefined) delete process.env.BACKEND_INTERNAL_URL;
  else process.env.BACKEND_INTERNAL_URL = saved;
});

describe("proxy route modules", () => {
  it("API route handles every method on the Node runtime, never cached", () => {
    for (const method of ["GET", "HEAD", "POST", "PUT", "PATCH", "DELETE", "OPTIONS"]) {
      expect(typeof (apiRoute as Record<string, unknown>)[method]).toBe("function");
    }
    expect(apiRoute.runtime).toBe("nodejs");
    expect(apiRoute.dynamic).toBe("force-dynamic");
  });

  it("media route is read-only", () => {
    expect(typeof mediaRoute.GET).toBe("function");
    expect(typeof mediaRoute.HEAD).toBe("function");
    expect((mediaRoute as Record<string, unknown>).POST).toBeUndefined();
    expect(mediaRoute.runtime).toBe("nodejs");
  });

  it("both answer 404 while BACKEND_INTERNAL_URL is unset", async () => {
    delete process.env.BACKEND_INTERNAL_URL;
    const api = await apiRoute.POST(
      new Request("https://pos.example.test/api/v1/orders", { method: "POST", body: "{}" }),
    );
    const media = await mediaRoute.GET(
      new Request("https://pos.example.test/media/menu/a.webp"),
    );
    expect(api.status).toBe(404);
    expect(media.status).toBe(404);
  });

  it("reads BACKEND_INTERNAL_URL per request (no rebuild or restart cache)", async () => {
    process.env.BACKEND_INTERNAL_URL = "http://127.0.0.1:1";
    const res = await apiRoute.GET(new Request("https://pos.example.test/api/v1/menu"));
    // Enabled now: the unreachable target yields 502 instead of 404.
    expect(res.status).toBe(502);
  });
});
