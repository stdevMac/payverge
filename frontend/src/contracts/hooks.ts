// Wagmi hooks for Payverge ecosystem contract interactions
import { useReadContract, useWriteContract, useWatchContractEvent } from 'wagmi';
import { useChainId, useAccount } from 'wagmi';
import { Address, parseUnits, formatUnits } from 'viem';
import { PAYVERGE_PAYMENTS_ABI, PAYVERGE_REFERRALS_ABI, PAYVERGE_PROFIT_SPLIT_ABI } from './abi';
import { getContractConfig } from './config';
import {
  Bill,
  Payment,
  BusinessInfo,
  CreateBillParams,
  ProcessPaymentParams,
  RegisterBusinessParams,
  ClaimableAmounts,
  Referrer,
  ReferralRecord,
  RegisterReferrerParams,
  Beneficiary,
  Distribution,
  AddBeneficiaryParams,
  DistributeProfitsParams
} from './types';

// Hook to get contract configuration for current chain
export const useContractConfig = () => {
  const chainId = useChainId();
  return getContractConfig(chainId);
};

// Read hooks
export const useBill = (billId: string) => {
  const config = useContractConfig();

  return useReadContract({
    address: config.payments,
    abi: PAYVERGE_PAYMENTS_ABI,
    functionName: 'getBill',
    args: [billId as `0x${string}`],
  });
};

export const useBillPayments = (billId: string) => {
  const config = useContractConfig();

  return useReadContract({
    address: config.payments,
    abi: PAYVERGE_PAYMENTS_ABI,
    functionName: 'getBillPayments',
    args: [billId as `0x${string}`],
  });
};

export const useBusinessInfo = (businessAddress: Address) => {
  const config = useContractConfig();

  return useReadContract({
    address: config.payments,
    abi: PAYVERGE_PAYMENTS_ABI,
    functionName: 'businessInfo',
    args: [businessAddress],
  });
};

export const useBusinessBillCount = (businessAddress: Address) => {
  const config = useContractConfig();

  return useReadContract({
    address: config.payments,
    abi: PAYVERGE_PAYMENTS_ABI,
    functionName: 'businessBillCount',
    args: [businessAddress],
  });
};

export const useGetClaimableAmounts = (businessAddress: Address, tippingAddress: Address) => {
  const config = useContractConfig();

  return useReadContract({
    address: config.payments,
    abi: PAYVERGE_PAYMENTS_ABI,
    functionName: 'getClaimableAmounts',
    args: [businessAddress, tippingAddress],
  });
};

export const useClaimableBalance = (businessAddress: Address) => {
  const config = useContractConfig();

  return useReadContract({
    address: config.payments,
    abi: PAYVERGE_PAYMENTS_ABI,
    functionName: 'claimableBalance',
    args: [businessAddress],
  });
};

export const usePlatformFeeRate = () => {
  const config = useContractConfig();

  return useReadContract({
    address: config.payments,
    abi: PAYVERGE_PAYMENTS_ABI,
    functionName: 'platformFeeRate',
  });
};

export const useUsdcToken = () => {
  const config = useContractConfig();

  return useReadContract({
    address: config.payments,
    abi: PAYVERGE_PAYMENTS_ABI,
    functionName: 'usdcToken',
  });
};

export const useDailyPaymentLimit = (userAddress?: Address) => {
  const config = useContractConfig();
  const { address } = useAccount();
  const targetAddress = userAddress || address;

  return useReadContract({
    address: config.payments,
    abi: PAYVERGE_PAYMENTS_ABI,
    functionName: 'getDailyPaymentLimit',
    args: targetAddress ? [targetAddress] : undefined,
    query: {
      enabled: !!targetAddress,
    },
  });
};

export const useRemainingDailyLimit = (userAddress?: Address) => {
  const config = useContractConfig();
  const { address } = useAccount();
  const targetAddress = userAddress || address;

  return useReadContract({
    address: config.payments,
    abi: PAYVERGE_PAYMENTS_ABI,
    functionName: 'getRemainingDailyLimit',
    args: targetAddress ? [targetAddress] : undefined,
    query: {
      enabled: !!targetAddress,
    },
  });
};

// Write hooks

export const useProcessPayment = () => {
  const config = useContractConfig();
  const { writeContractAsync } = useWriteContract();

  const processPayment = async (params: ProcessPaymentParams) => {
    return await writeContractAsync({
      address: config.payments,
      abi: PAYVERGE_PAYMENTS_ABI,
      functionName: 'processPayment',
      args: [
        params.billId as `0x${string}`,
        params.amount,
        params.tipAmount,
      ],
    });
  };

  return { processPayment };
};

export const useRegisterBusiness = () => {
  const config = useContractConfig();
  const { writeContractAsync } = useWriteContract();

  const registerBusiness = async (params: RegisterBusinessParams) => {
    const hash = await writeContractAsync({
      address: config.payments,
      abi: PAYVERGE_PAYMENTS_ABI,
      functionName: 'registerBusiness',
      args: [
        params.name,
        params.paymentAddress,
        params.tippingAddress,
        params.referralCode || '', // Always provide referralCode, empty string if not provided
      ],
    });

    return hash;
  };

  return { registerBusiness };
};

export const useUpdateBusinessPaymentAddress = () => {
  const config = useContractConfig();
  const { writeContract } = useWriteContract();

  const updatePaymentAddress = (newPaymentAddress: Address) => {
    return writeContract({
      address: config.payments,
      abi: PAYVERGE_PAYMENTS_ABI,
      functionName: 'updateBusinessPaymentAddress',
      args: [newPaymentAddress],
    });
  };

  return { updatePaymentAddress };
};

