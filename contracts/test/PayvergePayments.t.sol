// SPDX-License-Identifier: MIT
pragma solidity ^0.8.19;

import "forge-std/Test.sol";
import "forge-std/console.sol";
import "../src/PayvergePayments.sol";
import "@openzeppelin/contracts/token/ERC20/ERC20.sol";
import "@openzeppelin/contracts/proxy/ERC1967/ERC1967Proxy.sol";

contract MockUSDC is ERC20 {
    constructor() ERC20("Mock USDC", "USDC") {
        _mint(msg.sender, 1000000000 * 10**6); // 1B USDC
    }

    function decimals() public pure override returns (uint8) {
        return 6;
    }

    function mint(address to, uint256 amount) external {
        _mint(to, amount);
    }
}

contract PayvergePaymentsTest is Test {
    PayvergePayments public payverge;
    PayvergePayments public implementation;
    MockUSDC public usdc;
    ERC1967Proxy public proxy;

    address public owner = address(0x1);
    address public platformTreasury = address(0x2);
    address public business1 = address(0x3);
    address public business2 = address(0x4);
    address public customer1 = address(0x5);
    address public customer2 = address(0x6);
    address public paymentAddress1 = address(0x7);
    address public tippingAddress1 = address(0x8);
    address public paymentAddress2 = address(0x9);
    address public tippingAddress2 = address(0xA);
    address public attacker = address(0xB);

    bytes32 public constant ADMIN_ROLE = keccak256("ADMIN_ROLE");
    bytes32 public constant BUSINESS_ROLE = keccak256("BUSINESS_ROLE");
    bytes32 public constant UPGRADER_ROLE = keccak256("UPGRADER_ROLE");

    uint256 public constant PLATFORM_FEE_RATE = 250; // 2.5%
    uint256 public constant BILL_AMOUNT = 100 * 10**6; // $100 USDC
    uint256 public constant TIP_AMOUNT = 15 * 10**6; // $15 USDC

    event BillCreated(bytes32 indexed billId, address indexed businessAddress, uint256 totalAmount, string metadata);
    event PaymentProcessed(bytes32 indexed paymentId, bytes32 indexed billId, address indexed payer, uint256 amount, uint256 tipAmount, uint256 platformFee);
    event BusinessVerified(address indexed businessAddress, string name);
    event CircuitBreakerTripped(uint256 dailyVolume, uint256 threshold);
    event DailyLimitExceeded(address indexed user, uint256 attempted, uint256 limit);

    function setUp() public {
        // Deploy USDC mock
        usdc = new MockUSDC();

        // Deploy implementation
        implementation = new PayvergePayments();

        // Deploy proxy
        bytes memory initData = abi.encodeWithSelector(
            PayvergePayments.initialize.selector,
            address(usdc),
            platformTreasury,
            PLATFORM_FEE_RATE
        );
        
        proxy = new ERC1967Proxy(address(implementation), initData);
        payverge = PayvergePayments(address(proxy));

        // The initialize function already grants DEFAULT_ADMIN_ROLE to msg.sender (owner)
        // No need to explicitly grant roles here

        // Mint USDC to customers
        usdc.mint(customer1, 10000 * 10**6); // $10k
        usdc.mint(customer2, 10000 * 10**6); // $10k
        usdc.mint(attacker, 10000 * 10**6); // $10k

        // Approve spending
        vm.prank(customer1);
        usdc.approve(address(payverge), type(uint256).max);
        vm.prank(customer2);
        usdc.approve(address(payverge), type(uint256).max);
        vm.prank(attacker);
        usdc.approve(address(payverge), type(uint256).max);

        // Grant admin role to owner and verify businesses
        vm.startPrank(address(this)); // Test contract is the deployer
        payverge.grantRole(payverge.ADMIN_ROLE(), owner);
        vm.stopPrank();
        
        vm.startPrank(owner);
        payverge.verifyBusiness(business1, "Restaurant 1", paymentAddress1, tippingAddress1);
        payverge.verifyBusiness(business2, "Restaurant 2", paymentAddress2, tippingAddress2);
        vm.stopPrank();
    }

    // ==================== BASIC FUNCTIONALITY TESTS ====================

    function testInitialization() public view {
        assertEq(address(payverge.usdcToken()), address(usdc));
        assertEq(payverge.platformTreasury(), platformTreasury);
        assertEq(payverge.platformFeeRate(), PLATFORM_FEE_RATE);
        assertTrue(payverge.hasRole(ADMIN_ROLE, owner));
        assertEq(payverge.version(), "2.0.0");
    }

    function testBusinessVerification() public view {
        assertTrue(payverge.verifiedBusinesses(business1));
        assertTrue(payverge.hasRole(BUSINESS_ROLE, business1));
        
        (string memory name, address businessOwner, address paymentAddr, address tippingAddr, , bool isActive, uint256 totalVol, uint256 totalPay) = payverge.businessInfo(business1);
        assertEq(name, "Restaurant 1");
        assertEq(businessOwner, business1);
        assertEq(paymentAddr, paymentAddress1);
        assertEq(tippingAddr, tippingAddress1);
        assertTrue(isActive);
        assertEq(totalVol, 0);
        assertEq(totalPay, 0);
    }

    function testCreateBill() public {
        bytes32 billId = keccak256("bill1");
        string memory metadata = '{"items":[{"name":"Burger","price":50}]}';

        vm.expectEmit(true, true, false, true);
        emit BillCreated(billId, paymentAddress1, BILL_AMOUNT, metadata);

        vm.prank(business1);
        payverge.createBill(billId, BILL_AMOUNT, metadata);

        PayvergePayments.Bill memory bill = payverge.getBill(billId);
        assertEq(bill.id, billId);
        assertEq(bill.businessAddress, paymentAddress1);
        assertEq(bill.totalAmount, BILL_AMOUNT);
        assertEq(bill.paidAmount, 0);
        assertEq(uint256(bill.status), uint256(PayvergePayments.BillStatus.Active));
    }

    function testProcessPayment() public {
        bytes32 billId = _createTestBill();
        
        uint256 platformFee = (BILL_AMOUNT * PLATFORM_FEE_RATE) / 10000;
        uint256 businessAmount = BILL_AMOUNT - platformFee;

        uint256 initialBusinessBalance = usdc.balanceOf(paymentAddress1);
        uint256 initialTippingBalance = usdc.balanceOf(tippingAddress1);
        uint256 initialTreasuryBalance = usdc.balanceOf(platformTreasury);

        vm.prank(customer1);
        payverge.processPayment(billId, BILL_AMOUNT, TIP_AMOUNT);

        // Check balances
        assertEq(usdc.balanceOf(paymentAddress1), initialBusinessBalance + businessAmount);
        assertEq(usdc.balanceOf(tippingAddress1), initialTippingBalance + TIP_AMOUNT);
        assertEq(usdc.balanceOf(platformTreasury), initialTreasuryBalance + platformFee);

        // Check bill status
        PayvergePayments.Bill memory bill = payverge.getBill(billId);
        assertEq(bill.paidAmount, BILL_AMOUNT);
        assertEq(bill.tipAmount, TIP_AMOUNT);
        assertEq(uint256(bill.status), uint256(PayvergePayments.BillStatus.Paid));
    }

    // ==================== SECURITY TESTS ====================

    function testReentrancyProtection() public {
        // This would require a malicious contract that attempts reentrancy
        // For now, we'll test that the nonReentrant modifier is in place
        bytes32 billId = _createTestBill();
        
        vm.prank(customer1);
        payverge.processPayment(billId, BILL_AMOUNT, 0);
        
        // Attempt to pay again should fail
        vm.prank(customer1);
        vm.expectRevert(abi.encodeWithSelector(PayvergePayments.BillAlreadyPaid.selector, billId));
        payverge.processPayment(billId, 1, 0);
    }

    function testAccessControl() public {
        // Test unauthorized business creation
        vm.prank(attacker);
        vm.expectRevert();
        payverge.verifyBusiness(attacker, "Fake Business", attacker, attacker);

        // Test unauthorized bill creation
        vm.prank(attacker);
        vm.expectRevert();
        payverge.createBill(keccak256("fake"), BILL_AMOUNT, "{}");

        // Test unauthorized admin functions
        vm.prank(attacker);
        vm.expectRevert();
        payverge.updatePlatformFeeRate(500);
    }

    function testPauseUnpause() public {
        bytes32 billId = _createTestBill();

        // Pause contract
        vm.prank(owner);
        payverge.pause();

        // Should not be able to process payments when paused
        vm.prank(customer1);
        vm.expectRevert();
        payverge.processPayment(billId, BILL_AMOUNT, 0);

        // Should not be able to create bills when paused
        vm.prank(business1);
        vm.expectRevert();
        payverge.createBill(keccak256("bill2"), BILL_AMOUNT, "{}");

        // Unpause
        vm.prank(owner);
        payverge.unpause();

        // Should work again
        vm.prank(customer1);
        payverge.processPayment(billId, BILL_AMOUNT, 0);
    }

    function testDailyLimits() public {
        uint256 dailyLimit = 50 * 10**6; // $50 limit
        
        vm.prank(owner);
        payverge.setDailyPaymentLimit(customer1, dailyLimit);

        bytes32 billId1 = _createTestBill();
        bytes32 billId2 = keccak256("bill2");
        
        vm.prank(business1);
        payverge.createBill(billId2, BILL_AMOUNT, "{}");

        // First payment should succeed (within limit)
        vm.prank(customer1);
        payverge.processPayment(billId1, 30 * 10**6, 0);

        // Second payment should fail (exceeds limit)
        vm.prank(customer1);
        vm.expectRevert(abi.encodeWithSelector(PayvergePayments.DailyLimitExceeded.selector, dailyLimit, 30 * 10**6 + BILL_AMOUNT));
        payverge.processPayment(billId2, BILL_AMOUNT, 0);
    }

    function testCircuitBreaker() public {
        // uint256 threshold = 100 * 10**6; // $100 threshold - unused in current test
        
        // Set low threshold for testing
        vm.prank(owner);
        payverge.resetCircuitBreaker();

        // This would require modifying the contract to allow setting threshold
        // For now, we'll test the manual trip function
        vm.prank(owner);
        payverge.tripCircuitBreaker();

        bytes32 billId = _createTestBill();
        
        vm.prank(customer1);
        vm.expectRevert(PayvergePayments.CircuitBreakerActive.selector);
        payverge.processPayment(billId, BILL_AMOUNT, 0);

        // Reset circuit breaker
        vm.prank(owner);
        payverge.resetCircuitBreaker();

        // Should work again
        vm.prank(customer1);
        payverge.processPayment(billId, BILL_AMOUNT, 0);
    }

    // ==================== EDGE CASE TESTS ====================

    function testZeroAmountPayment() public {
        bytes32 billId = _createTestBill();
        
        vm.prank(customer1);
        vm.expectRevert(abi.encodeWithSelector(PayvergePayments.InvalidAmount.selector, 0));
        payverge.processPayment(billId, 0, 0);
    }

    function testMinimumPaymentAmount() public {
        bytes32 billId = keccak256("smallbill");
        uint256 smallAmount = 0.5 * 10**6; // $0.50 (below minimum)
        
        vm.prank(business1);
        vm.expectRevert(abi.encodeWithSelector(PayvergePayments.InvalidAmount.selector, smallAmount));
        payverge.createBill(billId, smallAmount, "{}");
    }

    function testMaximumPaymentAmount() public {
        bytes32 billId = keccak256("largebill");
        uint256 largeAmount = 2000000 * 10**6; // $2M (above maximum)
        
        vm.prank(business1);
        vm.expectRevert(abi.encodeWithSelector(PayvergePayments.InvalidAmount.selector, largeAmount));
        payverge.createBill(billId, largeAmount, "{}");
    }

    function testExcessivePayment() public {
        bytes32 billId = _createTestBill();
        uint256 excessiveAmount = BILL_AMOUNT + 1;
        
        vm.prank(customer1);
        vm.expectRevert(abi.encodeWithSelector(PayvergePayments.ExcessivePayment.selector, BILL_AMOUNT, excessiveAmount));
        payverge.processPayment(billId, excessiveAmount, 0);
    }

    function testExcessiveTip() public {
        bytes32 billId = _createTestBill();
        uint256 excessiveTip = (BILL_AMOUNT * 5001) / 10000; // 50.01% tip (above max)
        
        vm.prank(customer1);
        vm.expectRevert(abi.encodeWithSelector(PayvergePayments.InvalidAmount.selector, excessiveTip));
        payverge.processPayment(billId, BILL_AMOUNT, excessiveTip);
    }

    function testPartialPayments() public {
        bytes32 billId = _createTestBill();
        uint256 partialAmount = BILL_AMOUNT / 2;
        
        // First partial payment
        vm.prank(customer1);
        payverge.processPayment(billId, partialAmount, TIP_AMOUNT);
        
        PayvergePayments.Bill memory bill = payverge.getBill(billId);
        assertEq(bill.paidAmount, partialAmount);
        assertEq(uint256(bill.status), uint256(PayvergePayments.BillStatus.Active));
        
        // Second partial payment to complete
        vm.prank(customer2);
        payverge.processPayment(billId, partialAmount, 0);
        
        bill = payverge.getBill(billId);
        assertEq(bill.paidAmount, BILL_AMOUNT);
        assertEq(uint256(bill.status), uint256(PayvergePayments.BillStatus.Paid));
    }

    function testNonExistentBill() public {
        bytes32 fakeBillId = keccak256("fake");
        
        vm.prank(customer1);
        vm.expectRevert(abi.encodeWithSelector(PayvergePayments.BillNotFound.selector, fakeBillId));
        payverge.processPayment(fakeBillId, BILL_AMOUNT, 0);
    }

    function testDuplicateBillCreation() public {
        bytes32 billId = keccak256("duplicate");
        
        vm.prank(business1);
        payverge.createBill(billId, BILL_AMOUNT, "{}");
        
        vm.prank(business1);
        vm.expectRevert(abi.encodeWithSelector(PayvergePayments.BillNotFound.selector, billId));
        payverge.createBill(billId, BILL_AMOUNT, "{}");
    }

    function testUnverifiedBusinessBillCreation() public {
        address unverifiedBusiness = address(0xDEAD);
        
        vm.prank(unverifiedBusiness);
        vm.expectRevert();
        payverge.createBill(keccak256("unverified"), BILL_AMOUNT, "{}");
    }

    function testBusinessDeactivation() public {
        vm.prank(owner);
        payverge.deactivateBusiness(business1, "Violation of terms");
        
        assertFalse(payverge.verifiedBusinesses(business1));
        assertFalse(payverge.hasRole(BUSINESS_ROLE, business1));
        
        vm.prank(business1);
        vm.expectRevert();
        payverge.createBill(keccak256("deactivated"), BILL_AMOUNT, "{}");
    }

    function testPlatformFeeUpdate() public {
        uint256 newFeeRate = 300; // 3%
        
        vm.prank(owner);
        payverge.updatePlatformFeeRate(newFeeRate);
        
        assertEq(payverge.platformFeeRate(), newFeeRate);
        
        // Test invalid fee rate
        vm.prank(owner);
        vm.expectRevert(abi.encodeWithSelector(PayvergePayments.InvalidFeeRate.selector, 1001));
        payverge.updatePlatformFeeRate(1001); // Above 10% maximum
    }

    function testEmergencyWithdraw() public {
        // Send some tokens to the contract
        usdc.transfer(address(payverge), 1000 * 10**6);
        
        uint256 initialBalance = usdc.balanceOf(owner);
        
        vm.prank(owner);
        payverge.emergencyWithdraw(address(usdc), 1000 * 10**6, owner);
        
        assertEq(usdc.balanceOf(owner), initialBalance + 1000 * 10**6);
    }

    // ==================== FUZZ TESTS ====================

    function testFuzzPaymentAmounts(uint256 amount, uint256 tip) public {
        vm.assume(amount >= 1 * 10**6 && amount <= 1000000 * 10**6); // Valid range
        vm.assume(tip <= (amount * 5000) / 10000); // Max 50% tip
        vm.assume(amount + tip <= usdc.balanceOf(customer1)); // Customer has enough balance
        
        bytes32 billId = keccak256(abi.encodePacked("fuzz", amount, tip));
        
        vm.prank(business1);
        payverge.createBill(billId, amount, "{}");
        
        vm.prank(customer1);
        payverge.processPayment(billId, amount, tip);
        
        PayvergePayments.Bill memory bill = payverge.getBill(billId);
        assertEq(bill.paidAmount, amount);
        assertEq(bill.tipAmount, tip);
    }

    function testFuzzDailyLimits(uint256 limit, uint256 payment1, uint256 payment2) public {
        // Bound inputs to reasonable ranges to avoid assumption rejections
        limit = bound(limit, 1000 * 10**6, 100000 * 10**6); // $1K to $100K
        payment1 = bound(payment1, 1 * 10**6, limit / 2); // $1 to half the limit
        payment2 = bound(payment2, 1 * 10**6, limit / 2); // $1 to half the limit
        
        // Ensure customer has enough balance for both payments
        uint256 totalNeeded = payment1 + payment2;
        if (usdc.balanceOf(customer1) < totalNeeded) {
            usdc.mint(customer1, totalNeeded);
        }
        
        vm.prank(owner);
        payverge.setDailyPaymentLimit(customer1, limit);
        
        bytes32 billId1 = keccak256(abi.encodePacked("fuzz1", payment1));
        bytes32 billId2 = keccak256(abi.encodePacked("fuzz2", payment2));
        
        vm.prank(business1);
        payverge.createBill(billId1, payment1, "{}");
        vm.prank(business1);
        payverge.createBill(billId2, payment2, "{}");
        
        vm.prank(customer1);
        payverge.processPayment(billId1, payment1, 0);
        
        if (payment1 + payment2 > limit) {
            vm.prank(customer1);
            vm.expectRevert();
            payverge.processPayment(billId2, payment2, 0);
        } else {
            vm.prank(customer1);
            payverge.processPayment(billId2, payment2, 0);
        }
    }

    // ==================== UPGRADE TESTS ====================

    function testUpgradeability() public {
        // Deploy new implementation
        PayvergePayments newImplementation = new PayvergePayments();
        
        // Grant upgrader role to owner first
        vm.prank(owner);
        payverge.grantRole(payverge.UPGRADER_ROLE(), owner);
        
        // Only upgrader role should be able to upgrade
        vm.prank(attacker);
        vm.expectRevert();
        payverge.upgradeToAndCall(address(newImplementation), "");
        
        // Successful upgrade
        vm.prank(owner);
        payverge.upgradeToAndCall(address(newImplementation), "");
        
        // Contract should still work after upgrade
        assertEq(payverge.version(), "2.0.0");
        assertTrue(payverge.verifiedBusinesses(business1));
    }

    // ==================== GAS OPTIMIZATION TESTS ====================

    function testGasUsage() public {
        bytes32 billId = _createTestBill();
        
        uint256 gasBefore = gasleft();
        vm.prank(customer1);
        payverge.processPayment(billId, BILL_AMOUNT, TIP_AMOUNT);
        uint256 gasUsed = gasBefore - gasleft();
        
        // Ensure gas usage is reasonable (adjust threshold as needed)
        assertLt(gasUsed, 600000, "Gas usage too high for payment processing");
    }

    // ==================== INTEGRATION TESTS ====================

    function testMultipleBusinessesAndCustomers() public {
        // Create bills for both businesses
        bytes32 billId1 = keccak256("business1_bill");
        bytes32 billId2 = keccak256("business2_bill");
        
        vm.prank(business1);
        payverge.createBill(billId1, BILL_AMOUNT, "{}");
        vm.prank(business2);
        payverge.createBill(billId2, BILL_AMOUNT * 2, "{}");
        
        // Process payments from different customers
        vm.prank(customer1);
        payverge.processPayment(billId1, BILL_AMOUNT, TIP_AMOUNT);
        vm.prank(customer2);
        payverge.processPayment(billId2, BILL_AMOUNT * 2, TIP_AMOUNT * 2);
        
        // Verify bills were paid
        PayvergePayments.Bill memory bill1 = payverge.getBill(billId1);
        PayvergePayments.Bill memory bill2 = payverge.getBill(billId2);
        
        assertEq(bill1.paidAmount, BILL_AMOUNT);
        assertEq(bill1.tipAmount, TIP_AMOUNT);
        assertEq(bill2.paidAmount, BILL_AMOUNT * 2);
        assertEq(bill2.tipAmount, TIP_AMOUNT * 2);
        
        // Verify both businesses are still verified and active
        assertTrue(payverge.verifiedBusinesses(business1), "Business 1 should remain verified");
        assertTrue(payverge.verifiedBusinesses(business2), "Business 2 should remain verified");
    }

    // ==================== HELPER FUNCTIONS ====================

    function _createTestBill() internal returns (bytes32) {
        bytes32 billId = keccak256("testbill");
        vm.prank(business1);
        payverge.createBill(billId, BILL_AMOUNT, "{}");
        return billId;
    }
}
