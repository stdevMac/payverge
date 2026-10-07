export function deliveryStatusAnnouncement(
  status: string,
  tString: (key: string, vars?: Record<string, string | number>) => string,
): string {
  if (status === "picked_up") {
    return tString("deliveryTracking.courierPickedUpAnnouncement");
  }
  if (!status) return "";
  return tString(`deliveryTracking.steps.${status}`);
}
