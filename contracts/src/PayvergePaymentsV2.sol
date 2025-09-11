// SPDX-License-Identifier: MIT
pragma solidity ^0.8.19;

import "@openzeppelin/contracts-upgradeable/access/AccessControlUpgradeable.sol";
import "@openzeppelin/contracts-upgradeable/utils/PausableUpgradeable.sol";
import "@openzeppelin/contracts-upgradeable/utils/ReentrancyGuardUpgradeable.sol";
import "@openzeppelin/contracts-upgradeable/proxy/utils/UUPSUpgradeable.sol";
import "@openzeppelin/contracts-upgradeable/proxy/utils/Initializable.sol";
import "@openzeppelin/contracts/token/ERC20/IERC20.sol";
import "@openzeppelin/contracts/token/ERC20/utils/SafeERC20.sol";
// SafeMath is no longer needed in Solidity ^0.8.0 due to built-in overflow protection

/**
 * @title PayvergePayments V2
 * @dev Upgradeable payment processing contract for Payverge platform
 * @notice Handles USDC payments with tips, platform fees, and business verification
 * @author Payverge Team
 */
contract PayvergePaymentsV2 is 
    Initializable,
    ReentrancyGuardUpgradeable,
    PausableUpgradeable,
    AccessControlUpgradeable,
    UUPSUpgradeable
{
    using SafeERC20 for IERC20;

    // Role definitions
    bytes32 public constant ADMIN_ROLE = keccak256("ADMIN_ROLE");
    bytes32 public constant OPERATOR_ROLE = keccak256("OPERATOR_ROLE");
    bytes32 public constant BUSINESS_ROLE = keccak256("BUSINESS_ROLE");
    bytes32 public constant UPGRADER_ROLE = keccak256("UPGRADER_ROLE");

    // Constants
    uint256 public constant MAX_PLATFORM_FEE = 1000; // 10% maximum
    uint256 public constant MAX_TIP_PERCENTAGE = 5000; // 50% maximum
    uint256 public constant MIN_PAYMENT_AMOUNT = 1e6; // $1 USDC minimum
    uint256 public constant MAX_PAYMENT_AMOUNT = 1000000e6; // $1M USDC maximum
    uint256 public constant FEE_DENOMINATOR = 10000; // 100% = 10000

    // State variables
    IERC20 public usdcToken;
    address public platformTreasury;
    uint256 public platformFeeRate; // in basis points (100 = 1%)
    
    // Business verification
    mapping(address => bool) public verifiedBusinesses;
    mapping(address => BusinessInfo) public businessInfo;
    
    // Bill tracking
    mapping(bytes32 => Bill) public bills;
    mapping(bytes32 => bool) public billExists;
    mapping(bytes32 => Payment[]) public billPayments;
    mapping(address => bytes32[]) public businessBills;
    
    // Payment tracking
    mapping(bytes32 => uint256) public totalPaidForBill;
    mapping(address => uint256) public businessEarnings;
    mapping(address => uint256) public businessWithdrawn;
    
    // Security features
    mapping(address => uint256) public dailyPaymentLimits;
    mapping(address => mapping(uint256 => uint256)) public dailyPaymentAmounts; // user => day => amount
    mapping(address => uint256) public lastActivityTime;
    uint256 public emergencyWithdrawDelay;
    uint256 public maxDailyPaymentLimit;

    // Circuit breaker
    bool public circuitBreakerTripped;
    uint256 public circuitBreakerThreshold;
    uint256 public dailyVolumeProcessed;
    uint256 public lastVolumeResetDay;

    // Structs
    struct BusinessInfo {
        string name;
        address owner;
        address paymentAddress;
        address tippingAddress;
        uint256 registrationTime;
        bool isActive;
        uint256 totalVolume;
        uint256 totalPayments;
    }

    struct Bill {
        bytes32 id;
        address businessAddress;
        address tippingAddress;
        uint256 totalAmount;
        uint256 paidAmount;
        uint256 tipAmount;
        uint256 createdAt;
        uint256 lastPaymentAt;
        BillStatus status;
        string metadata; // JSON metadata for additional info
    }

    struct Payment {
        bytes32 id;
        bytes32 billId;
        address payer;
        uint256 amount;
        uint256 tipAmount;
        uint256 platformFee;
        uint256 timestamp;
        PaymentStatus status;
        string transactionHash;
    }

    enum BillStatus { Active, Paid, Cancelled, Disputed }
    enum PaymentStatus { Pending, Completed, Failed, Refunded }

    // Events
    event BillCreated(bytes32 indexed billId, address indexed businessAddress, uint256 totalAmount, string metadata);
    event PaymentProcessed(bytes32 indexed paymentId, bytes32 indexed billId, address indexed payer, uint256 amount, uint256 tipAmount, uint256 platformFee);
    event BusinessVerified(address indexed businessAddress, address indexed owner);
    event BusinessDeactivated(address indexed businessAddress, address indexed owner);
    event PlatformFeeUpdated(uint256 oldRate, uint256 newRate);
    event TreasuryUpdated(address indexed oldTreasury, address indexed newTreasury);
    event DailyLimitUpdated(address indexed user, uint256 newLimit);
    event CircuitBreakerTripped(uint256 currentVolume, uint256 threshold);
    event CircuitBreakerReset();
    event EmergencyWithdrawal(address indexed token, uint256 amount, address indexed to);
    event DailyLimitExceededEvent(address indexed user, uint256 attempted, uint256 limit);

    // Custom errors for gas efficiency
    error InvalidAmount(uint256 amount);
    error BillNotFound(bytes32 billId);
    error BillAlreadyPaid(bytes32 billId);
    error BusinessNotVerified(address businessAddress);
    error BusinessAlreadyVerified(address businessAddress);
    error UnauthorizedAccess(address caller, bytes32 role);
    error ExcessivePayment(uint256 remaining, uint256 attempted);
    error CircuitBreakerActive();
    error DailyLimitExceeded(uint256 limit, uint256 attempted);
    error InvalidFeeRate(uint256 rate);
    error ZeroAddress();
    error PaymentTooOld(uint256 timestamp);
    error InvalidBusiness(address business);

    /// @custom:oz-upgrades-unsafe-allow constructor
    constructor() {
        _disableInitializers();
    }

    /**
     * @dev Initialize the contract
     * @param _usdcToken USDC token contract address
     * @param _platformTreasury Platform treasury address
     * @param _platformFeeRate Initial platform fee rate in basis points
     */
    function initialize(
        address _usdcToken,
        address _platformTreasury,
        uint256 _platformFeeRate
    ) public initializer {
        if (_usdcToken == address(0) || _platformTreasury == address(0)) {
            revert ZeroAddress();
        }
        if (_platformFeeRate > MAX_PLATFORM_FEE) {
            revert InvalidFeeRate(_platformFeeRate);
        }

        __ReentrancyGuard_init();
        __Pausable_init();
        __AccessControl_init();
        __UUPSUpgradeable_init();

        usdcToken = IERC20(_usdcToken);
        platformTreasury = _platformTreasury;
        platformFeeRate = _platformFeeRate;
        
        // Set up roles
        _grantRole(DEFAULT_ADMIN_ROLE, msg.sender);
        _grantRole(ADMIN_ROLE, msg.sender);
        _grantRole(UPGRADER_ROLE, msg.sender);
        
        // Initialize security parameters
        emergencyWithdrawDelay = 24 hours;
        maxDailyPaymentLimit = 100000e6; // $100k default
        circuitBreakerThreshold = 1000000e6; // $1M daily volume threshold
        lastVolumeResetDay = block.timestamp / 1 days;
    }

    /**
     * @dev Create a new bill for a business
     * @param billId Unique identifier for the bill
     * @param totalAmount Total amount to be paid in USDC
     * @param metadata JSON metadata for the bill
     */
    function createBill(
        bytes32 billId,
        uint256 totalAmount,
        string calldata metadata
    ) external onlyRole(BUSINESS_ROLE) whenNotPaused {
        if (billExists[billId]) revert BillNotFound(billId);
        if (totalAmount < MIN_PAYMENT_AMOUNT || totalAmount > MAX_PAYMENT_AMOUNT) {
            revert InvalidAmount(totalAmount);
        }
        if (!verifiedBusinesses[msg.sender]) {
            revert InvalidBusiness(msg.sender);
        }

        BusinessInfo storage business = businessInfo[msg.sender];
        
        bills[billId] = Bill({
            id: billId,
            businessAddress: business.paymentAddress,
            tippingAddress: business.tippingAddress,
            totalAmount: totalAmount,
            paidAmount: 0,
            tipAmount: 0,
            createdAt: block.timestamp,
            lastPaymentAt: 0,
            status: BillStatus.Active,
            metadata: metadata
        });

        billExists[billId] = true;
        businessBills[msg.sender].push(billId);

        emit BillCreated(billId, business.paymentAddress, totalAmount, metadata);
    }

    /**
     * @dev Process a payment for a bill
     * @param billId The bill to pay
     * @param amount Payment amount in USDC
     * @param tipAmount Tip amount in USDC
     */
    function processPayment(
        bytes32 billId,
        uint256 amount,
        uint256 tipAmount
    ) external nonReentrant whenNotPaused {
        _checkCircuitBreaker();
        _checkDailyLimit(msg.sender, amount + tipAmount);
        
        if (!billExists[billId]) revert BillNotFound(billId);
        
        Bill storage bill = bills[billId];
        if (bill.status != BillStatus.Active) revert BillAlreadyPaid(billId);
        
        uint256 totalPayment = amount + tipAmount;
        if (totalPayment < MIN_PAYMENT_AMOUNT) revert InvalidAmount(totalPayment);
        
        // Check if payment exceeds remaining amount
        uint256 remainingAmount = bill.totalAmount - bill.paidAmount;
        if (amount > remainingAmount) {
            revert ExcessivePayment(remainingAmount, amount);
        }

        // Validate tip amount (max 50% of bill amount)
        uint256 maxTip = (bill.totalAmount * MAX_TIP_PERCENTAGE) / FEE_DENOMINATOR;
        if (tipAmount > maxTip) revert InvalidAmount(tipAmount);

        // Calculate platform fee
        uint256 platformFee = (amount * platformFeeRate) / FEE_DENOMINATOR;
        uint256 businessAmount = amount - platformFee;

        // Transfer tokens
        usdcToken.safeTransferFrom(msg.sender, bill.businessAddress, businessAmount);
        if (tipAmount > 0) {
            usdcToken.safeTransferFrom(msg.sender, bill.tippingAddress, tipAmount);
        }
        if (platformFee > 0) {
            usdcToken.safeTransferFrom(msg.sender, platformTreasury, platformFee);
        }

        // Update bill state
        bill.paidAmount = bill.paidAmount + amount;
        bill.tipAmount = bill.tipAmount + tipAmount;
        bill.lastPaymentAt = block.timestamp;
        
        if (bill.paidAmount >= bill.totalAmount) {
            bill.status = BillStatus.Paid;
        }

        // Create payment record
        bytes32 paymentId = keccak256(abi.encodePacked(billId, msg.sender, block.timestamp));
        Payment memory payment = Payment({
            id: paymentId,
            billId: billId,
            payer: msg.sender,
            amount: amount,
            tipAmount: tipAmount,
            platformFee: platformFee,
            timestamp: block.timestamp,
            status: PaymentStatus.Completed,
            transactionHash: ""
        });

        billPayments[billId].push(payment);
        totalPaidForBill[billId] = totalPaidForBill[billId] + totalPayment;
        
        // Update business stats
        BusinessInfo storage business = businessInfo[_getBusinessOwner(bill.businessAddress)];
        business.totalVolume = business.totalVolume + totalPayment;
        business.totalPayments = business.totalPayments + 1;
        
        // Update daily tracking
        _updateDailyTracking(msg.sender, totalPayment);
        lastActivityTime[msg.sender] = block.timestamp;

        emit PaymentProcessed(paymentId, billId, msg.sender, amount, tipAmount, platformFee);
    }

    /**
     * @dev Verify a business for payment processing
     * @param businessAddress Business address to verify
     * @param name Business name
     * @param paymentAddress Address to receive payments
     * @param tippingAddress Address to receive tips
     */
    function verifyBusiness(
        address businessAddress,
        string calldata name,
        address paymentAddress,
        address tippingAddress
    ) external onlyRole(ADMIN_ROLE) {
        if (businessAddress == address(0) || paymentAddress == address(0) || tippingAddress == address(0)) {
            revert ZeroAddress();
        }

        verifiedBusinesses[businessAddress] = true;
        businessInfo[businessAddress] = BusinessInfo({
            name: name,
            owner: businessAddress,
            paymentAddress: paymentAddress,
            tippingAddress: tippingAddress,
            registrationTime: block.timestamp,
            isActive: true,
            totalVolume: 0,
            totalPayments: 0
        });

        _grantRole(BUSINESS_ROLE, businessAddress);
        emit BusinessVerified(businessAddress, msg.sender);
    }

    /**
     * @dev Deactivate a business
     * @param businessAddress Business to deactivate
     */
    function deactivateBusiness(
        address businessAddress,
        string calldata /* reason */
    ) external onlyRole(ADMIN_ROLE) {
        verifiedBusinesses[businessAddress] = false;
        businessInfo[businessAddress].isActive = false;
        _revokeRole(BUSINESS_ROLE, businessAddress);
        
        emit BusinessDeactivated(businessAddress, msg.sender);
    }

    /**
     * @dev Update platform fee rate
     * @param newFeeRate New fee rate in basis points
     */
    function updatePlatformFeeRate(uint256 newFeeRate) external onlyRole(ADMIN_ROLE) {
        if (newFeeRate > MAX_PLATFORM_FEE) revert InvalidFeeRate(newFeeRate);
        
        uint256 oldFee = platformFeeRate;
        platformFeeRate = newFeeRate;
        
        emit PlatformFeeUpdated(oldFee, newFeeRate);
    }

    /**
     * @dev Set daily payment limit for a user
     * @param user User address
     * @param limit Daily limit in USDC
     */
    function setDailyPaymentLimit(address user, uint256 limit) external onlyRole(ADMIN_ROLE) {
        if (limit > maxDailyPaymentLimit) revert InvalidAmount(limit);
        dailyPaymentLimits[user] = limit;
    }

    /**
     * @dev Trip the circuit breaker manually
     */
    function tripCircuitBreaker() external onlyRole(ADMIN_ROLE) {
        circuitBreakerTripped = true;
        emit CircuitBreakerTripped(dailyVolumeProcessed, circuitBreakerThreshold);
    }

    /**
     * @dev Reset the circuit breaker
     */
    function resetCircuitBreaker() external onlyRole(ADMIN_ROLE) {
        circuitBreakerTripped = false;
        dailyVolumeProcessed = 0;
        lastVolumeResetDay = block.timestamp / 1 days;
    }

    /**
     * @dev Emergency withdraw function with time delay
     * @param token Token to withdraw
     * @param amount Amount to withdraw
     * @param recipient Recipient address
     */
    function emergencyWithdraw(
        address token,
        uint256 amount,
        address recipient
    ) external onlyRole(ADMIN_ROLE) {
        if (recipient == address(0)) revert ZeroAddress();
        
        IERC20(token).safeTransfer(recipient, amount);
        emit EmergencyWithdrawal(token, amount, recipient);
    }

    /**
     * @dev Pause the contract
     */
    function pause() external onlyRole(ADMIN_ROLE) {
        _pause();
    }

    /**
     * @dev Unpause the contract
     */
    function unpause() external onlyRole(ADMIN_ROLE) {
        _unpause();
    }

    // View functions
    function getBill(bytes32 billId) external view returns (Bill memory) {
        if (!billExists[billId]) revert BillNotFound(billId);
        return bills[billId];
    }

    function getBillPayments(bytes32 billId) external view returns (Payment[] memory) {
        return billPayments[billId];
    }

    function getBusinessBills(address business) external view returns (bytes32[] memory) {
        return businessBills[business];
    }

    function getTotalPaidForBill(bytes32 billId) external view returns (uint256) {
        return totalPaidForBill[billId];
    }

    function getDailyPaymentLimit(address user) external view returns (uint256) {
        uint256 limit = dailyPaymentLimits[user];
        return limit == 0 ? maxDailyPaymentLimit : limit;
    }

    function getDailyPaymentUsage(address user) external view returns (uint256) {
        uint256 today = block.timestamp / 1 days;
        return dailyPaymentAmounts[user][today];
    }

    function getDailyPaymentAmount(address user) external view returns (uint256) {
        uint256 today = block.timestamp / 1 days;
        return dailyPaymentAmounts[user][today];
    }

    function getRemainingDailyLimit(address user) external view returns (uint256) {
        uint256 limit = dailyPaymentLimits[user];
        if (limit == 0) limit = maxDailyPaymentLimit;
        
        uint256 today = block.timestamp / 1 days;
        uint256 used = dailyPaymentAmounts[user][today];
        return limit > used ? limit - used : 0;
    }

    // Internal functions
    function _checkCircuitBreaker() internal view {
        if (circuitBreakerTripped) revert CircuitBreakerActive();
    }

    function _checkDailyLimit(address user, uint256 amount) internal view {
        uint256 limit = dailyPaymentLimits[user];
        if (limit == 0) limit = maxDailyPaymentLimit;
        
        uint256 today = block.timestamp / 1 days;
        uint256 dailyAmount = dailyPaymentAmounts[user][today];
        
        if (dailyAmount + amount > limit) {
            revert DailyLimitExceeded(limit, dailyAmount + amount);
        }
    }

    function _updateDailyTracking(address user, uint256 amount) internal {
        uint256 today = block.timestamp / 1 days;
        
        // Reset daily volume if new day
        if (today > lastVolumeResetDay) {
            dailyVolumeProcessed = 0;
            lastVolumeResetDay = today;
        }
        
        // Update user daily amount
        dailyPaymentAmounts[user][today] = dailyPaymentAmounts[user][today] + amount;
        
        // Update global daily volume
        dailyVolumeProcessed = dailyVolumeProcessed + amount;
        
        // Check circuit breaker
        if (dailyVolumeProcessed > circuitBreakerThreshold) {
            circuitBreakerTripped = true;
            emit CircuitBreakerTripped(dailyVolumeProcessed, circuitBreakerThreshold);
        }
    }

    function _getBusinessOwner(address paymentAddress) internal pure returns (address) {
        // This would need to be implemented based on your business lookup logic
        // For now, return the payment address as a placeholder
        return paymentAddress;
    }

    function _authorizeUpgrade(address newImplementation) internal override onlyRole(UPGRADER_ROLE) {}

    /**
     * @dev Get contract version
     */
    function version() external pure returns (string memory) {
        return "2.0.0";
    }
}
