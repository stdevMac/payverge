import { useEffect, useState } from "react";
import { Button } from "@nextui-org/button";
import { Modal, ModalBody, ModalContent, ModalHeader } from "@nextui-org/modal";
import { Link } from "@nextui-org/link";
import { toast } from "react-hot-toast";
import { useAccount } from "wagmi";
import { writeContract, readContract } from "wagmi/actions";
import MultisigCallerABI from "../artifacts/MultisigCaller.json";
import { decodeFunctionData, encodeFunctionData } from "viem";
import { config } from "@/config";
import {
  getLatestMultisigTransaction,
  deleteMultisigTransaction,
  MULTISIG_CALLER_ADDRESS,
  patchMultisigTxId,
} from "@/api/multisig";
import { getExplorerUrl } from "@/utils/network";
import { User } from "@nextui-org/user";
import { waitForTransactionReceipt } from "wagmi/actions";
import { getNetworkId } from "@/config/network";

const CheckIcon = (props: { className?: string }) => (
  <svg
    xmlns="http://www.w3.org/2000/svg"
    width="24"
    height="24"
    viewBox="0 0 24 24"
    fill="none"
    stroke="currentColor"
    strokeWidth="2"
    strokeLinecap="round"
    strokeLinejoin="round"
    className={props.className}
  >
    <polyline points="20 6 9 17 4 12" />
  </svg>
);

const PendingIcon = (props: { className?: string }) => (
  <svg
    xmlns="http://www.w3.org/2000/svg"
    width="24"
    height="24"
    viewBox="0 0 24 24"
    fill="none"
    stroke="currentColor"
    strokeWidth="2"
    strokeLinecap="round"
    strokeLinejoin="round"
    className={props.className}
  >
    <circle cx="12" cy="12" r="10" />
    <polyline points="12 6 12 12 16 14" />
  </svg>
);

const LockIcon = (props: { className?: string }) => (
  <svg
    xmlns="http://www.w3.org/2000/svg"
    width="24"
    height="24"
    viewBox="0 0 24 24"
    fill="none"
    stroke="currentColor"
    strokeWidth="2"
    strokeLinecap="round"
    strokeLinejoin="round"
    className={props.className}
  >
    <rect x="3" y="11" width="18" height="11" rx="2" ry="2" />
    <path d="M7 11V7a5 5 0 0 1 10 0v4" />
  </svg>
);

function stringifyToJson(value: any) {
  return JSON.stringify(value, (_, value) =>
    typeof value === "bigint" ? value.toString() : value
  );
}

function getMultisigTransaction(txId: bigint) {
  return readContract(config.wagmiConfig, {
    address: MULTISIG_CALLER_ADDRESS,
    abi: MultisigCallerABI,
    functionName: "transactions",
    args: [txId],
  }) as Promise<Transaction>;
}

export async function isPendingMultisigTx() {
  const response = await getLatestMultisigTransaction();
  return response && typeof response.txId !== "undefined";
}

export const DONT_COMPARE_PARAMS = -1;

const isDifferentFunctionCall = ({
  abi,
  currentData,
  methodName,
  params,
  discriminatorParamIndex,
  targetAddress,
  value,
}: {
  abi: any;
  currentData: `0x${string}`;
  methodName: string;
  params: any[];
  discriminatorParamIndex?: number;
  targetAddress: string;
  value: bigint;
}) => {
  if (typeof discriminatorParamIndex !== "undefined") {
    const { functionName: decodedFunctionName, args: decodedArgs } =
      decodeFunctionData({
        abi,
        data: currentData,
      });
    const areArgsDifferent =
      discriminatorParamIndex === DONT_COMPARE_PARAMS
        ? false
        : decodedArgs[discriminatorParamIndex] !==
          params[discriminatorParamIndex];
    return decodedFunctionName !== methodName || areArgsDifferent;
  } else {
    const encodedData = encodeFunctionData({
      abi,
      functionName: methodName,
      args: params,
    });
    const isDifferent = currentData.toLowerCase() !== encodedData.toLowerCase();

    return isDifferent;
  }
};