export const useUpdateBusinessTippingAddress = () => {
  const config = useContractConfig();
  const { writeContract } = useWriteContract();

  const updateTippingAddress = (newTippingAddress: Address) => {
    return writeContract({
      address: config.payments,
      abi: PAYVERGE_PAYMENTS_ABI,
      functionName: 'updateBusinessTippingAddress',
      args: [newTippingAddress],
    });
  };

  return { updateTippingAddress };
};

export const useClaimEarnings = () => {
  const config = useContractConfig();
  const { writeContractAsync } = useWriteContract();

  const claimEarnings = async () => {
    return await writeContractAsync({
      address: config.payments,
      abi: PAYVERGE_PAYMENTS_ABI,
      functionName: 'claimEarnings',
    });
  };

  return { claimEarnings };
};

export const useCreateBill = () => {
  const config = useContractConfig();
  const { writeContract } = useWriteContract();

  const createBill = (params: CreateBillParams) => {
    return writeContract({
      address: config.payments,
      abi: PAYVERGE_PAYMENTS_ABI,
      functionName: 'createBill',
      args: [
        params.billId as `0x${string}`,
        params.businessAddress,
        params.totalAmount,
        params.metadata,
        params.nonce as `0x${string}`,
      ],
    });
  };

  return { createBill };
};

// Event watching hooks
export const useWatchBillCreated = (
  onBillCreated: (log: any) => void,
  businessAddress?: Address
) => {
  const config = useContractConfig();

  return useWatchContractEvent({
    address: config.payments,
    abi: PAYVERGE_PAYMENTS_ABI,
    eventName: 'BillCreated',
    args: businessAddress ? { businessAddress } : undefined,
    onLogs: onBillCreated,
  });
};

export const useWatchPaymentProcessed = (
  onPaymentProcessed: (log: any) => void,
  billId?: string
) => {
  const config = useContractConfig();

  return useWatchContractEvent({
    address: config.payments,
    abi: PAYVERGE_PAYMENTS_ABI,
    eventName: 'PaymentProcessed',
    args: billId ? { billId: billId as `0x${string}` } : undefined,
    onLogs: onPaymentProcessed,
  });
};

export const useWatchBusinessRegistered = (
  onBusinessRegistered: (log: any) => void,
  businessAddress?: Address
) => {
  const config = useContractConfig();

  return useWatchContractEvent({
    address: config.payments,
    abi: PAYVERGE_PAYMENTS_ABI,
    eventName: 'BusinessRegistered',
    args: businessAddress ? { businessAddress } : undefined,
    onLogs: onBusinessRegistered,
  });
};

// Utility hooks for USDC interactions
export const useUsdcBalance = (address?: Address) => {
  const config = useContractConfig();
  const { address: connectedAddress } = useAccount();
  const targetAddress = address || connectedAddress;

  return useReadContract({
    address: config.usdcAddress,
    abi: [
      {
        name: 'balanceOf',
        type: 'function',
        stateMutability: 'view',
        inputs: [{ name: 'account', type: 'address' }],
        outputs: [{ name: '', type: 'uint256' }],
      },
    ],
    functionName: 'balanceOf',
    args: targetAddress ? [targetAddress] : undefined,
    query: {
      enabled: !!targetAddress,
    },
  });
};

export const useUsdcAllowance = (spender?: Address) => {
  const config = useContractConfig();
  const { address } = useAccount();
  const spenderAddress = spender || config.payments;

  return useReadContract({
    address: config.usdcAddress,
    abi: [
      {
        name: 'allowance',
        type: 'function',
        stateMutability: 'view',
        inputs: [
          { name: 'owner', type: 'address' },
          { name: 'spender', type: 'address' },
        ],
        outputs: [{ name: '', type: 'uint256' }],
      },
    ],
    functionName: 'allowance',
    args: address ? [address, spenderAddress] : undefined,
    query: {
      enabled: !!address,
    },
  });
};

export const useApproveUsdc = () => {
  const config = useContractConfig();
  const { writeContractAsync } = useWriteContract();

  const approveUsdc = async (amount: bigint, spender?: Address) => {
    const spenderAddress = spender || config.payments;

    const hash = await writeContractAsync({
      address: config.usdcAddress,
      abi: [
        {
          name: 'approve',
          type: 'function',
          stateMutability: 'nonpayable',
          inputs: [
            { name: 'spender', type: 'address' },
            { name: 'amount', type: 'uint256' },
          ],
          outputs: [{ name: '', type: 'bool' }],
        },
      ],
      functionName: 'approve',
      args: [spenderAddress, amount],
    });

    return hash;
  };

  return { approveUsdc };
};

// Utility functions for amount formatting
export const formatUsdcAmount = (amount: bigint): string => {
  return formatUnits(amount, 6); // USDC has 6 decimals
};

export const parseUsdcAmount = (amount: string): bigint => {
  return parseUnits(amount, 6); // USDC has 6 decimals
};

// New hooks for unified payment system features

export const useBillParticipants = (billId: string) => {
  const config = useContractConfig();

  return useReadContract({
    address: config.payments,
    abi: PAYVERGE_PAYMENTS_ABI,
    functionName: 'getBillParticipants',
    args: [billId as `0x${string}`],
  });
};

