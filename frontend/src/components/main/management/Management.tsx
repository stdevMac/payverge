"use client";
import { useState, useCallback, useEffect } from "react";
import { Button, Input, Card, CardBody, CardHeader, Spinner, Tabs, Tab, Table, TableHeader, TableColumn, TableBody, TableRow, TableCell, Divider } from "@nextui-org/react";
import { MultisigTransaction } from "@/components/MultisigTransaction";
import { writeContract, readContract } from "wagmi/actions";
import { config } from "@/config";
// Placeholder ABIs - replace with actual contract ABIs when available
const DealManagerABI = [
  {
    "type": "function",
    "name": "treasury",
    "inputs": [],
    "outputs": [{"name": "", "type": "address"}]
  },
  {
    "type": "function",
    "name": "setTreasury",
    "inputs": [{"name": "_treasury", "type": "address"}],
    "outputs": []
  },
  {
    "type": "function",
    "name": "setReferralRewardsPercent",
    "inputs": [
      {"name": "dealId", "type": "string"},
      {"name": "percent", "type": "uint256"}
    ],
    "outputs": []
  }
];

const PortfolioManagerABI = [
  {
    "type": "function",
    "name": "whitelistSigner",
    "inputs": [],
    "outputs": [{"name": "", "type": "address"}]
  },
  {
    "type": "function",
    "name": "setWhitelistSigner",
    "inputs": [{"name": "_signer", "type": "address"}],
    "outputs": []
  }
];

const MultisigCallerABI = [
  {
    "type": "function",
    "name": "requiredApprovals",
    "inputs": [],
    "outputs": [{"name": "", "type": "uint256"}]
  },
  {
    "type": "function",
    "name": "APPROVER_ROLE",
    "inputs": [],
    "outputs": [{"name": "", "type": "bytes32"}]
  },
  {
    "type": "function",
    "name": "getRoleMemberCount",
    "inputs": [{"name": "role", "type": "bytes32"}],
    "outputs": [{"name": "", "type": "uint256"}]
  },
  {
    "type": "function",
    "name": "getRoleMembers",
    "inputs": [{"name": "role", "type": "bytes32"}],
    "outputs": [{"name": "", "type": "address[]"}]
  },
  {
    "type": "function",
    "name": "grantRole",
    "inputs": [
      {"name": "role", "type": "bytes32"},
      {"name": "account", "type": "address"}
    ],
    "outputs": []
  },
  {
    "type": "function",
    "name": "revokeRole",
    "inputs": [
      {"name": "role", "type": "bytes32"},
      {"name": "account", "type": "address"}
    ],
    "outputs": []
  },
  {
    "type": "function",
    "name": "setRequiredApprovals",
    "inputs": [{"name": "_required", "type": "uint256"}],
    "outputs": []
  }
];
import { MULTISIG_CALLER_ADDRESS } from "@/api/multisig";
import { useTransactionState } from "@/hooks/useTransactionState";
import toast from "react-hot-toast";
import { ethers } from "ethers";

type Props = {};

