/** @jest-environment node */
import { fetchGuestTableCatalog } from "./fetchGuestTableCatalog";

const realFetch = global.fetch;
const originalPublic = process.env.API_URL;

afterEach(() => {
  global.fetch = realFetch;
  if (originalPublic === undefined) delete process.env.API_URL;
  else process.env.API_URL = originalPublic;
});

describe("fetchGuestTableCatalog", () => {
  it("extracts dish and bundle names from the guest table payload", async () => {
    process.env.API_URL = "http://api.test/api/v1";
    global.fetch = jest.fn().mockResolvedValue({
      ok: true,
      status: 200,
      json: async () => ({
        business: { name: "Payverge AI Pro Demo Lounge" },
        categories: [
          {
            name: "Mains",
            items: [
              { name: "Harvest Bowl" },
              { name: "Steak Plate" },
            ],
          },
        ],
        bundles: [{ name: "Date Night for Two" }],
      }),
    }) as unknown as typeof fetch;

    await expect(fetchGuestTableCatalog("M03Y18GB3P")).resolves.toEqual({
      businessName: "Payverge AI Pro Demo Lounge",
      dishes: ["Mains", "Harvest Bowl", "Steak Plate", "Date Night for Two"],
    });
  });
});