export const useParticipantInfo = (billId: string, participant: Address) => {
  const config = useContractConfig();

  return useReadContract({
    address: config.payments,
    abi: PAYVERGE_PAYMENTS_ABI,
    functionName: 'getParticipantInfo',
    args: [billId as `0x${string}`, participant],
  });
};

export const useHasParticipated = (billId: string, participant: Address) => {
  const config = useContractConfig();

  return useReadContract({
    address: config.payments,
    abi: PAYVERGE_PAYMENTS_ABI,
    functionName: 'hasParticipatedInBill',
    args: [billId as `0x${string}`, participant],
  });
};

export const useBillSummary = (billId: string) => {
  const config = useContractConfig();

  return useReadContract({
    address: config.payments,
    abi: PAYVERGE_PAYMENTS_ABI,
    functionName: 'getBillSummary',
    args: [billId as `0x${string}`],
  });
};

export const useRegistrationFee = () => {
  const config = useContractConfig();

  return useReadContract({
    address: config.payments,
    abi: PAYVERGE_PAYMENTS_ABI,
    functionName: 'getRegistrationFee',
  });
};

// ============ PayvergeReferrals Hooks ============

// Read hooks for referrals
export const useReferrer = (referrerAddress: Address) => {
  const config = useContractConfig();

  return useReadContract({
    address: config.referrals,
    abi: PAYVERGE_REFERRALS_ABI,
    functionName: 'getReferrer',
    args: [referrerAddress],
  });
};

export const useReferralCodeAvailability = (referralCode: string) => {
  const config = useContractConfig();

  return useReadContract({
    address: config.referrals,
    abi: PAYVERGE_REFERRALS_ABI,
    functionName: 'isReferralCodeAvailable',
    args: [referralCode],
  });
};

export const useReferralRecords = (referrerAddress: Address) => {
  const config = useContractConfig();

  return useReadContract({
    address: config.referrals,
    abi: PAYVERGE_REFERRALS_ABI,
    functionName: 'getReferralRecords',
    args: [referrerAddress],
  });
};

export const useTotalReferrers = () => {
  const config = useContractConfig();

  return useReadContract({
    address: config.referrals,
    abi: PAYVERGE_REFERRALS_ABI,
    functionName: 'totalReferrers',
  });
};

export const useBasicReferrerFee = () => {
  const config = useContractConfig();

  return useReadContract({
    address: config.referrals,
    abi: PAYVERGE_REFERRALS_ABI,
    functionName: 'basicReferrerFee',
  });
};

export const usePremiumReferrerFee = () => {
  const config = useContractConfig();

  return useReadContract({
    address: config.referrals,
    abi: PAYVERGE_REFERRALS_ABI,
    functionName: 'premiumReferrerFee',
  });
};

// Write hooks for referrals
export const useRegisterBasicReferrer = () => {
  const config = useContractConfig();
  const { writeContract } = useWriteContract();

  const registerBasicReferrer = (referralCode: string) => {
    return writeContract({
      address: config.referrals,
      abi: PAYVERGE_REFERRALS_ABI,
      functionName: 'registerBasicReferrer',
      args: [referralCode],
    });
  };

  return { registerBasicReferrer };
};

export const useRegisterPremiumReferrer = () => {
  const config = useContractConfig();
  const { writeContract } = useWriteContract();

  const registerPremiumReferrer = (referralCode: string) => {
    return writeContract({
      address: config.referrals,
      abi: PAYVERGE_REFERRALS_ABI,
      functionName: 'registerPremiumReferrer',
      args: [referralCode],
    });
  };

  return { registerPremiumReferrer };
};

export const useUpgradeToPremium = () => {
  const config = useContractConfig();
  const { writeContract } = useWriteContract();

  const upgradeToPremium = () => {
    return writeContract({
      address: config.referrals,
      abi: PAYVERGE_REFERRALS_ABI,
      functionName: 'upgradeToPremium',
    });
  };

  return { upgradeToPremium };
};

export const useUpdateReferralCode = () => {
  const config = useContractConfig();
  const { writeContract } = useWriteContract();

  const updateReferralCode = (newReferralCode: string) => {
    return writeContract({
      address: config.referrals,
      abi: PAYVERGE_REFERRALS_ABI,
      functionName: 'updateReferralCode',
      args: [newReferralCode],
    });
  };

  return { updateReferralCode };
};

export const useClaimCommissions = () => {
  const config = useContractConfig();
  const { writeContract } = useWriteContract();

  const claimCommissions = () => {
    return writeContract({
      address: config.referrals,
      abi: PAYVERGE_REFERRALS_ABI,
      functionName: 'claimCommissions',
    });
  };

  return { claimCommissions };
};

export const useDeactivateReferrer = () => {
  const config = useContractConfig();
  const { writeContract } = useWriteContract();

  const deactivateReferrer = (referrerAddress: Address) => {
    return writeContract({
      address: config.referrals,
      abi: PAYVERGE_REFERRALS_ABI,
      functionName: 'deactivateReferrer',
      args: [referrerAddress],
    });
  };

  return { deactivateReferrer };
};

// Event watching hooks for referrals
export const useWatchReferrerRegistered = (
  onReferrerRegistered: (log: any) => void,
  referrerAddress?: Address
) => {
  const config = useContractConfig();

  return useWatchContractEvent({
    address: config.referrals,
    abi: PAYVERGE_REFERRALS_ABI,
    eventName: 'ReferrerRegistered',
    args: referrerAddress ? { referrer: referrerAddress } : undefined,
    onLogs: onReferrerRegistered,
  });
};

