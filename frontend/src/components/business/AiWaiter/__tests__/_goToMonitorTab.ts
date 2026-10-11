import { act, screen } from "@testing-library/react";

/** Overview is the default landing tab; monitor content needs an explicit switch. */
export async function goToMonitorTab(): Promise<void> {
  const tab = await screen.findByRole("tab", { name: /tabs\.monitor/i });
  await act(async () => {
    tab.click();
  });
}
