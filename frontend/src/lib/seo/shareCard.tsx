import { ImageResponse } from "next/og";
import { fetchImageDataUrl, loadOgFonts } from "@/lib/storefront/ogImageData";
import { DEFAULT_SITE_DESCRIPTION } from "@/lib/seo/openGraphImages";

/**
 * The instance-wide share card (Open Graph + Twitter) served at
 * SHARE_CARD_URL. It carries the instance name, its logo (LOGO_URL) and its
 * brand color from GET /api/v1/instance, so a link preview shows this site,
 * never the upstream product. Venue pages keep their own card
 * (/b/<slug>/opengraph-image).
 */
const SHARE_CARD_SIZE = { width: 1200, height: 630 } as const;

export interface ShareCardInput {
  name: string;
  description?: string;
  logoUrl?: string;
  brandColor?: string;
}

/* eslint-disable no-restricted-syntax -- ImageResponse style props (not
   Tailwind classes); satori needs literal colors. */
const FALLBACK_BRAND = "#1a6b6a";
const INK = "#1c1917";
const CREAM = "#faf9f6";
/* eslint-enable no-restricted-syntax */

const HEX_COLOR_RE = /^#[0-9a-fA-F]{6}$/;

export function shareCardModel(input: ShareCardInput): {
  name: string;
  description: string;
  initial: string;
  brand: string;
} {
  const name = input.name.trim() || "Restaurant";
  return {
    name,
    description: (input.description ?? DEFAULT_SITE_DESCRIPTION).trim(),
    initial: name.charAt(0).toUpperCase(),
    brand:
      input.brandColor && HEX_COLOR_RE.test(input.brandColor)
        ? input.brandColor
        : FALLBACK_BRAND,
  };
}

export async function renderShareCard(
  input: ShareCardInput,
): Promise<ImageResponse> {
  const model = shareCardModel(input);
  const [fonts, logo] = await Promise.all([
    loadOgFonts(),
    input.logoUrl ? fetchImageDataUrl(input.logoUrl) : Promise.resolve(null),
  ]);
  const serif = fonts?.serif ?? "sans serif";
  const sans = fonts?.sans ?? "sans serif";

  return new ImageResponse(
    (
      <div
        style={{
          width: SHARE_CARD_SIZE.width,
          height: SHARE_CARD_SIZE.height,
          display: "flex",
          flexDirection: "column",
          justifyContent: "center",
          padding: "0 96px",
          background: CREAM,
          borderTop: `16px solid ${model.brand}`,
          fontFamily: sans,
        }}
      >
        <div style={{ display: "flex", alignItems: "center", gap: 36 }}>
          <div
            style={{
              width: 144,
              height: 144,
              borderRadius: 28,
              background: logo ? "white" : model.brand,
              display: "flex",
              alignItems: "center",
              justifyContent: "center",
              overflow: "hidden",
              flexShrink: 0,
            }}
          >
            {logo ? (
              // satori renders plain <img>; next/image does not apply here.
              // eslint-disable-next-line @next/next/no-img-element
              <img
                src={logo}
                alt=""
                width={120}
                height={120}
                style={{ objectFit: "contain" }}
              />
            ) : (
              <div
                style={{
                  display: "flex",
                  fontSize: 76,
                  color: "white",
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
              gap: 16,
              maxWidth: 820,
            }}
          >
            <div
              style={{
                display: "flex",
                fontSize: 72,
                lineHeight: 1.05,
                color: INK,
                fontFamily: serif,
              }}
            >
              {model.name}
            </div>
            {model.description ? (
              <div
                style={{
                  display: "flex",
                  fontSize: 30,
                  lineHeight: 1.3,
                  color: model.brand,
                }}
              >
                {model.description}
              </div>
            ) : null}
          </div>
        </div>
      </div>
    ),
    {
      ...SHARE_CARD_SIZE,
      ...(fonts ? { fonts: fonts.fonts } : {}),
      headers: { "Cache-Control": "public, max-age=300" },
    },
  );
}