export const useWatchReferralProcessed = (
  onReferralProcessed: (log: any) => void,
  referrerAddress?: Address
) => {
  const config = useContractConfig();

  return useWatchContractEvent({
    address: config.referrals,
    abi: PAYVERGE_REFERRALS_ABI,
    eventName: 'ReferralProcessed',
    args: referrerAddress ? { referrer: referrerAddress } : undefined,
    onLogs: onReferralProcessed,
  });
};

export const useWatchCommissionClaimed = (
  onCommissionClaimed: (log: any) => void,
  referrerAddress?: Address
) => {
  const config = useContractConfig();

  return useWatchContractEvent({
    address: config.referrals,
    abi: PAYVERGE_REFERRALS_ABI,
    eventName: 'CommissionClaimed',
    args: referrerAddress ? { referrer: referrerAddress } : undefined,
    onLogs: onCommissionClaimed,
  });
};

// ============ PayvergeProfitSplit Hooks ============

// Read hooks for profit split
export const useBeneficiary = (beneficiaryAddress: Address) => {
  const config = useContractConfig();

  return useReadContract({
    address: config.profitSplit,
    abi: PAYVERGE_PROFIT_SPLIT_ABI,
    functionName: 'getBeneficiary',
    args: [beneficiaryAddress],
  });
};

export const useBeneficiaryCount = () => {
  const config = useContractConfig();
  
  return useReadContract({
    address: config.profitSplit,
    abi: PAYVERGE_PROFIT_SPLIT_ABI,
    functionName: 'beneficiaryCount',
  });
};

export const useDistributionStats = () => {
  const config = useContractConfig();

  return useReadContract({
    address: config.profitSplit,
    abi: PAYVERGE_PROFIT_SPLIT_ABI,
    functionName: 'getDistributionStats',
  });
};

export const useCalculatePayouts = (amount: bigint) => {
  const config = useContractConfig();

  return useReadContract({
    address: config.profitSplit,
    abi: PAYVERGE_PROFIT_SPLIT_ABI,
    functionName: 'calculatePayouts',
    args: [amount],
    query: {
      enabled: amount > BigInt(0),
    },
  });
};

// Write hooks for profit split
export const useAddBeneficiary = () => {
  const config = useContractConfig();
  const { writeContract } = useWriteContract();

  const addBeneficiary = (beneficiary: Address, name: string, percentage: number) => {
    return writeContract({
      address: config.profitSplit,
      abi: PAYVERGE_PROFIT_SPLIT_ABI,
      functionName: 'addBeneficiary',
      args: [beneficiary, name, percentage],
    });
  };

  return { addBeneficiary };
};

export const useUpdateBeneficiaryPercentage = () => {
  const config = useContractConfig();
  const { writeContract } = useWriteContract();

  const updateBeneficiaryPercentage = (beneficiary: Address, newPercentage: number) => {
    return writeContract({
      address: config.profitSplit,
      abi: PAYVERGE_PROFIT_SPLIT_ABI,
      functionName: 'updateBeneficiaryPercentage',
      args: [beneficiary, newPercentage],
    });
  };

  return { updateBeneficiaryPercentage };
};

export const useRemoveBeneficiary = () => {
  const config = useContractConfig();
  const { writeContract } = useWriteContract();

  const removeBeneficiary = (beneficiary: Address) => {
    return writeContract({
      address: config.profitSplit,
      abi: PAYVERGE_PROFIT_SPLIT_ABI,
      functionName: 'removeBeneficiary',
      args: [beneficiary],
    });
  };

  return { removeBeneficiary };
};

export const useDistributeProfits = () => {
  const config = useContractConfig();
  const { writeContract } = useWriteContract();

  const distributeProfits = (amount: bigint) => {
    return writeContract({
      address: config.profitSplit,
      abi: PAYVERGE_PROFIT_SPLIT_ABI,
      functionName: 'distributeProfits',
      args: [amount],
    });
  };

  return { distributeProfits };
};

export const useDistributeAllProfits = () => {
  const config = useContractConfig();
  const { writeContract } = useWriteContract();

  const distributeAllProfits = () => {
    return writeContract({
      address: config.profitSplit,
      abi: PAYVERGE_PROFIT_SPLIT_ABI,
      functionName: 'distributeAllProfits',
    });
  };

  return { distributeAllProfits };
};

export const useDepositForDistribution = () => {
  const config = useContractConfig();
  const { writeContract } = useWriteContract();

  const depositForDistribution = (amount: bigint) => {
    return writeContract({
      address: config.profitSplit,
      abi: PAYVERGE_PROFIT_SPLIT_ABI,
      functionName: 'depositForDistribution',
      args: [amount],
    });
  };

  return { depositForDistribution };
};

export const useGrantDistributorRole = () => {
  const config = useContractConfig();
  const { writeContract } = useWriteContract();

  const grantDistributorRole = (account: Address) => {
    return writeContract({
      address: config.profitSplit,
      abi: PAYVERGE_PROFIT_SPLIT_ABI,
      functionName: 'grantDistributorRole',
      args: [account],
    });
  };

  return { grantDistributorRole };
};

export const useRevokeDistributorRole = () => {
  const config = useContractConfig();
  const { writeContract } = useWriteContract();

  const revokeDistributorRole = (account: Address) => {
    return writeContract({
      address: config.profitSplit,
      abi: PAYVERGE_PROFIT_SPLIT_ABI,
      functionName: 'revokeDistributorRole',
      args: [account],
    });
  };

  return { revokeDistributorRole };
};

