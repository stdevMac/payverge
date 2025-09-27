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

export const useProcessPayment = () => {
  const config = useContractConfig();
  const { writeContract } = useWriteContract();
  
  const processPayment = (params: ProcessPaymentParams) => {
    return writeContract({
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
  const { writeContract } = useWriteContract();
  
  const registerBusiness = (params: RegisterBusinessParams) => {
    return writeContract({
      address: config.payments,
      abi: PAYVERGE_PAYMENTS_ABI,
      functionName: 'registerBusiness',
      args: params.referralCode ? [
        params.name,
        params.paymentAddress,
        params.tippingAddress,
        params.referralCode,
      ] : [
        params.name,
        params.paymentAddress,
        params.tippingAddress,
      ],
    });
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
  const { writeContract } = useWriteContract();
  
  const claimEarnings = () => {
    return writeContract({
      address: config.payments,
      abi: PAYVERGE_PAYMENTS_ABI,
      functionName: 'claimEarnings',
    });
  };
  
  return { claimEarnings };
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
  const { writeContract } = useWriteContract();
  
  const approveUsdc = (amount: bigint, spender?: Address) => {
    const spenderAddress = spender || config.payments;
    
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
      args: [spenderAddress, amount],
    });
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

export const useActiveBeneficiaries = () => {
  const config = useContractConfig();
  
  return useReadContract({
    address: config.profitSplit,
    abi: PAYVERGE_PROFIT_SPLIT_ABI,
    functionName: 'getActiveBeneficiaries',
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

export const useTotalDistributed = () => {
  const config = useContractConfig();
  
  return useReadContract({
    address: config.profitSplit,
    abi: PAYVERGE_PROFIT_SPLIT_ABI,
    functionName: 'totalDistributed',
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
