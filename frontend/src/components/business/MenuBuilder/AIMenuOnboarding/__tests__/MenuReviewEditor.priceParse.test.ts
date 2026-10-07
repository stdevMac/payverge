import { mapExtractedMenu } from '../MenuReviewEditor.transform';
import type { ExtractedMenu } from '@/api/business';

const menu = (price: string, addon: string): ExtractedMenu =>
  ({
    restaurant_name: 'R',
    currency: 'USD',
    categories: [
      {
        name: 'C',
        items: [
          {
            name: 'Item',
            description: '',
            category: 'C',
            allergens: [],
            price,
            add_ons: [{ name: 'Extra', price: addon }],
          },
        ],
      },
    ],
  }) as unknown as ExtractedMenu;

describe('mapExtractedMenu price parsing', () => {
  it('parses symbol/comma/thousands prices correctly', () => {
    // mapExtractedMenu returns MenuCategory[]; index [0] for the single category.
    expect(mapExtractedMenu(menu('$10', '0'))[0].items[0].price).toBe(10);
    expect(mapExtractedMenu(menu('5,50', '0'))[0].items[0].price).toBe(5.5);
    expect(mapExtractedMenu(menu('€8,90', '0'))[0].items[0].price).toBe(8.9);
    expect(mapExtractedMenu(menu('1.250,00', '0'))[0].items[0].price).toBe(1250);
  });

  it('parses add-on price changes', () => {
    expect(
      mapExtractedMenu(menu('0', '2,50'))[0].items[0].options![0].price_change,
    ).toBe(2.5);
  });

  it('retains extracted dietary tags for sanitization review', () => {
    const extracted = menu('10', '0');
    extracted.categories[0].items[0].dietary_tags = ['vegan', 'keto'];
    expect(mapExtractedMenu(extracted)[0].items[0].dietary_tags).toEqual([
      'vegan',
      'keto',
    ]);
  });
});