// Event watching hooks for profit split
export const useWatchBeneficiaryAdded = (
  onBeneficiaryAdded: (log: any) => void,
  beneficiaryAddress?: Address
) => {
  const config = useContractConfig();

  return useWatchContractEvent({
    address: config.profitSplit,
    abi: PAYVERGE_PROFIT_SPLIT_ABI,
    eventName: 'BeneficiaryAdded',
    args: beneficiaryAddress ? { beneficiary: beneficiaryAddress } : undefined,
    onLogs: onBeneficiaryAdded,
  });
};

export const useWatchProfitDistributed = (
  onProfitDistributed: (log: any) => void
) => {
  const config = useContractConfig();

  return useWatchContractEvent({
    address: config.profitSplit,
    abi: PAYVERGE_PROFIT_SPLIT_ABI,
    eventName: 'ProfitDistributed',
    onLogs: onProfitDistributed,
  });
};

export const useWatchBeneficiaryPayout = (
  onBeneficiaryPayout: (log: any) => void,
  beneficiaryAddress?: Address
) => {
  const config = useContractConfig();

  return useWatchContractEvent({
    address: config.profitSplit,
    abi: PAYVERGE_PROFIT_SPLIT_ABI,
    eventName: 'BeneficiaryPayout',
    args: beneficiaryAddress ? { beneficiary: beneficiaryAddress } : undefined,
    onLogs: onBeneficiaryPayout,
  });
};

// ============ Utility Functions for All Contracts ============

// USDC approval helpers for different contracts
export const useApproveUsdcForReferrals = () => {
  const config = useContractConfig();
  const { writeContract } = useWriteContract();

  const approveUsdcForReferrals = (amount: bigint) => {
    return writeContract({
      address: config.usdcAddress,
      abi: [
        {
          name: 'approve',
          type: 'function',
          stateMutability: 'nonpayable',
          inputs: [
            { name: 'spender', type: 'address' },
            { name: 'amount', type: 'uint256' },
          ],
          outputs: [{ name: '', type: 'bool' }],
        },
      ],
      functionName: 'approve',
      args: [config.referrals, amount],
    });
  };

  return { approveUsdcForReferrals };
};

export const useApproveUsdcForProfitSplit = () => {
  const config = useContractConfig();
  const { writeContract } = useWriteContract();

  const approveUsdcForProfitSplit = (amount: bigint) => {
    return writeContract({
      address: config.usdcAddress,
      abi: [
        {
          name: 'approve',
          type: 'function',
          stateMutability: 'nonpayable',
          inputs: [
            { name: 'spender', type: 'address' },
            { name: 'amount', type: 'uint256' },
          ],
          outputs: [{ name: '', type: 'bool' }],
        },
      ],
      functionName: 'approve',
      args: [config.profitSplit, amount],
    });
  };

  return { approveUsdcForProfitSplit };
};

// Check USDC allowances for different contracts
export const useUsdcAllowanceForReferrals = () => {
  const config = useContractConfig();
  const { address } = useAccount();

  return useReadContract({
    address: config.usdcAddress,
    abi: [
      {
        name: 'allowance',
        type: 'function',
        stateMutability: 'view',
        inputs: [
          { name: 'owner', type: 'address' },
          { name: 'spender', type: 'address' },
        ],
        outputs: [{ name: '', type: 'uint256' }],
      },
    ],
    functionName: 'allowance',
    args: address ? [address, config.referrals] : undefined,
    query: {
      enabled: !!address,
    },
  });
};

export const useUsdcAllowanceForProfitSplit = () => {
  const config = useContractConfig();
  const { address } = useAccount();

  return useReadContract({
    address: config.usdcAddress,
    abi: [
      {
        name: 'allowance',
        type: 'function',
        stateMutability: 'view',
        inputs: [
          { name: 'owner', type: 'address' },
          { name: 'spender', type: 'address' },
        ],
        outputs: [{ name: '', type: 'uint256' }],
      },
    ],
    functionName: 'allowance',
    args: address ? [address, config.profitSplit] : undefined,
    query: {
      enabled: !!address,
    },
  });
};


// ============ Admin Dashboard Hooks ============

// Get platform fee rate (this exists in the contract)
export const useCurrentPlatformFeeRate = () => {
  const config = useContractConfig();

  return useReadContract({
    address: config.payments,
    abi: [
      {
        name: 'platformFeeRate',
        type: 'function',
        stateMutability: 'view',
        inputs: [],
        outputs: [{ name: '', type: 'uint256' }],
      },
    ],
    functionName: 'platformFeeRate',
  });
};

// Get current registration fee (this exists in the contract)
export const useCurrentRegistrationFee = () => {
  const config = useContractConfig();

  return useReadContract({
    address: config.payments,
    abi: [
      {
        name: 'getRegistrationFee',
        type: 'function',
        stateMutability: 'view',
        inputs: [],
        outputs: [{ name: '', type: 'uint256' }],
      },
    ],
    functionName: 'getRegistrationFee',
  });
};

// Get total referrers from referral contract (this exists in the contract)
export const useTotalReferrersOnChain = () => {
  const config = useContractConfig();

  return useReadContract({
    address: config.referrals,
    abi: [
      {
        name: 'totalReferrers',
        type: 'function',
        stateMutability: 'view',
        inputs: [],
        outputs: [{ name: '', type: 'uint256' }],
      },
    ],
    functionName: 'totalReferrers',
  });
};

