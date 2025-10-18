"use client";
import {
  useEffect,
  useState,
  useCallback,
  useMemo,
} from "react";
import {
  Table,
  TableHeader,
  TableColumn,
  TableBody,
  TableRow,
  TableCell,
  Chip,
  Tooltip,
  Pagination,
  Select,
  SelectItem,
  Input,
  Button,
  Modal,
  ModalContent,
  ModalHeader,
  ModalBody,
  ModalFooter,
  useDisclosure,
} from "@nextui-org/react";
import { SearchIcon } from "@/components/icons/SearchIcon";
import { DeleteIcon } from "@/components/icons/DeleteIcon";
import { PrimarySpinner, Title } from "@/components";
import { useAccount } from "wagmi";
import { keccak256, toBytes } from "viem";
import {
  useCreateCoupon,
  useDeactivateCoupon,
  useWatchCouponCreated,
  useWatchCouponUsed,
  useFormatUSDC,
} from "@/contracts/hooks";
import {
  getAllCoupons,
  createCoupon as apiCreateCoupon,
  deactivateCoupon as apiDeactivateCoupon,
  CouponDetails,
  formatCouponStatus,
  getCouponStatusColor,
  formatAddress,
  formatExpiryTime,
} from "@/api/coupons";
import { toast } from "react-hot-toast";

const rowsPerPageOptions = [
  { value: "10", label: "10" },
  { value: "20", label: "20" },
  { value: "50", label: "50" },
  { value: "100", label: "100" },
];

const couponColumns = [
  { name: "CODE", uid: "code" },
  { name: "DISCOUNT (USDC)", uid: "discount" },
  { name: "STATUS", uid: "status" },
  { name: "EXPIRY", uid: "expiry" },
  { name: "USED BY", uid: "usedBy" },
  { name: "ACTIONS", uid: "actions" },
];

