/**
 * Human countdown labels for payment/order windows.
 * mm:ss under an hour; "23h 25m" under a day; "2d 5h" beyond —
 * a multi-day "1378:23" (raw minutes:seconds) is unreadable.
 */
export function formatCountdownLabel(remainingMs: number): string {
  const totalMinutes = Math.floor(Math.max(0, remainingMs) / 60_000);
  if (totalMinutes >= 60 * 24) {
    const days = Math.floor(totalMinutes / (60 * 24));
    const hours = Math.floor((totalMinutes % (60 * 24)) / 60);
    return `${days}d ${hours}h`;
  }
  if (totalMinutes >= 60) {
    const hours = Math.floor(totalMinutes / 60);
    const minutes = totalMinutes % 60;
    return `${hours}h ${minutes}m`;
  }
  const seconds = Math.floor((Math.max(0, remainingMs) % 60_000) / 1_000);
  return `${String(totalMinutes).padStart(2, "0")}:${String(seconds).padStart(2, "0")}`;
}
