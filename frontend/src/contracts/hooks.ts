// Wagmi hooks for PayvergePayments contract interactions
import { useReadContract, useWriteContract, useWatchContractEvent } from 'wagmi';
import { useChainId, useAccount } from 'wagmi';
import { Address, parseUnits, formatUnits } from 'viem';
import { PAYVERGE_PAYMENTS_ABI } from './abi';
import { getContractConfig } from './config';
import { 
  Bill, 
  Payment, 
  BusinessInfo, 
  CreateBillParams, 
  ProcessPaymentParams, 
  RegisterBusinessParams,
  ClaimableAmounts 
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
    address: config.address,
    abi: PAYVERGE_PAYMENTS_ABI,
    functionName: 'getBill',
    args: [billId as `0x${string}`],
  });
};

export const useBillPayments = (billId: string) => {
  const config = useContractConfig();
  
  return useReadContract({
    address: config.address,
    abi: PAYVERGE_PAYMENTS_ABI,
    functionName: 'getBillPayments',
    args: [billId as `0x${string}`],
  });
};

export const useBusinessInfo = (businessAddress: Address) => {
  const config = useContractConfig();
  
  return useReadContract({
    address: config.address,
    abi: PAYVERGE_PAYMENTS_ABI,
    functionName: 'businessInfo',
    args: [businessAddress],
  });
};

export const useBusinessBillCount = (businessAddress: Address) => {
  const config = useContractConfig();
  
  return useReadContract({
    address: config.address,
    abi: PAYVERGE_PAYMENTS_ABI,
    functionName: 'businessBillCount',
    args: [businessAddress],
  });
};

export const useClaimableBalance = (businessAddress: Address) => {
  const config = useContractConfig();
  
  return useReadContract({
    address: config.address,
    abi: PAYVERGE_PAYMENTS_ABI,
    functionName: 'claimableBalance',
    args: [businessAddress],
  });
};

export const usePlatformFeeRate = () => {
  const config = useContractConfig();
  
  return useReadContract({
    address: config.address,
    abi: PAYVERGE_PAYMENTS_ABI,
    functionName: 'platformFeeRate',
  });
};

export const useUsdcToken = () => {
  const config = useContractConfig();
  
  return useReadContract({
    address: config.address,
    abi: PAYVERGE_PAYMENTS_ABI,
    functionName: 'usdcToken',
  });
};

export const useDailyPaymentLimit = (userAddress?: Address) => {
  const config = useContractConfig();
  const { address } = useAccount();
  const targetAddress = userAddress || address;
  
  return useReadContract({
    address: config.address,
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
    address: config.address,
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
      address: config.address,
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
      address: config.address,
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
      address: config.address,
      abi: PAYVERGE_PAYMENTS_ABI,
      functionName: 'registerBusiness',
      args: [
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
      address: config.address,
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
      address: config.address,
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
      address: config.address,
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
    address: config.address,
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
    address: config.address,
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
    address: config.address,
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
  const spenderAddress = spender || config.address;
  
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
    const spenderAddress = spender || config.address;
    
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
    address: config.address,
    abi: PAYVERGE_PAYMENTS_ABI,
    functionName: 'getBillParticipants',
    args: [billId as `0x${string}`],
  });
};

export const useParticipantInfo = (billId: string, participant: Address) => {
  const config = useContractConfig();
  
  return useReadContract({
    address: config.address,
    abi: PAYVERGE_PAYMENTS_ABI,
    functionName: 'getParticipantInfo',
    args: [billId as `0x${string}`, participant],
  });
};

export const useHasParticipated = (billId: string, participant: Address) => {
  const config = useContractConfig();
  
  return useReadContract({
    address: config.address,
    abi: PAYVERGE_PAYMENTS_ABI,
    functionName: 'hasParticipatedInBill',
    args: [billId as `0x${string}`, participant],
  });
};

export const useBillSummary = (billId: string) => {
  const config = useContractConfig();
  
  return useReadContract({
    address: config.address,
    abi: PAYVERGE_PAYMENTS_ABI,
    functionName: 'getBillSummary',
    args: [billId as `0x${string}`],
  });
};

export const useRegistrationFee = () => {
  const config = useContractConfig();
  
  return useReadContract({
    address: config.address,
    abi: PAYVERGE_PAYMENTS_ABI,
    functionName: 'getRegistrationFee',
  });
};
