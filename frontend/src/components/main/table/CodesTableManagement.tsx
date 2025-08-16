"use client";
import {
  useEffect,
  useState,
  useCallback,
  Dispatch,
  SetStateAction,
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
import { CodeDetails, UpdateCodeRequest, CreateCodeRequest } from "@/interface/codes";
import { getAllCodes, deleteCode, updateCode, createCode } from "@/api/codes/adminCodes";

type Props = {
  // No props needed anymore
};

const rowsPerPageOptions = [
  { value: "10", label: "10" },
  { value: "20", label: "20" },
  { value: "50", label: "50" },
  { value: "100", label: "100" },
];

const codeColumns = [
  { name: "CODE", uid: "code" },
  { name: "AMOUNT (USDC)", uid: "amount" },
  { name: "STATUS", uid: "status" },
  { name: "EXPIRY", uid: "expiry" },
  { name: "USED BY", uid: "address" },
  { name: "ACTIONS", uid: "actions" },
];

export const CodesTableManagement = () => {
  // Edit modal disclosure
  const { isOpen, onOpen, onClose } = useDisclosure();
  
  // Create modal disclosure
  const { 
    isOpen: isCreateOpen, 
    onOpen: onCreateOpen, 
    onClose: onCreateClose 
  } = useDisclosure();
  const [codes, setCodes] = useState<CodeDetails[]>([]);
  const [loading, setLoading] = useState(true);
  const [page, setPage] = useState(1);
  const [rowsPerPage, setRowsPerPage] = useState(10);
  const [filterValue, setFilterValue] = useState("");
  const [selectedCode, setSelectedCode] = useState<CodeDetails | null>(null);
  const [isDeleting, setIsDeleting] = useState(false);
  const [isUpdating, setIsUpdating] = useState(false);
  const [error, setError] = useState("");
  const [amount, setAmount] = useState("");
  const [expiry, setExpiry] = useState("");
  const [isUsed, setIsUsed] = useState(false);
  
  // Create code state
  const [createAmount, setCreateAmount] = useState("");
  const [createExpiry, setCreateExpiry] = useState("");
  const [isCreating, setIsCreating] = useState(false);
  const [createdCode, setCreatedCode] = useState("");

  const fetchCodes = useCallback(async () => {
    setLoading(true);
    try {
      const response = await getAllCodes();
      if (Array.isArray(response)) {
        // Sort codes: available first, then used, then expired
        const sortedCodes = [...response].sort((a, b) => {
          // Helper function to get sort priority
          const getPriority = (code: CodeDetails) => {
            if (code.used) return 2;
            if (isExpired(code.expiry)) return 3;
            return 1; // Available
          };
          
          return getPriority(a) - getPriority(b);
        });
        
        setCodes(sortedCodes);
      } else {
        console.error("Expected an array of codes but got:", response);
        setCodes([]);
      }
    } catch (error) {
      console.error("Error fetching codes:", error);
      setCodes([]);
    } finally {
      setLoading(false);
    }
  }, []);

  useEffect(() => {
    fetchCodes();
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, []);

  // State for copy feedback
  const [copiedCode, setCopiedCode] = useState<string | null>(null);
  
  // Function to copy code to clipboard
  const copyToClipboard = useCallback((code: string) => {
    navigator.clipboard.writeText(code)
      .then(() => {
        console.log('Code copied to clipboard:', code);
        setCopiedCode(code);
        
        // Reset the copied state after 2 seconds
        setTimeout(() => {
          setCopiedCode(null);
        }, 2000);
      })
      .catch(err => {
        console.error('Failed to copy code:', err);
        alert('Failed to copy code to clipboard');
      });
  }, []);
  
  // Reset copied code state when opening modals
  const resetCopiedState = useCallback(() => {
    setCopiedCode(null);
  }, []);

  // Initial load of codes
  useEffect(() => {
    fetchCodes();
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, []);

  const handleDeleteCode = useCallback(async (code: string) => {
    if (!confirm('Are you sure you want to delete this code?')) {
      return;
    }
    
    setIsDeleting(true);
    try {
      console.log('Deleting code:', code);
      const result = await deleteCode({ code });
      console.log('Delete code response:', result);
      
      if (result.message) {
        // Success
        await fetchCodes(); // Refresh the list after deletion
        alert('Code deleted successfully');
      }
    } catch (error) {
      console.error("Error deleting code:", error);
    } finally {
      setIsDeleting(false);
    }
  }, [fetchCodes]);

  const openEditModal = useCallback((code: CodeDetails) => {
    setSelectedCode(code);
    setAmount(code.amount.toString());
    setExpiry(code.expiry ? new Date(code.expiry).toISOString().slice(0, 16) : "");
    setIsUsed(code.used);
    // Reset copied code state when opening the modal
    resetCopiedState();
    onOpen();
  }, [onOpen, resetCopiedState]);

  const resetModalState = useCallback(() => {
    setSelectedCode(null);
    setAmount("");
    setExpiry("");
    setIsUsed(false);
    setError("");
    // Reset copied code state when resetting modal state
    resetCopiedState();
  }, [resetCopiedState]);
  
  const resetCreateModalState = useCallback(() => {
    setCreateAmount("");
    setCreateExpiry("");
    setCreatedCode("");
    setError("");
  }, []);

  const handleCloseModal = useCallback(() => {
    resetModalState();
    onClose();
  }, [resetModalState, onClose]);
  
  const handleCloseCreateModal = useCallback(() => {
    // Reset all state first
    setCreateAmount("");
    setCreateExpiry("");
    setCreatedCode("");
    setError("");
    
    // Force close the modal
    document.body.classList.remove("overflow-hidden");
    onCreateClose();
  }, [onCreateClose]);

  const handleCreateCode = async () => {
    if (!createAmount) {
      setError("Amount is required");
      return;
    }
    setIsCreating(true);
    setError("");
    
    try {
      // Ensure amount is a valid number
      if (isNaN(parseFloat(createAmount))) {
        setError("Amount must be a valid number");
        setIsCreating(false);
        return;
      }
      
      const request: CreateCodeRequest = {
        amount: createAmount, // Keep as string to match interface
        expiry: createExpiry || undefined
      };
      
      console.log('Creating code with request:', request);
      const response = await createCode(request);
      console.log('Create code response:', response);
      
      if (response.code) {
        // Show success message
        setCreatedCode(response.code);
        
        // Reset form fields
        setCreateAmount("");
        setCreateExpiry("");
        
        // Refresh the codes list
        await fetchCodes();
        
        // Show alert and close modal immediately
        alert(`Code created successfully: ${response.code}`);
        
        // Force modal to close completely
        document.body.classList.remove("overflow-hidden");
        onCreateClose();
        
        // Give time for UI to update
        setTimeout(() => {
          document.body.classList.remove("overflow-hidden");
        }, 100);
      } else if (response.error) {
        setError(response.error);
      }
    } catch (err: any) {
      console.error('Error creating code:', err);
      setError(err.message || "Failed to create code");
    } finally {
      setIsCreating(false);
    }
  };

  const handleUpdateCode = async () => {
    if (!selectedCode) return;
    
    setIsUpdating(true);
    setError("");
    
    try {
      // Ensure amount is a valid number if provided
      if (amount && isNaN(parseFloat(amount))) {
        setError("Amount must be a valid number");
        setIsUpdating(false);
        return;
      }
      
      const request: UpdateCodeRequest = {
        code: selectedCode.code,
        amount: amount || undefined, // Keep as string to match interface
        expiry: expiry || undefined,
        used: isUsed
      };
      
      console.log('Updating code with request:', request);
      const result = await updateCode(request);
      console.log('Update code response:', result);
      
      if (result.message) {
        // Success
        await fetchCodes(); // Refresh the list after update
        resetModalState();
        onClose();
        alert('Code updated successfully');
      } else if (result.error) {
        setError(result.error);
      }
    } catch (err: any) {
      console.error('Error updating code:', err);
      setError(err.message || "Failed to update code");
    } finally {
      setIsUpdating(false);
    }
  };

  const filteredCodes = useMemo(() => {
    let filtered = [...codes];
    if (filterValue) {
      filtered = filtered.filter((code) => {
        const searchStr = filterValue.toLowerCase();
        return (
          code.code.toLowerCase().includes(searchStr) ||
          code.amount.toString().includes(searchStr) ||
          (code.address?.toLowerCase() || "").includes(searchStr)
        );
      });
    }
    return filtered;
  }, [codes, filterValue]);

  const pages = Math.ceil(filteredCodes.length / rowsPerPage);
  const items = useMemo(() => {
    const start = (page - 1) * rowsPerPage;
    const end = start + rowsPerPage;
    return filteredCodes.slice(start, end);
  }, [page, filteredCodes, rowsPerPage]);

  const formatDate = (dateString: string) => {
    if (!dateString) return "No expiry";
    const date = new Date(dateString);
    return date.toLocaleDateString() + " " + date.toLocaleTimeString();
  };

  const isExpired = (dateString?: string) => {
    if (!dateString) return false;
    const expiryDate = new Date(dateString);
    return expiryDate < new Date();
  };

  const renderCell = useCallback(
    (code: CodeDetails, columnKey: React.Key) => {
      switch (columnKey) {
        case "code":
          const isCopied = copiedCode === code.code;
          return (
            <div className="flex items-center gap-2">
              <div className="font-mono text-sm">
                <span className="select-all">{code.code}</span>
              </div>
              <Button 
                size="sm" 
                variant="light" 
                color={isCopied ? "success" : "primary"}
                onPress={() => copyToClipboard(code.code)}
              >
                {isCopied ? (
                  <span className="text-xs">Copied!</span>
                ) : (
                  <span className="text-xs">Copy</span>
                )}
              </Button>
            </div>
          );
        case "amount":
          return (
            <div className="flex flex-col">
              <span className="font-semibold">${code.amount}</span>
            </div>
          );
        case "status":
          const expired = isExpired(code.expiry);
          return (
            <div className="flex items-center gap-2">
              <Chip
                className="capitalize"
                color={
                  code.used
                    ? "success"
                    : expired
                    ? "danger"
                    : "primary"
                }
                size="sm"
                variant="flat"
              >
                {code.used ? "Used" : expired ? "Expired" : "Available"}
              </Chip>
            </div>
          );
        case "expiry":
          return (
            <div className="flex flex-col">
              <span className={`text-sm ${isExpired(code.expiry) ? "text-danger" : ""}`}>
                {code.expiry ? formatDate(code.expiry) : "No expiry"}
              </span>
            </div>
          );
        case "address":
          return (
            <div className="flex flex-col">
              {code.address ? (
                <Tooltip content={code.address}>
                  <span className="text-sm font-mono truncate max-w-[150px]">
                    {code.address}
                  </span>
                </Tooltip>
              ) : (
                <span className="text-sm text-default-400">Not used</span>
              )}
            </div>
          );
        case "actions":
          return (
            <div className="relative flex items-center gap-2">
              <Tooltip content="Edit code">
                <Button
                  isIconOnly
                  size="sm"
                  variant="light"
                  onPress={() => openEditModal(code)}
                >
                  <EditIcon className="text-default-500" />
                </Button>
              </Tooltip>
              <Tooltip content="Delete code">
                <Button
                  isIconOnly
                  size="sm"
                  variant="light"
                  color="danger"
                  isDisabled={isDeleting}
                  onPress={() => handleDeleteCode(code.code)}
                >
                  <DeleteIcon className="text-danger" />
                </Button>
              </Tooltip>
            </div>
          );
        default:
          return null;
      }
    },
    [isDeleting, handleDeleteCode, openEditModal, copyToClipboard, copiedCode]
  );

  const onRowsPerPageChange = useCallback(
    (e: React.ChangeEvent<HTMLSelectElement>) => {
      setRowsPerPage(Number(e.target.value));
      setPage(1);
    },
    []
  );

  const onSearchChange = useCallback((value?: string) => {
    if (value) {
      setFilterValue(value);
      setPage(1);
    } else {
      setFilterValue("");
    }
  }, []);

  if (loading) return <PrimarySpinner />;

  return (
    <div className="w-full flex flex-col gap-4">
      <div className="flex flex-col gap-4 mb-4">
        <div className="flex justify-between items-center">
          <h1 className="text-2xl font-bold">{"Reward Codes Management"}</h1>
          <Button 
            color="primary" 
            onPress={onCreateOpen}
          >
            {"Create New Code"}
          </Button>
        </div>
        
        <div className="flex flex-col sm:flex-row justify-between items-center gap-4 bg-default-50 p-4 rounded-lg shadow-sm">
          <div className="w-full sm:w-auto">
            <Input
              isClearable
              className="w-full sm:max-w-[350px]"
              placeholder="Search by code or amount..."
              startContent={<SearchIcon className="text-default-300" />}
              value={filterValue}
              onClear={() => onSearchChange("")}
              onValueChange={onSearchChange}
              size="sm"
            />
          </div>
          <div className="flex items-center gap-3 w-full sm:w-auto justify-end">
            <div className="flex items-center gap-2">
              <Chip color="primary" variant="flat" size="sm">Available</Chip>
              <Chip color="success" variant="flat" size="sm">Used</Chip>
              <Chip color="danger" variant="flat" size="sm">Expired</Chip>
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
              onPress={() => fetchCodes()}
            >
              Refresh
            </Button>
          </div>
        </div>
      </div>

      <Table
        aria-label="Codes management table"
        isHeaderSticky
        shadow="sm"
        classNames={{
          wrapper: "max-h-[calc(100vh-300px)]",
          table: "min-h-[400px]",
          th: "bg-default-100/80 text-default-800 font-semibold",
          td: "py-3",
          base: "overflow-x-auto overflow-y-hidden",
        }}
        bottomContent={
          pages > 0 ? (
            <div className="flex w-full justify-center">
              <Pagination
                isCompact
                showControls
                showShadow
                color="primary"
                page={page}
                total={pages}
                onChange={setPage}
              />
            </div>
          ) : null
        }
      >
        <TableHeader columns={codeColumns}>
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
          items={items}
          emptyContent={"No codes found"}
          loadingContent={<PrimarySpinner />}
          className="h-full"
        >
          {(item) => (
            <TableRow key={item.code} className="h-[60px]">
              {(columnKey) => (
                <TableCell>{renderCell(item, columnKey)}</TableCell>
              )}
            </TableRow>
          )}
        </TableBody>
      </Table>

      {/* Edit Code Modal */}
      <Modal isOpen={isOpen} onClose={handleCloseModal}>
        <ModalContent>
          <ModalHeader className="flex flex-col gap-1">Edit Code</ModalHeader>
          <ModalBody>
            {error && <div className="text-red-500 mb-2">{error}</div>}
            
            <div className="flex flex-col gap-2">
              <div className="flex flex-col">
                <span className="text-sm font-semibold">Code:</span>
                <span className="font-mono blur-sm select-none">{selectedCode?.code}</span>
              </div>
              
              <Input
                label="Amount (USDC)"
                placeholder="100"
                type="number"
                value={amount}
                onChange={(e) => setAmount(e.target.value)}
              />
              
              <Input
                label="Expiry Date"
                placeholder="YYYY-MM-DDTHH:MM:SSZ"
                type="datetime-local"
                value={expiry}
                onChange={(e) => setExpiry(e.target.value)}
                description="Leave empty for no expiration"
              />
              
              <div className="flex items-center gap-2 mt-2">
                <input 
                  type="checkbox" 
                  id="isUsed" 
                  checked={isUsed} 
                  onChange={(e) => setIsUsed(e.target.checked)} 
                />
                <label htmlFor="isUsed">Mark as used</label>
              </div>
            </div>
          </ModalBody>
          <ModalFooter>
            <Button color="danger" variant="light" onPress={handleCloseModal}>
              Cancel
            </Button>
            <Button color="primary" onPress={handleUpdateCode} isLoading={isUpdating}>
              Update Code
            </Button>
          </ModalFooter>
        </ModalContent>
      </Modal>
      
      {/* Create Code Modal */}
      <Modal isOpen={isCreateOpen} onClose={handleCloseCreateModal}>
        <ModalContent>
          <ModalHeader className="flex flex-col gap-1">{"Create New Reward Code"}</ModalHeader>
          <ModalBody>
            {error && <div className="text-red-500 mb-2">{error}</div>}
            {createdCode && (
              <div className="bg-green-100 p-3 rounded-md mb-4">
                <p className="text-green-700 font-semibold">Code created successfully!</p>
                <p className="text-green-700">Code: <span className="font-mono">{createdCode}</span></p>
              </div>
            )}
            
            <Input
              label="Amount (USDC)"
              placeholder="100"
              type="number"
              value={createAmount}
              onChange={(e) => setCreateAmount(e.target.value)}
              isRequired
            />
            
            <Input
              label="Expiry Date (Optional)"
              placeholder="YYYY-MM-DDTHH:MM:SSZ"
              type="datetime-local"
              value={createExpiry}
              onChange={(e) => setCreateExpiry(e.target.value)}
              description="Leave empty for no expiration"
            />
          </ModalBody>
          <ModalFooter>
            <Button color="danger" variant="light" onPress={handleCloseCreateModal}>Cancel</Button>
            <Button color="primary" onPress={handleCreateCode} isLoading={isCreating}>
              Create Code
            </Button>
          </ModalFooter>
        </ModalContent>
      </Modal>
    </div>
  );
};
