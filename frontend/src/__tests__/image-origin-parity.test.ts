import fs from "node:fs";
import path from "node:path";
import imageOrigins from "@/config/imageOrigins.json";
import { imageCspOrigins } from "@/config/imageCspOrigins";
import { canOptimizeImageSrc, optimizableImageHosts } from "@/config/imageOrigins";

describe("remote image origin contract", () => {
  it("ships a generic manifest: no first-party SaaS or storage-provider hosts", () => {
    const hosts = imageOrigins.map((entry) => entry.hostname);
    expect(hosts.join(" ")).not.toMatch(/payverge|e2-3\.dev|idrivee2/);
    expect(new Set(hosts).size).toBe(hosts.length);
  });

  it("derives CSP img-src hosts from the same manifest (#290)", () => {
    expect(imageCspOrigins().sort()).toEqual(
      imageOrigins.map((entry) => `https://${entry.hostname}`).sort(),
    );
    expect(imageCspOrigins()).toContain("https://lh3.googleusercontent.com");
  });

  it("allows images.unsplash.com so guest offer photos are not CSP-blocked (issue 346)", () => {
    expect(imageCspOrigins()).toContain("https://images.unsplash.com");
  });

  it("optimizes exactly the manifest hosts plus same-origin paths", () => {
    expect([...optimizableImageHosts].sort()).toEqual(
      imageOrigins.map((entry) => entry.hostname).sort(),
    );
    expect(canOptimizeImageSrc("/media/uploads/a.jpg")).toBe(true);
    expect(canOptimizeImageSrc("/images/allergens/celery.svg")).toBe(true);
    expect(canOptimizeImageSrc("data:image/png;base64,AAAA")).toBe(true);
    expect(canOptimizeImageSrc({ src: "/_next/static/a.png", width: 1, height: 1 })).toBe(true);
    expect(canOptimizeImageSrc("https://images.unsplash.com/photo-1")).toBe(true);
    expect(canOptimizeImageSrc("https://LH3.googleusercontent.com/a")).toBe(true);
    // Runtime upload buckets (MEDIA_ORIGINS) and protocol-relative hosts are
    // not in the build-time remotePatterns: they must render unoptimized.
    expect(canOptimizeImageSrc("https://bucket.example.test/a.jpg")).toBe(false);
    expect(canOptimizeImageSrc("//bucket.example.test/a.jpg")).toBe(false);
    expect(canOptimizeImageSrc("")).toBe(false);
    expect(canOptimizeImageSrc(undefined)).toBe(false);
  });

  it.each([
    "src/components/guest/ImageCarousel.tsx",
    "src/components/business-page/MenuItemMedia.tsx",
    "src/components/business-page/BusinessHeroSection.tsx",
    "src/components/business/DashboardSidebar.tsx",
    "src/components/business/ExternalPartnerLinksEditor.tsx",
    "src/components/receipt/ReceiptGenerator.tsx",
  ])("guards uploaded-media next/image in %s against unknown hosts", (file) => {
    const source = fs.readFileSync(path.join(process.cwd(), file), "utf8");
    expect(source).toMatch(/unoptimized=\{!canOptimizeImageSrc\(/);
    expect(source).not.toMatch(/OPTIMIZABLE_IMAGE_HOSTS\s*=\s*new Set/);
  });

  it("makes next.config consume the manifest instead of a second literal list", () => {
    const source = fs.readFileSync(
      path.join(process.cwd(), "next.config.mjs"),
      "utf8",
    );
    expect(source).toContain(
      'readFileSync(new URL("./src/config/imageOrigins.json", import.meta.url)',
    );
    expect(source).toContain("remotePatterns: imageOrigins");
    expect(source).not.toMatch(/hostname:\s*["'][^"']*payverge\.io["']/);
  });
});
