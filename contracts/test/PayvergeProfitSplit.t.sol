// SPDX-License-Identifier: MIT
pragma solidity ^0.8.19;

import "forge-std/Test.sol";
import "../src/PayvergeProfitSplit.sol";
import "@openzeppelin/contracts/token/ERC20/ERC20.sol";
import "@openzeppelin/contracts/proxy/ERC1967/ERC1967Proxy.sol";

contract MockUSDC is ERC20 {
    constructor() ERC20("Mock USDC", "USDC") {
        _mint(msg.sender, 1000000 * 10 ** 6); // 1M USDC
    }

    function mint(address to, uint256 amount) external {
        _mint(to, amount);
    }

    function decimals() public pure override returns (uint8) {
        return 6;
    }
}

contract PayvergeProfitSplitTest is Test {
    PayvergeProfitSplit public profitSplit;
    MockUSDC public usdc;

    address public admin = makeAddr("admin");
    address public treasury = makeAddr("treasury");
    address public distributor = makeAddr("distributor");
    address public partner1 = makeAddr("partner1");
    address public partner2 = makeAddr("partner2");
    address public partner3 = makeAddr("partner3");

    uint256 public constant DISTRIBUTION_AMOUNT = 1000 * 10 ** 6; // $1000 USDC

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

    function setUp() public {
        // Deploy USDC mock
        usdc = new MockUSDC();

        // Deploy profit split contract with proxy
        PayvergeProfitSplit implementation = new PayvergeProfitSplit();
        bytes memory initData =
            abi.encodeWithSelector(PayvergeProfitSplit.initialize.selector, address(usdc), treasury, admin);

        ERC1967Proxy proxy = new ERC1967Proxy(address(implementation), initData);
        profitSplit = PayvergeProfitSplit(payable(address(proxy)));

        // Grant distributor role
        vm.prank(admin);
        profitSplit.grantDistributorRole(distributor);

        // Distribute USDC to test accounts
        usdc.mint(address(profitSplit), 10000 * 10 ** 6); // Fund contract
        usdc.mint(distributor, 1000 * 10 ** 6);
    }

    function testAddBeneficiary() public {
        string memory name = "Partner 1";
        uint256 percentage = 3000; // 30%

        vm.expectEmit(true, false, false, true);
        emit BeneficiaryAdded(partner1, name, percentage, admin);

        vm.prank(admin);
        profitSplit.addBeneficiary(partner1, name, percentage);

        PayvergeProfitSplit.Beneficiary memory beneficiary = profitSplit.getBeneficiary(partner1);
        assertEq(beneficiary.beneficiaryAddress, partner1);
        assertEq(beneficiary.percentage, percentage);
        assertEq(beneficiary.name, name);
        assertTrue(beneficiary.isActive);
        assertEq(beneficiary.totalReceived, 0);

        // Check global stats
        assertEq(profitSplit.beneficiaryCount(), 1);
        assertEq(profitSplit.totalPercentageAllocated(), percentage);
    }

    function testAddMultipleBeneficiaries() public {
        // Add three beneficiaries with different percentages
        vm.startPrank(admin);
        profitSplit.addBeneficiary(partner1, "Partner 1", 4000); // 40%
        profitSplit.addBeneficiary(partner2, "Partner 2", 3500); // 35%
        profitSplit.addBeneficiary(partner3, "Partner 3", 2500); // 25%
        vm.stopPrank();

        // Check total allocation
        assertEq(profitSplit.totalPercentageAllocated(), 10000); // 100%
        assertEq(profitSplit.beneficiaryCount(), 3);

        // Verify each beneficiary
        PayvergeProfitSplit.Beneficiary memory ben1 = profitSplit.getBeneficiary(partner1);
        PayvergeProfitSplit.Beneficiary memory ben2 = profitSplit.getBeneficiary(partner2);
        PayvergeProfitSplit.Beneficiary memory ben3 = profitSplit.getBeneficiary(partner3);

        assertEq(ben1.percentage, 4000);
        assertEq(ben2.percentage, 3500);
        assertEq(ben3.percentage, 2500);
    }

    function testCannotExceed100Percent() public {
        // Add beneficiary with 60%
        vm.prank(admin);
        profitSplit.addBeneficiary(partner1, "Partner 1", 6000);

        // Try to add another with 50% (would exceed 100%)
        vm.prank(admin);
        vm.expectRevert(PayvergeProfitSplit.TotalPercentageExceeded.selector);
        profitSplit.addBeneficiary(partner2, "Partner 2", 5000);
    }

    function testUpdateBeneficiaryPercentage() public {
        // Add beneficiary
        vm.prank(admin);
        profitSplit.addBeneficiary(partner1, "Partner 1", 3000);

        uint256 newPercentage = 5000;

        vm.expectEmit(true, false, false, true);
        emit BeneficiaryUpdated(partner1, 3000, newPercentage, admin);

        vm.prank(admin);
        profitSplit.updateBeneficiaryPercentage(partner1, newPercentage);

        PayvergeProfitSplit.Beneficiary memory beneficiary = profitSplit.getBeneficiary(partner1);
        assertEq(beneficiary.percentage, newPercentage);
        assertEq(profitSplit.totalPercentageAllocated(), newPercentage);
    }

    function testRemoveBeneficiary() public {
        // Add beneficiary
        vm.prank(admin);
        profitSplit.addBeneficiary(partner1, "Partner 1", 3000);

        vm.expectEmit(true, false, false, true);
        emit BeneficiaryRemoved(partner1, 3000, admin);

        vm.prank(admin);
        profitSplit.removeBeneficiary(partner1);

        PayvergeProfitSplit.Beneficiary memory beneficiary = profitSplit.getBeneficiary(partner1);
        assertFalse(beneficiary.isActive);
        assertEq(profitSplit.totalPercentageAllocated(), 0);
        assertEq(profitSplit.beneficiaryCount(), 0);
    }

    function testDistributeProfits() public {
        // Add beneficiaries
        vm.startPrank(admin);
        profitSplit.addBeneficiary(partner1, "Partner 1", 4000); // 40%
        profitSplit.addBeneficiary(partner2, "Partner 2", 6000); // 60%
        vm.stopPrank();

        uint256 distributionAmount = 1000 * 10 ** 6; // $1000
        uint256 expectedPayout1 = (distributionAmount * 4000) / 10000; // $400
        uint256 expectedPayout2 = (distributionAmount * 6000) / 10000; // $600

        uint256 balance1Before = usdc.balanceOf(partner1);
        uint256 balance2Before = usdc.balanceOf(partner2);

        vm.expectEmit(false, false, false, true);
        emit ProfitDistributed(bytes32(0), distributionAmount, 2, distributor);

        vm.prank(distributor);
        profitSplit.distributeProfits(distributionAmount);

        // Check balances
        assertEq(usdc.balanceOf(partner1), balance1Before + expectedPayout1);
        assertEq(usdc.balanceOf(partner2), balance2Before + expectedPayout2);

        // Check beneficiary stats
        PayvergeProfitSplit.Beneficiary memory ben1 = profitSplit.getBeneficiary(partner1);
        PayvergeProfitSplit.Beneficiary memory ben2 = profitSplit.getBeneficiary(partner2);

        assertEq(ben1.totalReceived, expectedPayout1);
        assertEq(ben1.lastReceived, expectedPayout1);
        assertEq(ben2.totalReceived, expectedPayout2);
        assertEq(ben2.lastReceived, expectedPayout2);

        // Check global stats
        assertEq(profitSplit.totalDistributed(), distributionAmount);
        assertEq(profitSplit.distributionCount(), 1);
    }

    function testDistributeAllProfits() public {
        // Add beneficiaries
        vm.startPrank(admin);
        profitSplit.addBeneficiary(partner1, "Partner 1", 5000); // 50%
        profitSplit.addBeneficiary(partner2, "Partner 2", 5000); // 50%
        vm.stopPrank();

        uint256 contractBalance = usdc.balanceOf(address(profitSplit));
        uint256 expectedPayout = contractBalance / 2; // 50% each

        uint256 balance1Before = usdc.balanceOf(partner1);
        uint256 balance2Before = usdc.balanceOf(partner2);

        vm.prank(distributor);
        profitSplit.distributeAllProfits();

        // Check balances
        assertEq(usdc.balanceOf(partner1), balance1Before + expectedPayout);
        assertEq(usdc.balanceOf(partner2), balance2Before + expectedPayout);
        assertEq(usdc.balanceOf(address(profitSplit)), 0);
    }

    function testGetActiveBeneficiaries() public {
        // Add beneficiaries
        vm.startPrank(admin);
        profitSplit.addBeneficiary(partner1, "Partner 1", 3000);
        profitSplit.addBeneficiary(partner2, "Partner 2", 4000);
        profitSplit.addBeneficiary(partner3, "Partner 3", 3000);
        vm.stopPrank();

        address[] memory activeBeneficiaries = profitSplit.getActiveBeneficiaries();
        assertEq(activeBeneficiaries.length, 3);
        assertEq(activeBeneficiaries[0], partner1);
        assertEq(activeBeneficiaries[1], partner2);
        assertEq(activeBeneficiaries[2], partner3);

        // Remove one beneficiary
        vm.prank(admin);
        profitSplit.removeBeneficiary(partner2);

        activeBeneficiaries = profitSplit.getActiveBeneficiaries();
        assertEq(activeBeneficiaries.length, 2);
    }

    function testCalculatePayouts() public {
        // Add beneficiaries
        vm.startPrank(admin);
        profitSplit.addBeneficiary(partner1, "Partner 1", 2500); // 25%
        profitSplit.addBeneficiary(partner2, "Partner 2", 7500); // 75%
        vm.stopPrank();

        uint256 testAmount = 2000 * 10 ** 6; // $2000
        (address[] memory beneficiaryAddrs, uint256[] memory payouts) = profitSplit.calculatePayouts(testAmount);

        assertEq(beneficiaryAddrs.length, 2);
        assertEq(payouts.length, 2);

        assertEq(beneficiaryAddrs[0], partner1);
        assertEq(beneficiaryAddrs[1], partner2);
        assertEq(payouts[0], 500 * 10 ** 6); // 25% of $2000 = $500
        assertEq(payouts[1], 1500 * 10 ** 6); // 75% of $2000 = $1500
    }

    function testGetDistributionStats() public {
        // Add beneficiary and distribute
        vm.prank(admin);
        profitSplit.addBeneficiary(partner1, "Partner 1", 10000); // 100%

        vm.prank(distributor);
        profitSplit.distributeProfits(1000 * 10 ** 6);

        (uint256 balance, uint256 totalDist, uint256 distCount, uint256 lastDist) = profitSplit.getDistributionStats();

        assertTrue(balance < 10000 * 10 ** 6); // Should be less than original
        assertEq(totalDist, 1000 * 10 ** 6);
        assertEq(distCount, 1);
        assertTrue(lastDist > 0);
    }

    function testDepositForDistribution() public {
        uint256 depositAmount = 500 * 10 ** 6;
        uint256 balanceBefore = usdc.balanceOf(address(profitSplit));

        vm.prank(distributor);
        usdc.approve(address(profitSplit), depositAmount);

        vm.prank(distributor);
        profitSplit.depositForDistribution(depositAmount);

        assertEq(usdc.balanceOf(address(profitSplit)), balanceBefore + depositAmount);
        assertEq(usdc.balanceOf(distributor), 1000 * 10 ** 6 - depositAmount);
    }

    function testAccessControl() public {
        // Non-admin cannot add beneficiaries
        vm.prank(partner1);
        vm.expectRevert();
        profitSplit.addBeneficiary(partner2, "Partner 2", 5000);

        // Non-distributor cannot distribute profits
        vm.prank(partner1);
        vm.expectRevert();
        profitSplit.distributeProfits(1000 * 10 ** 6);

        // Admin can grant/revoke distributor role
        vm.prank(admin);
        profitSplit.grantDistributorRole(partner1);

        vm.prank(admin);
        profitSplit.revokeDistributorRole(partner1);
    }

    function testPauseUnpause() public {
        // Admin can pause
        vm.prank(admin);
        profitSplit.pause();

        // Operations should fail when paused
        vm.prank(admin);
        vm.expectRevert(abi.encodeWithSignature("EnforcedPause()"));
        profitSplit.addBeneficiary(partner1, "Partner 1", 5000);

        // Admin can unpause
        vm.prank(admin);
        profitSplit.unpause();

        // Operations should work after unpause
        vm.prank(admin);
        profitSplit.addBeneficiary(partner1, "Partner 1", 5000);
    }

    function testEmergencyWithdraw() public {
        uint256 withdrawAmount = 1000 * 10 ** 6;
        uint256 treasuryBalanceBefore = usdc.balanceOf(treasury);

        vm.prank(admin);
        profitSplit.emergencyWithdraw(address(usdc), withdrawAmount, treasury);

        assertEq(usdc.balanceOf(treasury), treasuryBalanceBefore + withdrawAmount);
    }

    function testInvalidInputs() public {
        // Invalid beneficiary address
        vm.prank(admin);
        vm.expectRevert(PayvergeProfitSplit.InvalidBeneficiaryAddress.selector);
        profitSplit.addBeneficiary(address(0), "Invalid", 5000);

        // Empty name
        vm.prank(admin);
        vm.expectRevert(PayvergeProfitSplit.EmptyBeneficiaryName.selector);
        profitSplit.addBeneficiary(partner1, "", 5000);

        // Invalid percentage (0%)
        vm.prank(admin);
        vm.expectRevert(PayvergeProfitSplit.InvalidPercentage.selector);
        profitSplit.addBeneficiary(partner1, "Partner 1", 0);

        // Invalid percentage (>100%)
        vm.prank(admin);
        vm.expectRevert(PayvergeProfitSplit.InvalidPercentage.selector);
        profitSplit.addBeneficiary(partner1, "Partner 1", 10001);
    }

    function testDistributionWithInactiveBeneficiary() public {
        // Add beneficiaries
        vm.startPrank(admin);
        profitSplit.addBeneficiary(partner1, "Partner 1", 5000); // 50%
        profitSplit.addBeneficiary(partner2, "Partner 2", 5000); // 50%
        vm.stopPrank();

        // Remove one beneficiary
        vm.prank(admin);
        profitSplit.removeBeneficiary(partner2);

        // Add new beneficiary with same percentage
        vm.prank(admin);
        profitSplit.addBeneficiary(partner3, "Partner 3", 5000); // 50%

        uint256 distributionAmount = 1000 * 10 ** 6;
        uint256 expectedPayout = distributionAmount / 2; // 50% each

        uint256 balance1Before = usdc.balanceOf(partner1);
        uint256 balance3Before = usdc.balanceOf(partner3);

        vm.prank(distributor);
        profitSplit.distributeProfits(distributionAmount);

        // Only active beneficiaries should receive payouts
        assertEq(usdc.balanceOf(partner1), balance1Before + expectedPayout);
        assertEq(usdc.balanceOf(partner2), 0); // Inactive, should receive nothing
        assertEq(usdc.balanceOf(partner3), balance3Before + expectedPayout);
    }

    function testMaxBeneficiaries() public {
        // Add maximum number of beneficiaries (50)
        vm.startPrank(admin);
        for (uint256 i = 0; i < 50; i++) {
            address beneficiary = address(uint160(1000 + i));
            profitSplit.addBeneficiary(beneficiary, string(abi.encodePacked("Partner ", i)), 200); // 2% each
        }
        vm.stopPrank();

        // Try to add one more (should fail)
        vm.prank(admin);
        vm.expectRevert(PayvergeProfitSplit.MaxBeneficiariesReached.selector);
        profitSplit.addBeneficiary(partner1, "Extra Partner", 100);
    }

    function testVersion() public view {
        assertEq(profitSplit.version(), "1.0.0");
    }

    // Fuzz testing
    function testFuzzDistribution(uint256 amount, uint16 percentage1, uint16 percentage2) public {
        vm.assume(amount >= 1 * 10 ** 6 && amount <= 1000000 * 10 ** 6); // $1 to $1M
        vm.assume(percentage1 > 0 && percentage1 <= 5000); // 0.01% to 50%
        vm.assume(percentage2 > 0 && percentage2 <= 5000); // 0.01% to 50%
        vm.assume(uint256(percentage1) + uint256(percentage2) <= 10000); // Total <= 100%

        // Ensure contract has enough balance
        if (usdc.balanceOf(address(profitSplit)) < amount) {
            usdc.mint(address(profitSplit), amount);
        }

        // Add beneficiaries
        vm.startPrank(admin);
        profitSplit.addBeneficiary(partner1, "Partner 1", percentage1);
        profitSplit.addBeneficiary(partner2, "Partner 2", percentage2);
        vm.stopPrank();

        uint256 expectedPayout1 = (amount * percentage1) / 10000;
        uint256 expectedPayout2 = (amount * percentage2) / 10000;

        uint256 balance1Before = usdc.balanceOf(partner1);
        uint256 balance2Before = usdc.balanceOf(partner2);

        vm.prank(distributor);
        profitSplit.distributeProfits(amount);

        // Verify payouts
        assertEq(usdc.balanceOf(partner1), balance1Before + expectedPayout1);
        assertEq(usdc.balanceOf(partner2), balance2Before + expectedPayout2);

        // Verify beneficiary stats
        PayvergeProfitSplit.Beneficiary memory ben1 = profitSplit.getBeneficiary(partner1);
        PayvergeProfitSplit.Beneficiary memory ben2 = profitSplit.getBeneficiary(partner2);

        assertEq(ben1.totalReceived, expectedPayout1);
        assertEq(ben2.totalReceived, expectedPayout2);
    }
}
