"use client";
import { useState, useCallback, useEffect } from "react";
import { 
  Button, 
  Input, 
  Card, 
  CardBody, 
  CardHeader, 
  Spinner, 
  Tabs, 
  Tab, 
  Divider,
  Chip,
  Switch,
  Select,
  SelectItem
} from "@nextui-org/react";
import { useReadContract, useWriteContract, useWaitForTransactionReceipt } from "wagmi";
import { useContractConfig } from "@/contracts/hooks";
import { formatUnits, parseUnits } from "viem";
import toast from "react-hot-toast";

// Contract ABIs for admin functions
const PAYVERGE_PAYMENTS_ADMIN_ABI = [
  // View functions
  { name: "platformFeeRate", type: "function", stateMutability: "view", inputs: [], outputs: [{ name: "", type: "uint256" }] },
  { name: "getRegistrationFee", type: "function", stateMutability: "view", inputs: [], outputs: [{ name: "", type: "uint256" }] },
  { name: "getPendingRegistrationFeeInfo", type: "function", stateMutability: "view", inputs: [], outputs: [{ name: "pendingFee", type: "uint256" }, { name: "executeAfter", type: "uint256" }] },
  { name: "billCreatorAddress", type: "function", stateMutability: "view", inputs: [], outputs: [{ name: "", type: "address" }] },
  { name: "feeUpdateDelay", type: "function", stateMutability: "view", inputs: [], outputs: [{ name: "", type: "uint256" }] },
  { name: "getReferralsContract", type: "function", stateMutability: "view", inputs: [], outputs: [{ name: "", type: "address" }] },
  { name: "getProfitSplitContract", type: "function", stateMutability: "view", inputs: [], outputs: [{ name: "", type: "address" }] },
  { name: "paused", type: "function", stateMutability: "view", inputs: [], outputs: [{ name: "", type: "bool" }] },
  
  // Admin functions
  { name: "proposePlatformFeeUpdate", type: "function", stateMutability: "nonpayable", inputs: [{ name: "newFeeRate", type: "uint256" }], outputs: [] },
  { name: "executePlatformFeeUpdate", type: "function", stateMutability: "nonpayable", inputs: [], outputs: [] },
  { name: "cancelPlatformFeeUpdate", type: "function", stateMutability: "nonpayable", inputs: [], outputs: [] },
  { name: "proposeRegistrationFeeUpdate", type: "function", stateMutability: "nonpayable", inputs: [{ name: "newFee", type: "uint256" }], outputs: [] },
  { name: "executeRegistrationFeeUpdate", type: "function", stateMutability: "nonpayable", inputs: [], outputs: [] },
  { name: "cancelRegistrationFeeUpdate", type: "function", stateMutability: "nonpayable", inputs: [], outputs: [] },
  { name: "setBillCreator", type: "function", stateMutability: "nonpayable", inputs: [{ name: "newBillCreator", type: "address" }], outputs: [] },
  { name: "setFeeUpdateDelay", type: "function", stateMutability: "nonpayable", inputs: [{ name: "newDelay", type: "uint256" }], outputs: [] },
  { name: "setReferralsContract", type: "function", stateMutability: "nonpayable", inputs: [{ name: "_referralsContract", type: "address" }], outputs: [] },
  { name: "setProfitSplitContract", type: "function", stateMutability: "nonpayable", inputs: [{ name: "_profitSplitContract", type: "address" }], outputs: [] },
  { name: "pause", type: "function", stateMutability: "nonpayable", inputs: [], outputs: [] },
  { name: "unpause", type: "function", stateMutability: "nonpayable", inputs: [], outputs: [] }
];

const PAYVERGE_REFERRALS_ADMIN_ABI = [
  // View functions
  { name: "totalReferrers", type: "function", stateMutability: "view", inputs: [], outputs: [{ name: "", type: "uint256" }] },
  { name: "totalCommissionsPaid", type: "function", stateMutability: "view", inputs: [], outputs: [{ name: "", type: "uint256" }] },
  { name: "platformTreasury", type: "function", stateMutability: "view", inputs: [], outputs: [{ name: "", type: "address" }] },
  { name: "payvergePaymentsContract", type: "function", stateMutability: "view", inputs: [], outputs: [{ name: "", type: "address" }] },
  { name: "paused", type: "function", stateMutability: "view", inputs: [], outputs: [{ name: "", type: "bool" }] },
  
  // Admin functions
  { name: "setPayvergePaymentsContract", type: "function", stateMutability: "nonpayable", inputs: [{ name: "_payvergePayments", type: "address" }], outputs: [] },
  { name: "deactivateReferrer", type: "function", stateMutability: "nonpayable", inputs: [{ name: "_referrer", type: "address" }], outputs: [] },
  { name: "pause", type: "function", stateMutability: "nonpayable", inputs: [], outputs: [] },
  { name: "unpause", type: "function", stateMutability: "nonpayable", inputs: [], outputs: [] },
  { name: "emergencyWithdraw", type: "function", stateMutability: "nonpayable", inputs: [{ name: "_token", type: "address" }, { name: "_amount", type: "uint256" }], outputs: [] }
];

