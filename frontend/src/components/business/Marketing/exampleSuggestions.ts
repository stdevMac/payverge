import type { CampaignSuggestion } from "@/api/marketing";
import { getSiteUrl } from "@/config/publicConfig";

/** Bundled, same-origin demo photos. Served from /public, so the canvas export
 *  path never taints — examples carry no S3-CORS dependency. */
export const EXAMPLE_IMAGE_DISH = "/marketing-examples/example-featured-dish.jpg";
export const EXAMPLE_IMAGE_OFFER = "/marketing-examples/example-offer.jpg";

type Translate = (key: string, params?: Record<string, string | number>) => string;

/** Two generic "this is what a finished post looks like" cards, rendered in the
 *  operator's own brand colors. Shown only when the real feed is empty so the
 *  gallery is never blank; the caller marks these cards isExample. */
export function buildExampleSuggestions(t: Translate): CampaignSuggestion[] {
  return [
    {
      id: "example-featured",
      play: "featured_dish",
      title: t("example.cards.featured.title"),
      why_data: t("example.cards.featured.why"),
      why_factors: [
        { key: "qty_sold", value: t("example.cards.featured.factorQty") },
        { key: "margin_per_unit", value: t("example.cards.featured.factorMargin") },
        { key: "photo_ready", value: t("why.factors.photo_ready") },
      ],
      ranking_version: "s2",
      source: "example",
      target_name: t("example.cards.featured.name"),
      copy_angle: "warm",
      rank: 0,
      image_url: EXAMPLE_IMAGE_DISH,
      image_source: "menu",
      guest_url: `${getSiteUrl()}/b/demo-bistro?tab=menu`,
      guest_url_kind: "menu",
    },
    {
      id: "example-offer",
      play: "offer",
      title: t("example.cards.offer.title"),
      why_data: t("example.cards.offer.why"),
      why_factors: [
        // #826: wire shape, not display copy — formatWhyFactorValue owns the
        // localized "off" wording ("20% off off" came from double-wrapping).
        { key: "discount", value: "20%" },
        { key: "photo_ready", value: t("why.factors.photo_ready") },
      ],
      ranking_version: "s2",
      source: "example",
      target_name: t("example.cards.offer.name"),
      copy_angle: "punchy",
      rank: 1,
      image_url: EXAMPLE_IMAGE_OFFER,
      image_source: "offer",
      discount_type: "percentage",
      discount_value: 20,
      guest_url: `${getSiteUrl()}/b/demo-bistro?tab=menu`,
      guest_url_kind: "menu",
    },
  ];
}
