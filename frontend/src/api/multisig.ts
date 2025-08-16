import { axiosInstance } from "@/api";
import { AxiosError } from "axios";

export const MULTISIG_CALLER_ADDRESS = process.env
  .NEXT_PUBLIC_MULTISIG_CALLER_ADDRESS as `0x${string}`;

type MultisigTxResponse = {
  txId?: bigint;
} & Record<string, any>;
type NotFound = null;
type Error = undefined;

export const getLatestMultisigTransaction = async (): Promise<
  MultisigTxResponse | NotFound | Error
> => {
  try {
    const response =
      await axiosInstance.get<Record<string, any>>("/admin/multisig-tx");

    if (response.data["txId"]) {
      const { txId: txIdReceived, ...restOfResponseData } = response.data;
      if (response.data["txId"] !== "-1") {
        return { txId: BigInt(txIdReceived), ...restOfResponseData };
      }
      return restOfResponseData;
    }
    return response.data;
  } catch (error) {
    if (error instanceof AxiosError && error.response?.status === 404) {
      return null as NotFound;
    }
    throw error;
  }
};

export const putMultisigTransaction = async (data: any): Promise<void> => {
  try {
    await axiosInstance.put("/admin/multisig-tx", data);
  } catch (error) {
    console.error("Error putting multisig transaction:", error);
    throw error;
  }
};

export const patchMultisigTxId = async (txId: bigint): Promise<void> => {
  try {
    await axiosInstance.patch("/admin/multisig-tx", { txId: txId.toString() });
  } catch (error) {
    console.error("Error patching multisig transaction ID:", error);
    throw error;
  }
};

export const deleteMultisigTransaction = async (): Promise<void> => {
  try {
    await axiosInstance.delete("/admin/multisig-tx");
  } catch (error) {
    console.error("Error deleting multisig transaction:", error);
    throw error;
  }
};