// Get total distributed from profit split contract (this exists in the contract)
export const useTotalDistributed = () => {
  const config = useContractConfig();

  return useReadContract({
    address: config.profitSplit,
    abi: [
      {
        name: 'totalDistributed',
        type: 'function',
        stateMutability: 'view',
        inputs: [],
        outputs: [{ name: '', type: 'uint256' }],
      },
    ],
    functionName: 'totalDistributed',
  });
};

// Get active beneficiaries from profit split contract (this exists in the contract)
export const useActiveBeneficiaries = () => {
  const config = useContractConfig();

  return useReadContract({
    address: config.profitSplit,
    abi: [
      {
        name: 'getActiveBeneficiaries',
        type: 'function',
        stateMutability: 'view',
        inputs: [],
        outputs: [{ name: '', type: 'address[]' }],
      },
    ],
    functionName: 'getActiveBeneficiaries',
  });
};

// Get USDC balance of profit split contract (pending distributions)
export const useProfitSplitBalance = () => {
  const config = useContractConfig();

  return useReadContract({
    address: config.usdcAddress,
    abi: [
      {
        name: 'balanceOf',
        type: 'function',
        stateMutability: 'view',
        inputs: [{ name: 'account', type: 'address' }],
        outputs: [{ name: '', type: 'uint256' }],
      },
    ],
    functionName: 'balanceOf',
    args: [config.profitSplit],
  });
};

// Get USDC balance of payments contract (collected fees)
export const usePaymentsContractBalance = () => {
  const config = useContractConfig();

  return useReadContract({
    address: config.usdcAddress,
    abi: [
      {
        name: 'balanceOf',
        type: 'function',
        stateMutability: 'view',
        inputs: [{ name: 'account', type: 'address' }],
        outputs: [{ name: '', type: 'uint256' }],
      },
    ],
    functionName: 'balanceOf',
    args: [config.payments],
  });
};

// ==================== COUPON & SUBSCRIPTION HOOKS ====================

// Coupon Management Hooks
export const useCreateCoupon = () => {
  const config = useContractConfig();
  const { writeContractAsync } = useWriteContract();

  const createCoupon = async (
    couponCode: string,
    discountAmount: bigint,
    expiryTime: bigint
  ) => {
    return await writeContractAsync({
      address: config.payments,
      abi: PAYVERGE_PAYMENTS_ABI,
      functionName: 'createCoupon',
      args: [couponCode, discountAmount, expiryTime],
    });
  };

  return { createCoupon };
};

export const useDeactivateCoupon = () => {
  const config = useContractConfig();
  const { writeContractAsync } = useWriteContract();

  const deactivateCoupon = async (couponCode: string) => {
    return await writeContractAsync({
      address: config.payments,
      abi: PAYVERGE_PAYMENTS_ABI,
      functionName: 'deactivateCoupon',
      args: [couponCode],
    });
  };

  return { deactivateCoupon };
};

export const useGetCouponInfo = (couponHash: `0x${string}`) => {
  const config = useContractConfig();

  return useReadContract({
    address: config.payments,
    abi: PAYVERGE_PAYMENTS_ABI,
    functionName: 'getCouponInfo',
    args: [couponHash],
  });
};

// Helper hook to validate coupon code and get discount
export const useValidateCoupon = (couponCode: string) => {
  // Create coupon hash using keccak256 (same as smart contract)
  const couponHash = couponCode ? 
    `0x${require('js-sha3').keccak256(couponCode)}` as `0x${string}` : 
    `0x${'0'.repeat(64)}` as `0x${string}`;
  
  const { data: couponInfo, error, isLoading } = useGetCouponInfo(couponHash);
  
  // Type the coupon info properly
  const typedCouponInfo = couponInfo as {
    discountAmount: bigint;
    expiryTime: bigint;
    isUsed: boolean;
    isActive: boolean;
  } | undefined;
  
  // Validate coupon
  const isValid = typedCouponInfo && !error && 
    typedCouponInfo.isActive && 
    !typedCouponInfo.isUsed && 
    Number(typedCouponInfo.expiryTime) > Math.floor(Date.now() / 1000);
  
  return {
    isValid,
    discountAmount: typedCouponInfo?.discountAmount || BigInt(0),
    expiryTime: typedCouponInfo?.expiryTime || BigInt(0),
    isLoading,
    error,
    couponInfo: typedCouponInfo
  };
};

// Business Registration with Coupon
export const useRegisterBusinessWithCoupon = () => {
  const config = useContractConfig();
  const { writeContractAsync } = useWriteContract();

  const registerBusinessWithCoupon = async (
    name: string,
    paymentAddress: Address,
    tippingAddress: Address,
    couponCode: string
  ) => {
    return await writeContractAsync({
      address: config.payments,
      abi: PAYVERGE_PAYMENTS_ABI,
      functionName: 'registerBusinessWithCoupon',
      args: [name, paymentAddress, tippingAddress, couponCode],
    });
  };

  return { registerBusinessWithCoupon };
};

// Subscription Management Hooks
export const useCalculateSubscriptionTime = (paymentAmount: bigint) => {
  const config = useContractConfig();

  return useReadContract({
    address: config.payments,
    abi: PAYVERGE_PAYMENTS_ABI,
    functionName: 'calculateSubscriptionTime',
    args: [paymentAmount],
  });
};

export const useCalculatePaymentForTime = (subscriptionSeconds: bigint) => {
  const config = useContractConfig();

  return useReadContract({
    address: config.payments,
    abi: PAYVERGE_PAYMENTS_ABI,
    functionName: 'calculatePaymentForTime',
    args: [subscriptionSeconds],
  });
};