const PAYVERGE_PROFIT_SPLIT_ADMIN_ABI = [
  // View functions
  { name: "totalDistributed", type: "function", stateMutability: "view", inputs: [], outputs: [{ name: "", type: "uint256" }] },
  { name: "distributionCount", type: "function", stateMutability: "view", inputs: [], outputs: [{ name: "", type: "uint256" }] },
  { name: "beneficiaryCount", type: "function", stateMutability: "view", inputs: [], outputs: [{ name: "", type: "uint256" }] },
  { name: "totalPercentageAllocated", type: "function", stateMutability: "view", inputs: [], outputs: [{ name: "", type: "uint256" }] },
  { name: "getActiveBeneficiaries", type: "function", stateMutability: "view", inputs: [], outputs: [{ name: "", type: "address[]" }] },
  { name: "getBeneficiary", type: "function", stateMutability: "view", inputs: [{ name: "_beneficiary", type: "address" }], outputs: [{ name: "", type: "tuple", components: [{ name: "beneficiaryAddress", type: "address" }, { name: "name", type: "string" }, { name: "percentage", type: "uint256" }, { name: "totalReceived", type: "uint256" }, { name: "isActive", type: "bool" }] }] },
  { name: "paused", type: "function", stateMutability: "view", inputs: [], outputs: [{ name: "", type: "bool" }] },
  
  // Admin functions
  { name: "addBeneficiary", type: "function", stateMutability: "nonpayable", inputs: [{ name: "_beneficiary", type: "address" }, { name: "_name", type: "string" }, { name: "_percentage", type: "uint256" }], outputs: [] },
  { name: "updateBeneficiaryPercentage", type: "function", stateMutability: "nonpayable", inputs: [{ name: "_beneficiary", type: "address" }, { name: "_newPercentage", type: "uint256" }], outputs: [] },
  { name: "removeBeneficiary", type: "function", stateMutability: "nonpayable", inputs: [{ name: "_beneficiary", type: "address" }], outputs: [] },
  { name: "distributeProfits", type: "function", stateMutability: "nonpayable", inputs: [{ name: "_amount", type: "uint256" }], outputs: [] },
  { name: "distributeAllProfits", type: "function", stateMutability: "nonpayable", inputs: [], outputs: [] },
  { name: "grantDistributorRole", type: "function", stateMutability: "nonpayable", inputs: [{ name: "_distributor", type: "address" }], outputs: [] },
  { name: "revokeDistributorRole", type: "function", stateMutability: "nonpayable", inputs: [{ name: "_distributor", type: "address" }], outputs: [] },
  { name: "pause", type: "function", stateMutability: "nonpayable", inputs: [], outputs: [] },
  { name: "unpause", type: "function", stateMutability: "nonpayable", inputs: [], outputs: [] },
  { name: "emergencyWithdraw", type: "function", stateMutability: "nonpayable", inputs: [{ name: "_token", type: "address" }, { name: "_amount", type: "uint256" }, { name: "_to", type: "address" }], outputs: [] }
];

