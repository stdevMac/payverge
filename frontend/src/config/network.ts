import { base, baseSepolia } from "@/config/chains";
import { getPublicConfig } from "@/config/publicConfig";

export const getNetwork = () => {
  return getPublicConfig().network === "base" ? base : baseSepolia;
};
