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
 * @title PayvergeReferrals
 * @dev Referral system for Payverge platform with tiered access and rewards
 * @notice Handles referrer registration, code generation, and commission distribution
 */
contract PayvergeReferrals is 
    Initializable,
    ReentrancyGuardUpgradeable,
    PausableUpgradeable,
    AccessControlUpgradeable,
    UUPSUpgradeable
{
    using SafeERC20 for IERC20;

    // Role definitions
    bytes32 public constant ADMIN_ROLE = keccak256("ADMIN_ROLE");
    bytes32 public constant UPGRADER_ROLE = keccak256("UPGRADER_ROLE");

    // Constants
    uint256 public constant BASIC_REFERRER_FEE = 10 * 10**6; // $10 USDC
    uint256 public constant PREMIUM_REFERRER_FEE = 25 * 10**6; // $25 USDC
    uint256 public constant BASIC_BUSINESS_DISCOUNT_RATE = 1000; // 10% discount for basic referrals
    uint256 public constant PREMIUM_BUSINESS_DISCOUNT_RATE = 1500; // 15% discount for premium referrals
    uint256 public constant BASIC_COMMISSION_RATE = 1000; // 10% commission for basic referrers
    uint256 public constant PREMIUM_COMMISSION_RATE = 1500; // 15% commission for premium referrers
    uint256 public constant FEE_DENOMINATOR = 10000; // 100% = 10000
    uint256 public constant MAX_REFERRAL_CODE_LENGTH = 12;
    uint256 public constant MIN_REFERRAL_CODE_LENGTH = 6;

    // State variables
    IERC20 public usdcToken;
    address public platformTreasury;
    address public payvergePaymentsContract;
    uint256 public totalReferrers;
    uint256 public totalCommissionsPaid;

    // Referrer tiers
    enum ReferrerTier {
        None,
        Basic,
        Premium
    }

    // Referrer information
    struct Referrer {
        address referrerAddress;     // 20 bytes
        ReferrerTier tier;           // 1 byte
        bool isActive;               // 1 byte
        uint64 registrationDate;     // 8 bytes
        uint256 totalReferrals;      // 32 bytes
        uint256 totalCommissions;    // 32 bytes
        uint256 claimableCommissions; // 32 bytes - amount available to claim
        uint256 lastClaimedAt;       // 32 bytes - timestamp of last claim
        string referralCode;         // Dynamic - stored separately for gas optimization
    }

    // Referral tracking
    struct ReferralRecord {
        bytes32 id;                 // 32 bytes
        address referrer;           // 20 bytes
        address business;           // 20 bytes
        uint64 timestamp;           // 8 bytes
        uint256 registrationFee;    // 32 bytes
        uint256 discount;           // 32 bytes
        uint256 commission;         // 32 bytes
        bool commissionPaid;        // 1 byte
    }

    // Mappings
    mapping(address => Referrer) public referrers;
    mapping(string => address) public referralCodeToAddress;
    mapping(bytes32 => ReferralRecord) public referralRecords;
    mapping(address => bytes32[]) public referrerToRecords;
    mapping(address => bytes32) public businessToReferralRecord;

    // Events
    event ReferrerRegistered(
        address indexed referrer,
        ReferrerTier tier,
        string referralCode,
        uint256 fee
    );

    event ReferralUsed(
        bytes32 indexed referralId,
        address indexed referrer,
        string referralCode,
        uint256 discount,
        uint256 commission
    );

    event CommissionEarned(address indexed referrer, uint256 amount, uint256 totalClaimable);
    event CommissionClaimed(address indexed referrer, uint256 amount, uint256 remainingClaimable);

    event ReferralCodeUpdated(
        address indexed referrer,
        string oldCode,
        string newCode
    );

    event ReferrerTierUpgraded(
        address indexed referrer,
        ReferrerTier oldTier,
        ReferrerTier newTier
    );

    // Custom errors
    error InvalidReferralCode();
    error ReferralCodeTaken();
    error ReferrerAlreadyExists();
    error ReferrerNotFound();
    error ReferralNotFound();
    error UnauthorizedCaller();
    error InsufficientBalance();
    error InvalidUpgrade();
    error InvalidTier();
    error NoCommissionsToClaim();

    // Modifiers
    modifier onlyPayvergePayments() {
        if (msg.sender != payvergePaymentsContract) revert UnauthorizedCaller();
        _;
    }

    /// @custom:oz-upgrades-unsafe-allow constructor
    constructor() {
        _disableInitializers();
    }

    /**
     * @dev Initialize the contract
     * @param _usdcToken USDC token contract address
     * @param _platformTreasury Platform treasury address
     * @param _admin Admin address
     */
    function initialize(
        address _usdcToken,
        address _platformTreasury,
        address _admin
    ) public initializer {
        require(_usdcToken != address(0), "Invalid USDC token");
        require(_platformTreasury != address(0), "Invalid treasury");
        require(_admin != address(0), "Invalid admin");

        __ReentrancyGuard_init();
        __Pausable_init();
        __AccessControl_init();
        __UUPSUpgradeable_init();

        usdcToken = IERC20(_usdcToken);
        platformTreasury = _platformTreasury;

        _grantRole(DEFAULT_ADMIN_ROLE, _admin);
        _grantRole(ADMIN_ROLE, _admin);
        _grantRole(UPGRADER_ROLE, _admin);
    }

    /**
     * @dev Register as a basic referrer with $10 USDC fee
     * @param _referralCode Desired referral code (6-12 characters, alphanumeric)
     */
    function registerBasicReferrer(string calldata _referralCode) 
        external 
        nonReentrant 
        whenNotPaused 
    {
        _registerReferrer(_referralCode, ReferrerTier.Basic, BASIC_REFERRER_FEE);
    }

    /**
     * @dev Register as a premium referrer with $25 USDC fee
     * @param _referralCode Desired referral code (6-12 characters, alphanumeric)
     */
    function registerPremiumReferrer(string calldata _referralCode) 
        external 
        nonReentrant 
        whenNotPaused 
    {
        _registerReferrer(_referralCode, ReferrerTier.Premium, PREMIUM_REFERRER_FEE);
    }

    /**
     * @dev Upgrade from basic to premium referrer
     */
    function upgradeToPremium() external nonReentrant whenNotPaused {
        Referrer storage referrer = referrers[msg.sender];
        if (referrer.tier != ReferrerTier.Basic) revert InvalidTier();

        uint256 upgradeFee = PREMIUM_REFERRER_FEE - BASIC_REFERRER_FEE; // $15 USDC
        usdcToken.safeTransferFrom(msg.sender, platformTreasury, upgradeFee);

        ReferrerTier oldTier = referrer.tier;
        referrer.tier = ReferrerTier.Premium;

        emit ReferrerTierUpgraded(msg.sender, oldTier, ReferrerTier.Premium);
    }

    /**
     * @dev Process a referral when a business registers (called by PayvergePayments contract)
     * @param _business Business address
     * @param _referralCode Referral code used
     * @param _registrationFee Original registration fee
     * @return discount Amount to discount from registration fee
     * @return referrer Address of the referrer (for commission payment)
     * @return commission Commission amount to pay referrer
     */
    function processReferral(
        address _business,
        string calldata _referralCode,
        uint256 _registrationFee
    ) 
        external 
        returns (uint256 discount, address referrer, uint256 commission) 
    {
        if (msg.sender != payvergePaymentsContract) revert UnauthorizedCaller();
        
        referrer = referralCodeToAddress[_referralCode];
        if (referrer == address(0)) revert ReferralNotFound();

        Referrer storage referrerData = referrers[referrer];
        if (!referrerData.isActive) revert ReferrerNotFound();

        // Calculate discount based on tier
        uint256 discountRate = referrerData.tier == ReferrerTier.Premium 
            ? PREMIUM_BUSINESS_DISCOUNT_RATE 
            : BASIC_BUSINESS_DISCOUNT_RATE;
        discount = (_registrationFee * discountRate) / FEE_DENOMINATOR;

        // Calculate commission based on tier
        uint256 commissionRate = referrerData.tier == ReferrerTier.Premium 
            ? PREMIUM_COMMISSION_RATE 
            : BASIC_COMMISSION_RATE;
        
        commission = (_registrationFee * commissionRate) / FEE_DENOMINATOR;

        // Create referral record
        bytes32 referralId = keccak256(abi.encodePacked(
            _business,
            referrer,
            block.timestamp,
            block.number
        ));

        ReferralRecord storage record = referralRecords[referralId];
        record.id = referralId;
        record.referrer = referrer;
        record.business = _business;
        record.timestamp = uint64(block.timestamp);
        record.registrationFee = _registrationFee;
        record.discount = discount;
        record.commission = commission;
        record.commissionPaid = false;

        // Update referrer stats
        referrerData.totalReferrals++;
        referrerData.claimableCommissions += commission;

        // Update mappings
        referrerToRecords[referrer].push(referralId);
        businessToReferralRecord[_business] = referralId;

        totalCommissionsPaid += commission;

        emit ReferralUsed(
            referralId,
            referrer,
            _referralCode,
            discount,
            commission
        );
    }

    /**
     * @dev Claim available commissions (called by referrer)
     */
    function claimCommissions() 
        external 
        nonReentrant 
        whenNotPaused 
    {
        Referrer storage referrerData = referrers[msg.sender];
        require(referrerData.isActive, "Not an active referrer");
        require(referrerData.claimableCommissions > 0, "No commissions to claim");

        uint256 claimAmount = referrerData.claimableCommissions;
        
        // Update referrer data
        referrerData.claimableCommissions = 0;
        referrerData.totalCommissions += claimAmount;
        referrerData.lastClaimedAt = block.timestamp;

        // Transfer commission
        usdcToken.safeTransfer(msg.sender, claimAmount);

        emit CommissionClaimed(msg.sender, claimAmount, referrerData.claimableCommissions);
    }

    /**
     * @dev Mark commission as earned (called by PayvergePayments)
     * @param _business Business address that was referred
     */
    function markCommissionEarned(address _business) 
        external 
        onlyPayvergePayments 
        whenNotPaused 
    {
        bytes32 referralId = businessToReferralRecord[_business];
        require(referralId != bytes32(0), "No referral found");

        ReferralRecord storage record = referralRecords[referralId];
        require(!record.commissionPaid, "Commission already earned");

        address referrer = record.referrer;
        uint256 commission = record.commission;

        // Mark as earned
        record.commissionPaid = true;

        Referrer storage referrerData = referrers[referrer];
        emit CommissionEarned(referrer, commission, referrerData.claimableCommissions);
    }

    /**
     * @dev Update referral code (once per month limit)
     * @param _newReferralCode New referral code
     */
    function updateReferralCode(string calldata _newReferralCode) 
        external 
        whenNotPaused 
    {
        Referrer storage referrer = referrers[msg.sender];
        if (referrer.tier == ReferrerTier.None) revert ReferrerNotFound();

        _validateReferralCode(_newReferralCode);
        
        if (referralCodeToAddress[_newReferralCode] != address(0)) {
            revert ReferralCodeTaken();
        }

        string memory oldCode = referrer.referralCode;
        
        // Update mappings
        delete referralCodeToAddress[oldCode];
        referralCodeToAddress[_newReferralCode] = msg.sender;
        referrer.referralCode = _newReferralCode;

        emit ReferralCodeUpdated(msg.sender, oldCode, _newReferralCode);
    }

    /**
     * @dev Set the PayvergePayments contract address (admin only)
     * @param _payvergePayments PayvergePayments contract address
     */
    function setPayvergePaymentsContract(address _payvergePayments) 
        external 
        onlyRole(ADMIN_ROLE) 
    {
        require(_payvergePayments != address(0), "Invalid contract address");
        payvergePaymentsContract = _payvergePayments;
    }

    /**
     * @dev Get referrer information
     * @param _referrer Referrer address
     * @return Referrer struct data
     */
    function getReferrer(address _referrer) 
        external 
        view 
        returns (Referrer memory) 
    {
        return referrers[_referrer];
    }

    /**
     * @dev Get referral records for a referrer
     * @param _referrer Referrer address
     * @return Array of referral record IDs
     */
    function getReferralRecords(address _referrer) 
        external 
        view 
        returns (bytes32[] memory) 
    {
        return referrerToRecords[_referrer];
    }

    /**
     * @dev Check if referral code is available
     * @param _referralCode Code to check
     * @return True if available
     */
    function isReferralCodeAvailable(string calldata _referralCode) 
        external 
        view 
        returns (bool) 
    {
        return referralCodeToAddress[_referralCode] == address(0) && 
               _isValidReferralCode(_referralCode);
    }

    /**
     * @dev Get referrer by referral code
     * @param _referralCode Referral code
     * @return Referrer address
     */
    function getReferrerByCode(string calldata _referralCode) 
        external 
        view 
        returns (address) 
    {
        return referralCodeToAddress[_referralCode];
    }

    // Internal functions

    /**
     * @dev Internal function to register a referrer
     */
    function _registerReferrer(
        string calldata _referralCode,
        ReferrerTier _tier,
        uint256 _fee
    ) internal {
        if (referrers[msg.sender].tier != ReferrerTier.None) {
            revert ReferrerAlreadyExists();
        }

        _validateReferralCode(_referralCode);
        
        if (referralCodeToAddress[_referralCode] != address(0)) {
            revert ReferralCodeTaken();
        }

        // Transfer registration fee
        usdcToken.safeTransferFrom(msg.sender, platformTreasury, _fee);

        // Create referrer
        Referrer storage referrer = referrers[msg.sender];
        referrer.referrerAddress = msg.sender;
        referrer.tier = _tier;
        referrer.isActive = true;
        referrer.registrationDate = uint64(block.timestamp);
        referrer.referralCode = _referralCode;

        // Update mappings
        referralCodeToAddress[_referralCode] = msg.sender;
        totalReferrers++;

        emit ReferrerRegistered(msg.sender, _tier, _referralCode, _fee);
    }

    /**
     * @dev Validate referral code format
     */
    function _validateReferralCode(string calldata _referralCode) internal pure {
        if (!_isValidReferralCode(_referralCode)) revert InvalidReferralCode();
    }

    /**
     * @dev Check if referral code is valid format
     */
    function _isValidReferralCode(string calldata _referralCode) 
        internal 
        pure 
        returns (bool) 
    {
        bytes memory codeBytes = bytes(_referralCode);
        uint256 length = codeBytes.length;
        
        if (length < MIN_REFERRAL_CODE_LENGTH || length > MAX_REFERRAL_CODE_LENGTH) {
            return false;
        }

        // Check alphanumeric characters only
        for (uint256 i = 0; i < length; i++) {
            bytes1 char = codeBytes[i];
            if (!(char >= 0x30 && char <= 0x39) && // 0-9
                !(char >= 0x41 && char <= 0x5A) && // A-Z
                !(char >= 0x61 && char <= 0x7A)) { // a-z
                return false;
            }
        }

        return true;
    }

    // Admin functions

    /**
     * @dev Pause the contract (admin only)
     */
    function pause() external onlyRole(ADMIN_ROLE) {
        _pause();
    }

    /**
     * @dev Unpause the contract (admin only)
     */
    function unpause() external onlyRole(ADMIN_ROLE) {
        _unpause();
    }

    /**
     * @dev Deactivate a referrer (admin only)
     * @param _referrer Referrer address to deactivate
     */
    function deactivateReferrer(address _referrer) 
        external 
        onlyRole(ADMIN_ROLE) 
    {
        referrers[_referrer].isActive = false;
    }

    /**
     * @dev Emergency withdrawal (admin only)
     * @param _token Token address (use address(0) for ETH)
     * @param _amount Amount to withdraw
     */
    function emergencyWithdraw(address _token, uint256 _amount) 
        external 
        onlyRole(ADMIN_ROLE) 
    {
        if (_token == address(0)) {
            payable(platformTreasury).transfer(_amount);
        } else {
            IERC20(_token).safeTransfer(platformTreasury, _amount);
        }
    }

    /**
     * @dev Authorize upgrade (upgrader role only)
     */
    function _authorizeUpgrade(address newImplementation) 
        internal 
        override 
        onlyRole(UPGRADER_ROLE) 
    {}

    /**
     * @dev Get contract version
     */
    function version() external pure returns (string memory) {
        return "1.0.0";
    }
}