function _isDifferentThanCurrentTx(
  target: {
    address: string;
    methodName: string;
    params: any[];
    abi: any[];
    discriminatorParamIndex?: number;
    value: bigint;
  },
  currentTx: Transaction
) {
  const {
    address: targetAddress,
    methodName,
    params,
    abi,
    discriminatorParamIndex,
    value,
  } = target;

  const [currentTarget, currentValue, currentData] = currentTx;
  if (
    targetAddress.toLowerCase() !== currentTarget.toLowerCase() ||
    value !== currentValue
  )
    return true;

  return isDifferentFunctionCall({
    abi,
    currentData,
    methodName,
    params,
    discriminatorParamIndex,
    targetAddress,
    value,
  });
}

type Target = `0x${string}`;
type Value = bigint;
type Data = `0x${string}`;
type Executed = boolean;
type Approvals = bigint;
type Transaction = [Target, Value, Data, Executed, Approvals];

interface MultisigTransactionProps {
  isOpen: boolean;
  onClose: () => void;
  targetAddress: string;
  targetAddressType?: string;
  methodName: string;
  methodType?: string;
  params: any[];
  abi: any[];
  discriminatorParamIndex?: number;
  value?: bigint;
  onExecuted?: (txHash: `0x${string}`) => void;
}

export const MultisigTransaction = ({
  isOpen,
  onClose,
  targetAddress,
  targetAddressType = "",
  methodName,
  methodType = "",
  params,
  abi,
  discriminatorParamIndex,
  value = BigInt(0),
  onExecuted,
}: MultisigTransactionProps) => {
  const { address } = useAccount();
  const chainId = getNetworkId();
  const [lastTxId, setLastTxId] = useState<bigint | undefined>(undefined);
  const [isLoading, setIsLoading] = useState(false);
  const [isLoadingTx, setIsLoadingTx] = useState(true);
  const [hasPendingTx, setHasPendingTx] = useState(false);
  const [hasApproved, setHasApproved] = useState(false);
  const [hasExecuted, setHasExecuted] = useState(false);
  const [requiredApprovals, setRequiredApprovals] = useState<bigint>();
  const [currentTx, setCurrentTx] = useState<Transaction | undefined>();

  useEffect(() => {
    const fetchCurrentTx = async () => {
      if (lastTxId === undefined) return;

      try {
        const tx = await getMultisigTransaction(lastTxId);
        setCurrentTx(tx);
      } catch (error) {
        const msg = "Error fetching current transaction";
        toast.error(msg);
      }
    };

    fetchCurrentTx();
  }, [lastTxId, hasApproved, hasExecuted]);

  const TARGET_IDX = 0;
  const VALUE_IDX = 1;
  const DATA_IDX = 2;
  const EXECUTED_IDX = 3;
  const APPROVAL_COUNT_IDX = 4;

  useEffect(() => {
    const fetchRequiredApprovals = async () => {
      try {
        const required = (await readContract(config.wagmiConfig, {
          address: MULTISIG_CALLER_ADDRESS,
          abi: MultisigCallerABI,
          functionName: "requiredApprovals",
        })) as bigint;
        setRequiredApprovals(required);
      } catch (error) {
        toast.error("Error fetching required approvals");
      }
    };

    fetchRequiredApprovals();
  }, []);

  useEffect(() => {
    const fetchUserApproval = async () => {
      if (lastTxId === undefined || !address) return;

      try {
        const approved = (await readContract(config.wagmiConfig, {
          address: MULTISIG_CALLER_ADDRESS,
          abi: MultisigCallerABI,
          functionName: "approvals",
          args: [lastTxId, address as `0x${string}`],
        })) as boolean;
        setHasApproved(approved);
      } catch (error) {
        toast.error("Error fetching user approval");
      }
    };

    fetchUserApproval();
  }, [lastTxId, address]);

  useEffect(() => {
    const checkExistingTransaction = async () => {
      try {
        setIsLoadingTx(true);
        const latestTx = await getLatestMultisigTransaction();
        if (latestTx?.txId) {
          setLastTxId(latestTx.txId);
        }
      } catch (error) {
        toast.error("Error checking existing transaction");
      } finally {
        setIsLoadingTx(false);
      }
    };

    checkExistingTransaction();
  }, []);

  useEffect(() => {
    const validateTransaction = async () => {
      if (!isOpen || !currentTx || typeof lastTxId !== "bigint" || isLoading)
        return;

      try {
        const isDifferentTx = _isDifferentThanCurrentTx(
          {
            address: targetAddress,
            methodName,
            params,
            abi,
            discriminatorParamIndex,
            value,
          },
          currentTx
        );
        setHasPendingTx(isDifferentTx);
        setIsLoadingTx(false);
      } catch (error) {
        toast.error("Error validating transaction data");
        setIsLoadingTx(false);
      }
    };

    validateTransaction();
  }, [
    currentTx,
    lastTxId,
    targetAddress,
    methodName,
    params,
    abi,
    value,
    isLoading,
    isOpen,
    discriminatorParamIndex,
  ]);

  const handleSubmitTransaction = async () => {
    if (!address) return;

    try {
      setIsLoading(true);
      const encodedData = encodeFunctionData({
        abi,
        functionName: methodName,
        args: params,
      });

      const submitTxHash = await writeContract(config.wagmiConfig, {
        address: MULTISIG_CALLER_ADDRESS,
        abi: MultisigCallerABI,
        functionName: "submitTransaction",
        args: [targetAddress, value, encodedData],
      });

      // Wait for transaction receipt
      const receipt = await waitForTransactionReceipt(config.wagmiConfig, {
        hash: submitTxHash,
        chainId: chainId,
      });

      if (!receipt) {
        throw new Error("Transaction failed");
      }

      // Find the TransactionSubmitted event
      const TRANSACTION_SUBMITTED_TOPIC =
        "0x3f6db45da5513bd4fe9f8a31e60ea668bdd4bca57e30a85a9a26f90c9fbcb56c";
      const submitEvent = receipt.logs.find(
        (log: any) => log.topics[0] === TRANSACTION_SUBMITTED_TOPIC
      );

      if (!submitEvent) {
        throw new Error("Could not find TransactionSubmitted event");
      }

      const txId = BigInt(submitEvent.topics[1]!);

      await patchMultisigTxId(txId);
      setLastTxId(txId);

      toast.success("Transaction submitted successfully");
    } catch (error: any) {
      toast.error(error.message || "Failed to submit transaction");
    } finally {
      setIsLoading(false);
    }
  };

  const handleApproveTransaction = async () => {
    if (!address || lastTxId === undefined) return;

    try {
      setIsLoading(true);
      const approveTxHash = await writeContract(config.wagmiConfig, {
        address: MULTISIG_CALLER_ADDRESS,
        abi: MultisigCallerABI,
        functionName: "approveTransaction",
        args: [lastTxId],
      });
      const receipt = await waitForTransactionReceipt(config.wagmiConfig, {
        hash: approveTxHash,
        chainId: chainId,
      });

      if (!receipt) {
        throw new Error("Transaction failed");
      }

      // Find the TransactionExecuted event
      const TRANSACTION_EXECUTED_TOPIC =
        "0x15ed165a284872ea7017f03df402a0cadfbfab588320ffaf83f160c2f82781c7";
      const executedEvent = receipt.logs.find(
        (log: any) => log.topics[0] === TRANSACTION_EXECUTED_TOPIC
      );
      let successMessagePrefix = "Transaction approved";
      if (executedEvent) {
        successMessagePrefix += " and executed";
        await deleteMultisigTransaction();
        setHasExecuted(true);
      } else {
        setHasApproved(true);
      }
      toast.success(`${successMessagePrefix} successfully`);

      if (executedEvent) onExecuted?.(approveTxHash);
    } catch (error: any) {
      toast.error(error.message || "Failed to approve transaction");
    } finally {
      setIsLoading(false);
    }
  };

  const content = isLoadingTx ? (
    <div>Loading transaction data...</div>
  ) : (
    <div className="space-y-4">
      <div className="space-y-2">
        <div className="space-y-1">
          <p className="font-semibold">Target Address:</p>
          <div className="flex items-center gap-2 pl-2">
            <Link
              href={getExplorerUrl(chainId, targetAddress)}
              target="_blank"
              rel="noopener noreferrer"
              className="hover:text-primary transition-colors"
            >
              {targetAddress}
            </Link>
            {targetAddressType && <span>({targetAddressType})</span>}
          </div>
        </div>
        <p>
          <span className="font-semibold">Method:</span>{" "}
          <span className="font-mono">{methodName}</span>
          {methodType && <span> ({methodType})</span>}
        </p>

        {(() => {
          try {
            let displayParams;
            if (typeof discriminatorParamIndex === "undefined") {
              displayParams = params;
            } else {
              const decoded = decodeFunctionData({
                abi,
                data: currentTx![DATA_IDX],
              });
              displayParams = decoded.args || {};
            }

            // Find the function in the ABI
            const functionAbi = abi.find(
              (item: any) =>
                item.type === "function" && item.name === methodName
            );

            return (
              <div>
                <div className="font-semibold">Parameters:</div>
                <div className="font-mono">
                  {Object.entries(displayParams).map(([index, value], i) => {
                    const paramName =
                      functionAbi?.inputs?.[Number(index)]?.name;
                    return (
                      <div key={i} className="break-all">
                        <span className="text-default-500">
                          {paramName || `param_${index}`}:
                        </span>{" "}
                        {stringifyToJson(value)}
                      </div>
                    );
                  })}
                </div>
              </div>
            );
          } catch (e) {
            return null;
          }
        })()}

        <div className="space-y-2">
          <div className="flex items-center justify-between">
            <span className="font-semibold">Approvals Progress:</span>
            <span className="text-sm">
              {Number(currentTx?.[APPROVAL_COUNT_IDX] ?? 0)}/
              {requiredApprovals ? Number(requiredApprovals) : "..."}
            </span>
          </div>
          <div className="flex gap-2 mb-3">
            {[...Array(Number(requiredApprovals ?? 0))].map((_, index) => {
              const isApproved =
                index < Number(currentTx?.[APPROVAL_COUNT_IDX] ?? 0);
              const isNext =
                index === Number(currentTx?.[APPROVAL_COUNT_IDX] ?? 0);
              return (
                <div
                  key={index}
                  className={`flex-1 h-12 rounded-lg border-2 transition-all duration-300 flex items-center justify-center
                    ${
                      isApproved
                        ? "border-success bg-success/20 shadow-md"
                        : isNext
                          ? "border-warning bg-warning/10 border-dashed animate-pulse"
                          : "border-default-300 bg-default-100"
                    }`}
                >
                  <User
                    name={`Approval ${index + 1}`}
                    avatarProps={{
                      icon: isApproved ? (
                        <CheckIcon className="text-success" />
                      ) : isNext ? (
                        <PendingIcon className="text-warning" />
                      ) : (
                        <LockIcon className="text-default-400" />
                      ),
                      classNames: {
                        base: "bg-transparent",
                      },
                    }}
                    classNames={{
                      name: `text-xs ${
                        isApproved
                          ? "text-success"
                          : isNext
                            ? "text-warning"
                            : "text-default-500"
                      }`,
                    }}
                  />
                </div>
              );
            })}
          </div>
          <div className="flex items-center gap-2 text-sm">
            <div
              className={`px-2 py-1 rounded-full ${
                currentTx?.[EXECUTED_IDX]
                  ? "bg-success/20 text-success"
                  : Number(currentTx?.[APPROVAL_COUNT_IDX] ?? 0) >=
                      Number(requiredApprovals ?? 0) - 1
                    ? "bg-warning/20 text-warning"
                    : "bg-primary/20 text-primary"
              }`}
            >
              {currentTx?.[EXECUTED_IDX]
                ? "Executed"
                : Number(currentTx?.[APPROVAL_COUNT_IDX] ?? 0) >=
                    Number(requiredApprovals ?? 0) - 1
                  ? "Ready for Final Approval"
                  : `${Number(requiredApprovals ?? 0) - Number(currentTx?.[APPROVAL_COUNT_IDX] ?? 0)} more approval${Number(requiredApprovals ?? 0) - Number(currentTx?.[APPROVAL_COUNT_IDX] ?? 0) !== 1 ? "s" : ""} needed`}
            </div>
          </div>
        </div>
      </div>

      {!currentTx?.[EXECUTED_IDX] &&
        Number(currentTx?.[APPROVAL_COUNT_IDX] ?? 0) === 0 && (
          <Button
            className="w-full"
            color="primary"
            isLoading={isLoading}
            onClick={handleSubmitTransaction}
            disabled={!address}
          >
            Submit and Approve Transaction
          </Button>
        )}

      {!currentTx?.[EXECUTED_IDX] &&
        Number(currentTx?.[APPROVAL_COUNT_IDX] ?? 0) > 0 && (
          <Button
            className="w-full mt-4"
            color="primary"
            isLoading={isLoading}
            onClick={handleApproveTransaction}
            disabled={!address}
          >
            {Number(currentTx?.[APPROVAL_COUNT_IDX] ?? 0) >=
            Number(requiredApprovals ?? 0) - 1
              ? "Approve and Execute Transaction"
              : "Approve Transaction"}
          </Button>
        )}
    </div>
  );

  return (
    <Modal
      isOpen={isOpen}
      onClose={onClose}
      size="2xl"
      classNames={{
        wrapper: "z-[1000]",
        base: "h-[95vh] sm:h-auto max-h-[95vh] overflow-hidden",
        body: "overflow-y-auto max-h-[calc(95vh-8rem)] pb-6",
        footer: "border-t border-gray-200",
      }}
    >
      <ModalContent>
        <ModalHeader>
          {isLoadingTx
            ? "Loading..."
            : hasPendingTx
              ? "Multisig Tx: Pending"
              : hasExecuted
                ? "Multisig Tx: Executed"
                : "Multisig Tx: Submit"}
        </ModalHeader>
        <ModalBody>
          {hasExecuted ? (
            <div className="space-y-4">
              <div className="p-4 bg-success/10 rounded-lg">
                <h3 className="font-semibold text-success mb-2">
                  Transaction Successfully Executed
                </h3>
                <p className="text-sm mb-4">
                  The transaction has been successfully approved and executed on
                  the blockchain.
                </p>
                <Button
                  color="success"
                  variant="light"
                  onPress={onClose}
                  className="w-full"
                >
                  Close
                </Button>
              </div>
            </div>
          ) : hasPendingTx ? (
            <div className="space-y-4">
              <div className="p-4 bg-warning/10 rounded-lg">
                <h3 className="font-semibold text-warning mb-2">
                  Different Transaction Pending
                </h3>
                <p className="text-sm mb-4">
                  There is already a pending transaction that needs to be signed
                  by other administrators. Please coordinate with other admins
                  to complete the existing transaction before submitting a new
                  one.
                </p>
                <div className="space-y-4 text-sm border-t border-warning/20 pt-4">
                  <h4 className="font-semibold mb-2">
                    Pending Transaction Details:
                  </h4>
                  <div>
                    <div className="text-default-500 mb-1">
                      ID in Multisig Contract:
                    </div>
                    <div className="font-mono">{lastTxId?.toString()}</div>
                  </div>
                  <div>
                    <div className="text-default-500 mb-1">Target Address:</div>
                    <div className="font-mono break-all">
                      <Link
                        href={getExplorerUrl(chainId, currentTx![TARGET_IDX])}
                        target="_blank"
                        rel="noopener noreferrer"
                        className="text-primary hover:opacity-80"
                      >
                        {currentTx?.[TARGET_IDX]}
                      </Link>
                    </div>
                  </div>
                  {currentTx?.[VALUE_IDX] &&
                    currentTx?.[VALUE_IDX] > BigInt(0) && (
                      <div>
                        <div className="text-default-500 mb-1">Value:</div>
                        <div className="font-mono">
                          {currentTx?.[VALUE_IDX].toString()} wei
                        </div>
                      </div>
                    )}
                  <div>
                    <div className="text-default-500 mb-1">Data:</div>
                    <div className="font-mono break-all">
                      {currentTx?.[DATA_IDX] &&
                        (() => {
                          try {
                            const decoded = decodeFunctionData({
                              abi,
                              data: currentTx[DATA_IDX],
                            });

                            // Find the function in the ABI
                            const functionAbi = abi.find(
                              (item: any) =>
                                item.type === "function" &&
                                item.name === decoded.functionName
                            );

                            return (
                              <div className="text-sm">
                                <div className="mb-1">
                                  Function: {decoded.functionName}
                                </div>
                                <div>Parameters:</div>
                                {Object.entries(decoded.args || {}).map(
                                  ([index, value], i) => {
                                    const paramName =
                                      functionAbi?.inputs?.[Number(index)]
                                        ?.name;
                                    return (
                                      <div key={i} className="pl-4">
                                        <span className="text-default-500">
                                          {paramName || `param_${index}`}:
                                        </span>{" "}
                                        {stringifyToJson(value)}
                                      </div>
                                    );
                                  }
                                )}
                              </div>
                            );
                          } catch (e) {
                            return currentTx[DATA_IDX];
                          }
                        })()}
                    </div>
                  </div>
                </div>
              </div>
              <Button
                color="warning"
                variant="light"
                onPress={onClose}
                className="w-full"
              >
                Close
              </Button>
            </div>
          ) : hasApproved ? (
            <div className="space-y-4">
              <div className="p-4 bg-warning/10 rounded-lg">
                <h3 className="font-semibold text-warning mb-2">
                  Already Signed
                </h3>
                <p className="text-sm mb-4">
                  You have already signed this transaction. Please coordinate
                  with other administrators to complete the transaction.
                </p>
                <div className="space-y-4 text-sm border-t border-warning/20 pt-4">
                  <h4 className="font-semibold mb-2">
                    Current Transaction Details:
                  </h4>
                  <div>
                    <div className="text-default-500 mb-1">
                      ID in Multisig Contract:
                    </div>
                    <div className="font-mono">{lastTxId?.toString()}</div>
                  </div>
                  <div>
                    <div className="text-default-500 mb-1">Target Address:</div>
                    <div className="font-mono break-all">
                      <Link
                        href={getExplorerUrl(chainId, targetAddress)}
                        target="_blank"
                        rel="noopener noreferrer"
                        className="text-primary hover:opacity-80"
                      >
                        {targetAddress}
                      </Link>
                      {targetAddressType && (
                        <span className="ml-2 text-xs text-default-400">
                          ({targetAddressType})
                        </span>
                      )}
                    </div>
                  </div>
                  <div>
                    <div className="text-default-500 mb-1">Method:</div>
                    <div className="font-mono">
                      {methodName}
                      {methodType && (
                        <span className="ml-2 text-xs text-default-400">
                          {" "}
                          ({methodType})
                        </span>
                      )}
                    </div>
                  </div>
                  {currentTx?.[DATA_IDX] &&
                    (() => {
                      try {
                        const decoded = decodeFunctionData({
                          abi,
                          data: currentTx[DATA_IDX],
                        });

                        // Find the function in the ABI
                        const functionAbi = abi.find(
                          (item: any) =>
                            item.type === "function" &&
                            item.name === decoded.functionName
                        );

                        return (
                          <div>
                            <div className="text-default-500 mb-1">
                              Parameters:
                            </div>
                            <div className="font-mono">
                              {Object.entries(decoded.args || {}).map(
                                ([index, value], i) => {
                                  const paramName =
                                    functionAbi?.inputs?.[Number(index)]?.name;
                                  return (
                                    <div key={i} className="break-all">
                                      <span className="text-default-500">
                                        {paramName || `param_${index}`}:
                                      </span>{" "}
                                      {stringifyToJson(value)}
                                    </div>
                                  );
                                }
                              )}
                            </div>
                          </div>
                        );
                      } catch (e) {
                        return null;
                      }
                    })()}
                  {value && value > BigInt(0) && (
                    <div>
                      <div className="text-default-500 mb-1">Value:</div>
                      <div className="font-mono">{value.toString()} wei</div>
                    </div>
                  )}
                </div>
              </div>
              <Button
                color="warning"
                variant="light"
                onPress={onClose}
                className="w-full"
              >
                Close
              </Button>
            </div>
          ) : (
            <div className="space-y-4">{content}</div>
          )}
        </ModalBody>
      </ModalContent>
    </Modal>
  );
};
