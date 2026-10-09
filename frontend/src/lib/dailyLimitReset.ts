/** Picks the reset-window copy key and its {hours} value for a limit rejection. */
export function resetWindowCopy(resetsInSeconds: number): {
  key: "resets" | "resetsHour" | "resetsSoon";
  hours: number;
} {
  const hours = Math.round(resetsInSeconds / 3600);
  if (hours < 1) return { key: "resetsSoon", hours: 0 };
  if (hours === 1) return { key: "resetsHour", hours: 1 };
  return { key: "resets", hours };
}