export const CodesTableManagement = () => {
  const { address } = useAccount();
  const { formatUSDC, parseUSDC } = useFormatUSDC();

  // Modal disclosures
  const { 
    isOpen: isCreateOpen, 
    onOpen: onCreateOpen, 
    onClose: onCreateClose 
  } = useDisclosure();

  // State
  const [coupons, setCoupons] = useState<CouponDetails[]>([]);
  const [loading, setLoading] = useState(true);
  const [page, setPage] = useState(1);
  const [rowsPerPage, setRowsPerPage] = useState(10);
  const [filterValue, setFilterValue] = useState("");
  const [error, setError] = useState("");

  // Create coupon state
  const [createCode, setCreateCode] = useState("");
  const [createAmount, setCreateAmount] = useState("");
  const [createExpiry, setCreateExpiry] = useState("");
  const [isCreating, setIsCreating] = useState(false);

  // Copy to clipboard state
  const [copiedCode, setCopiedCode] = useState<string | null>(null);

  // Smart contract hooks
  const { createCoupon } = useCreateCoupon();
  const { deactivateCoupon } = useDeactivateCoupon();

  // Fetch coupons from API
  const fetchCoupons = useCallback(async () => {
    setLoading(true);
    try {
      const response = await getAllCoupons();
      if (response.success && response.coupons) {
        // Sort coupons: active first, then used, then expired/deactivated
        const sortedCoupons = [...response.coupons].sort((a, b) => {
          const getPriority = (coupon: CouponDetails) => {
            if (coupon.isUsed) return 3;
            if (!coupon.isActive) return 4;
            if (coupon.expiryTime && new Date(coupon.expiryTime) < new Date()) return 2;
            return 1; // Active
          };
          
          return getPriority(a) - getPriority(b);
        });
        
        setCoupons(sortedCoupons);
      } else {
        console.error("Failed to fetch coupons:", response.error);
        setCoupons([]);
        toast.error("Failed to fetch coupons");
      }
    } catch (error) {
      console.error("Error fetching coupons:", error);
      setCoupons([]);
      toast.error("Error fetching coupons");
    } finally {
      setLoading(false);
    }
  }, []);

  // Initialize
  useEffect(() => {
    fetchCoupons();
  }, [fetchCoupons]);

  // Watch for coupon events from smart contract
  useWatchCouponCreated((logs) => {
    logs.forEach((log: any) => {
      console.log('Coupon created on blockchain:', log);
      toast.success('Coupon created on blockchain!');
      fetchCoupons(); // Refresh when new coupons are created
    });
  });

  useWatchCouponUsed((logs) => {
    logs.forEach((log: any) => {
      console.log('Coupon used on blockchain:', log);
      toast.success('Coupon used!');
      fetchCoupons(); // Refresh when coupons are used
    });
  });

  // Copy to clipboard function
  const copyToClipboard = useCallback((code: string) => {
    navigator.clipboard.writeText(code)
      .then(() => {
        setCopiedCode(code);
        toast.success('Code copied to clipboard!');
        setTimeout(() => setCopiedCode(null), 2000);
      })
      .catch(() => {
        toast.error('Failed to copy code');
      });
  }, []);

  // Create coupon handler (smart contract + API)
  const handleCreateCoupon = async () => {
    if (!createCode || !createAmount) {
      setError("Code and amount are required");
      return;
    }

    if (!address) {
      toast.error("Please connect your wallet");
      return;
    }

    setIsCreating(true);
    setError("");

    try {
      const discountAmount = parseUSDC(createAmount);
      const expiryTime = createExpiry 
        ? BigInt(Math.floor(new Date(createExpiry).getTime() / 1000))
        : BigInt(0); // 0 means no expiry

      console.log('Creating coupon:', {
        code: createCode,
        discountAmount: discountAmount.toString(),
        expiryTime: expiryTime.toString()
      });

      // Step 1: Save coupon to backend database first
      console.log('Saving coupon to database...');
      const apiResponse = await apiCreateCoupon({
        code: createCode,
        amount: createAmount,
        expiry: createExpiry || undefined,
      });

      if (!apiResponse.success) {
        throw new Error(apiResponse.error || 'Failed to save coupon to database');
      }

      toast.success(`Coupon "${createCode}" saved to database!`);

      // Step 2: Create coupon on smart contract
      console.log('Creating coupon on blockchain...');
      const hash = await createCoupon(createCode, discountAmount, expiryTime);
      
      toast.success(`Coupon "${createCode}" created on blockchain!`);
      
      // Reset form
      setCreateCode("");
      setCreateAmount("");
      setCreateExpiry("");
      onCreateClose();

      // Refresh coupons (the event watcher will also trigger a refresh)
      setTimeout(() => fetchCoupons(), 2000);

    } catch (error: any) {
      console.error('Error creating coupon:', error);
      toast.error(error.message || "Failed to create coupon");
      setError(error.message || "Failed to create coupon");
    } finally {
      setIsCreating(false);
    }
  };

  // Deactivate coupon handler (smart contract + API)
  const handleDeactivateCoupon = useCallback(async (coupon: CouponDetails) => {
    if (!confirm(`Are you sure you want to deactivate coupon "${coupon.code}"?`)) {
      return;
    }

    if (!address) {
      toast.error("Please connect your wallet");
      return;
    }

    setLoading(true);
    try {
      // Deactivate on smart contract
      await deactivateCoupon(coupon.code);
      
      toast.success(`Coupon "${coupon.code}" deactivated on blockchain!`);
      
      // Refresh coupons
      setTimeout(() => fetchCoupons(), 2000);
    } catch (error: any) {
      console.error('Error deactivating coupon:', error);
      toast.error(error.message || "Failed to deactivate coupon");
    } finally {
      setLoading(false);
    }
  }, [address, deactivateCoupon, fetchCoupons]);

  // Filter and pagination
  const filteredCoupons = useMemo(() => {
    return coupons.filter(coupon =>
      coupon.code.toLowerCase().includes(filterValue.toLowerCase()) ||
      (coupon.usedBy && coupon.usedBy.toLowerCase().includes(filterValue.toLowerCase()))
    );
  }, [coupons, filterValue]);

  const paginatedCoupons = useMemo(() => {
    const start = (page - 1) * rowsPerPage;
    const end = start + rowsPerPage;
    return filteredCoupons.slice(start, end);
  }, [filteredCoupons, page, rowsPerPage]);

  const totalPages = Math.ceil(filteredCoupons.length / rowsPerPage);

  // Render table cell
  const renderCell = useCallback((coupon: CouponDetails, columnKey: React.Key) => {
    switch (columnKey) {
      case "code":
        const isCopied = copiedCode === coupon.code;
        return (
          <div className="flex items-center gap-2">
            <span className="font-mono text-sm select-all">{coupon.code}</span>
            <Button
              size="sm"
              variant="light"
              color={isCopied ? "success" : "primary"}
              onPress={() => copyToClipboard(coupon.code)}
              className="min-w-0 px-2"
            >
              {isCopied ? "✓" : "📋"}
            </Button>
          </div>
        );
      case "discount":
        // Handle both formatted ($25.00) and raw (25000000) amounts
        const formatDiscountAmount = (amount: string) => {
          // If it's already formatted (contains $ or .), return as is
          if (amount.includes('$') || amount.includes('.')) {
            return amount.startsWith('$') ? amount : `$${amount}`;
          }
          // If it's raw USDC units (like 25000000), format it
          const numAmount = parseFloat(amount);
          if (numAmount >= 1000000) {
            return `$${(numAmount / 1000000).toFixed(2)}`;
          }
          return `$${amount}`;
        };

        return (
          <span className="font-semibold">
            {formatDiscountAmount(coupon.discountAmount)}
          </span>
        );
      case "status":
        return (
          <Chip
            color={getCouponStatusColor(coupon)}
            variant="flat"
            size="sm"
          >
            {formatCouponStatus(coupon)}
          </Chip>
        );
      case "expiry":
        return (
          <span className={`text-sm ${
            coupon.expiryTime && new Date(coupon.expiryTime) < new Date() 
              ? "text-danger" 
              : ""
          }`}>
            {formatExpiryTime(coupon.expiryTime)}
          </span>
        );
      case "usedBy":
        return (
          <span className="text-sm font-mono">
            {formatAddress(coupon.usedBy)}
          </span>
        );
      case "actions":
        return (
          <div className="flex items-center gap-2">
            {coupon.isActive && !coupon.isUsed && (
              <Tooltip content="Deactivate coupon">
                <Button
                  size="sm"
                  variant="light"
                  color="danger"
                  onPress={() => handleDeactivateCoupon(coupon)}
                  isLoading={loading}
                  isIconOnly
                >
                  <DeleteIcon />
                </Button>
              </Tooltip>
            )}
          </div>
        );
      default:
        return null;
    }
  }, [copiedCode, copyToClipboard, loading, handleDeactivateCoupon]);

  // Search and pagination handlers
  const onSearchChange = useCallback((value?: string) => {
    setFilterValue(value || "");
    setPage(1);
  }, []);

  const onRowsPerPageChange = useCallback((e: React.ChangeEvent<HTMLSelectElement>) => {
    setRowsPerPage(Number(e.target.value));
    setPage(1);
  }, []);

  if (loading && coupons.length === 0) {
    return <PrimarySpinner />;
  }

  return (
    <div className="w-full flex flex-col gap-4">
      {/* Header */}
      <div className="flex justify-between items-center">
        <Title title="Coupon Management" />
        <Button
          color="primary"
          onPress={onCreateOpen}
          disabled={!address}
        >
          Create Coupon
        </Button>
      </div>

      {/* Wallet Connection Warning */}
      {!address && (
        <div className="bg-warning-50 border border-warning-200 rounded-lg p-4">
          <p className="text-warning-800">
            Please connect your wallet to manage coupons.
          </p>
        </div>
      )}

      {/* Filters and Controls */}
      <div className="flex flex-col sm:flex-row justify-between items-center gap-4 bg-default-50 p-4 rounded-lg shadow-sm">
        <div className="w-full sm:w-auto">
          <Input
            isClearable
            className="w-full sm:max-w-[350px]"
            placeholder="Search by code or address..."
            startContent={<SearchIcon className="text-default-300" />}
            value={filterValue}
            onClear={() => onSearchChange("")}
            onValueChange={onSearchChange}
            size="sm"
          />
        </div>
        <div className="flex items-center gap-3 w-full sm:w-auto justify-end">
          <div className="flex items-center gap-2">
            <Chip color="primary" variant="flat" size="sm">Active</Chip>
            <Chip color="success" variant="flat" size="sm">Used</Chip>
            <Chip color="danger" variant="flat" size="sm">Deactivated</Chip>
            <Chip color="warning" variant="flat" size="sm">Expired</Chip>
          </div>
          <Select
            label="Rows"
            className="w-20"
            size="sm"
            value={rowsPerPage.toString()}
            onChange={onRowsPerPageChange}
          >
            {rowsPerPageOptions.map((option) => (
              <SelectItem key={option.value} value={option.value}>
                {option.label}
              </SelectItem>
            ))}
          </Select>
          <Button 
            size="sm" 
            color="primary" 
            variant="flat" 
            onPress={fetchCoupons}
            isLoading={loading}
          >
            Refresh
          </Button>
        </div>
      </div>

      {/* Table */}
      <Table
        aria-label="Coupon management table"
        isHeaderSticky
        shadow="sm"
        classNames={{
          wrapper: "max-h-[calc(100vh-300px)]",
          table: "min-h-[200px]",
          th: "bg-default-100/80 text-default-800 font-semibold",
          td: "py-3",
          base: "overflow-x-auto",
          tbody: "divide-y divide-default-200",
        }}
        bottomContent={
          totalPages > 1 ? (
            <div className="flex w-full justify-center">
              <Pagination
                isCompact
                showControls
                showShadow
                color="primary"
                page={page}
                total={totalPages}
                onChange={setPage}
              />
            </div>
          ) : null
        }
      >
        <TableHeader columns={couponColumns}>
          {(column) => (
            <TableColumn
              key={column.uid}
              align={column.uid === "actions" ? "center" : "start"}
              className="bg-default-100/50 text-sm uppercase"
            >
              {column.name}
            </TableColumn>
          )}
        </TableHeader>
        <TableBody
          items={paginatedCoupons}
          emptyContent="No coupons found"
          loadingContent={<PrimarySpinner />}
          isLoading={loading}
        >
          {(item) => (
            <TableRow key={item.hash} className="h-[60px]">
              {(columnKey) => (
                <TableCell>{renderCell(item, columnKey)}</TableCell>
              )}
            </TableRow>
          )}
        </TableBody>
      </Table>

      {/* Create Coupon Modal */}
      <Modal isOpen={isCreateOpen} onClose={onCreateClose} size="lg">
        <ModalContent>
          <ModalHeader>Create New Coupon</ModalHeader>
          <ModalBody className="space-y-4">
            {error && (
              <div className="bg-danger-50 border border-danger-200 rounded-lg p-3">
                <p className="text-danger-800 text-sm">{error}</p>
              </div>
            )}
            
            <Input
              label="Coupon Code"
              placeholder="Enter coupon code (e.g., SAVE20)"
              value={createCode}
              onChange={(e) => setCreateCode(e.target.value.toUpperCase())}
              description="This will be the code users enter"
              isRequired
            />
            
            <Input
              label="Discount Amount (USDC)"
              placeholder="Enter discount amount"
              type="number"
              step="0.01"
              min="0"
              value={createAmount}
              onChange={(e) => setCreateAmount(e.target.value)}
              description="Amount to discount from registration fee"
              isRequired
            />
            
            <Input
              label="Expiry Date (Optional)"
              placeholder="Select expiry date"
              type="datetime-local"
              value={createExpiry}
              onChange={(e) => setCreateExpiry(e.target.value)}
              description="Leave empty for no expiration"
            />
          </ModalBody>
          <ModalFooter>
            <Button color="danger" variant="light" onPress={onCreateClose}>
              Cancel
            </Button>
            <Button 
              color="primary" 
              onPress={handleCreateCoupon} 
              isLoading={isCreating}
              disabled={!createCode || !createAmount || !address}
            >
              Create Coupon
            </Button>
          </ModalFooter>
        </ModalContent>
      </Modal>
    </div>
  );
};
