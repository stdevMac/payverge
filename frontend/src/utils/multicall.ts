import { encodeFunctionData } from "viem";
import MultiCall3ABI from "@/artifacts/Multicall3.json";
import { waitForTransactionReceipt, writeContract } from "wagmi/actions";
import { CHAIN_ID } from "./network";
import { config } from "@/config";

export const multicall3Address = process.env.NEXT_PUBLIC_MULTICALL3_ADDRESS;

export interface ContractCall {
  address: string;
  abi: any;
  functionName: string;
  args: any[];
}

export interface EncodedCall {
  target: `0x${string}`;
  callData: `0x${string}`;
  allowFailure: boolean;
}

/**
 * Encode contract calls into Multicall3 format
 * @param contractCalls Array of contract calls to encode
 * @param allowFailure Whether to allow failure for the calls
 * @returns Encoded calls
 */
export const encodeMulticalls = (
  contractCalls: ContractCall[],
  allowFailure = false
): EncodedCall[] => {
  return contractCalls.map((call) => ({
    target: call.address as `0x${string}`,
    callData: encodeFunctionData({
      abi: call.abi,
      functionName: call.functionName,
      args: call.args,
    }),
    allowFailure,
  }));
};

/**
 * Execute multiple contract calls in a single transaction using Multicall3
 * @param contractCalls Array of contract calls to execute
 * @returns Transaction hash
 */
export const multicall = async (
  contractCalls: ContractCall[]
): Promise<`0x${string}`> => {
  console.log("contractCalls:", contractCalls);
  const encodedCalls = encodeMulticalls(contractCalls);

  const tx = await writeContract(config.wagmiConfig, {
    address: multicall3Address as `0x${string}`,
    abi: MultiCall3ABI,
    functionName: "aggregate3",
    args: [encodedCalls],
    chainId: CHAIN_ID,
  });

  const receipt = await waitForTransactionReceipt(config.wagmiConfig, {
    hash: tx,
    chainId: CHAIN_ID,
  });

  if (receipt.status !== "success") {
    throw new Error("Multicall transaction failed");
  }
  return tx;
};