export const PayvergeManagement = () => {
  const config = useContractConfig();
  const { writeContract } = useWriteContract();
  
  // Form states
  const [newPlatformFee, setNewPlatformFee] = useState("");
  const [newRegistrationFee, setNewRegistrationFee] = useState("");
  const [newBillCreator, setNewBillCreator] = useState("");
  const [newFeeDelay, setNewFeeDelay] = useState("");
  const [newReferralsContract, setNewReferralsContract] = useState("");
  const [newProfitSplitContract, setNewProfitSplitContract] = useState("");
  
  // Profit Split states
  const [newBeneficiary, setNewBeneficiary] = useState("");
  const [newBeneficiaryName, setNewBeneficiaryName] = useState("");
  const [newBeneficiaryPercentage, setNewBeneficiaryPercentage] = useState("");
  const [distributionAmount, setDistributionAmount] = useState("");
  const [beneficiaryToUpdate, setBeneficiaryToUpdate] = useState("");
  const [newPercentage, setNewPercentage] = useState("");
  const [beneficiaryToRemove, setBeneficiaryToRemove] = useState("");
  const [distributorToGrant, setDistributorToGrant] = useState("");
  const [distributorToRevoke, setDistributorToRevoke] = useState("");
  const [emergencyWithdrawToken, setEmergencyWithdrawToken] = useState("");
  const [emergencyWithdrawAmount, setEmergencyWithdrawAmount] = useState("");
  const [emergencyWithdrawTo, setEmergencyWithdrawTo] = useState("");
  
  // Referrals states
  const [referrerToDeactivate, setReferrerToDeactivate] = useState("");
  const [referralsEmergencyToken, setReferralsEmergencyToken] = useState("");
  const [referralsEmergencyAmount, setReferralsEmergencyAmount] = useState("");
  const [newPayvergePaymentsContract, setNewPayvergePaymentsContract] = useState("");
  
  const [isLoading, setIsLoading] = useState(false);
  const [selectedTab, setSelectedTab] = useState("payments");

  // PayvergePayments contract reads
  const { data: platformFeeRate } = useReadContract({
    address: config.payments,
    abi: PAYVERGE_PAYMENTS_ADMIN_ABI,
    functionName: "platformFeeRate"
  });

  const { data: registrationFee } = useReadContract({
    address: config.payments,
    abi: PAYVERGE_PAYMENTS_ADMIN_ABI,
    functionName: "getRegistrationFee"
  });

  const { data: billCreatorAddress } = useReadContract({
    address: config.payments,
    abi: PAYVERGE_PAYMENTS_ADMIN_ABI,
    functionName: "billCreatorAddress"
  });

  const { data: feeUpdateDelay } = useReadContract({
    address: config.payments,
    abi: PAYVERGE_PAYMENTS_ADMIN_ABI,
    functionName: "feeUpdateDelay"
  });

  const { data: referralsContractAddress } = useReadContract({
    address: config.payments,
    abi: PAYVERGE_PAYMENTS_ADMIN_ABI,
    functionName: "getReferralsContract"
  });

  const { data: profitSplitContractAddress } = useReadContract({
    address: config.payments,
    abi: PAYVERGE_PAYMENTS_ADMIN_ABI,
    functionName: "getProfitSplitContract"
  });

  const { data: paymentsContractPaused } = useReadContract({
    address: config.payments,
    abi: PAYVERGE_PAYMENTS_ADMIN_ABI,
    functionName: "paused"
  });

  // PayvergeReferrals contract reads
  const { data: totalReferrers } = useReadContract({
    address: config.referrals,
    abi: PAYVERGE_REFERRALS_ADMIN_ABI,
    functionName: "totalReferrers"
  });

  const { data: totalCommissionsPaid } = useReadContract({
    address: config.referrals,
    abi: PAYVERGE_REFERRALS_ADMIN_ABI,
    functionName: "totalCommissionsPaid"
  });

  const { data: referralsContractPaused } = useReadContract({
    address: config.referrals,
    abi: PAYVERGE_REFERRALS_ADMIN_ABI,
    functionName: "paused"
  });

  // PayvergeProfitSplit contract reads
  const { data: totalDistributed } = useReadContract({
    address: config.profitSplit,
    abi: PAYVERGE_PROFIT_SPLIT_ADMIN_ABI,
    functionName: "totalDistributed"
  });

  const { data: beneficiaryCount } = useReadContract({
    address: config.profitSplit,
    abi: PAYVERGE_PROFIT_SPLIT_ADMIN_ABI,
    functionName: "beneficiaryCount"
  });

  const { data: totalPercentageAllocated } = useReadContract({
    address: config.profitSplit,
    abi: PAYVERGE_PROFIT_SPLIT_ADMIN_ABI,
    functionName: "totalPercentageAllocated"
  });

  const { data: activeBeneficiaries } = useReadContract({
    address: config.profitSplit,
    abi: PAYVERGE_PROFIT_SPLIT_ADMIN_ABI,
    functionName: "getActiveBeneficiaries"
  });

  const { data: profitSplitContractPaused } = useReadContract({
    address: config.profitSplit,
    abi: PAYVERGE_PROFIT_SPLIT_ADMIN_ABI,
    functionName: "paused"
  });

  // Helper functions
  const formatUSDC = (value: unknown) => {
    if (!value || typeof value !== 'bigint') return "$0.00";
    return `$${formatUnits(value as bigint, 6)}`;
  };

  const formatPercentage = (value: unknown) => {
    if (!value || typeof value !== 'bigint') return "0%";
    return `${Number(value) / 100}%`;
  };

  const formatDelay = (seconds: unknown) => {
    if (!seconds || typeof seconds !== 'bigint') return "0 hours";
    const hours = Number(seconds) / 3600;
    return `${hours} hours`;
  };

  const formatAddress = (address: unknown) => {
    if (!address || typeof address !== 'string') return "Not set";
    return `${address.slice(0, 6)}...${address.slice(-4)}`;
  };

  // Transaction handlers
  const handleProposePlatformFeeUpdate = async () => {
    if (!newPlatformFee) {
      toast.error("Please enter a platform fee rate");
      return;
    }

    try {
      setIsLoading(true);
      const feeInBasisPoints = Math.round(parseFloat(newPlatformFee) * 100);
      
      await writeContract({
        address: config.payments,
        abi: PAYVERGE_PAYMENTS_ADMIN_ABI,
        functionName: "proposePlatformFeeUpdate",
        args: [BigInt(feeInBasisPoints)]
      });

      toast.success("Platform fee update proposed successfully");
      setNewPlatformFee("");
    } catch (error: any) {
      console.error("Error proposing platform fee update:", error);
      toast.error(error.message || "Failed to propose platform fee update");
    } finally {
      setIsLoading(false);
    }
  };

  const handleExecutePlatformFeeUpdate = async () => {
    try {
      setIsLoading(true);
      
      await writeContract({
        address: config.payments,
        abi: PAYVERGE_PAYMENTS_ADMIN_ABI,
        functionName: "executePlatformFeeUpdate",
        args: []
      });

      toast.success("Platform fee update executed successfully");
    } catch (error: any) {
      console.error("Error executing platform fee update:", error);
      toast.error(error.message || "Failed to execute platform fee update");
    } finally {
      setIsLoading(false);
    }
  };

  const handlePauseContract = async (contractType: "payments" | "referrals" | "profitSplit") => {
    try {
      setIsLoading(true);
      
      const contractAddress = contractType === "payments" ? config.payments : 
                            contractType === "referrals" ? config.referrals : 
                            config.profitSplit;
      
      const abi = contractType === "payments" ? PAYVERGE_PAYMENTS_ADMIN_ABI :
                 contractType === "referrals" ? PAYVERGE_REFERRALS_ADMIN_ABI :
                 PAYVERGE_PROFIT_SPLIT_ADMIN_ABI;

      await writeContract({
        address: contractAddress,
        abi: abi,
        functionName: "pause",
        args: []
      });

      toast.success(`${contractType} contract paused successfully`);
    } catch (error: any) {
      console.error(`Error pausing ${contractType} contract:`, error);
      toast.error(error.message || `Failed to pause ${contractType} contract`);
    } finally {
      setIsLoading(false);
    }
  };

  const handleUnpauseContract = async (contractType: "payments" | "referrals" | "profitSplit") => {
    try {
      setIsLoading(true);
      
      const contractAddress = contractType === "payments" ? config.payments : 
                            contractType === "referrals" ? config.referrals : 
                            config.profitSplit;
      
      const abi = contractType === "payments" ? PAYVERGE_PAYMENTS_ADMIN_ABI :
                 contractType === "referrals" ? PAYVERGE_REFERRALS_ADMIN_ABI :
                 PAYVERGE_PROFIT_SPLIT_ADMIN_ABI;

      await writeContract({
        address: contractAddress,
        abi: abi,
        functionName: "unpause",
        args: []
      });

      toast.success(`${contractType} contract unpaused successfully`);
    } catch (error: any) {
      console.error(`Error unpausing ${contractType} contract:`, error);
      toast.error(error.message || `Failed to unpause ${contractType} contract`);
    } finally {
      setIsLoading(false);
    }
  };

  // Additional admin function handlers
  const handleAddBeneficiary = async () => {
    if (!newBeneficiary || !newBeneficiaryName || !newBeneficiaryPercentage) {
      toast.error("Please fill in all beneficiary fields");
      return;
    }

    try {
      setIsLoading(true);
      const percentageInBasisPoints = Math.round(parseFloat(newBeneficiaryPercentage) * 100);
      
      await writeContract({
        address: config.profitSplit,
        abi: PAYVERGE_PROFIT_SPLIT_ADMIN_ABI,
        functionName: "addBeneficiary",
        args: [newBeneficiary, newBeneficiaryName, BigInt(percentageInBasisPoints)]
      });

      toast.success("Beneficiary added successfully");
      setNewBeneficiary("");
      setNewBeneficiaryName("");
      setNewBeneficiaryPercentage("");
    } catch (error: any) {
      console.error("Error adding beneficiary:", error);
      toast.error(error.message || "Failed to add beneficiary");
    } finally {
      setIsLoading(false);
    }
  };

  const handleUpdateBeneficiaryPercentage = async () => {
    if (!beneficiaryToUpdate || !newPercentage) {
      toast.error("Please enter beneficiary address and new percentage");
      return;
    }

    try {
      setIsLoading(true);
      const percentageInBasisPoints = Math.round(parseFloat(newPercentage) * 100);
      
      await writeContract({
        address: config.profitSplit,
        abi: PAYVERGE_PROFIT_SPLIT_ADMIN_ABI,
        functionName: "updateBeneficiaryPercentage",
        args: [beneficiaryToUpdate, BigInt(percentageInBasisPoints)]
      });

      toast.success("Beneficiary percentage updated successfully");
      setBeneficiaryToUpdate("");
      setNewPercentage("");
    } catch (error: any) {
      console.error("Error updating beneficiary percentage:", error);
      toast.error(error.message || "Failed to update beneficiary percentage");
    } finally {
      setIsLoading(false);
    }
  };

  const handleRemoveBeneficiary = async () => {
    if (!beneficiaryToRemove) {
      toast.error("Please enter beneficiary address to remove");
      return;
    }

    try {
      setIsLoading(true);
      
      await writeContract({
        address: config.profitSplit,
        abi: PAYVERGE_PROFIT_SPLIT_ADMIN_ABI,
        functionName: "removeBeneficiary",
        args: [beneficiaryToRemove]
      });

      toast.success("Beneficiary removed successfully");
      setBeneficiaryToRemove("");
    } catch (error: any) {
      console.error("Error removing beneficiary:", error);
      toast.error(error.message || "Failed to remove beneficiary");
    } finally {
      setIsLoading(false);
    }
  };

  const handleDistributeProfits = async () => {
    if (!distributionAmount) {
      toast.error("Please enter distribution amount");
      return;
    }

    try {
      setIsLoading(true);
      const amountInUSDC = parseUnits(distributionAmount, 6);
      
      await writeContract({
        address: config.profitSplit,
        abi: PAYVERGE_PROFIT_SPLIT_ADMIN_ABI,
        functionName: "distributeProfits",
        args: [amountInUSDC]
      });

      toast.success("Profits distributed successfully");
      setDistributionAmount("");
    } catch (error: any) {
      console.error("Error distributing profits:", error);
      toast.error(error.message || "Failed to distribute profits");
    } finally {
      setIsLoading(false);
    }
  };

  const handleDistributeAllProfits = async () => {
    try {
      setIsLoading(true);
      
      await writeContract({
        address: config.profitSplit,
        abi: PAYVERGE_PROFIT_SPLIT_ADMIN_ABI,
        functionName: "distributeAllProfits",
        args: []
      });

      toast.success("All available profits distributed successfully");
    } catch (error: any) {
      console.error("Error distributing all profits:", error);
      toast.error(error.message || "Failed to distribute all profits");
    } finally {
      setIsLoading(false);
    }
  };

  const handleGrantDistributorRole = async () => {
    if (!distributorToGrant) {
      toast.error("Please enter distributor address");
      return;
    }

    try {
      setIsLoading(true);
      
      await writeContract({
        address: config.profitSplit,
        abi: PAYVERGE_PROFIT_SPLIT_ADMIN_ABI,
        functionName: "grantDistributorRole",
        args: [distributorToGrant]
      });

      toast.success("Distributor role granted successfully");
      setDistributorToGrant("");
    } catch (error: any) {
      console.error("Error granting distributor role:", error);
      toast.error(error.message || "Failed to grant distributor role");
    } finally {
      setIsLoading(false);
    }
  };

  const handleRevokeDistributorRole = async () => {
    if (!distributorToRevoke) {
      toast.error("Please enter distributor address");
      return;
    }

    try {
      setIsLoading(true);
      
      await writeContract({
        address: config.profitSplit,
        abi: PAYVERGE_PROFIT_SPLIT_ADMIN_ABI,
        functionName: "revokeDistributorRole",
        args: [distributorToRevoke]
      });

      toast.success("Distributor role revoked successfully");
      setDistributorToRevoke("");
    } catch (error: any) {
      console.error("Error revoking distributor role:", error);
      toast.error(error.message || "Failed to revoke distributor role");
    } finally {
      setIsLoading(false);
    }
  };

  const handleDeactivateReferrer = async () => {
    if (!referrerToDeactivate) {
      toast.error("Please enter referrer address");
      return;
    }

    try {
      setIsLoading(true);
      
      await writeContract({
        address: config.referrals,
        abi: PAYVERGE_REFERRALS_ADMIN_ABI,
        functionName: "deactivateReferrer",
        args: [referrerToDeactivate]
      });

      toast.success("Referrer deactivated successfully");
      setReferrerToDeactivate("");
    } catch (error: any) {
      console.error("Error deactivating referrer:", error);
      toast.error(error.message || "Failed to deactivate referrer");
    } finally {
      setIsLoading(false);
    }
  };

  const handleSetPayvergePaymentsContract = async () => {
    if (!newPayvergePaymentsContract) {
      toast.error("Please enter PayvergePayments contract address");
      return;
    }

    try {
      setIsLoading(true);
      
      await writeContract({
        address: config.referrals,
        abi: PAYVERGE_REFERRALS_ADMIN_ABI,
        functionName: "setPayvergePaymentsContract",
        args: [newPayvergePaymentsContract]
      });

      toast.success("PayvergePayments contract address updated successfully");
      setNewPayvergePaymentsContract("");
    } catch (error: any) {
      console.error("Error setting PayvergePayments contract:", error);
      toast.error(error.message || "Failed to set PayvergePayments contract");
    } finally {
      setIsLoading(false);
    }
  };

  const handleReferralsEmergencyWithdraw = async () => {
    if (!referralsEmergencyAmount) {
      toast.error("Please enter withdrawal amount");
      return;
    }

    try {
      setIsLoading(true);
      
      // Use zero address for ETH if no token specified
      const tokenAddress = referralsEmergencyToken || "0x0000000000000000000000000000000000000000";
      const amount = referralsEmergencyToken ? 
        parseUnits(referralsEmergencyAmount, 6) : // Assume USDC decimals if token specified
        parseUnits(referralsEmergencyAmount, 18); // ETH decimals
      
      await writeContract({
        address: config.referrals,
        abi: PAYVERGE_REFERRALS_ADMIN_ABI,
        functionName: "emergencyWithdraw",
        args: [tokenAddress, amount]
      });

      toast.success("Emergency withdrawal completed successfully");
      setReferralsEmergencyToken("");
      setReferralsEmergencyAmount("");
    } catch (error: any) {
      console.error("Error performing emergency withdrawal:", error);
      toast.error(error.message || "Failed to perform emergency withdrawal");
    } finally {
      setIsLoading(false);
    }
  };

  // PayvergePayments additional handlers
  const handleCancelPlatformFeeUpdate = async () => {
    try {
      setIsLoading(true);
      
      await writeContract({
        address: config.payments,
        abi: PAYVERGE_PAYMENTS_ADMIN_ABI,
        functionName: "cancelPlatformFeeUpdate",
        args: []
      });

      toast.success("Platform fee update cancelled successfully");
    } catch (error: any) {
      console.error("Error cancelling platform fee update:", error);
      toast.error(error.message || "Failed to cancel platform fee update");
    } finally {
      setIsLoading(false);
    }
  };

  const handleProposeRegistrationFeeUpdate = async () => {
    if (!newRegistrationFee) {
      toast.error("Please enter a registration fee");
      return;
    }

    try {
      setIsLoading(true);
      const feeInUSDC = parseUnits(newRegistrationFee, 6);
      
      await writeContract({
        address: config.payments,
        abi: PAYVERGE_PAYMENTS_ADMIN_ABI,
        functionName: "proposeRegistrationFeeUpdate",
        args: [feeInUSDC]
      });

      toast.success("Registration fee update proposed successfully");
      setNewRegistrationFee("");
    } catch (error: any) {
      console.error("Error proposing registration fee update:", error);
      toast.error(error.message || "Failed to propose registration fee update");
    } finally {
      setIsLoading(false);
    }
  };

  const handleExecuteRegistrationFeeUpdate = async () => {
    try {
      setIsLoading(true);
      
      await writeContract({
        address: config.payments,
        abi: PAYVERGE_PAYMENTS_ADMIN_ABI,
        functionName: "executeRegistrationFeeUpdate",
        args: []
      });

      toast.success("Registration fee update executed successfully");
    } catch (error: any) {
      console.error("Error executing registration fee update:", error);
      toast.error(error.message || "Failed to execute registration fee update");
    } finally {
      setIsLoading(false);
    }
  };

  const handleCancelRegistrationFeeUpdate = async () => {
    try {
      setIsLoading(true);
      
      await writeContract({
        address: config.payments,
        abi: PAYVERGE_PAYMENTS_ADMIN_ABI,
        functionName: "cancelRegistrationFeeUpdate",
        args: []
      });

      toast.success("Registration fee update cancelled successfully");
    } catch (error: any) {
      console.error("Error cancelling registration fee update:", error);
      toast.error(error.message || "Failed to cancel registration fee update");
    } finally {
      setIsLoading(false);
    }
  };

  const handleSetBillCreator = async () => {
    if (!newBillCreator) {
      toast.error("Please enter bill creator address");
      return;
    }

    try {
      setIsLoading(true);
      
      await writeContract({
        address: config.payments,
        abi: PAYVERGE_PAYMENTS_ADMIN_ABI,
        functionName: "setBillCreator",
        args: [newBillCreator]
      });

      toast.success("Bill creator address updated successfully");
      setNewBillCreator("");
    } catch (error: any) {
      console.error("Error setting bill creator:", error);
      toast.error(error.message || "Failed to set bill creator");
    } finally {
      setIsLoading(false);
    }
  };

  const handleSetFeeUpdateDelay = async () => {
    if (!newFeeDelay) {
      toast.error("Please enter fee update delay");
      return;
    }

    try {
      setIsLoading(true);
      const delayInSeconds = BigInt(parseInt(newFeeDelay) * 3600); // Convert hours to seconds
      
      await writeContract({
        address: config.payments,
        abi: PAYVERGE_PAYMENTS_ADMIN_ABI,
        functionName: "setFeeUpdateDelay",
        args: [delayInSeconds]
      });

      toast.success("Fee update delay updated successfully");
      setNewFeeDelay("");
    } catch (error: any) {
      console.error("Error setting fee update delay:", error);
      toast.error(error.message || "Failed to set fee update delay");
    } finally {
      setIsLoading(false);
    }
  };

  const handleSetReferralsContract = async () => {
    if (!newReferralsContract) {
      toast.error("Please enter referrals contract address");
      return;
    }

    try {
      setIsLoading(true);
      
      await writeContract({
        address: config.payments,
        abi: PAYVERGE_PAYMENTS_ADMIN_ABI,
        functionName: "setReferralsContract",
        args: [newReferralsContract]
      });

      toast.success("Referrals contract address updated successfully");
      setNewReferralsContract("");
    } catch (error: any) {
      console.error("Error setting referrals contract:", error);
      toast.error(error.message || "Failed to set referrals contract");
    } finally {
      setIsLoading(false);
    }
  };

  const handleSetProfitSplitContract = async () => {
    if (!newProfitSplitContract) {
      toast.error("Please enter profit split contract address");
      return;
    }

    try {
      setIsLoading(true);
      
      await writeContract({
        address: config.payments,
        abi: PAYVERGE_PAYMENTS_ADMIN_ABI,
        functionName: "setProfitSplitContract",
        args: [newProfitSplitContract]
      });

      toast.success("Profit split contract address updated successfully");
      setNewProfitSplitContract("");
    } catch (error: any) {
      console.error("Error setting profit split contract:", error);
      toast.error(error.message || "Failed to set profit split contract");
    } finally {
      setIsLoading(false);
    }
  };

  return (
    <div className="flex flex-col gap-6 p-6">
      <div className="flex flex-col gap-2">
        <h1 className="text-3xl font-bold">Payverge Contract Management</h1>
        <p className="text-default-500">
          Manage and configure your Payverge smart contracts
        </p>
      </div>

      <Tabs 
        selectedKey={selectedTab} 
        onSelectionChange={(key) => setSelectedTab(key as string)}
        className="w-full"
      >
        <Tab key="payments" title="PayvergePayments">
          <div className="grid grid-cols-1 lg:grid-cols-2 gap-6">
            {/* Current Values */}
            <Card>
              <CardHeader>
                <h3 className="text-xl font-semibold">Current Configuration</h3>
              </CardHeader>
              <Divider />
              <CardBody className="space-y-4">
                <div className="flex justify-between items-center">
                  <span className="text-sm text-default-500">Platform Fee Rate:</span>
                  <Chip color="primary" variant="flat">
                    {formatPercentage(platformFeeRate)}
                  </Chip>
                </div>
                
                <div className="flex justify-between items-center">
                  <span className="text-sm text-default-500">Registration Fee:</span>
                  <Chip color="warning" variant="flat">
                    {formatUSDC(registrationFee)}
                  </Chip>
                </div>
                
                <div className="flex justify-between items-center">
                  <span className="text-sm text-default-500">Bill Creator:</span>
                  <Chip color="secondary" variant="flat" className="font-mono text-xs">
                    {formatAddress(billCreatorAddress)}
                  </Chip>
                </div>
                
                <div className="flex justify-between items-center">
                  <span className="text-sm text-default-500">Fee Update Delay:</span>
                  <Chip color="default" variant="flat">
                    {formatDelay(feeUpdateDelay)}
                  </Chip>
                </div>
                
                <div className="flex justify-between items-center">
                  <span className="text-sm text-default-500">Contract Status:</span>
                  <Chip color={paymentsContractPaused ? "danger" : "success"} variant="flat">
                    {paymentsContractPaused ? "Paused" : "Active"}
                  </Chip>
                </div>
              </CardBody>
            </Card>

            {/* Admin Actions */}
            <div className="space-y-6">
              <Card>
                <CardHeader>
                  <h3 className="text-xl font-semibold">Fee Management</h3>
                </CardHeader>
                <Divider />
                <CardBody className="space-y-4">
                  <div className="space-y-2">
                    <label className="text-sm font-medium">Platform Fee Rate (%)</label>
                    <div className="flex gap-2">
                      <Input
                        placeholder="e.g., 2.5"
                        value={newPlatformFee}
                        onChange={(e) => setNewPlatformFee(e.target.value)}
                        type="number"
                        step="0.1"
                        max="10"
                      />
                      <Button 
                        color="primary" 
                        onClick={handleProposePlatformFeeUpdate}
                        isLoading={isLoading}
                      >
                        Propose
                      </Button>
                    </div>
                  </div>

                  <div className="flex gap-2">
                    <Button 
                      color="warning" 
                      onClick={handleExecutePlatformFeeUpdate}
                      isLoading={isLoading}
                      className="flex-1"
                    >
                      Execute Pending Fee Update
                    </Button>
                    <Button 
                      color="default" 
                      onClick={handleCancelPlatformFeeUpdate}
                      isLoading={isLoading}
                      className="flex-1"
                    >
                      Cancel Pending Update
                    </Button>
                  </div>

                  <Divider />

                  <div className="space-y-2">
                    <label className="text-sm font-medium">Registration Fee (USDC)</label>
                    <div className="flex gap-2">
                      <Input
                        placeholder="e.g., 50.00"
                        value={newRegistrationFee}
                        onChange={(e) => setNewRegistrationFee(e.target.value)}
                        type="number"
                        step="0.01"
                        max="1000"
                      />
                      <Button 
                        color="primary" 
                        onClick={handleProposeRegistrationFeeUpdate}
                        isLoading={isLoading}
                      >
                        Propose
                      </Button>
                    </div>
                  </div>

                  <div className="flex gap-2">
                    <Button 
                      color="warning" 
                      onClick={handleExecuteRegistrationFeeUpdate}
                      isLoading={isLoading}
                      className="flex-1"
                    >
                      Execute Pending Fee Update
                    </Button>
                    <Button 
                      color="default" 
                      onClick={handleCancelRegistrationFeeUpdate}
                      isLoading={isLoading}
                      className="flex-1"
                    >
                      Cancel Pending Update
                    </Button>
                  </div>
                </CardBody>
              </Card>

              <Card>
                <CardHeader>
                  <h3 className="text-xl font-semibold">Contract Configuration</h3>
                </CardHeader>
                <Divider />
                <CardBody className="space-y-4">
                  <div className="space-y-2">
                    <label className="text-sm font-medium">Bill Creator Address</label>
                    <div className="flex gap-2">
                      <Input
                        placeholder="0x..."
                        value={newBillCreator}
                        onChange={(e) => setNewBillCreator(e.target.value)}
                      />
                      <Button 
                        color="primary" 
                        onClick={handleSetBillCreator}
                        isLoading={isLoading}
                      >
                        Update
                      </Button>
                    </div>
                  </div>

                  <div className="space-y-2">
                    <label className="text-sm font-medium">Fee Update Delay (hours)</label>
                    <div className="flex gap-2">
                      <Input
                        placeholder="e.g., 24"
                        value={newFeeDelay}
                        onChange={(e) => setNewFeeDelay(e.target.value)}
                        type="number"
                        min="1"
                        max="720"
                      />
                      <Button 
                        color="primary" 
                        onClick={handleSetFeeUpdateDelay}
                        isLoading={isLoading}
                      >
                        Update
                      </Button>
                    </div>
                  </div>

                  <div className="space-y-2">
                    <label className="text-sm font-medium">Referrals Contract Address</label>
                    <div className="flex gap-2">
                      <Input
                        placeholder="0x..."
                        value={newReferralsContract}
                        onChange={(e) => setNewReferralsContract(e.target.value)}
                      />
                      <Button 
                        color="primary" 
                        onClick={handleSetReferralsContract}
                        isLoading={isLoading}
                      >
                        Update
                      </Button>
                    </div>
                  </div>

                  <div className="space-y-2">
                    <label className="text-sm font-medium">Profit Split Contract Address</label>
                    <div className="flex gap-2">
                      <Input
                        placeholder="0x..."
                        value={newProfitSplitContract}
                        onChange={(e) => setNewProfitSplitContract(e.target.value)}
                      />
                      <Button 
                        color="primary" 
                        onClick={handleSetProfitSplitContract}
                        isLoading={isLoading}
                      >
                        Update
                      </Button>
                    </div>
                  </div>
                </CardBody>
              </Card>

              <Card>
                <CardHeader>
                  <h3 className="text-xl font-semibold">Emergency Controls</h3>
                </CardHeader>
                <Divider />
                <CardBody className="space-y-4">
                  <Button 
                    color={paymentsContractPaused ? "success" : "danger"}
                    onClick={() => paymentsContractPaused ? 
                      handleUnpauseContract("payments") : 
                      handlePauseContract("payments")
                    }
                    isLoading={isLoading}
                    className="w-full"
                  >
                    {paymentsContractPaused ? "Unpause" : "Pause"} Contract
                  </Button>
                </CardBody>
              </Card>
            </div>
          </div>
        </Tab>

        <Tab key="referrals" title="PayvergeReferrals">
          <div className="grid grid-cols-1 lg:grid-cols-2 gap-6">
            {/* Current Values */}
            <Card>
              <CardHeader>
                <h3 className="text-xl font-semibold">Referrals Overview</h3>
              </CardHeader>
              <Divider />
              <CardBody className="space-y-4">
                <div className="flex justify-between items-center">
                  <span className="text-sm text-default-500">Total Referrers:</span>
                  <Chip color="primary" variant="flat">
                    {totalReferrers ? Number(totalReferrers).toLocaleString() : "0"}
                  </Chip>
                </div>
                
                <div className="flex justify-between items-center">
                  <span className="text-sm text-default-500">Total Commissions Paid:</span>
                  <Chip color="success" variant="flat">
                    {formatUSDC(totalCommissionsPaid)}
                  </Chip>
                </div>
                
                <div className="flex justify-between items-center">
                  <span className="text-sm text-default-500">Contract Status:</span>
                  <Chip color={referralsContractPaused ? "danger" : "success"} variant="flat">
                    {referralsContractPaused ? "Paused" : "Active"}
                  </Chip>
                </div>
              </CardBody>
            </Card>

            {/* Admin Actions */}
            <Card>
              <CardHeader>
                <h3 className="text-xl font-semibold">Referrals Management</h3>
              </CardHeader>
              <Divider />
              <CardBody className="space-y-4">
                <div className="space-y-2">
                  <label className="text-sm font-medium">Deactivate Referrer</label>
                  <div className="flex gap-2">
                    <Input
                      placeholder="0x..."
                      value={referrerToDeactivate}
                      onChange={(e) => setReferrerToDeactivate(e.target.value)}
                    />
                    <Button 
                      color="danger" 
                      onClick={handleDeactivateReferrer}
                      isLoading={isLoading}
                    >
                      Deactivate
                    </Button>
                  </div>
                </div>

                <Divider />

                <div className="space-y-2">
                  <label className="text-sm font-medium">Set PayvergePayments Contract</label>
                  <div className="flex gap-2">
                    <Input
                      placeholder="Contract Address (0x...)"
                      value={newPayvergePaymentsContract}
                      onChange={(e) => setNewPayvergePaymentsContract(e.target.value)}
                    />
                    <Button 
                      color="primary" 
                      onClick={handleSetPayvergePaymentsContract}
                      isLoading={isLoading}
                    >
                      Update
                    </Button>
                  </div>
                </div>

                <Divider />

                <div className="space-y-2">
                  <label className="text-sm font-medium">Emergency Withdraw</label>
                  <div className="space-y-2">
                    <Input
                      placeholder="Token Address (0x... or leave empty for ETH)"
                      value={referralsEmergencyToken}
                      onChange={(e) => setReferralsEmergencyToken(e.target.value)}
                    />
                    <div className="flex gap-2">
                      <Input
                        placeholder="Amount"
                        value={referralsEmergencyAmount}
                        onChange={(e) => setReferralsEmergencyAmount(e.target.value)}
                        type="number"
                        step="0.000001"
                      />
                      <Button 
                        color="warning" 
                        onClick={handleReferralsEmergencyWithdraw}
                        isLoading={isLoading}
                      >
                        Withdraw
                      </Button>
                    </div>
                  </div>
                </div>

                <Divider />

                <Button 
                  color={referralsContractPaused ? "success" : "danger"}
                  onClick={() => referralsContractPaused ? 
                    handleUnpauseContract("referrals") : 
                    handlePauseContract("referrals")
                  }
                  isLoading={isLoading}
                  className="w-full"
                >
                  {referralsContractPaused ? "Unpause" : "Pause"} Contract
                </Button>
              </CardBody>
            </Card>
          </div>
        </Tab>

        <Tab key="profitSplit" title="PayvergeProfitSplit">
          <div className="grid grid-cols-1 lg:grid-cols-2 gap-6">
            {/* Current Values */}
            <Card>
              <CardHeader>
                <h3 className="text-xl font-semibold">Profit Split Overview</h3>
              </CardHeader>
              <Divider />
              <CardBody className="space-y-4">
                <div className="flex justify-between items-center">
                  <span className="text-sm text-default-500">Total Distributed:</span>
                  <Chip color="success" variant="flat">
                    {formatUSDC(totalDistributed)}
                  </Chip>
                </div>
                
                <div className="flex justify-between items-center">
                  <span className="text-sm text-default-500">Active Beneficiaries:</span>
                  <Chip color="primary" variant="flat">
                    {beneficiaryCount ? Number(beneficiaryCount) : 0}
                  </Chip>
                </div>
                
                <div className="flex justify-between items-center">
                  <span className="text-sm text-default-500">Total Allocated:</span>
                  <Chip color="warning" variant="flat">
                    {formatPercentage(totalPercentageAllocated)}
                  </Chip>
                </div>
                
                <div className="flex justify-between items-center">
                  <span className="text-sm text-default-500">Contract Status:</span>
                  <Chip color={profitSplitContractPaused ? "danger" : "success"} variant="flat">
                    {profitSplitContractPaused ? "Paused" : "Active"}
                  </Chip>
                </div>
              </CardBody>
            </Card>

            {/* Admin Actions */}
            <div className="space-y-6">
              <Card>
                <CardHeader>
                  <h3 className="text-xl font-semibold">Beneficiary Management</h3>
                </CardHeader>
                <Divider />
                <CardBody className="space-y-4">
                  <div className="space-y-2">
                    <label className="text-sm font-medium">Add New Beneficiary</label>
                    <div className="space-y-2">
                      <Input
                        placeholder="Beneficiary Address (0x...)"
                        value={newBeneficiary}
                        onChange={(e) => setNewBeneficiary(e.target.value)}
                      />
                      <Input
                        placeholder="Beneficiary Name"
                        value={newBeneficiaryName}
                        onChange={(e) => setNewBeneficiaryName(e.target.value)}
                      />
                      <div className="flex gap-2">
                        <Input
                          placeholder="Percentage (e.g., 25.5)"
                          value={newBeneficiaryPercentage}
                          onChange={(e) => setNewBeneficiaryPercentage(e.target.value)}
                          type="number"
                          step="0.1"
                          max="100"
                        />
                        <Button 
                          color="primary" 
                          onClick={handleAddBeneficiary}
                          isLoading={isLoading}
                        >
                          Add
                        </Button>
                      </div>
                    </div>
                  </div>

                  <Divider />

                  <div className="space-y-2">
                    <label className="text-sm font-medium">Update Beneficiary Percentage</label>
                    <div className="flex gap-2">
                      <Input
                        placeholder="Beneficiary Address (0x...)"
                        value={beneficiaryToUpdate}
                        onChange={(e) => setBeneficiaryToUpdate(e.target.value)}
                      />
                      <Input
                        placeholder="New %"
                        value={newPercentage}
                        onChange={(e) => setNewPercentage(e.target.value)}
                        type="number"
                        step="0.1"
                        max="100"
                      />
                      <Button 
                        color="warning" 
                        onClick={handleUpdateBeneficiaryPercentage}
                        isLoading={isLoading}
                      >
                        Update
                      </Button>
                    </div>
                  </div>

                  <div className="space-y-2">
                    <label className="text-sm font-medium">Remove Beneficiary</label>
                    <div className="flex gap-2">
                      <Input
                        placeholder="Beneficiary Address (0x...)"
                        value={beneficiaryToRemove}
                        onChange={(e) => setBeneficiaryToRemove(e.target.value)}
                      />
                      <Button 
                        color="danger" 
                        onClick={handleRemoveBeneficiary}
                        isLoading={isLoading}
                      >
                        Remove
                      </Button>
                    </div>
                  </div>
                </CardBody>
              </Card>

              <Card>
                <CardHeader>
                  <h3 className="text-xl font-semibold">Profit Distribution</h3>
                </CardHeader>
                <Divider />
                <CardBody className="space-y-4">
                  <div className="space-y-2">
                    <label className="text-sm font-medium">Distribute Specific Amount (USDC)</label>
                    <div className="flex gap-2">
                      <Input
                        placeholder="100.00"
                        value={distributionAmount}
                        onChange={(e) => setDistributionAmount(e.target.value)}
                        type="number"
                        step="0.01"
                      />
                      <Button 
                        color="success" 
                        onClick={handleDistributeProfits}
                        isLoading={isLoading}
                      >
                        Distribute
                      </Button>
                    </div>
                  </div>

                  <Button 
                    color="primary" 
                    onClick={handleDistributeAllProfits}
                    isLoading={isLoading}
                    className="w-full"
                  >
                    Distribute All Available Profits
                  </Button>
                </CardBody>
              </Card>

              <Card>
                <CardHeader>
                  <h3 className="text-xl font-semibold">Role Management</h3>
                </CardHeader>
                <Divider />
                <CardBody className="space-y-4">
                  <div className="space-y-2">
                    <label className="text-sm font-medium">Grant Distributor Role</label>
                    <div className="flex gap-2">
                      <Input
                        placeholder="Address (0x...)"
                        value={distributorToGrant}
                        onChange={(e) => setDistributorToGrant(e.target.value)}
                      />
                      <Button 
                        color="success" 
                        onClick={handleGrantDistributorRole}
                        isLoading={isLoading}
                      >
                        Grant
                      </Button>
                    </div>
                  </div>

                  <div className="space-y-2">
                    <label className="text-sm font-medium">Revoke Distributor Role</label>
                    <div className="flex gap-2">
                      <Input
                        placeholder="Address (0x...)"
                        value={distributorToRevoke}
                        onChange={(e) => setDistributorToRevoke(e.target.value)}
                      />
                      <Button 
                        color="danger" 
                        onClick={handleRevokeDistributorRole}
                        isLoading={isLoading}
                      >
                        Revoke
                      </Button>
                    </div>
                  </div>
                </CardBody>
              </Card>

              <Card>
                <CardHeader>
                  <h3 className="text-xl font-semibold">Emergency Controls</h3>
                </CardHeader>
                <Divider />
                <CardBody className="space-y-4">
                  <Button 
                    color={profitSplitContractPaused ? "success" : "danger"}
                    onClick={() => profitSplitContractPaused ? 
                      handleUnpauseContract("profitSplit") : 
                      handlePauseContract("profitSplit")
                    }
                    isLoading={isLoading}
                    className="w-full"
                  >
                    {profitSplitContractPaused ? "Unpause" : "Pause"} Contract
                  </Button>
                </CardBody>
              </Card>
            </div>
          </div>
        </Tab>
      </Tabs>
    </div>
  );
};
