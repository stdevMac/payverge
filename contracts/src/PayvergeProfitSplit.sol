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
 * @title PayvergeProfitSplit
 * @dev Manages profit distribution among stakeholders with flexible percentage allocation
 * @notice Allows admin to set custom profit sharing percentages for partners, team members, and stakeholders
 */
contract PayvergeProfitSplit is
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
    bytes32 public constant DISTRIBUTOR_ROLE = keccak256("DISTRIBUTOR_ROLE");

    // Constants
    uint256 public constant MAX_PERCENTAGE = 10000; // 100% = 10000 basis points
    uint256 public constant MAX_BENEFICIARIES = 50; // Maximum number of beneficiaries
    uint256 public constant MIN_DISTRIBUTION_AMOUNT = 1 * 10 ** 6; // $1 USDC minimum

    // State variables
    IERC20 public usdcToken;
    uint256 public totalDistributed;
    uint256 public distributionCount;
    uint256 public lastDistributionTime;

    // Beneficiary information
    struct Beneficiary {
        address beneficiaryAddress; // 20 bytes
        uint16 percentage; // 2 bytes - percentage in basis points (0-10000)
        bool isActive; // 1 byte
        uint64 addedAt; // 8 bytes
        uint256 totalReceived; // 32 bytes - total received across all distributions
        uint256 lastReceived; // 32 bytes - amount received in last distribution
        string name; // Dynamic - beneficiary name/description
    }

    // Distribution record
    struct Distribution {
        bytes32 id; // 32 bytes
        uint64 timestamp; // 8 bytes
        uint256 totalAmount; // 32 bytes
        uint256 beneficiaryCount; // 32 bytes
        address triggeredBy; // 20 bytes
    }

    // Mappings
    mapping(address => Beneficiary) public beneficiaries;
    mapping(uint256 => address) public beneficiaryIndex; // For iteration
    mapping(bytes32 => Distribution) public distributions;
    mapping(bytes32 => mapping(address => uint256)) public distributionPayouts; // distribution => beneficiary => amount

    uint256 public beneficiaryCount;
    uint256 public totalPercentageAllocated;

    // Events
    event BeneficiaryAdded(address indexed beneficiary, string name, uint256 percentage, address indexed addedBy);

    event BeneficiaryUpdated(
        address indexed beneficiary, uint256 oldPercentage, uint256 newPercentage, address indexed updatedBy
    );

    event BeneficiaryRemoved(address indexed beneficiary, uint256 percentage, address indexed removedBy);

    event ProfitDistributed(
        bytes32 indexed distributionId, uint256 totalAmount, uint256 beneficiaryCount, address indexed triggeredBy
    );

    event BeneficiaryPayout(
        bytes32 indexed distributionId, address indexed beneficiary, uint256 amount, uint256 percentage
    );

    event EmergencyWithdrawal(address indexed token, uint256 amount, address indexed to, address indexed triggeredBy);

    // Custom errors
    error InvalidPercentage();
    error BeneficiaryNotFound();
    error BeneficiaryAlreadyExists();
    error MaxBeneficiariesReached();
    error TotalPercentageExceeded();
    error InsufficientBalance();
    error InvalidDistributionAmount();
    error NoActiveBeneficiaries();
    error InvalidBeneficiaryAddress();
    error EmptyBeneficiaryName();

    /// @custom:oz-upgrades-unsafe-allow constructor
    constructor() {
        _disableInitializers();
    }

    /**
     * @dev Initialize the contract
     * @param _usdcToken USDC token contract address
     * @param _admin Admin address
     */
    function initialize(address _usdcToken, address _admin) public initializer {
        require(_usdcToken != address(0), "Invalid USDC token");
        require(_admin != address(0), "Invalid admin");

        __ReentrancyGuard_init();
        __Pausable_init();
        __AccessControl_init();
        __UUPSUpgradeable_init();

        usdcToken = IERC20(_usdcToken);

        _grantRole(DEFAULT_ADMIN_ROLE, _admin);
        _grantRole(ADMIN_ROLE, _admin);
        _grantRole(UPGRADER_ROLE, _admin);
        _grantRole(DISTRIBUTOR_ROLE, _admin);
    }

    /**
     * @dev Add a new beneficiary with profit sharing percentage
     * @param _beneficiary Beneficiary address
     * @param _name Beneficiary name/description
     * @param _percentage Percentage in basis points (100 = 1%, 10000 = 100%)
     */
    function addBeneficiary(address _beneficiary, string calldata _name, uint256 _percentage)
        external
        onlyRole(ADMIN_ROLE)
        whenNotPaused
    {
        if (_beneficiary == address(0)) revert InvalidBeneficiaryAddress();
        if (bytes(_name).length == 0) revert EmptyBeneficiaryName();
        if (_percentage == 0 || _percentage > MAX_PERCENTAGE) revert InvalidPercentage();
        if (beneficiaries[_beneficiary].isActive) revert BeneficiaryAlreadyExists();
        if (beneficiaryCount >= MAX_BENEFICIARIES) revert MaxBeneficiariesReached();
        if (totalPercentageAllocated + _percentage > MAX_PERCENTAGE) revert TotalPercentageExceeded();

        // Add beneficiary
        beneficiaries[_beneficiary] = Beneficiary({
            beneficiaryAddress: _beneficiary,
            percentage: uint16(_percentage),
            isActive: true,
            addedAt: uint64(block.timestamp),
            totalReceived: 0,
            lastReceived: 0,
            name: _name
        });

        // Update indexes
        beneficiaryIndex[beneficiaryCount] = _beneficiary;
        beneficiaryCount++;
        totalPercentageAllocated += _percentage;

        emit BeneficiaryAdded(_beneficiary, _name, _percentage, msg.sender);
    }

    /**
     * @dev Update beneficiary percentage
     * @param _beneficiary Beneficiary address
     * @param _newPercentage New percentage in basis points
     */
    function updateBeneficiaryPercentage(address _beneficiary, uint256 _newPercentage)
        external
        onlyRole(ADMIN_ROLE)
        whenNotPaused
    {
        if (!beneficiaries[_beneficiary].isActive) revert BeneficiaryNotFound();
        if (_newPercentage == 0 || _newPercentage > MAX_PERCENTAGE) revert InvalidPercentage();

        Beneficiary storage beneficiary = beneficiaries[_beneficiary];
        uint256 oldPercentage = beneficiary.percentage;

        // Check if new total percentage would exceed 100%
        uint256 newTotalPercentage = totalPercentageAllocated - oldPercentage + _newPercentage;
        if (newTotalPercentage > MAX_PERCENTAGE) revert TotalPercentageExceeded();

        // Update percentage
        beneficiary.percentage = uint16(_newPercentage);
        totalPercentageAllocated = newTotalPercentage;

        emit BeneficiaryUpdated(_beneficiary, oldPercentage, _newPercentage, msg.sender);
    }

    /**
     * @dev Remove a beneficiary from profit sharing
     * @param _beneficiary Beneficiary address to remove
     */
    function removeBeneficiary(address _beneficiary) external onlyRole(ADMIN_ROLE) whenNotPaused {
        if (!beneficiaries[_beneficiary].isActive) revert BeneficiaryNotFound();

        Beneficiary storage beneficiary = beneficiaries[_beneficiary];
        uint256 percentage = beneficiary.percentage;

        // Mark as inactive
        beneficiary.isActive = false;
        totalPercentageAllocated -= percentage;

        // Remove from index (swap with last element)
        for (uint256 i = 0; i < beneficiaryCount; i++) {
            if (beneficiaryIndex[i] == _beneficiary) {
                beneficiaryIndex[i] = beneficiaryIndex[beneficiaryCount - 1];
                delete beneficiaryIndex[beneficiaryCount - 1];
                break;
            }
        }
        beneficiaryCount--;

        emit BeneficiaryRemoved(_beneficiary, percentage, msg.sender);
    }

    /**
     * @dev Distribute profits to all active beneficiaries
     * @param _amount Amount to distribute (must be available in contract balance)
     */
    function distributeProfits(uint256 _amount) external onlyRole(DISTRIBUTOR_ROLE) nonReentrant whenNotPaused {
        _distributeProfits(_amount);
    }

    /**
     * @dev Internal function to distribute profits
     * @param _amount Amount to distribute
     */
    function _distributeProfits(uint256 _amount) internal {
        if (_amount < MIN_DISTRIBUTION_AMOUNT) revert InvalidDistributionAmount();
        if (beneficiaryCount == 0) revert NoActiveBeneficiaries();
        if (usdcToken.balanceOf(address(this)) < _amount) revert InsufficientBalance();

        // Create distribution record
        bytes32 distributionId = keccak256(abi.encodePacked(block.timestamp, block.number, _amount, msg.sender));

        distributions[distributionId] = Distribution({
            id: distributionId,
            timestamp: uint64(block.timestamp),
            totalAmount: _amount,
            beneficiaryCount: beneficiaryCount,
            triggeredBy: msg.sender
        });

        uint256 totalDistributedAmount = 0;

        // Distribute to each active beneficiary
        for (uint256 i = 0; i < beneficiaryCount; i++) {
            address beneficiaryAddr = beneficiaryIndex[i];
            Beneficiary storage beneficiary = beneficiaries[beneficiaryAddr];

            if (beneficiary.isActive) {
                uint256 payout = (_amount * beneficiary.percentage) / MAX_PERCENTAGE;

                if (payout > 0) {
                    // Update beneficiary stats
                    beneficiary.totalReceived += payout;
                    beneficiary.lastReceived = payout;

                    // Record payout
                    distributionPayouts[distributionId][beneficiaryAddr] = payout;
                    totalDistributedAmount += payout;

                    // Transfer tokens
                    usdcToken.safeTransfer(beneficiaryAddr, payout);

                    emit BeneficiaryPayout(distributionId, beneficiaryAddr, payout, beneficiary.percentage);
                }
            }
        }

        // Update global stats
        totalDistributed += totalDistributedAmount;
        distributionCount++;
        lastDistributionTime = block.timestamp;

        emit ProfitDistributed(distributionId, _amount, beneficiaryCount, msg.sender);
    }

    /**
     * @dev Distribute all available profits in the contract
     */
    function distributeAllProfits() external onlyRole(DISTRIBUTOR_ROLE) {
        uint256 balance = usdcToken.balanceOf(address(this));
        if (balance >= MIN_DISTRIBUTION_AMOUNT) {
            _distributeProfits(balance);
        }
    }

    /**
     * @dev Get beneficiary information
     * @param _beneficiary Beneficiary address
     * @return Beneficiary struct data
     */
    function getBeneficiary(address _beneficiary) external view returns (Beneficiary memory) {
        return beneficiaries[_beneficiary];
    }

    /**
     * @dev Get all active beneficiaries
     * @return Array of active beneficiary addresses
     */
    function getActiveBeneficiaries() external view returns (address[] memory) {
        address[] memory activeBeneficiaries = new address[](beneficiaryCount);
        uint256 activeCount = 0;

        for (uint256 i = 0; i < beneficiaryCount; i++) {
            address beneficiaryAddr = beneficiaryIndex[i];
            if (beneficiaries[beneficiaryAddr].isActive) {
                activeBeneficiaries[activeCount] = beneficiaryAddr;
                activeCount++;
            }
        }

        // Resize array to actual count
        assembly {
            mstore(activeBeneficiaries, activeCount)
        }

        return activeBeneficiaries;
    }

    /**
     * @dev Get distribution details
     * @param _distributionId Distribution ID
     * @return Distribution struct data
     */
    function getDistribution(bytes32 _distributionId) external view returns (Distribution memory) {
        return distributions[_distributionId];
    }

    /**
     * @dev Get payout amount for a specific beneficiary in a distribution
     * @param _distributionId Distribution ID
     * @param _beneficiary Beneficiary address
     * @return Payout amount
     */
    function getDistributionPayout(bytes32 _distributionId, address _beneficiary) external view returns (uint256) {
        return distributionPayouts[_distributionId][_beneficiary];
    }

    /**
     * @dev Get contract balance and distribution stats
     * @return balance Current USDC balance
     * @return totalDist Total amount distributed
     * @return distCount Number of distributions
     * @return lastDist Timestamp of last distribution
     */
    function getDistributionStats()
        external
        view
        returns (uint256 balance, uint256 totalDist, uint256 distCount, uint256 lastDist)
    {
        balance = usdcToken.balanceOf(address(this));
        totalDist = totalDistributed;
        distCount = distributionCount;
        lastDist = lastDistributionTime;
    }

    /**
     * @dev Calculate potential payouts for current beneficiaries
     * @param _amount Amount to potentially distribute
     * @return beneficiaryAddrs Array of beneficiary addresses
     * @return payouts Array of corresponding payout amounts
     */
    function calculatePayouts(uint256 _amount)
        external
        view
        returns (address[] memory beneficiaryAddrs, uint256[] memory payouts)
    {
        beneficiaryAddrs = new address[](beneficiaryCount);
        payouts = new uint256[](beneficiaryCount);

        for (uint256 i = 0; i < beneficiaryCount; i++) {
            address beneficiaryAddr = beneficiaryIndex[i];
            Beneficiary memory beneficiary = beneficiaries[beneficiaryAddr];

            beneficiaryAddrs[i] = beneficiaryAddr;
            if (beneficiary.isActive) {
                payouts[i] = (_amount * beneficiary.percentage) / MAX_PERCENTAGE;
            } else {
                payouts[i] = 0;
            }
        }
    }

    /**
     * @dev Deposit USDC to the contract for distribution
     * @param _amount Amount to deposit
     */
    function depositForDistribution(uint256 _amount) external nonReentrant whenNotPaused {
        require(_amount > 0, "Invalid amount");
        usdcToken.safeTransferFrom(msg.sender, address(this), _amount);
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
     * @dev Grant distributor role to an address
     * @param _distributor Address to grant distributor role
     */
    function grantDistributorRole(address _distributor) external onlyRole(ADMIN_ROLE) {
        _grantRole(DISTRIBUTOR_ROLE, _distributor);
    }

    /**
     * @dev Revoke distributor role from an address
     * @param _distributor Address to revoke distributor role
     */
    function revokeDistributorRole(address _distributor) external onlyRole(ADMIN_ROLE) {
        _revokeRole(DISTRIBUTOR_ROLE, _distributor);
    }

    /**
     * @dev Emergency withdrawal (admin only)
     * @param _token Token address (use address(0) for ETH)
     * @param _amount Amount to withdraw
     * @param _to Recipient address
     */
    function emergencyWithdraw(address _token, uint256 _amount, address _to) external onlyRole(ADMIN_ROLE) {
        require(_to != address(0), "Invalid recipient");

        if (_token == address(0)) {
            payable(_to).transfer(_amount);
        } else {
            IERC20(_token).safeTransfer(_to, _amount);
        }

        emit EmergencyWithdrawal(_token, _amount, _to, msg.sender);
    }

    /**
     * @dev Authorize upgrade (upgrader role only)
     */
    function _authorizeUpgrade(address newImplementation) internal override onlyRole(UPGRADER_ROLE) {}

    /**
     * @dev Get contract version
     */
    function version() external pure returns (string memory) {
        return "1.0.0";
    }

    /**
     * @dev Receive function to accept ETH deposits
     */
    receive() external payable {
        // Allow ETH deposits for gas or emergency purposes
    }
}
