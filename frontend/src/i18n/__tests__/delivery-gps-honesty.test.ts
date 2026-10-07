import fs from "node:fs";
import path from "node:path";

import enGuest from "@/i18n/guest-messages/en.json";
import esArGuest from "@/i18n/guest-messages/es-AR.json";

const gpsClaim = /live tracking|seguimiento en vivo|driver location|ubicación del repartidor|\bgps\b/i;

describe("delivery tracking honesty without a driver GPS producer", () => {
  it("describes status and ETA tracking, never live driver location", () => {
    // Guest copy must describe delivery as status/ETA tracking. There is no
    // driver-GPS producer, so a "live tracking / seguimiento en vivo" claim
    // would be a promise we cannot keep.
    const copy = JSON.stringify({
      guestEn: enGuest.deliveryTracking,
      guestEsAr: esArGuest.deliveryTracking,
    });

    expect(copy).not.toMatch(gpsClaim);
    expect(enGuest.deliveryTracking).not.toHaveProperty("locationLabel");
    expect(esArGuest.deliveryTracking).not.toHaveProperty("locationLabel");
  });

  it("keeps the public tracker status-only with no map or raw-coordinate UI", () => {
    const trackDir = path.resolve(
      process.cwd(),
      "src/app/delivery/[deliveryNumber]/track",
    );
    const page = fs.readFileSync(path.join(trackDir, "page.tsx"), "utf8");
    const layout = fs.readFileSync(path.join(trackDir, "layout.tsx"), "utf8");

    expect(page).not.toMatch(/data-testid=["']delivery-map["']/);
    expect(page).not.toMatch(/openstreetmap|current_location|latitude|longitude/i);
    // The layout must not justify itself with a since-removed OpenStreetMap
    // iframe — the route renders no map, and a stale map reference in the source
    // reads as a live-location surface that does not exist.
    expect(layout).not.toMatch(/openstreetmap/i);
  });
});
