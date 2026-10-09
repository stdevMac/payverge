import fs from "node:fs";
import path from "node:path";
import sharp from "sharp";

const publicDirectory = path.join(process.cwd(), "public");

function readPngDimensions(bytes: Buffer) {
  if (!bytes.subarray(0, 8).equals(Buffer.from([137, 80, 78, 71, 13, 10, 26, 10]))) {
    throw new Error("Expected a PNG signature");
  }
  return {
    width: bytes.readUInt32BE(16),
    height: bytes.readUInt32BE(20),
  };
}

function readPngFileDimensions(filename: string) {
  const file = path.join(publicDirectory, filename);
  if (!fs.existsSync(file)) return undefined;
  return readPngDimensions(fs.readFileSync(file));
}

import { pwaManifestHref } from "./manifestHref";

describe("PWA manifest and offline assets", () => {
  const manifest = JSON.parse(
    fs.readFileSync(path.join(publicDirectory, "site.webmanifest"), "utf8"),
  );

  it("localizes the PWA manifest and start_url for es/es-AR (#914)", () => {
    expect(pwaManifestHref("en")).toBe("/site.webmanifest");
    expect(pwaManifestHref("es")).toBe("/site.es.webmanifest");
    expect(pwaManifestHref("es-ar")).toBe("/site.es-ar.webmanifest");

    const es = JSON.parse(
      fs.readFileSync(path.join(publicDirectory, "site.es.webmanifest"), "utf8"),
    );
    const esAr = JSON.parse(
      fs.readFileSync(
        path.join(publicDirectory, "site.es-ar.webmanifest"),
        "utf8",
      ),
    );
    expect(es.lang).toBe("es");
    expect(es.start_url).toBe("/es/app");
    expect(es.name).not.toMatch(/Restaurant Management System/i);
    expect(esAr.lang).toBe("es-AR");
    expect(esAr.start_url).toBe("/es-ar/app");
    expect(esAr.name).not.toMatch(/Restaurant Management System/i);
  });

  it("preserves the installed-app identity while launching through /app", () => {
    expect(manifest).toMatchObject({
      id: "/",
      start_url: "/app",
      scope: "/",
      display: "standalone",
      name: "Payverge - Restaurant Management System",
      short_name: "Payverge",
      description: "Restaurant operations, service, and payments in one app-like workspace.",
      theme_color: "#1a6b6a",
      background_color: "#ffffff",
      categories: ["business", "food", "productivity"],
    });
  });

  it("declares standard and maskable icons backed by correctly sized PNGs", () => {
    expect(manifest.icons).toEqual(
      expect.arrayContaining([
        { src: "/android-chrome-192x192.png", sizes: "192x192", type: "image/png", purpose: "any" },
        { src: "/android-chrome-512x512.png", sizes: "512x512", type: "image/png", purpose: "any" },
        { src: "/maskable-icon-192x192.png", sizes: "192x192", type: "image/png", purpose: "maskable" },
        { src: "/maskable-icon-512x512.png", sizes: "512x512", type: "image/png", purpose: "maskable" },
      ]),
    );
    for (const icon of manifest.icons) {
      const [width, height] = icon.sizes.split("x").map(Number);
      expect(readPngFileDimensions(icon.src.slice(1))).toEqual({ width, height });
    }
  });

  it("rejects bytes without a PNG signature before reading their dimensions", () => {
    const bytes = Buffer.alloc(24);
    bytes.writeUInt32BE(192, 16);
    bytes.writeUInt32BE(192, 20);

    expect(() => readPngDimensions(bytes)).toThrow("Expected a PNG signature");
  });

  it("keeps every maskable mark pixel inside the central 40% safe circle", async () => {
    const teal = [26, 107, 106];
    const maskableIcons = manifest.icons.filter((icon: { purpose: string }) => icon.purpose === "maskable");

    for (const icon of maskableIcons) {
      const { data, info } = await sharp(path.join(publicDirectory, icon.src)).ensureAlpha().raw().toBuffer({
        resolveWithObject: true,
      });
      const radius = info.width * 0.4;
      const center = (info.width - 1) / 2;

      for (let y = 0; y < info.height; y += 1) {
        for (let x = 0; x < info.width; x += 1) {
          const offset = (y * info.width + x) * info.channels;
          const isBackground = teal.every((value, channel) => data[offset + channel] === value);
          if (isBackground) continue;

          expect(Math.hypot(x - center, y - center)).toBeLessThanOrEqual(radius);
        }
      }
    }
  });

  it.each([
    ["offline/en.html", 'lang="en"', "Payverge — Offline", "You’re offline", "Payverge needs an internet connection to show current restaurant data.", "Try again"],
    ["offline/es.html", 'lang="es"', "Payverge — Sin conexión", "No tienes conexión", "Payverge necesita internet para mostrar los datos actuales del restaurante.", "Reintentar"],
    ["offline/es-ar.html", 'lang="es-AR"', "Payverge — Sin conexión", "No tenés conexión", "Payverge necesita internet para mostrar los datos actuales del restaurante.", "Reintentá"],
  ])("ships an approved localized offline document at %s", (filename, lang, title, heading, paragraph, retry) => {
    const file = path.join(publicDirectory, filename);
    expect(fs.existsSync(file)).toBe(true);
    const html = fs.readFileSync(file, "utf8");

    expect(html).toContain(lang);
    expect(html).toContain(`<title>${title}</title>`);
    expect(html).toContain(heading);
    expect(html).toContain(paragraph);
    expect(html).toContain(retry);
    expect(html).toContain('href="/app"');
    expect(html).toContain('content="width=device-width,initial-scale=1,viewport-fit=cover"');
    expect(html).toContain('<main class="card">');
    expect(html).toContain("background:#faf9f6");
    expect(html).toContain("background:#fff");
  });
});