export const Management = (props: Props) => {
  // Treasury and whitelist signer management
  const [newTreasuryAddress, setNewTreasuryAddress] = useState("");
  const [newWhitelistSigner, setNewWhitelistSigner] = useState("");
  const [currentTreasury, setCurrentTreasury] = useState("");
  const [currentWhitelistSigner, setCurrentWhitelistSigner] = useState("");
  
  // Referral rewards management
  const [dealId, setDealId] = useState("");
  const [referralRewardsPercent, setReferralRewardsPercent] = useState("");
  

  

  
  // Multisig management
  const [multisigAdmins, setMultisigAdmins] = useState<string[]>([]);
  const [newAdmin, setNewAdmin] = useState("");
  const [requiredApprovals, setRequiredApprovals] = useState(0);
  const [newRequiredApprovals, setNewRequiredApprovals] = useState("");
  
  // Loading states
  const [isLoadingValues, setIsLoadingValues] = useState(true);
  const [isLoadingMultisig, setIsLoadingMultisig] = useState(true);
  
  // Selected tab
  const [selectedTab, setSelectedTab] = useState("contracts");
  
  // Multisig modal
  const [multisigModal, setMultisigModal] = useState<{
    isOpen: boolean;
    targetAddress?: string;
    targetAddressType?: string;
    methodName?: string;
    methodType?: string;
    params?: any[];
    abi?: any[];
    value?: bigint;
    onExecuted?: (tx: `0x${string}`) => void;
  }>({
    isOpen: false,
  });

  const { isProcessing, startProcessing, stopProcessing } = useTransactionState({});

  const fetchCurrentValues = useCallback(async () => {
    try {
      setIsLoadingValues(true);
      
      // Fetch current treasury address
      const treasuryAddress = await readContract(config.wagmiConfig, {
        // @ts-ignore
        address: process.env.NEXT_PUBLIC_DEAL_MANAGER_ADDRESS,
        abi: DealManagerABI,
        functionName: "treasury",
      });
      setCurrentTreasury(treasuryAddress as string);

      // Fetch current whitelist signer
      const whitelistSigner = await readContract(config.wagmiConfig, {
        // @ts-ignore
        address: process.env.NEXT_PUBLIC_PORTFOLIO_MANAGER_ADDRESS,
        abi: PortfolioManagerABI,
        functionName: "whitelistSigner",
      });
      setCurrentWhitelistSigner(whitelistSigner as string);
    } catch (error) {
      console.error("Error fetching current values:", error);
      toast.error("Failed to fetch current values");
    } finally {
      setIsLoadingValues(false);
    }
  }, []);
  
  const fetchMultisigData = useCallback(async () => {
    try {
      setIsLoadingMultisig(true);
      
      // Fetch required approvals
      const requiredApprovalsCount = await readContract(config.wagmiConfig, {
        // @ts-ignore
        address: MULTISIG_CALLER_ADDRESS,
        abi: MultisigCallerABI,
        functionName: "requiredApprovals",
      });
      setRequiredApprovals(Number(requiredApprovalsCount));
      
      // Get APPROVER_ROLE constant
      const approverRole = await readContract(config.wagmiConfig, {
        // @ts-ignore
        address: MULTISIG_CALLER_ADDRESS,
        abi: MultisigCallerABI,
        functionName: "APPROVER_ROLE",
      });
      
      // Get approvers count using getRoleMemberCount
      const approversCount = await readContract(config.wagmiConfig, {
        // @ts-ignore
        address: MULTISIG_CALLER_ADDRESS,
        abi: MultisigCallerABI,
        functionName: "getRoleMemberCount",
        args: [approverRole],
      });
      
      // Get the actual list of approvers using getRoleMembers
      const approvers = await readContract(config.wagmiConfig, {
        // @ts-ignore
        address: MULTISIG_CALLER_ADDRESS,
        abi: MultisigCallerABI,
        functionName: "getRoleMembers",
        args: [approverRole],
      });
      
      setMultisigAdmins(approvers as string[]);
    } catch (error) {
      console.error("Error fetching multisig data:", error);
      toast.error("Failed to fetch multisig data");
    } finally {
      setIsLoadingMultisig(false);
    }
  }, []);

  useEffect(() => {
    fetchCurrentValues();
    fetchMultisigData();
  }, [fetchCurrentValues, fetchMultisigData]);

  const handleSetTreasury = useCallback(async () => {
    if (!newTreasuryAddress) {
      toast.error("Please enter a treasury address");
      return;
    }

    // Validate address format
    if (!ethers.isAddress(newTreasuryAddress)) {
      toast.error("Invalid address format");
      return;
    }

    // Check if address is the same as current
    if (newTreasuryAddress.toLowerCase() === currentTreasury.toLowerCase()) {
      toast.error("New address is the same as current");
      return;
    }

    try {
      startProcessing();
      setMultisigModal({
        isOpen: true,
        targetAddress: process.env.NEXT_PUBLIC_DEAL_MANAGER_ADDRESS,
        targetAddressType: "DealManager",
        methodName: "setTreasury",
        methodType: "admin",
        params: [newTreasuryAddress],
        abi: DealManagerABI,
        onExecuted: async (tx) => {
          toast.success("Treasury address updated successfully");
          setNewTreasuryAddress("");
        },
      });
    } catch (error: any) {
      console.error("Error setting treasury:", error);
      toast.error(error.message || "Failed to set treasury");
    } finally {
      stopProcessing();
    }
  }, [newTreasuryAddress, currentTreasury, startProcessing, stopProcessing]);

  const handleSetWhitelistSigner = useCallback(async () => {
    if (!newWhitelistSigner) {
      toast.error("Please enter a whitelist signer address");
      return;
    }

    // Validate address format
    if (!ethers.isAddress(newWhitelistSigner)) {
      toast.error("Invalid address format");
      return;
    }

    // Check if address is the same as current
    if (newWhitelistSigner.toLowerCase() === currentWhitelistSigner.toLowerCase()) {
      toast.error("New address is the same as current");
      return;
    }

    try {
      startProcessing();
      setMultisigModal({
        isOpen: true,
        targetAddress: process.env.NEXT_PUBLIC_PORTFOLIO_MANAGER_ADDRESS,
        targetAddressType: "PortfolioManager",
        methodName: "setWhitelistSigner",
        methodType: "admin",
        params: [newWhitelistSigner],
        abi: PortfolioManagerABI,
        onExecuted: async (tx) => {
          toast.success("Whitelist signer updated successfully");
          setNewWhitelistSigner("");
          fetchCurrentValues();
        },
      });
    } catch (error: any) {
      console.error("Error setting whitelist signer:", error);
      toast.error(error.message || "Failed to set whitelist signer");
    } finally {
      stopProcessing();
    }
  }, [newWhitelistSigner, currentWhitelistSigner, startProcessing, stopProcessing, fetchCurrentValues]);
  
  const handleSetReferralRewards = useCallback(async () => {
    if (!dealId) {
      toast.error("Please enter a deal ID");
      return;
    }

    if (!referralRewardsPercent) {
      toast.error("Please enter referral rewards percentage");
      return;
    }

    const rewardsPercent = parseInt(referralRewardsPercent);
    if (isNaN(rewardsPercent) || rewardsPercent < 0 || rewardsPercent > 100) {
      toast.error("Referral rewards percentage must be between 0 and 100");
      return;
    }

    try {
      startProcessing();
      setMultisigModal({
        isOpen: true,
        targetAddress: process.env.NEXT_PUBLIC_DEAL_MANAGER_ADDRESS,
        targetAddressType: "DealManager",
        methodName: "setReferralRewardsPercent",
        methodType: "admin",
        params: [dealId, rewardsPercent],
        abi: DealManagerABI,
        onExecuted: async (tx) => {
          toast.success("Referral rewards percentage updated successfully");
          setDealId("");
          setReferralRewardsPercent("");
        },
      });
    } catch (error: any) {
      console.error("Error setting referral rewards percentage:", error);
      toast.error(error.message || "Failed to set referral rewards percentage");
    } finally {
      stopProcessing();
    }
  }, [dealId, referralRewardsPercent, startProcessing, stopProcessing]);







  const handleAddAdmin = useCallback(async () => {
    if (!newAdmin) {
      toast.error("Please enter an admin address");
      return;
    }

    if (!ethers.isAddress(newAdmin)) {
      toast.error("Invalid address format");
      return;
    }

    try {
      startProcessing();
      
      // Get APPROVER_ROLE constant
      const approverRole = await readContract(config.wagmiConfig, {
        // @ts-ignore
        address: MULTISIG_CALLER_ADDRESS,
        abi: MultisigCallerABI,
        functionName: "APPROVER_ROLE",
      });
      
      setMultisigModal({
        isOpen: true,
        targetAddress: MULTISIG_CALLER_ADDRESS,
        targetAddressType: "MultisigCaller",
        methodName: "grantRole",
        methodType: "admin",
        params: [approverRole, newAdmin],
        abi: MultisigCallerABI,
        onExecuted: async (tx) => {
          toast.success("Admin added successfully");
          setNewAdmin("");
          fetchMultisigData();
        },
      });
    } catch (error: any) {
      console.error("Error adding admin:", error);
      toast.error(error.message || "Failed to add admin");
    } finally {
      stopProcessing();
    }
  }, [newAdmin, startProcessing, stopProcessing, fetchMultisigData]);

  const handleRemoveAdmin = useCallback(async (adminAddress: string) => {
    if (!ethers.isAddress(adminAddress)) {
      toast.error("Invalid address format");
      return;
    }
    
    try {
      startProcessing();
      
      // Get APPROVER_ROLE constant
      const approverRole = await readContract(config.wagmiConfig, {
        // @ts-ignore
        address: MULTISIG_CALLER_ADDRESS,
        abi: MultisigCallerABI,
        functionName: "APPROVER_ROLE",
      });
      
      setMultisigModal({
        isOpen: true,
        targetAddress: MULTISIG_CALLER_ADDRESS,
        targetAddressType: "MultisigCaller",
        methodName: "revokeRole",
        methodType: "admin",
        params: [approverRole, adminAddress],
        abi: MultisigCallerABI,
        onExecuted: async (tx) => {
          toast.success("Admin removed successfully");
          fetchMultisigData();
        },
      });
    } catch (error: any) {
      console.error("Error removing admin:", error);
      toast.error(error.message || "Failed to remove admin");
    } finally {
      stopProcessing();
    }
  }, [startProcessing, stopProcessing, fetchMultisigData]);

  const handleSetRequiredApprovals = useCallback(async () => {
    if (!newRequiredApprovals) {
      toast.error("Please enter required approvals");
      return;
    }

    const approvals = parseInt(newRequiredApprovals);
    if (isNaN(approvals) || approvals <= 0 || approvals > multisigAdmins.length) {
      toast.error(`Required approvals must be between 1 and ${multisigAdmins.length}`);
      return;
    }

    try {
      startProcessing();
      setMultisigModal({
        isOpen: true,
        targetAddress: MULTISIG_CALLER_ADDRESS,
        targetAddressType: "MultisigCaller",
        methodName: "setRequiredApprovals",
        methodType: "admin",
        params: [approvals],
        abi: MultisigCallerABI,
        onExecuted: async (tx) => {
          toast.success("Required approvals updated successfully");
          setNewRequiredApprovals("");
          fetchMultisigData();
        },
      });
    } catch (error: any) {
      console.error("Error setting required approvals:", error);
      toast.error(error.message || "Failed to set required approvals");
    } finally {
      stopProcessing();
    }
  }, [newRequiredApprovals, multisigAdmins.length, startProcessing, stopProcessing, fetchMultisigData]);

  return (
    <div className="w-full flex flex-col gap-4 p-4">
      <h1 className="text-2xl font-bold mb-4">Admin Management</h1>
      
      <Tabs 
        aria-label="Admin Management Tabs" 
        selectedKey={selectedTab}
        onSelectionChange={(key) => setSelectedTab(key as string)}
        className="mb-4"
      >
        <Tab key="contracts" title="Contract Management">
          <div className="grid grid-cols-1 md:grid-cols-2 gap-4">
            {/* Deal Manager Settings */}
            <Card>
              <CardHeader>
                <h2 className="text-xl font-semibold">Deal Manager Settings</h2>
              </CardHeader>
              <CardBody className="flex flex-col gap-4">
                <div className="flex flex-col gap-2">
                  {isLoadingValues ? (
                    <div className="flex justify-center p-4">
                      <Spinner size="sm" />
                    </div>
                  ) : (
                    <div className="text-sm text-gray-500 mb-2">
                      Current Treasury: {currentTreasury || "Not set"}
                    </div>
                  )}
                  <Input
                    label="New Treasury Address"
                    placeholder="Enter new treasury address"
                    value={newTreasuryAddress}
                    onChange={(e) => setNewTreasuryAddress(e.target.value)}
                    description="Enter a valid Ethereum address"
                    isInvalid={newTreasuryAddress !== "" && !ethers.isAddress(newTreasuryAddress)}
                    errorMessage={newTreasuryAddress !== "" && !ethers.isAddress(newTreasuryAddress) ? "Invalid address format" : ""}
                  />
                  <Button
                    color="primary"
                    onClick={handleSetTreasury}
                    isLoading={isProcessing}
                    isDisabled={!ethers.isAddress(newTreasuryAddress) || newTreasuryAddress.toLowerCase() === currentTreasury.toLowerCase()}
                  >
                    Set Treasury
                  </Button>
                </div>
                
                <Divider className="my-2" />
                
                <div className="flex flex-col gap-2">
                  <h3 className="text-md font-semibold">Set Referral Rewards</h3>
                  <Input
                    label="Deal ID"
                    placeholder="Enter deal ID"
                    value={dealId}
                    onChange={(e) => setDealId(e.target.value)}
                  />
                  <Input
                    label="Referral Rewards Percentage"
                    placeholder="Enter percentage (0-100)"
                    value={referralRewardsPercent}
                    onChange={(e) => setReferralRewardsPercent(e.target.value)}
                    description="Percentage value between 0 and 100"
                    isInvalid={referralRewardsPercent !== "" && (isNaN(parseInt(referralRewardsPercent)) || parseInt(referralRewardsPercent) < 0 || parseInt(referralRewardsPercent) > 100)}
                    errorMessage={referralRewardsPercent !== "" && (isNaN(parseInt(referralRewardsPercent)) || parseInt(referralRewardsPercent) < 0 || parseInt(referralRewardsPercent) > 100) ? "Value must be between 0 and 100" : ""}
                  />
                  <Button
                    color="primary"
                    onClick={handleSetReferralRewards}
                    isLoading={isProcessing}
                    isDisabled={!dealId || !referralRewardsPercent || isNaN(parseInt(referralRewardsPercent)) || parseInt(referralRewardsPercent) < 0 || parseInt(referralRewardsPercent) > 100}
                  >
                    Set Referral Rewards
                  </Button>
                </div>
              </CardBody>
            </Card>

            {/* Portfolio Manager Settings */}
            <Card>
              <CardHeader>
                <h2 className="text-xl font-semibold">Portfolio Manager Settings</h2>
              </CardHeader>
              <CardBody className="flex flex-col gap-4">
                <div className="flex flex-col gap-2">
                  {isLoadingValues ? (
                    <div className="flex justify-center p-4">
                      <Spinner size="sm" />
                    </div>
                  ) : (
                    <div className="text-sm text-gray-500 mb-2">
                      Current Whitelist Signer: {currentWhitelistSigner || "Not set"}
                    </div>
                  )}
                  <Input
                    label="New Whitelist Signer"
                    placeholder="Enter new whitelist signer address"
                    value={newWhitelistSigner}
                    onChange={(e) => setNewWhitelistSigner(e.target.value)}
                    description="Enter a valid Ethereum address"
                    isInvalid={newWhitelistSigner !== "" && !ethers.isAddress(newWhitelistSigner)}
                    errorMessage={newWhitelistSigner !== "" && !ethers.isAddress(newWhitelistSigner) ? "Invalid address format" : ""}
                  />
                  <Button
                    color="primary"
                    onClick={handleSetWhitelistSigner}
                    isLoading={isProcessing}
                    isDisabled={!ethers.isAddress(newWhitelistSigner) || newWhitelistSigner.toLowerCase() === currentWhitelistSigner.toLowerCase()}
                  >
                    Set Whitelist Signer
                  </Button>
                </div>
              </CardBody>
            </Card>

          </div>
        </Tab>
        
        <Tab key="multisig" title="Multisig Management">
          <div className="grid grid-cols-1 gap-4">
            {/* Multisig Admins */}
            <Card>
              <CardHeader>
                <h2 className="text-xl font-semibold">Multisig Admins</h2>
              </CardHeader>
              <CardBody className="flex flex-col gap-4">
                {isLoadingMultisig ? (
                  <div className="flex justify-center p-4">
                    <Spinner size="sm" />
                  </div>
                ) : (
                  <>
                    <div className="text-sm text-gray-500 mb-2">
                      Current Required Approvals: {requiredApprovals || "Not set"}
                    </div>
                    <Table aria-label="Multisig Admins Table">
                      <TableHeader>
                        <TableColumn>ADMIN ADDRESS</TableColumn>
                        <TableColumn>ACTIONS</TableColumn>
                      </TableHeader>
                      <TableBody>
                        {multisigAdmins.length > 0 ? (
                          multisigAdmins.map((admin, index) => (
                            <TableRow key={index}>
                              <TableCell>{admin}</TableCell>
                              <TableCell>
                                <Button
                                  color="danger"
                                  size="sm"
                                  onClick={() => handleRemoveAdmin(admin)}
                                  isDisabled={isProcessing}
                                >
                                  Remove
                                </Button>
                              </TableCell>
                            </TableRow>
                          ))
                        ) : (
                          <TableRow>
                            <TableCell>No approvers found</TableCell>
                            <TableCell>Add an approver below</TableCell>
                          </TableRow>
                        )}
                      </TableBody>
                    </Table>
                    
                    <Divider className="my-2" />
                    
                    <div className="flex flex-col gap-2">
                      <h3 className="text-md font-semibold">Add New Admin</h3>
                      <Input
                        label="Admin Address"
                        placeholder="Enter new admin address"
                        value={newAdmin}
                        onChange={(e) => setNewAdmin(e.target.value)}
                        description="Enter a valid Ethereum address"
                        isInvalid={newAdmin !== "" && !ethers.isAddress(newAdmin)}
                        errorMessage={newAdmin !== "" && !ethers.isAddress(newAdmin) ? "Invalid address format" : ""}
                      />
                      <Button
                        color="primary"
                        onClick={handleAddAdmin}
                        isLoading={isProcessing}
                        isDisabled={!ethers.isAddress(newAdmin) || multisigAdmins.some(admin => admin.toLowerCase() === newAdmin.toLowerCase())}
                      >
                        Add Admin
                      </Button>
                    </div>
                    
                    <Divider className="my-2" />
                    
                    <div className="flex flex-col gap-2">
                      <h3 className="text-md font-semibold">Set Required Approvals</h3>
                      <Input
                        label="Required Approvals"
                        placeholder="Enter required approvals count"
                        value={newRequiredApprovals}
                        onChange={(e) => setNewRequiredApprovals(e.target.value)}
                        description={`Value between 1 and ${multisigAdmins.length}`}
                        isInvalid={newRequiredApprovals !== "" && (isNaN(parseInt(newRequiredApprovals)) || parseInt(newRequiredApprovals) <= 0 || parseInt(newRequiredApprovals) > multisigAdmins.length)}
                        errorMessage={newRequiredApprovals !== "" && (isNaN(parseInt(newRequiredApprovals)) || parseInt(newRequiredApprovals) <= 0 || parseInt(newRequiredApprovals) > multisigAdmins.length) ? `Value must be between 1 and ${multisigAdmins.length}` : ""}
                      />
                      <Button
                        color="primary"
                        onClick={handleSetRequiredApprovals}
                        isLoading={isProcessing}
                        isDisabled={!newRequiredApprovals || isNaN(parseInt(newRequiredApprovals)) || parseInt(newRequiredApprovals) <= 0 || parseInt(newRequiredApprovals) > multisigAdmins.length}
                      >
                        Set Required Approvals
                      </Button>
                    </div>
                  </>
                )}
              </CardBody>
            </Card>
          </div>
        </Tab>
      </Tabs>

      {multisigModal.isOpen && (
        <MultisigTransaction
          isOpen={multisigModal.isOpen}
          onClose={() => setMultisigModal({ isOpen: false })}
          targetAddress={multisigModal.targetAddress || ""}
          targetAddressType={multisigModal.targetAddressType || ""}
          methodName={multisigModal.methodName || ""}
          methodType={multisigModal.methodType || ""}
          params={multisigModal.params || []}
          abi={multisigModal.abi || []}
          value={multisigModal.value}
          onExecuted={multisigModal.onExecuted}
        />
      )}
    </div>
  );
};

