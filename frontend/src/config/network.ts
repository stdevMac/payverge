import { base, baseSepolia } from "@reown/appkit/networks";

export const getNetwork = () => {
  const network = process.env.NEXT_PUBLIC_NETWORK;
  return network === "base" ? base : baseSepolia;
};

export const getNetworkId = (): number => {
  return getNetwork().id;
};