export const useGetBusinessSubscriptionStatus = (businessAddress: Address) => {
  const config = useContractConfig();

  return useReadContract({
    address: config.payments,
    abi: PAYVERGE_PAYMENTS_ABI,
    functionName: 'getBusinessSubscriptionStatus',
    args: [businessAddress],
  });
};

export const useRenewSubscription = () => {
  const config = useContractConfig();
  const { writeContractAsync } = useWriteContract();

  const renewSubscription = async (paymentAmount: bigint) => {
    return await writeContractAsync({
      address: config.payments,
      abi: PAYVERGE_PAYMENTS_ABI,
      functionName: 'renewSubscription',
      args: [paymentAmount],
    });
  };

  return { renewSubscription };
};

export const useRenewSubscriptionWithCoupon = () => {
  const config = useContractConfig();
  const { writeContractAsync } = useWriteContract();

  const renewSubscriptionWithCoupon = async (
    paymentAmount: bigint,
    couponCode: string
  ) => {
    return await writeContractAsync({
      address: config.payments,
      abi: PAYVERGE_PAYMENTS_ABI,
      functionName: 'renewSubscriptionWithCoupon',
      args: [paymentAmount, couponCode],
    });
  };

  return { renewSubscriptionWithCoupon };
};

export const useProposeRegistrationFeeUpdate = () => {
  const config = useContractConfig();
  const { writeContractAsync } = useWriteContract();

  const proposeRegistrationFeeUpdate = async (newFee: bigint) => {
    return await writeContractAsync({
      address: config.payments,
      abi: PAYVERGE_PAYMENTS_ABI,
      functionName: 'proposeRegistrationFeeUpdate',
      args: [newFee],
    });
  };

  return { proposeRegistrationFeeUpdate };
};

export const useExecuteRegistrationFeeUpdate = () => {
  const config = useContractConfig();
  const { writeContractAsync } = useWriteContract();

  const executeRegistrationFeeUpdate = async () => {
    return await writeContractAsync({
      address: config.payments,
      abi: PAYVERGE_PAYMENTS_ABI,
      functionName: 'executeRegistrationFeeUpdate',
    });
  };

  return { executeRegistrationFeeUpdate };
};

export const useCancelRegistrationFeeUpdate = () => {
  const config = useContractConfig();
  const { writeContractAsync } = useWriteContract();

  const cancelRegistrationFeeUpdate = async () => {
    return await writeContractAsync({
      address: config.payments,
      abi: PAYVERGE_PAYMENTS_ABI,
      functionName: 'cancelRegistrationFeeUpdate',
    });
  };

  return { cancelRegistrationFeeUpdate };
};

export const useGetPendingRegistrationFeeInfo = () => {
  const config = useContractConfig();

  return useReadContract({
    address: config.payments,
    abi: PAYVERGE_PAYMENTS_ABI,
    functionName: 'getPendingRegistrationFeeInfo',
  });
};

// Contract Configuration Hooks
export const useSetReferralsContract = () => {
  const config = useContractConfig();
  const { writeContractAsync } = useWriteContract();

  const setReferralsContract = async (referralsContractAddress: Address) => {
    return await writeContractAsync({
      address: config.payments,
      abi: PAYVERGE_PAYMENTS_ABI,
      functionName: 'setReferralsContract',
      args: [referralsContractAddress],
    });
  };

  return { setReferralsContract };
};

export const useSetProfitSplitContract = () => {
  const config = useContractConfig();
  const { writeContractAsync } = useWriteContract();

  const setProfitSplitContract = async (profitSplitContractAddress: Address) => {
    return await writeContractAsync({
      address: config.payments,
      abi: PAYVERGE_PAYMENTS_ABI,
      functionName: 'setProfitSplitContract',
      args: [profitSplitContractAddress],
    });
  };

  return { setProfitSplitContract };
};

// Event Watching for Coupons & Subscriptions
export const useWatchCouponCreated = (
  onCouponCreated: (log: any) => void
) => {
  const config = useContractConfig();

  return useWatchContractEvent({
    address: config.payments,
    abi: PAYVERGE_PAYMENTS_ABI,
    eventName: 'CouponCreated',
    onLogs: onCouponCreated,
  });
};

export const useWatchCouponUsed = (
  onCouponUsed: (log: any) => void,
  businessAddress?: Address
) => {
  const config = useContractConfig();

  return useWatchContractEvent({
    address: config.payments,
    abi: PAYVERGE_PAYMENTS_ABI,
    eventName: 'CouponUsed',
    args: businessAddress ? { business: businessAddress } : undefined,
    onLogs: onCouponUsed,
  });
};

export const useWatchBusinessRegisteredWithCoupon = (
  onBusinessRegistered: (log: any) => void,
  businessAddress?: Address
) => {
  const config = useContractConfig();

  return useWatchContractEvent({
    address: config.payments,
    abi: PAYVERGE_PAYMENTS_ABI,
    eventName: 'BusinessRegisteredWithCoupon',
    args: businessAddress ? { businessAddress } : undefined,
    onLogs: onBusinessRegistered,
  });
};

export const useWatchBusinessSubscriptionRenewed = (
  onSubscriptionRenewed: (log: any) => void,
  businessAddress?: Address
) => {
  const config = useContractConfig();

  return useWatchContractEvent({
    address: config.payments,
    abi: PAYVERGE_PAYMENTS_ABI,
    eventName: 'BusinessSubscriptionRenewed',
    args: businessAddress ? { business: businessAddress } : undefined,
    onLogs: onSubscriptionRenewed,
  });
};

