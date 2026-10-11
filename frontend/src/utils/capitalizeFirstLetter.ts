/** Capitalize only the first visible letter. Leaves the rest of the string intact. */
export function capitalizeFirstLetter(value: string, locale?: string): string {
  if (!value) return value;
  const chars = Array.from(value);
  const index = chars.findIndex((char) => !/\s/u.test(char));
  if (index < 0) return value;
  chars[index] = chars[index].toLocaleUpperCase(locale);
  return chars.join("");
}
