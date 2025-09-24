// SPDX-License-Identifier: MIT
pragma solidity ^0.8.19;

import "@openzeppelin/contracts-upgradeable/utils/ReentrancyGuardUpgradeable.sol";
import "@openzeppelin/contracts-upgradeable/utils/PausableUpgradeable.sol";
import "@openzeppelin/contracts-upgradeable/access/AccessControlUpgradeable.sol";
import "@openzeppelin/contracts-upgradeable/proxy/utils/UUPSUpgradeable.sol";
import "@openzeppelin/contracts-upgradeable/proxy/utils/Initializable.sol";
import "@openzeppelin/contracts/token/ERC20/IERC20.sol";
import "@openzeppelin/contracts/token/ERC20/utils/SafeERC20.sol";

/**
 * @title PayvergePayments
 * @dev Secure payment processing contract for Payverge platform with all security fixes
 * @notice Handles USDC payments with tips and platform fees - security hardened version
 */
contract PayvergePayments is 
    Initializable,
    ReentrancyGuardUpgradeable,
    PausableUpgradeable,
    AccessControlUpgradeable,
    UUPSUpgradeable
{
    using SafeERC20 for IERC20;

    // Role definitions with least privilege
    bytes32 public constant ADMIN_ROLE = keccak256("ADMIN_ROLE");
    bytes32 public constant UPGRADER_ROLE = keccak256("UPGRADER_ROLE");
    bytes32 public constant BILL_MANAGER_ROLE = keccak256("BILL_MANAGER_ROLE");

    // Security constants
    uint256 public constant FEE_DENOMINATOR = 10000; // 100% = 10000
    uint256 public constant MAX_PLATFORM_FEE = 1000; // 10% maximum platform fee
    uint256 public constant MIN_PAYMENT_AMOUNT = 100; // Minimum payment amount to prevent dust attacks
    uint256 public constant MAX_BILL_AMOUNT = 1_000_000 * 10**6; // $1M max per bill
    uint256 public constant RATE_LIMIT_WINDOW = 60; // 1 minute rate limit window
    uint256 public constant MAX_REGISTRATION_FEE = 1000 * 10**6; // $1000 max registration fee
    uint256 public constant MAX_SPLIT_PARTICIPANTS = 20; // Maximum number of split participants

    // State variables
    IERC20 public usdcToken;
    address public platformTreasury;
    uint256 public platformFeeRate; // in basis points (200 = 2% as per PRD)
    uint256 public billModificationNonce; // To prevent replay attacks
    address public billCreatorAddress; // Admin-controlled address that can create bills
    uint256 public feeUpdateDelay; // Delay for fee updates (in seconds)
    uint256 public pendingFeeRate; // Pending fee rate
    uint256 public feeUpdateTimestamp; // When fee update can be executed
    uint256 public businessRegistrationFee; // Fee for business registration in USDC
    uint256 public pendingRegistrationFee; // Pending registration fee
    uint256 public registrationFeeUpdateTimestamp; // When registration fee update can be executed

    // Simplified unified bill structure
    struct Bill {
        address businessAddress;     // 20 bytes
        bool isPaid;                 // 1 byte
        bool isCancelled;            // 1 byte
        uint8 participantCount;      // 1 byte - current number of participants
        uint64 createdAt;            // 8 bytes
        uint64 lastPaymentAt;        // 8 bytes
        uint256 totalAmount;         // 32 bytes
        uint256 paidAmount;          // 32 bytes
        bytes32 nonce;               // 32 bytes
    }

    struct Participant {
        uint256 paidAmount;          // 32 bytes - total paid by this participant
        uint32 paymentCount;         // 4 bytes - number of payments made
        uint32 lastPaymentTime;      // 4 bytes - timestamp of last payment
    }

    struct BusinessInfo {
        address paymentAddress;      // 20 bytes
        address tippingAddress;      // 20 bytes
        bool isActive;               // 1 byte
        uint64 registrationDate;     // 8 bytes (sufficient until year 2554)
        uint256 totalVolume;         // 32 bytes
        uint256 totalTips;           // 32 bytes
    }

    struct Payment {
        bytes32 id;                  // 32 bytes
        bytes32 billId;             // 32 bytes
        address payer;               // 20 bytes
        uint64 timestamp;            // 8 bytes (sufficient until year 2554)
        uint256 amount;              // 32 bytes
        uint256 tipAmount;           // 32 bytes
        uint256 platformFee;         // 32 bytes
    }

    struct ClaimableBalance {
        uint256 amount;              // 32 bytes
        uint64 lastClaimed;          // 8 bytes (sufficient until year 2554)
    }

    struct AlternativePayment {
        address participant;         // 20 bytes - who paid
        uint256 amount;              // 32 bytes - amount paid
        uint64 timestamp;            // 8 bytes - when confirmed
        uint8 methodType;            // 1 byte - payment method enum
        bool verified;               // 1 byte - confirmed by business
    }

    // Payment method types
    enum PaymentMethod {
        CRYPTO,    // 0 - On-chain USDC payment
        CASH,      // 1 - Cash payment
        CARD,      // 2 - Credit/debit card
        VENMO,     // 3 - Venmo/PayPal
        OTHER      // 4 - Other payment methods
    }

    // Simplified mappings
    mapping(bytes32 => Bill) public bills;
    mapping(bytes32 => bool) public billExists;
    mapping(bytes32 => Payment[]) public billPayments;
    mapping(string => mapping(bytes32 => bool)) public usedNonces;
    mapping(address => BusinessInfo) public businessInfo;
    mapping(address => uint256) public businessBillCount;
    mapping(address => uint256) public lastBillCreation;
    mapping(address => ClaimableBalance) public claimablePayments;
    mapping(address => ClaimableBalance) public claimableTips;
    
    // Simple participant tracking
    mapping(bytes32 => mapping(address => Participant)) public billParticipants;
    mapping(bytes32 => address[]) public participantList;
    
    // Alternative payment tracking
    mapping(bytes32 => AlternativePayment[]) public billAlternativePayments;
    mapping(bytes32 => uint256) public totalAlternativeAmount;

    // Modifiers
    modifier onlyActiveBusiness(address business) {
        require(businessInfo[business].isActive, "Business not active");
        _;
    }
    
    modifier onlyBillOwner(bytes32 billId) {
        require(bills[billId].businessAddress == msg.sender, "Not bill owner");
        _;
    }
    
    modifier validAmount(uint256 amount) {
        require(amount >= MIN_PAYMENT_AMOUNT, "Amount too small");
        require(amount <= MAX_BILL_AMOUNT, "Amount exceeds limit");
        _;
    }

    modifier onlyBillCreator() {
        require(msg.sender == billCreatorAddress, "Not authorized bill creator");
        _;
    }
    
    modifier rateLimited(address user) {
        require(
            block.timestamp >= lastBillCreation[user] + RATE_LIMIT_WINDOW,
            "Rate limit exceeded"
        );
        _;
        lastBillCreation[user] = block.timestamp;
    }

    // Events
    event BusinessRegistered(address indexed businessAddress, string name, address paymentAddress, address tippingAddress, uint256 registrationFee);
    event BusinessPaymentAddressUpdated(address indexed businessAddress, address newPaymentAddress);
    event BusinessTippingAddressUpdated(address indexed businessAddress, address newTippingAddress);
    event BillCreated(bytes32 indexed billId, address indexed creator, address indexed businessAddress, uint256 totalAmount, string metadata);
    event PaymentProcessed(bytes32 indexed paymentId, bytes32 indexed billId, address indexed payer, uint256 amount, uint256 tipAmount, uint256 platformFee, bool billComplete);
    event NewParticipantAdded(bytes32 indexed billId, address indexed participant, uint256 paymentAmount);
    event BillCancelled(bytes32 indexed billId, address indexed businessAddress);
    event EarningsClaimed(address indexed businessAddress, address indexed recipient, uint256 amount, bool isTip);
    event PlatformFeeUpdateProposed(uint256 newFeeRate, uint256 executeAfter);
    event PlatformFeeUpdated(uint256 newFeeRate);
    event BillCreatorUpdated(address indexed oldCreator, address indexed newCreator);
    event FeeUpdateDelayChanged(uint256 newDelay);
    event RegistrationFeeUpdateProposed(uint256 newFee, uint256 executeAfter);
    event RegistrationFeeUpdated(uint256 newFee);
    event AlternativePaymentMarked(bytes32 indexed billId, address indexed participant, uint256 amount, PaymentMethod method, address indexed confirmedBy);
    event BillCompletedWithAlternativePayments(bytes32 indexed billId, uint256 totalCrypto, uint256 totalAlternative);

    // Custom errors
    error BillNotFound(bytes32 billId);
    error BillAlreadyExists(bytes32 billId);
    error BillNotActive(bytes32 billId);
    error InvalidAmount(uint256 amount);
    error ExcessivePayment(uint256 remaining, uint256 attempted);
    error ZeroAddress();
    error UnauthorizedBillModification(address caller, address creator);
    error NothingToClaim();
    error NonceAlreadyUsed(bytes32 nonce);

    /**
     * @dev Initialize the contract
     */
    function initialize(
        address _usdcToken,
        address _platformTreasury,
        uint256 _platformFeeRate,
        address _admin,
        address _billCreator,
        uint256 _registrationFee
    ) public initializer {
        require(_usdcToken != address(0), "Invalid USDC token");
        require(_platformTreasury != address(0), "Invalid treasury");
        require(_platformFeeRate <= MAX_PLATFORM_FEE, "Fee too high");
        require(_admin != address(0), "Invalid admin");
        require(_billCreator != address(0), "Invalid bill creator");
        require(_registrationFee <= MAX_REGISTRATION_FEE, "Registration fee too high");

        __ReentrancyGuard_init();
        __Pausable_init();
        __AccessControl_init();
        __UUPSUpgradeable_init();

        usdcToken = IERC20(_usdcToken);
        platformTreasury = _platformTreasury;
        platformFeeRate = _platformFeeRate;
        billCreatorAddress = _billCreator;
        businessRegistrationFee = _registrationFee;
        feeUpdateDelay = 24 hours; // Default 24 hour delay for fee updates

        _grantRole(DEFAULT_ADMIN_ROLE, _admin);
        _grantRole(ADMIN_ROLE, _admin);
        _grantRole(UPGRADER_ROLE, _admin);
        _grantRole(BILL_MANAGER_ROLE, _admin);
    }

    /**
     * @dev Register a new business (with registration fee)
     */
    function registerBusiness(
        string calldata name,
        address paymentAddress,
        address tippingAddress
    ) external whenNotPaused {
        require(paymentAddress != address(0) && tippingAddress != address(0), "Zero address");
        require(!businessInfo[msg.sender].isActive, "Business already registered");

        // Collect registration fee if set
        if (businessRegistrationFee > 0) {
            usdcToken.safeTransferFrom(msg.sender, platformTreasury, businessRegistrationFee);
        }

        businessInfo[msg.sender] = BusinessInfo({
            paymentAddress: paymentAddress,
            tippingAddress: tippingAddress,
            totalVolume: 0,
            totalTips: 0,
            registrationDate: uint64(block.timestamp),
            isActive: true
        });

        emit BusinessRegistered(msg.sender, name, paymentAddress, tippingAddress, businessRegistrationFee);
    }

    /**
     * @dev Update business payment address (only business owner)
     */
    function updateBusinessPaymentAddress(
        address newPaymentAddress
    ) external onlyActiveBusiness(msg.sender) whenNotPaused {
        require(newPaymentAddress != address(0), "Zero address");

        BusinessInfo storage business = businessInfo[msg.sender];
        business.paymentAddress = newPaymentAddress;

        emit BusinessPaymentAddressUpdated(msg.sender, newPaymentAddress);
    }

    /**
     * @dev Update business tipping address (only business owner)
     */
    function updateBusinessTippingAddress(
        address newTippingAddress
    ) external onlyActiveBusiness(msg.sender) whenNotPaused {
        require(newTippingAddress != address(0), "Zero address");

        BusinessInfo storage business = businessInfo[msg.sender];
        business.tippingAddress = newTippingAddress;

        emit BusinessTippingAddressUpdated(msg.sender, newTippingAddress);
    }

    /**
     * @dev Create a new bill (with proper authorization and nonce protection)
     */
    function createBill(
        bytes32 billId,
        address businessAddress,
        uint256 totalAmount,
        string calldata metadata,
        bytes32 nonce
    ) external 
        onlyBillCreator
        validAmount(totalAmount) 
        rateLimited(msg.sender)
        whenNotPaused 
    {
        require(!billExists[billId], "Bill already exists");
        require(businessAddress != address(0), "Zero address");
        require(!usedNonces["createBill"][nonce], "Nonce already used");
        require(businessInfo[businessAddress].isActive, "Business not active");

        usedNonces["createBill"][nonce] = true;

        bills[billId] = Bill({
            businessAddress: businessAddress,
            totalAmount: totalAmount,
            paidAmount: 0,
            createdAt: uint64(block.timestamp),
            lastPaymentAt: 0,
            isPaid: false,
            isCancelled: false,
            participantCount: 0,
            nonce: nonce
        });

        billExists[billId] = true;
        businessBillCount[businessAddress]++;

        emit BillCreated(billId, msg.sender, businessAddress, totalAmount, metadata);
    }


    /**
     * @dev Process payment for a bill (with security checks)
     */
    function processPayment(
        bytes32 billId,
        uint256 amount,
        uint256 tipAmount
    ) external nonReentrant validAmount(amount) whenNotPaused {
        require(billExists[billId], "Bill not found");
        
        Bill storage bill = bills[billId];
        require(!bill.isPaid && !bill.isCancelled, "Bill not active");
        require(amount <= (bill.totalAmount - bill.paidAmount), "Excessive payment");

        BusinessInfo storage business = businessInfo[bill.businessAddress];
        require(business.isActive, "Business not active");

        // Handle participant tracking - simple approach
        Participant storage participant = billParticipants[billId][msg.sender];
        bool isNewParticipant = (participant.paymentCount == 0 && participant.paidAmount == 0);
        
        if (isNewParticipant) {
            participantList[billId].push(msg.sender);
            bill.participantCount++;
            emit NewParticipantAdded(billId, msg.sender, amount);
        }

        // Process the payment
        uint256 totalTransfer = amount + tipAmount;
        uint256 platformFee = (amount * platformFeeRate) / FEE_DENOMINATOR;
        uint256 businessAmount = amount - platformFee;

        // Transfer tokens (external calls after state updates)
        usdcToken.safeTransferFrom(msg.sender, address(this), totalTransfer);
        
        if (platformFee > 0) {
            usdcToken.safeTransfer(platformTreasury, platformFee);
        }

        // Update claimable balances (CEI pattern)
        claimablePayments[business.paymentAddress].amount += businessAmount;
        if (tipAmount > 0) {
            claimableTips[business.tippingAddress].amount += tipAmount;
        }

        // Update participant state
        participant.paidAmount += amount;
        participant.paymentCount++;
        participant.lastPaymentTime = uint32(block.timestamp);

        // Update bill state
        bill.paidAmount += amount;
        bill.lastPaymentAt = uint64(block.timestamp);
        
        // Simple completion check - bill is complete when total amount is reached
        bool billComplete = false;
        if (bill.paidAmount >= bill.totalAmount) {
            bill.isPaid = true;
            billComplete = true;
        }

        // Update business stats
        business.totalVolume += amount;
        business.totalTips += tipAmount;

        // Create payment record
        bytes32 paymentId = keccak256(abi.encodePacked(billId, msg.sender, block.timestamp, participant.paymentCount));
        billPayments[billId].push(Payment({
            id: paymentId,
            billId: billId,
            payer: msg.sender,
            amount: amount,
            tipAmount: tipAmount,
            platformFee: platformFee,
            timestamp: uint64(block.timestamp)
        }));

        emit PaymentProcessed(paymentId, billId, msg.sender, amount, tipAmount, platformFee, billComplete);
    }

    /**
     * @dev Claim earnings (reentrancy protected with CEI pattern)
     * Only business owners can claim their own earnings using registered addresses
     */
    function claimEarnings() external nonReentrant whenNotPaused {
        BusinessInfo memory business = businessInfo[msg.sender];
        require(business.isActive, "Not business owner");
        require(business.paymentAddress != address(0) && business.tippingAddress != address(0), "Invalid addresses");
        
        // Use local variables to prevent reentrancy
        uint256 paymentAmount = claimablePayments[business.paymentAddress].amount;
        uint256 tipAmount = claimableTips[business.tippingAddress].amount;
        
        require(paymentAmount > 0 || tipAmount > 0, "Nothing to claim");

        // Reset state before external calls (CEI pattern)
        if (paymentAmount > 0) {
            claimablePayments[business.paymentAddress].amount = 0;
            claimablePayments[business.paymentAddress].lastClaimed = uint64(block.timestamp);
        }
        
        if (tipAmount > 0) {
            claimableTips[business.tippingAddress].amount = 0;
            claimableTips[business.tippingAddress].lastClaimed = uint64(block.timestamp);
        }
        
        // External calls after state updates
        if (paymentAmount > 0) {
            usdcToken.safeTransfer(business.paymentAddress, paymentAmount);
            emit EarningsClaimed(msg.sender, business.paymentAddress, paymentAmount, false);
        }
        
        if (tipAmount > 0) {
            usdcToken.safeTransfer(business.tippingAddress, tipAmount);
            emit EarningsClaimed(msg.sender, business.tippingAddress, tipAmount, true);
        }
    }

    /**
     * @dev Propose platform fee rate update (with timelock)
     */
    function proposePlatformFeeUpdate(uint256 newFeeRate) external onlyRole(ADMIN_ROLE) {
        require(newFeeRate <= MAX_PLATFORM_FEE, "Fee too high");
        
        pendingFeeRate = newFeeRate;
        feeUpdateTimestamp = block.timestamp + feeUpdateDelay;
        
        emit PlatformFeeUpdateProposed(newFeeRate, feeUpdateTimestamp);
    }

    /**
     * @dev Execute pending platform fee update (after timelock)
     */
    function executePlatformFeeUpdate() external onlyRole(ADMIN_ROLE) {
        require(feeUpdateTimestamp != 0, "No pending fee update");
        require(block.timestamp >= feeUpdateTimestamp, "Timelock not expired");
        
        platformFeeRate = pendingFeeRate;
        
        // Reset pending state
        pendingFeeRate = 0;
        feeUpdateTimestamp = 0;
        
        emit PlatformFeeUpdated(platformFeeRate);
    }

    /**
     * @dev Cancel pending platform fee update
     */
    function cancelPlatformFeeUpdate() external onlyRole(ADMIN_ROLE) {
        require(feeUpdateTimestamp != 0, "No pending fee update");
        
        pendingFeeRate = 0;
        feeUpdateTimestamp = 0;
        
        emit PlatformFeeUpdateProposed(0, 0); // Indicates cancellation
    }

    /**
     * @dev Update fee update delay (admin function)
     */
    function setFeeUpdateDelay(uint256 newDelay) external onlyRole(ADMIN_ROLE) {
        require(newDelay >= 1 hours, "Delay too short");
        require(newDelay <= 30 days, "Delay too long");
        
        feeUpdateDelay = newDelay;
        emit FeeUpdateDelayChanged(newDelay);
    }

    /**
     * @dev Set bill creator address (admin function)
     */
    function setBillCreator(address newBillCreator) external onlyRole(ADMIN_ROLE) {
        require(newBillCreator != address(0), "Zero address");
        address oldCreator = billCreatorAddress;
        billCreatorAddress = newBillCreator;
        emit BillCreatorUpdated(oldCreator, newBillCreator);
    }

    /**
     * @dev Propose business registration fee update (with timelock)
     */
    function proposeRegistrationFeeUpdate(uint256 newFee) external onlyRole(ADMIN_ROLE) {
        require(newFee <= MAX_REGISTRATION_FEE, "Registration fee too high");
        
        pendingRegistrationFee = newFee;
        registrationFeeUpdateTimestamp = block.timestamp + feeUpdateDelay;
        
        emit RegistrationFeeUpdateProposed(newFee, registrationFeeUpdateTimestamp);
    }

    /**
     * @dev Execute pending registration fee update (after timelock)
     */
    function executeRegistrationFeeUpdate() external onlyRole(ADMIN_ROLE) {
        require(registrationFeeUpdateTimestamp != 0, "No pending fee update");
        require(block.timestamp >= registrationFeeUpdateTimestamp, "Timelock not expired");
        
        businessRegistrationFee = pendingRegistrationFee;
        
        // Reset pending state
        pendingRegistrationFee = 0;
        registrationFeeUpdateTimestamp = 0;
        
        emit RegistrationFeeUpdated(businessRegistrationFee);
    }

    /**
     * @dev Cancel pending registration fee update
     */
    function cancelRegistrationFeeUpdate() external onlyRole(ADMIN_ROLE) {
        require(registrationFeeUpdateTimestamp != 0, "No pending fee update");
        
        pendingRegistrationFee = 0;
        registrationFeeUpdateTimestamp = 0;
        
        emit RegistrationFeeUpdateProposed(0, 0); // Indicates cancellation
    }

    /**
     * @dev Mark alternative payment (cash/card) for a bill participant
     * @param billId The bill identifier
     * @param participant The participant who paid alternatively
     * @param amount Amount paid via alternative method
     * @param method Payment method used
     */
    function markAlternativePayment(
        bytes32 billId,
        address participant,
        uint256 amount,
        PaymentMethod method
    ) external onlyRole(BILL_MANAGER_ROLE) nonReentrant whenNotPaused {
        require(billExists[billId], "Bill not found");
        require(participant != address(0), "Zero address");
        require(amount > 0, "Invalid amount");
        require(method != PaymentMethod.CRYPTO, "Use processPayment for crypto");
        
        Bill storage bill = bills[billId];
        require(!bill.isPaid && !bill.isCancelled, "Bill not active");
        
        // Check if this would exceed the bill total
        uint256 currentPaid = bill.paidAmount + totalAlternativeAmount[billId];
        require(currentPaid + amount <= bill.totalAmount, "Exceeds bill total");
        
        // Add alternative payment record
        billAlternativePayments[billId].push(AlternativePayment({
            participant: participant,
            amount: amount,
            timestamp: uint64(block.timestamp),
            methodType: uint8(method),
            verified: true
        }));
        
        // Update totals
        totalAlternativeAmount[billId] += amount;
        
        // Update participant tracking
        Participant storage participantInfo = billParticipants[billId][participant];
        if (participantInfo.paymentCount == 0) {
            // First time participant
            participantList[billId].push(participant);
            bill.participantCount++;
        }
        participantInfo.paidAmount += amount;
        participantInfo.paymentCount++;
        participantInfo.lastPaymentTime = uint32(block.timestamp);
        
        // Check if bill is now complete
        uint256 totalPaid = bill.paidAmount + totalAlternativeAmount[billId];
        if (totalPaid >= bill.totalAmount) {
            bill.isPaid = true;
            bill.lastPaymentAt = uint64(block.timestamp);
            
            emit BillCompletedWithAlternativePayments(
                billId, 
                bill.paidAmount, 
                totalAlternativeAmount[billId]
            );
        }
        
        emit AlternativePaymentMarked(billId, participant, amount, method, msg.sender);
    }

    /**
     * @dev Get business bill count (gas-efficient alternative to array)
     */
    function getBusinessBillCount(address businessAddress) external view returns (uint256) {
        return businessBillCount[businessAddress];
    }

    /**
     * @dev Pause contract (emergency function)
     */
    function pause() external onlyRole(ADMIN_ROLE) {
        _pause();
    }

    /**
     * @dev Unpause contract
     */
    function unpause() external onlyRole(ADMIN_ROLE) {
        _unpause();
    }

    /**
     * @dev Get bill information
     */
    function getBill(bytes32 billId) external view returns (Bill memory) {
        require(billExists[billId], "Bill not found");
        return bills[billId];
    }

    /**
     * @dev Get business information
     */
    function getBusinessInfo(address businessAddress) external view returns (BusinessInfo memory) {
        return businessInfo[businessAddress];
    }

    /**
     * @dev Get bill payments
     */
    function getBillPayments(bytes32 billId) external view returns (Payment[] memory) {
        return billPayments[billId];
    }

    /**
     * @dev Get claimable amounts
     */
    function getClaimableAmounts(address paymentAddress, address tippingAddress) 
        external 
        view 
        returns (uint256 payments, uint256 tips) 
    {
        return (
            claimablePayments[paymentAddress].amount,
            claimableTips[tippingAddress].amount
        );
    }

    /**
     * @dev Get all participants for a bill
     */
    function getBillParticipants(bytes32 billId) 
        external 
        view 
        returns (address[] memory) 
    {
        require(billExists[billId], "Bill not found");
        return participantList[billId];
    }

    /**
     * @dev Get specific participant's payment info
     */
    function getParticipantInfo(bytes32 billId, address participant) 
        external 
        view 
        returns (uint256 paidAmount, uint32 paymentCount, uint32 lastPaymentTime) 
    {
        require(billExists[billId], "Bill not found");
        Participant memory p = billParticipants[billId][participant];
        
        return (p.paidAmount, p.paymentCount, p.lastPaymentTime);
    }

    /**
     * @dev Check if an address has participated in a bill
     */
    function hasParticipatedInBill(bytes32 billId, address participant) 
        external 
        view 
        returns (bool) 
    {
        Participant memory p = billParticipants[billId][participant];
        return p.paymentCount > 0 || p.paidAmount > 0;
    }

    /**
     * @dev Get bill summary
     */
    function getBillSummary(bytes32 billId) 
        external 
        view 
        returns (
            uint8 participantCount,
            uint256 totalAmount,
            uint256 paidAmount,
            bool isPaid
        ) 
    {
        require(billExists[billId], "Bill not found");
        Bill memory bill = bills[billId];
        
        return (
            bill.participantCount,
            bill.totalAmount,
            bill.paidAmount,
            bill.isPaid
        );
    }

    /**
     * @dev Get current registration fee
     */
    function getRegistrationFee() external view returns (uint256) {
        return businessRegistrationFee;
    }

    /**
     * @dev Get pending registration fee info
     */
    function getPendingRegistrationFeeInfo() 
        external 
        view 
        returns (uint256 pendingFee, uint256 executeAfter) 
    {
        return (pendingRegistrationFee, registrationFeeUpdateTimestamp);
    }

    /**
     * @dev Get alternative payments for a bill
     */
    function getBillAlternativePayments(bytes32 billId) 
        external 
        view 
        returns (AlternativePayment[] memory) 
    {
        return billAlternativePayments[billId];
    }

    /**
     * @dev Get total alternative payment amount for a bill
     */
    function getTotalAlternativeAmount(bytes32 billId) 
        external 
        view 
        returns (uint256) 
    {
        return totalAlternativeAmount[billId];
    }

    /**
     * @dev Get complete bill payment breakdown (crypto + alternative)
     */
    function getBillPaymentBreakdown(bytes32 billId) 
        external 
        view 
        returns (
            uint256 totalAmount,
            uint256 cryptoPaid,
            uint256 alternativePaid,
            uint256 remaining,
            bool isComplete
        ) 
    {
        require(billExists[billId], "Bill not found");
        
        Bill memory bill = bills[billId];
        uint256 altPaid = totalAlternativeAmount[billId];
        uint256 totalPaid = bill.paidAmount + altPaid;
        
        return (
            bill.totalAmount,
            bill.paidAmount,
            altPaid,
            bill.totalAmount > totalPaid ? bill.totalAmount - totalPaid : 0,
            bill.isPaid
        );
    }

    /**
     * @dev Required by UUPSUpgradeable
     */
    function _authorizeUpgrade(address newImplementation) internal override onlyRole(UPGRADER_ROLE) {}

    /**
     * @dev Get contract version
     */
    function version() external pure returns (string memory) {
        return "5.0.0-unified-simple";
    }
}
