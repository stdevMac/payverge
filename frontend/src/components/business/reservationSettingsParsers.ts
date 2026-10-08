export const parseIntegerInput = (value: string, fallback: number) => {
  if (value.trim() === "") {
    return fallback;
  }

  const parsed = Number.parseInt(value, 10);
  return Number.isNaN(parsed) ? fallback : parsed;
};

export const parseFloatInput = (value: string, fallback: number) => {
  if (value.trim() === "") {
    return fallback;
  }

  const parsed = Number.parseFloat(value);
  return Number.isNaN(parsed) ? fallback : parsed;
};
