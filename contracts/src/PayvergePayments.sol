// SPDX-License-Identifier: MIT
pragma solidity ^0.8.19;

import "@openzeppelin/contracts/token/ERC20/IERC20.sol";
import "@openzeppelin/contracts/token/ERC20/utils/SafeERC20.sol";
import "@openzeppelin/contracts/utils/ReentrancyGuard.sol";
import "@openzeppelin/contracts/access/Ownable.sol";
import "@openzeppelin/contracts/utils/Pausable.sol";

/**
 * @title PayvergePayments
 * @dev Smart contract for handling USDC payments in the Payverge hospitality platform
 * @notice This contract manages bill payments, tips, and platform fees
 */
contract PayvergePayments is ReentrancyGuard, Ownable, Pausable {
    using SafeERC20 for IERC20;

    // USDC token contract
    IERC20 public immutable USDC;
    
    // Platform fee in basis points (200 = 2%)
    uint256 public constant PLATFORM_FEE_BPS = 200;
    uint256 public constant MAX_PLATFORM_FEE_BPS = 1000; // 10% maximum
    uint256 public constant BPS_DENOMINATOR = 10000;
    
    // Platform fee recipient
    address public platformFeeRecipient;
    
    // Payment structure
    struct Payment {
        bytes32 billId;
        address payer;
        uint256 amount;
        uint256 tipAmount;
        address businessAddress;
        address tipAddress;
        uint256 platformFee;
        uint256 timestamp;
        string transactionHash;
    }
    
    // Bill payment tracking
    mapping(bytes32 => Payment[]) public billPayments;
    mapping(bytes32 => uint256) public billTotalPaid;
    mapping(bytes32 => uint256) public billTotalTips;
    mapping(bytes32 => bool) public billExists;
    
    // Business verification (optional - can be managed off-chain)
    mapping(address => bool) public verifiedBusinesses;
    
    // Events
    event PaymentMade(
        bytes32 indexed billId,
        address indexed payer,
        uint256 amount,
        uint256 tipAmount,
        address indexed businessAddress,
        address tipAddress,
        uint256 platformFee,
        uint256 timestamp
    );
    
    event BillCreated(
        bytes32 indexed billId,
        address indexed businessAddress,
        address tipAddress,
        uint256 totalAmount
    );
    
    event BusinessVerified(address indexed businessAddress, bool verified);
    event PlatformFeeRecipientUpdated(address indexed oldRecipient, address indexed newRecipient);
    
    // Custom errors
    error InvalidAmount();
    error InvalidAddress();
    error BillNotFound();
    error PaymentExceedsBill();
    error TransferFailed();
    error InvalidBillId();
    error BusinessNotVerified();
    
    /**
     * @dev Constructor
     * @param _usdc USDC token contract address
     * @param _platformFeeRecipient Address to receive platform fees
     */
    constructor(
        address _usdc,
        address _platformFeeRecipient
    ) Ownable(msg.sender) {
        if (_usdc == address(0) || _platformFeeRecipient == address(0)) {
            revert InvalidAddress();
        }
        
        USDC = IERC20(_usdc);
        platformFeeRecipient = _platformFeeRecipient;
    }
    
    /**
     * @dev Create a new bill record
     * @param billId Unique identifier for the bill
     * @param businessAddress Address where business payments will be sent
     * @param tipAddress Address where tips will be sent
     * @param totalAmount Total bill amount in USDC (6 decimals)
     */
    function createBill(
        bytes32 billId,
        address businessAddress,
        address tipAddress,
        uint256 totalAmount
    ) external onlyOwner {
        if (billId == bytes32(0)) revert InvalidBillId();
        if (businessAddress == address(0) || tipAddress == address(0)) revert InvalidAddress();
        if (totalAmount == 0) revert InvalidAmount();
        
        billExists[billId] = true;
        
        emit BillCreated(billId, businessAddress, tipAddress, totalAmount);
    }
    
    /**
     * @dev Process a payment for a bill
     * @param billId Unique identifier for the bill
     * @param amount Payment amount in USDC (6 decimals)
     * @param tipAmount Tip amount in USDC (6 decimals)
     * @param businessAddress Address where business payment will be sent
     * @param tipAddress Address where tip will be sent
     */
    function payBill(
        bytes32 billId,
        uint256 amount,
        uint256 tipAmount,
        address businessAddress,
        address tipAddress
    ) external nonReentrant whenNotPaused {
        if (billId == bytes32(0)) revert InvalidBillId();
        if (amount == 0) revert InvalidAmount();
        if (businessAddress == address(0) || tipAddress == address(0)) revert InvalidAddress();
        
        uint256 totalPayment = amount + tipAmount;
        if (totalPayment == 0) revert InvalidAmount();
        
        // Calculate platform fee on the bill amount only (not tips)
        uint256 platformFee = (amount * PLATFORM_FEE_BPS) / BPS_DENOMINATOR;
        uint256 businessAmount = amount - platformFee;
        
        // Transfer USDC from payer
        USDC.safeTransferFrom(msg.sender, address(this), totalPayment);
        
        // Distribute payments
        if (businessAmount > 0) {
            USDC.safeTransfer(businessAddress, businessAmount);
        }
        
        if (tipAmount > 0) {
            USDC.safeTransfer(tipAddress, tipAmount);
        }
        
        if (platformFee > 0) {
            USDC.safeTransfer(platformFeeRecipient, platformFee);
        }
        
        // Record payment
        Payment memory payment = Payment({
            billId: billId,
            payer: msg.sender,
            amount: amount,
            tipAmount: tipAmount,
            businessAddress: businessAddress,
            tipAddress: tipAddress,
            platformFee: platformFee,
            timestamp: block.timestamp,
            transactionHash: ""
        });
        
        billPayments[billId].push(payment);
        billTotalPaid[billId] += amount;
        billTotalTips[billId] += tipAmount;
        
        emit PaymentMade(
            billId,
            msg.sender,
            amount,
            tipAmount,
            businessAddress,
            tipAddress,
            platformFee,
            block.timestamp
        );
    }
    
    /**
     * @dev Get all payments for a specific bill
     * @param billId The bill identifier
     * @return Array of payments for the bill
     */
    function getBillPayments(bytes32 billId) external view returns (Payment[] memory) {
        return billPayments[billId];
    }
    
    /**
     * @dev Get payment count for a bill
     * @param billId The bill identifier
     * @return Number of payments made for the bill
     */
    function getBillPaymentCount(bytes32 billId) external view returns (uint256) {
        return billPayments[billId].length;
    }
    
    /**
     * @dev Get total amount paid for a bill (excluding tips)
     * @param billId The bill identifier
     * @return Total amount paid for the bill
     */
    function getBillTotalPaid(bytes32 billId) external view returns (uint256) {
        return billTotalPaid[billId];
    }
    
    /**
     * @dev Get total tips for a bill
     * @param billId The bill identifier
     * @return Total tips paid for the bill
     */
    function getBillTotalTips(bytes32 billId) external view returns (uint256) {
        return billTotalTips[billId];
    }
    
    /**
     * @dev Verify or unverify a business address
     * @param businessAddress The business address to verify
     * @param verified Whether the business is verified
     */
    function setBusinessVerification(address businessAddress, bool verified) external onlyOwner {
        if (businessAddress == address(0)) revert InvalidAddress();
        
        verifiedBusinesses[businessAddress] = verified;
        emit BusinessVerified(businessAddress, verified);
    }
    
    /**
     * @dev Update platform fee recipient
     * @param newRecipient New platform fee recipient address
     */
    function setPlatformFeeRecipient(address newRecipient) external onlyOwner {
        if (newRecipient == address(0)) revert InvalidAddress();
        
        address oldRecipient = platformFeeRecipient;
        platformFeeRecipient = newRecipient;
        
        emit PlatformFeeRecipientUpdated(oldRecipient, newRecipient);
    }
    
    /**
     * @dev Pause the contract (emergency function)
     */
    function pause() external onlyOwner {
        _pause();
    }
    
    /**
     * @dev Unpause the contract
     */
    function unpause() external onlyOwner {
        _unpause();
    }
    
    /**
     * @dev Emergency withdrawal function (only for stuck tokens)
     * @param token Token contract address
     * @param amount Amount to withdraw
     * @param recipient Recipient address
     */
    function emergencyWithdraw(
        address token,
        uint256 amount,
        address recipient
    ) external onlyOwner {
        if (recipient == address(0)) revert InvalidAddress();
        
        IERC20(token).safeTransfer(recipient, amount);
    }
    
    /**
     * @dev Get contract version
     * @return Version string
     */
    function version() external pure returns (string memory) {
        return "1.0.0";
    }
}
