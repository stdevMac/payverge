import { ExtractedMenu, MenuCategory } from "@/api/business";
import { asDollars } from "@/types/money";
import { parseLocaleDecimal } from "@/lib/parseLocaleDecimal";

// mapExtractedMenu turns OCR/AI-extracted string prices into Dollars. Extracted
// prices arrive as locale-formatted strings ("$10", "5,50", "1.250,00"); parse
// them with parseLocaleDecimal, NOT parseFloat (which truncates comma decimals,
// e.g. "5,50" -> 5 and "1.250,00" -> 1.25).
export function mapExtractedMenu(extractedMenu: ExtractedMenu): MenuCategory[] {
  return extractedMenu.categories.map((cat) => ({
    name: cat.name,
    description: "",
    items: cat.items.map((item) => ({
      id: `temp-${Math.random().toString(36).slice(2)}`,
      name: item.name,
      description: item.description,
      price: asDollars(parseLocaleDecimal(item.price) || 0),
      currency: extractedMenu.currency.replace(/[^\w]/g, "") || "USD",
      image: item.image_url || "",
      images: item.image_url ? [item.image_url] : [],
      allergens: item.allergens || [],
      dietary_tags: item.dietary_tags || [],
      is_available: true,
      options:
        item.add_ons?.map((addon) => ({
          id: `addon-${Math.random().toString(36).slice(2)}`,
          name: addon.name,
          price_change: asDollars(parseLocaleDecimal(addon.price) || 0),
          is_required: false,
        })) || [],
    })),
  }));
}