export const useWatchBusinessSubscriptionRenewedWithCoupon = (
  onSubscriptionRenewed: (log: any) => void,
  businessAddress?: Address
) => {
  const config = useContractConfig();

  return useWatchContractEvent({
    address: config.payments,
    abi: PAYVERGE_PAYMENTS_ABI,
    eventName: 'BusinessSubscriptionRenewedWithCoupon',
    args: businessAddress ? { business: businessAddress } : undefined,
    onLogs: onSubscriptionRenewed,
  });
};

// Utility hooks for subscription calculations
export const useSubscriptionTimeInDays = (paymentAmount: bigint) => {
  const { data: subscriptionSeconds } = useCalculateSubscriptionTime(paymentAmount);
  
  if (!subscriptionSeconds) return 0;
  
  // Convert seconds to days (86400 seconds per day)
  return Number(subscriptionSeconds) / 86400;
};

export const usePaymentForDays = (days: number) => {
  const subscriptionSeconds = BigInt(days * 86400); // Convert days to seconds
  const { data: paymentAmount } = useCalculatePaymentForTime(subscriptionSeconds);
  
  return paymentAmount || BigInt(0);
};

// Helper hook to format USDC amounts
export const useFormatUSDC = () => {
  const formatUSDC = (amount: bigint) => {
    return formatUnits(amount, 6); // USDC has 6 decimals
  };

  const parseUSDC = (amount: string) => {
    return parseUnits(amount, 6);
  };

  return { formatUSDC, parseUSDC };
};

// Subscription Plans Configuration
export interface SubscriptionPlan {
  id: string;
  name: string;
  description: string;
  monthlyPrice: bigint; // USDC in wei (6 decimals)
  yearlyPrice: bigint; // USDC in wei (6 decimals)
  features: string[];
  maxTables: number;
  maxMenuItems: number;
  analyticsEnabled: boolean;
  prioritySupport: boolean;
  customBranding: boolean;
}

export const SUBSCRIPTION_PLANS: SubscriptionPlan[] = [
  {
    id: 'basic',
    name: 'Basic',
    description: 'Perfect for small restaurants and cafes',
    monthlyPrice: parseUnits('29.99', 6), // $29.99/month
    yearlyPrice: parseUnits('299.99', 6), // $299.99/year (2 months free)
    features: [
      'Up to 10 tables',
      'Up to 50 menu items',
      'Basic payment processing',
      'QR code generation',
      'Basic analytics',
      'Email support'
    ],
    maxTables: 10,
    maxMenuItems: 50,
    analyticsEnabled: true,
    prioritySupport: false,
    customBranding: false,
  },
  {
    id: 'premium',
    name: 'Premium',
    description: 'Ideal for growing restaurants with multiple locations',
    monthlyPrice: parseUnits('79.99', 6), // $79.99/month
    yearlyPrice: parseUnits('799.99', 6), // $799.99/year (2 months free)
    features: [
      'Up to 50 tables',
      'Unlimited menu items',
      'Advanced payment processing',
      'Split billing',
      'Advanced analytics & reports',
      'Multi-currency support',
      'Priority support',
      'Custom branding'
    ],
    maxTables: 50,
    maxMenuItems: -1, // unlimited
    analyticsEnabled: true,
    prioritySupport: true,
    customBranding: true,
  },
  {
    id: 'enterprise',
    name: 'Enterprise',
    description: 'For large restaurant chains and franchises',
    monthlyPrice: parseUnits('199.99', 6), // $199.99/month
    yearlyPrice: parseUnits('1999.99', 6), // $1999.99/year (2 months free)
    features: [
      'Unlimited tables',
      'Unlimited menu items',
      'White-label solution',
      'API access',
      'Advanced integrations',
      'Dedicated account manager',
      'Custom features',
      '24/7 phone support'
    ],
    maxTables: -1, // unlimited
    maxMenuItems: -1, // unlimited
    analyticsEnabled: true,
    prioritySupport: true,
    customBranding: true,
  },
];

// Get subscription plan by ID
export const getSubscriptionPlan = (planId: string): SubscriptionPlan | undefined => {
  return SUBSCRIPTION_PLANS.find(plan => plan.id === planId);
};

// Calculate subscription price with discounts
export const calculateSubscriptionPrice = (
  plan: SubscriptionPlan,
  frequency: 'monthly' | 'yearly',
  couponDiscount?: bigint,
  referralDiscount?: number // percentage (e.g., 10 for 10%)
): { originalPrice: bigint; finalPrice: bigint; totalDiscount: bigint } => {
  const originalPrice = frequency === 'yearly' ? plan.yearlyPrice : plan.monthlyPrice;
  let finalPrice = originalPrice;
  let totalDiscount = BigInt(0);

  // Apply referral discount (percentage)
  if (referralDiscount && referralDiscount > 0) {
    const referralDiscountAmount = (originalPrice * BigInt(referralDiscount)) / BigInt(100);
    finalPrice -= referralDiscountAmount;
    totalDiscount += referralDiscountAmount;
  }

  // Apply coupon discount (fixed amount)
  if (couponDiscount && couponDiscount > 0) {
    const couponDiscountAmount = couponDiscount > finalPrice ? finalPrice : couponDiscount;
    finalPrice -= couponDiscountAmount;
    totalDiscount += couponDiscountAmount;
  }

  // Ensure final price is not negative
  if (finalPrice < 0) {
    finalPrice = BigInt(0);
  }

  return {
    originalPrice,
    finalPrice,
    totalDiscount,
  };
};

