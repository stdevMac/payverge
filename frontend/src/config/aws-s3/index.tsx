import { base, baseSepolia } from "@reown/appkit/networks";
import { WagmiAdapter } from "@reown/appkit-adapter-wagmi";
import { getNetwork } from "../network";

export const projectId = process.env.NEXT_PUBLIC_PROJECT_ID;

if (!projectId) throw new Error("Project ID is not defined");

export const metadata = {
  name: "Web3 Boilerplate",
  description:
    "Web3 Boilerplate - A comprehensive starter template for Web3 applications with authentication and wallet integration",
  url: "https://yourapp.com",
  icons: ["/images/logo.svg"],
};

export const config = new WagmiAdapter({
  networks: [getNetwork()],
  projectId,
});
