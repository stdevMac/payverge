import { base, baseSepolia } from "@reown/appkit/networks";
import { WagmiAdapter } from "@reown/appkit-adapter-wagmi";
import { getNetwork } from "../network";

export const projectId = process.env.NEXT_PUBLIC_WALLETCONNECT_PROJECT_ID || process.env.NEXT_PUBLIC_WALLET_CONNECT_PROJECT_ID || "dummy-project-id";

if (!process.env.NEXT_PUBLIC_WALLETCONNECT_PROJECT_ID && !process.env.NEXT_PUBLIC_WALLET_CONNECT_PROJECT_ID) {
  console.warn("NEXT_PUBLIC_WALLETCONNECT_PROJECT_ID is not defined, using dummy value for build");
}

export const metadata = {
  name: "Payverge",
  description:
    "Payverge, the future of crypto payments for hospitality!",
  url: "https://payverge.io",
  icons: ["/images/logo.svg"],
};

export const config = new WagmiAdapter({
  networks: [getNetwork()],
  projectId,
});
