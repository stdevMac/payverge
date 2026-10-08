import { getSiteUrl } from "@/config/publicConfig";
import { ImageResponse } from "next/og";
import {
  fetchStorefrontBusiness,
  fetchStorefrontGoogleRating,
} from "@/lib/storefront/serverData";
import {
  buildOgCardModel,
  fetchImageDataUrl,
  loadOgFonts,
} from "@/lib/storefront/ogImageData";

// Per-business share card: banner backdrop + logo tile + name + rating, so a
// /b/<slug> link preview looks like the restaurant instead of a raw photo or
// the generic Payverge image. Served by the file convention — generateMetadata
// deliberately does NOT set openGraph.images so this takes over.
export const runtime = "nodejs";
export const dynamic = "force-dynamic";
export const alt = "Restaurant storefront on Payverge";
export const size = { width: 1200, height: 630 };
export const contentType = "image/png";

/* eslint-disable no-restricted-syntax -- ImageResponse style props (not
   Tailwind classes); brand hex values are required for the satori-rendered PNG. */
const BRAND_TEAL = "#1a6b6a";
const DEEP_TEAL = "#0c3534";
const STAR_AMBER = "#f5b942";
/* eslint-enable no-restricted-syntax */

export default async function StorefrontOgImage({
  params,
}: {
  params: Promise<{ customUrl: string }>;
}) {
  const { customUrl } = await params;
  const result = await fetchStorefrontBusiness(customUrl);
  const business = result.business;

  const baseUrl = getSiteUrl();
  // Rating is decorative; only fetched when the business advertises reviews.
  const googleRating =
    business && business.google_reviews_enabled && business.google_place_id
      ? await fetchStorefrontGoogleRating(customUrl)
      : null;

  const model = buildOgCardModel({ business, customUrl, baseUrl, googleRating });

  const [fonts, banner, logo] = await Promise.all([
    loadOgFonts(),
    model.bannerUrl ? fetchImageDataUrl(model.bannerUrl) : Promise.resolve(null),
    model.logoUrl ? fetchImageDataUrl(model.logoUrl) : Promise.resolve(null),
  ]);

  // When custom fonts fail to load, omit the option entirely — next/og then
  // injects its bundled default font instead of throwing.
  const serif = fonts?.serif ?? "sans serif";
  const sans = fonts?.sans ?? "sans serif";

  return new ImageResponse(
    (
      <div
        style={{
          width: size.width,
          height: size.height,
          display: "flex",
          position: "relative",
          background: `linear-gradient(135deg, ${BRAND_TEAL} 0%, ${DEEP_TEAL} 100%)`,
          fontFamily: sans,
        }}
      >
        {banner ? (
          <img
            src={banner}
            alt=""
            width={size.width}
            height={size.height}
            style={{
              position: "absolute",
              top: 0,
              left: 0,
              objectFit: "cover",
            }}
          />
        ) : null}
        {/* Scrim: keeps the name readable over any banner photo. */}
        <div
          style={{
            position: "absolute",
            top: 0,
            left: 0,
            right: 0,
            bottom: 0,
            display: "flex",
            background: banner
              ? "linear-gradient(180deg, rgba(8, 28, 28, 0.18) 0%, rgba(8, 28, 28, 0.55) 55%, rgba(8, 28, 28, 0.88) 100%)"
              : "linear-gradient(135deg, rgba(255,255,255,0.10) 0%, rgba(255,255,255,0) 45%, rgba(0,0,0,0.22) 100%)",
          }}
        />
        {/* Content block, bottom-left above the footer strip. */}
        <div
          style={{
            position: "absolute",
            top: 0,
            left: 0,
            right: 0,
            bottom: 0,
            display: "flex",
            flexDirection: "column",
            justifyContent: "flex-end",
            padding: "56px 56px 96px",
          }}
        >
          <div style={{ display: "flex", alignItems: "center", gap: 28 }}>
            <div
              style={{
                width: 112,
                height: 112,
                borderRadius: 24,
                background: "white",
                display: "flex",
                alignItems: "center",
                justifyContent: "center",
                overflow: "hidden",
                flexShrink: 0,
                boxShadow: "0 10px 32px rgba(0, 0, 0, 0.35)",
              }}
            >
              {logo ? (
                <img
                  src={logo}
                  alt=""
                  width={92}
                  height={92}
                  style={{ objectFit: "contain" }}
                />
              ) : (
                <div
                  style={{
                    display: "flex",
                    fontSize: 58,
                    color: BRAND_TEAL,
                    fontFamily: serif,
                  }}
                >
                  {model.initial}
                </div>
              )}
            </div>
            <div
              style={{
                display: "flex",
                flexDirection: "column",
                gap: 12,
                maxWidth: 960,
              }}
            >
              <div
                style={{
                  display: "flex",
                  fontSize: 62,
                  lineHeight: 1.05,
                  color: "white",
                  fontFamily: serif,
                  textShadow: "0 2px 18px rgba(0, 0, 0, 0.45)",
                }}
              >
                {model.name}
              </div>
              {model.description ? (
                <div
                  style={{
                    display: "flex",
                    fontSize: 26,
                    lineHeight: 1.3,
                    color: "rgba(255, 255, 255, 0.88)",
                  }}
                >
                  {model.description}
                </div>
              ) : null}
              {model.rating ? (
                <div
                  style={{
                    display: "flex",
                    alignItems: "center",
                    gap: 10,
                    color: "white",
                    fontSize: 24,
                  }}
                >
                  <svg width="26" height="26" viewBox="0 0 24 24">
                    <path
                      d="M12 2l2.9 6.26 6.6.57-5 4.4 1.5 6.47L12 16.9 5.99 19.7l1.5-6.47-5-4.4 6.6-.57z"
                      fill={STAR_AMBER}
                    />
                  </svg>
                  <span>
                    {`${model.rating.value.toFixed(1)} (${model.rating.count})`}
                  </span>
                </div>
              ) : null}
            </div>
          </div>
        </div>
        {/* Footer strip: brand teal with the canonical storefront path. */}
        <div
          style={{
            position: "absolute",
            left: 0,
            right: 0,
            bottom: 0,
            height: 56,
            background: BRAND_TEAL,
            borderTop: "1px solid rgba(255, 255, 255, 0.22)",
            display: "flex",
            alignItems: "center",
            justifyContent: "space-between",
            padding: "0 56px",
          }}
        >
          <div
            style={{
              display: "flex",
              color: "rgba(255, 255, 255, 0.92)",
              fontSize: 22,
            }}
          >
            {model.slugPath}
          </div>
          <div
            style={{
              display: "flex",
              color: "rgba(255, 255, 255, 0.68)",
              fontSize: 20,
              letterSpacing: 1,
            }}
          >
            Payverge
          </div>
        </div>
      </div>
    ),
    {
      ...size,
      ...(fonts ? { fonts: fonts.fonts } : {}),
    },
  );
}
