# PayvergePayments Smart Contract Integration

This module provides a complete wagmi-based integration for the PayvergePayments smart contract.

## Quick Start

```typescript
import { 
  useBill, 
  useProcessPayment, 
  useUsdcBalance, 
  formatUsdcAmount,
  parseUsdcAmount 
} from '@/contracts';

// In your component
function PaymentComponent({ billId }: { billId: string }) {
  const { data: bill } = useBill(billId);
  const { data: usdcBalance } = useUsdcBalance();
  const { processPayment } = useProcessPayment();
  
  const handlePayment = async () => {
    const amount = parseUsdcAmount('10.50'); // $10.50 USDC
    const tipAmount = parseUsdcAmount('2.00'); // $2.00 tip
    
    await processPayment({
      billId,
      amount,
      tipAmount,
    });
  };
  
  return (
    <div>
      <p>Bill Total: ${formatUsdcAmount(bill?.totalAmount || 0n)}</p>
      <p>Your Balance: ${formatUsdcAmount(usdcBalance || 0n)}</p>
      <button onClick={handlePayment}>Pay Bill</button>
    </div>
  );
}
```

## Available Hooks

### Read Hooks
- `useBill(billId)` - Get bill details
- `useBillPayments(billId)` - Get all payments for a bill
- `useBusinessInfo(address)` - Get business information
- `useBusinessBillCount(address)` - Get total bill count for a business
- `useClaimableBalance(address)` - Get claimable payments and tips
- `usePlatformFeeRate()` - Get current platform fee rate
- `useDailyPaymentLimit(address?)` - Get daily payment limit
- `useRemainingDailyLimit(address?)` - Get remaining daily limit

### Write Hooks
- `useCreateBill()` - Create a new bill (admin-controlled bill creator only)
- `useProcessPayment()` - Process a payment
- `useRegisterBusiness()` - Register a new business
- `useUpdateBusinessPaymentAddress()` - Update business payment address
- `useUpdateBusinessTippingAddress()` - Update business tipping address
- `useClaimEarnings()` - Claim accumulated earnings

### USDC Hooks
- `useUsdcBalance(address?)` - Get USDC balance
- `useUsdcAllowance(spender?)` - Get USDC allowance
- `useApproveUsdc()` - Approve USDC spending

### Event Watching Hooks
- `useWatchBillCreated(callback, businessAddress?)` - Watch for new bills
- `useWatchPaymentProcessed(callback, billId?)` - Watch for payments
- `useWatchBusinessRegistered(callback, businessAddress?)` - Watch for business registration

## Configuration

The contract addresses are automatically selected based on the current chain:

- **Ethereum Mainnet** (Chain ID: 1)
- **Sepolia Testnet** (Chain ID: 11155111) 
- **Polygon** (Chain ID: 137)
- **Base** (Chain ID: 8453)

Set these environment variables:
```env
NEXT_PUBLIC_PAYVERGE_CONTRACT_ADDRESS=0x...
NEXT_PUBLIC_PLATFORM_TREASURY=0x...
NEXT_PUBLIC_USDC_ADDRESS=0x... # Optional, defaults to chain-specific USDC
```

## Usage Examples

### Creating a Bill (Admin-Controlled)
```typescript
function CreateBillComponent() {
  const { createBill } = useCreateBill();
  
  const handleCreateBill = async () => {
    await createBill({
      billId: '0x1234...', // Generate unique bill ID
      businessAddress: '0xabc...', // Target business address
      totalAmount: parseUsdcAmount('25.99'),
      metadata: JSON.stringify({ tableNumber: 5, items: [...] }),
      nonce: '0x5678...' // Unique nonce for this operation
    });
  };
}
```

### Registering a Business
```typescript
function RegisterBusinessComponent() {
  const { registerBusiness } = useRegisterBusiness();
  
  const handleRegister = async () => {
    await registerBusiness({
      name: 'My Restaurant',
      paymentAddress: '0xpayment...', // Address to receive payments
      tippingAddress: '0xtipping...' // Address to receive tips
    });
  };
}
```

### Processing Payment with Approval
```typescript
function PaymentFlow({ billId, amount, tipAmount }: PaymentProps) {
  const { approveUsdc } = useApproveUsdc();
  const { processPayment } = useProcessPayment();
  const { data: allowance } = useUsdcAllowance();
  
  const handlePayment = async () => {
    const totalAmount = amount + tipAmount;
    
    // Check if approval is needed
    if (!allowance || allowance < totalAmount) {
      await approveUsdc(totalAmount);
    }
    
    // Process the payment
    await processPayment({ billId, amount, tipAmount });
  };
}
```

### Real-time Bill Monitoring
```typescript
function LiveBillMonitor({ businessAddress }: { businessAddress: Address }) {
  const [bills, setBills] = useState<string[]>([]);
  
  useWatchBillCreated(
    (logs) => {
      logs.forEach((log) => {
        setBills(prev => [...prev, log.args.billId]);
      });
    },
    businessAddress
  );
  
  return (
    <div>
      {bills.map(billId => (
        <BillCard key={billId} billId={billId} />
      ))}
    </div>
  );
}
```

### Claiming Business Earnings
```typescript
function ClaimEarningsComponent({ businessAddress }: { businessAddress: Address }) {
  const { data: claimableBalance } = useClaimableBalance(businessAddress);
  const { claimEarnings } = useClaimEarnings();
  
  const handleClaim = async () => {
    await claimEarnings();
  };
  
  return (
    <div>
      <p>Claimable Payments: ${formatUsdcAmount(claimableBalance?.payments || 0n)}</p>
      <p>Claimable Tips: ${formatUsdcAmount(claimableBalance?.tips || 0n)}</p>
      <button onClick={handleClaim}>Claim Earnings</button>
    </div>
  );
}
```

## Type Safety

All hooks are fully typed with TypeScript interfaces:

```typescript
interface Bill {
  businessAddress: string;
  isPaid: boolean;
  isCancelled: boolean;
  createdAt: bigint;
  lastPaymentAt: bigint;
  totalAmount: bigint;
  paidAmount: bigint;
  nonce: string;
}

interface Payment {
  id: string;
  billId: string;
  payer: string;
  timestamp: bigint;
  amount: bigint;
  tipAmount: bigint;
  platformFee: bigint;
}

interface BusinessInfo {
  paymentAddress: string;
  tippingAddress: string;
  isActive: boolean;
  registrationDate: bigint;
  totalVolume: bigint;
  totalTips: bigint;
}

interface ClaimableAmounts {
  payments: bigint;
  tips: bigint;
}

enum BillStatus {
  OPEN = 0,
  PAID = 1,
  CANCELLED = 2,
  EXPIRED = 3
}
```

## Error Handling

All wagmi hooks include built-in error handling. Access errors through the hook's return value:

```typescript
const { data: bill, error, isLoading } = useBill(billId);

if (error) {
  console.error('Failed to fetch bill:', error);
}
```

## Integration with Existing Payverge Components

This module integrates seamlessly with existing Payverge components like `PaymentProcessor`, `BillCreator`, and analytics dashboards. Simply import the hooks and replace any direct ethers.js calls with the wagmi equivalents.
