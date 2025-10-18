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
import { EditIcon } from "@/components/icons/EditIcon";
import { PrimarySpinner, Title } from "@/components";
import { useAccount } from "wagmi";
import { parseUnits, formatUnits, keccak256, toBytes } from "viem";
import {
  useCreateCoupon,
  useDeactivateCoupon,
  useWatchCouponCreated,
  useWatchCouponUsed,
  useFormatUSDC,
} from "@/contracts/hooks";
import { toast } from "react-hot-toast";

interface CouponData {
  code: string;
  hash: string;
  discountAmount: bigint;
  expiryTime: bigint;
  isActive: boolean;
  isUsed: boolean;
  usedBy?: string;
}

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

export const CouponTableManagement = () => {
  const { address } = useAccount();
  const { formatUSDC, parseUSDC } = useFormatUSDC();

  // Modal disclosures
  const { isOpen, onOpen, onClose } = useDisclosure();
  const { 
    isOpen: isCreateOpen, 
    onOpen: onCreateOpen, 
    onClose: onCreateClose 
  } = useDisclosure();

  // State
  const [coupons, setCoupons] = useState<CouponData[]>([]);
  const [loading, setLoading] = useState(false);
  const [page, setPage] = useState(1);
  const [rowsPerPage, setRowsPerPage] = useState(10);
  const [filterValue, setFilterValue] = useState("");
  const [selectedCoupon, setSelectedCoupon] = useState<CouponData | null>(null);
  
  // Create coupon state
  const [createCode, setCreateCode] = useState("");
  const [createAmount, setCreateAmount] = useState("");
  const [createExpiry, setCreateExpiry] = useState("");
  const [error, setError] = useState("");

  // Smart contract hooks
  const { createCoupon } = useCreateCoupon();
  const { deactivateCoupon } = useDeactivateCoupon();

  // Local storage for coupon tracking
  const getStoredCoupons = useCallback((): CouponData[] => {
    if (typeof window === 'undefined') return [];
    const stored = localStorage.getItem('payverge-coupons');
    return stored ? JSON.parse(stored) : [];
  }, []);

  const storeCoupon = useCallback((coupon: CouponData) => {
    if (typeof window === 'undefined') return;
    const existing = getStoredCoupons();
    const updated = [...existing.filter(c => c.hash !== coupon.hash), coupon];
    localStorage.setItem('payverge-coupons', JSON.stringify(updated));
    setCoupons(updated);
  }, [getStoredCoupons]);

  // Initialize coupons from localStorage
  useEffect(() => {
    const stored = getStoredCoupons();
    setCoupons(stored);
  }, [getStoredCoupons]);

  // Watch for coupon events
  useWatchCouponCreated((logs) => {
    logs.forEach((log: any) => {
      console.log('Coupon created:', log);
      // Refresh coupons when new ones are created
      const stored = getStoredCoupons();
      setCoupons([...stored]);
    });
  });

  useWatchCouponUsed((logs) => {
    logs.forEach((log: any) => {
      console.log('Coupon used:', log);
      // Update coupon status when used
      const couponHash = log.args?.couponHash;
      const business = log.args?.business;
      
      if (couponHash) {
        setCoupons(prev => prev.map(coupon => 
          coupon.hash === couponHash 
            ? { ...coupon, isUsed: true, usedBy: business }
            : coupon
        ));
        
        // Update localStorage
        const stored = getStoredCoupons();
        const updated = stored.map(coupon => 
          coupon.hash === couponHash 
            ? { ...coupon, isUsed: true, usedBy: business }
            : coupon
        );
        localStorage.setItem('payverge-coupons', JSON.stringify(updated));
      }
    });
  });

  // Copy to clipboard
  const [copiedCode, setCopiedCode] = useState<string | null>(null);
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

  // Create coupon handler
  const handleCreateCoupon = async () => {
    if (!createCode || !createAmount) {
      setError("Code and amount are required");
      return;
    }

    if (!address) {
      toast.error("Please connect your wallet");
      return;
    }

    setLoading(true);
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

      const hash = await createCoupon(createCode, discountAmount, expiryTime);
      
      // Store coupon locally
      const couponHash = keccak256(toBytes(createCode));
      const newCoupon: CouponData = {
        code: createCode,
        hash: couponHash,
        discountAmount,
        expiryTime,
        isActive: true,
        isUsed: false,
      };
      
      storeCoupon(newCoupon);
      
      toast.success(`Coupon "${createCode}" created successfully!`);
      
      // Reset form
      setCreateCode("");
      setCreateAmount("");
      setCreateExpiry("");
      onCreateClose();

    } catch (error: any) {
      console.error('Error creating coupon:', error);
      toast.error(error.message || "Failed to create coupon");
      setError(error.message || "Failed to create coupon");
    } finally {
      setLoading(false);
    }
  };

  // Deactivate coupon handler
  const handleDeactivateCoupon = async (coupon: CouponData) => {
    if (!confirm(`Are you sure you want to deactivate coupon "${coupon.code}"?`)) {
      return;
    }

    setLoading(true);
    try {
      await deactivateCoupon(coupon.code);
      
      // Update local storage
      setCoupons(prev => prev.map(c => 
        c.hash === coupon.hash ? { ...c, isActive: false } : c
      ));
      
      const stored = getStoredCoupons();
      const updated = stored.map(c => 
        c.hash === coupon.hash ? { ...c, isActive: false } : c
      );
      localStorage.setItem('payverge-coupons', JSON.stringify(updated));
      
      toast.success(`Coupon "${coupon.code}" deactivated successfully!`);
    } catch (error: any) {
      console.error('Error deactivating coupon:', error);
      toast.error(error.message || "Failed to deactivate coupon");
    } finally {
      setLoading(false);
    }
  };

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

  // Status helpers
  const getStatusChip = (coupon: CouponData) => {
    if (coupon.isUsed) {
      return <Chip color="success" variant="flat">Used</Chip>;
    }
    if (!coupon.isActive) {
      return <Chip color="danger" variant="flat">Deactivated</Chip>;
    }
    if (coupon.expiryTime > 0 && coupon.expiryTime < BigInt(Math.floor(Date.now() / 1000))) {
      return <Chip color="warning" variant="flat">Expired</Chip>;
    }
    return <Chip color="primary" variant="flat">Active</Chip>;
  };

  const formatExpiry = (expiryTime: bigint) => {
    if (expiryTime === BigInt(0)) return "No expiry";
    return new Date(Number(expiryTime) * 1000).toLocaleDateString();
  };

  const formatAddress = (address?: string) => {
    if (!address) return "—";
    return `${address.slice(0, 6)}...${address.slice(-4)}`;
  };

  // Render table cell
  const renderCell = useCallback((coupon: CouponData, columnKey: React.Key) => {
    switch (columnKey) {
      case "code":
        return (
          <div className="flex items-center gap-2">
            <span className="font-mono text-sm">{coupon.code}</span>
            <Button
              size="sm"
              variant="light"
              onPress={() => copyToClipboard(coupon.code)}
              className="min-w-0 px-2"
            >
              {copiedCode === coupon.code ? "✓" : "📋"}
            </Button>
          </div>
        );
      case "discount":
        return (
          <span className="font-semibold">
            ${formatUSDC(coupon.discountAmount)}
          </span>
        );
      case "status":
        return getStatusChip(coupon);
      case "expiry":
        return formatExpiry(coupon.expiryTime);
      case "usedBy":
        return formatAddress(coupon.usedBy);
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
  }, [copiedCode, copyToClipboard, formatUSDC, loading, handleDeactivateCoupon]);

  return (
    <div className="space-y-6">
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

      {!address && (
        <div className="bg-warning-50 border border-warning-200 rounded-lg p-4">
          <p className="text-warning-800">
            Please connect your wallet to manage coupons.
          </p>
        </div>
      )}

      {/* Filters */}
      <div className="flex justify-between gap-3 items-end">
        <Input
          isClearable
          className="w-full sm:max-w-[44%]"
          placeholder="Search by code or address..."
          startContent={<SearchIcon />}
          value={filterValue}
          onClear={() => setFilterValue("")}
          onValueChange={setFilterValue}
        />
        <div className="flex gap-3">
          <Select
            label="Rows per page"
            className="max-w-xs"
            selectedKeys={[rowsPerPage.toString()]}
            onChange={(e) => {
              setRowsPerPage(Number(e.target.value));
              setPage(1);
            }}
          >
            {rowsPerPageOptions.map((option) => (
              <SelectItem key={option.value} value={option.value}>
                {option.label}
              </SelectItem>
            ))}
          </Select>
        </div>
      </div>

      {/* Table */}
      <Table
        aria-label="Coupon management table"
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
            <TableColumn key={column.uid} align="start">
              {column.name}
            </TableColumn>
          )}
        </TableHeader>
        <TableBody
          items={paginatedCoupons}
          isLoading={loading}
          loadingContent={<PrimarySpinner />}
          emptyContent="No coupons found"
        >
          {(item) => (
            <TableRow key={item.hash}>
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
              isLoading={loading}
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
