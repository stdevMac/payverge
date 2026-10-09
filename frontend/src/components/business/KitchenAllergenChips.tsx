import type { Locale } from "@/i18n/config";
import { kitchenAllergenLabel } from "@/utils/kitchenTicketCopy";

interface KitchenAllergenChipsProps {
  allergens: string[];
  locale: Locale;
  className?: string;
}

export function KitchenAllergenChips({
  allergens,
  locale,
  className = "mt-1 flex flex-wrap gap-1",
}: KitchenAllergenChipsProps) {
  if (allergens.length === 0) return null;
  return (
    <div className={className} data-testid="kitchen-allergen-chips">
      {allergens.map((token) => (
        <span
          key={token}
          data-testid="kitchen-allergen-chip"
          className="inline-flex items-center rounded-lg border border-rose-200 bg-rose-50 px-1.5 py-0.5 text-[10px] font-semibold text-rose-700"
        >
          {kitchenAllergenLabel(token, locale)}
        </span>
      ))}
    </div>
  );
}
