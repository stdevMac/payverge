import { base, baseSepolia } from "@reown/appkit/networks";
import { WagmiAdapter } from "@reown/appkit-adapter-wagmi";
import { getNetwork } from "../network";

export const projectId = process.env.NEXT_PUBLIC_PROJECT_ID;

if (!projectId) throw new Error("Project ID is not defined");

export const metadata = {
  name: "Token Fleet",
  description:
    "Token Fleet - Revolutionary RWA platform for car rentals, allowing you to get profits from your car share",
  url: "https://app.tokenfleet.io",
  icons: ["/images/logo.svg"],
};

export const config = new WagmiAdapter({
  networks: [getNetwork()],
  projectId,
});
