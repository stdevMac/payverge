import { config } from "@/config";
import { erc20Abi } from "viem";
import {
  readContract,
  waitForTransactionReceipt,
  writeContract,
} from "wagmi/actions";
import { CHAIN_ID } from "./network";

export const usdcAddress = process.env
  .NEXT_PUBLIC_USDC_ADDRESS as `0x${string}`;

export const checkAllowance = async (address: string, dealId: string) => {
  console.log("Checking allowance...");
  console.log("Address:", address);
  console.log("Deal ID:", dealId);

  const allowance = await readContract(config.wagmiConfig, {
    address: usdcAddress as `0x${string}`,
    abi: erc20Abi,
    functionName: "allowance",
    // @ts-ignore
    args: [address, dealId], // [owner, spender]
    chainId: CHAIN_ID,
  });

  console.log("Current allowance:", allowance.toString());
  return BigInt(allowance);
};

export const approveTokens = async (dealId: string, requiredAmount: bigint) => {
  console.log("Approving tokens...");
  const usdcAddress = process.env.NEXT_PUBLIC_USDC_ADDRESS;

  const approveTx = await writeContract(config.wagmiConfig, {
    address: usdcAddress as `0x${string}`,
    abi: erc20Abi,
    functionName: "approve",
    args: [dealId as `0x${string}`, requiredAmount], // [spender, amount]
    chainId: CHAIN_ID,
  });

  console.log("Approve transaction sent:", approveTx);

  // Wait for transaction confirmation
  const receipt = await waitForTransactionReceipt(config.wagmiConfig, {
    chainId: CHAIN_ID,
    hash: approveTx,
  });

  console.log("Approve transaction confirmed:", receipt);
  if (receipt.status !== "success") {
    throw new Error("Approval transaction failed.");
  }

  return receipt;
};

export const ensureApproval = async (
  address: string,
  dealId: string,
  requiredAmount: bigint
) => {
  try {
    const currentAllowance = await checkAllowance(address, dealId);

    console.log("Required amount:", requiredAmount);
    console.log("Current allowance:", currentAllowance);

    if (currentAllowance < requiredAmount) {
      console.log("Current allowance is insufficient. Approving tokens...");
      await approveTokens(dealId, requiredAmount);
    }
    return true;
  } catch (error) {
    console.error("Error in ensuring approval:", error);
    throw error;
  }
};
