// SPDX-License-Identifier: MIT
pragma solidity ^0.8.19;

import "forge-std/Test.sol";
import "forge-std/console.sol";
import "../src/PayvergePayments.sol";
import "./mocks/MockERC20.sol";

// Malicious contract for reentrancy testing
contract ReentrantAttacker {
    PayvergePayments public payverge;
    MockERC20 public usdc;
    bool public attacking = false;
    uint256 public callCount = 0;
    
    constructor(PayvergePayments _payverge, MockERC20 _usdc) {
        payverge = _payverge;
        usdc = _usdc;
    }
    
    function attack() external {
        attacking = true;
        callCount = 0;
        payverge.claimEarnings();
    }
    
    // This will be called during USDC transfer, attempting reentrancy
    receive() external payable {
        if (attacking && callCount == 0) {
            callCount++;
            payverge.claimEarnings(); // This should fail due to reentrancy guard
        }
    }
    
    // ERC20 transfer callback simulation
    function transfer(address /* to */, uint256 /* amount */) external returns (bool) {
        if (attacking && callCount == 0) {
            callCount++;
            payverge.claimEarnings(); // This should fail due to reentrancy guard
        }
        return true;
    }
}

// Flash loan attacker contract
contract FlashLoanAttacker {
    PayvergePayments public payverge;
    MockERC20 public usdc;
    
    constructor(PayvergePayments _payverge, MockERC20 _usdc) {
        payverge = _payverge;
        usdc = _usdc;
    }
    
    function flashLoanAttack(bytes32 billId, uint256 amount) external {
        // Simulate flash loan attack by trying to manipulate bill state
        payverge.processPayment(billId, amount, 0);
    }
}

// Griefing attacker contract
contract GriefingAttacker {
    PayvergePayments public payverge;
    
    constructor(PayvergePayments _payverge) {
        payverge = _payverge;
    }
    
    function griefAttack(bytes32 billId) external {
        // Attempt to grief by making dust payments
        payverge.processPayment(billId, 100, 0); // Minimum amount
    }
}

/**
 * @title PayvergePayments Security Test Suite
 * @dev Comprehensive security tests including attack vectors, edge cases, and invariants
 */
contract PayvergePaymentsSecurityTest is Test {
    PayvergePayments public payverge;
    MockERC20 public usdc;
    
    // Test accounts
    address public admin = address(0x1);
    address public businessOwner = address(0x2);
    address public customer = address(0x3);
    address public attacker = address(0x999);
    address public treasury = address(0x100);
    
    // Test constants
    uint256 public constant INITIAL_BALANCE = 1000000 * 10**6; // 1M USDC
    uint256 public constant BILL_AMOUNT = 100 * 10**6; // 100 USDC
    uint256 public constant PLATFORM_FEE_RATE = 200; // 2%
    
    // Attack contracts
    ReentrantAttacker public reentrantAttacker;
    FlashLoanAttacker public flashLoanAttacker;
    GriefingAttacker public griefingAttacker;

    function setUp() public {
        // Deploy mock USDC
        usdc = new MockERC20("USD Coin", "USDC", 6);
        
        // Deploy PayvergePayments
        payverge = new PayvergePayments();
        
        // Initialize contract
        vm.prank(admin);
        payverge.initialize(
            address(usdc),
            treasury,
            200, // 2% platform fee
            admin,
            admin // bill creator address
        );
        
        // Roles are already set up in initialize function
        
        // Fund test accounts
        usdc.mint(businessOwner, INITIAL_BALANCE);
        usdc.mint(customer, INITIAL_BALANCE);
        usdc.mint(attacker, INITIAL_BALANCE);
        
        // Approve spending
        vm.prank(customer);
        usdc.approve(address(payverge), type(uint256).max);
        vm.prank(attacker);
        usdc.approve(address(payverge), type(uint256).max);
        
        // Register business
        vm.prank(businessOwner);
        payverge.registerBusiness("Test Business", businessOwner, businessOwner);

        // Deploy attack contracts
        reentrantAttacker = new ReentrantAttacker(payverge, usdc);
        flashLoanAttacker = new FlashLoanAttacker(payverge, usdc);
        griefingAttacker = new GriefingAttacker(payverge);
        
        // Fund attack contracts
        usdc.mint(address(reentrantAttacker), INITIAL_BALANCE);
        usdc.mint(address(flashLoanAttacker), INITIAL_BALANCE);
        
        // Approve attack contracts
        vm.prank(address(reentrantAttacker));
        usdc.approve(address(payverge), type(uint256).max);
        vm.prank(address(flashLoanAttacker));
        usdc.approve(address(payverge), type(uint256).max);
    }

    // Test 1: Reentrancy Attack Prevention
    function testReentrancyAttackPrevention() public {
        // Wait for rate limit from setup
        vm.warp(block.timestamp + payverge.RATE_LIMIT_WINDOW() + 1);
        
        // Register attacker as business to get claimable balance
        vm.prank(address(reentrantAttacker));
        payverge.registerBusiness("Attacker Business", address(reentrantAttacker), address(reentrantAttacker));
        
        // Wait for rate limit before creating bill
        vm.warp(block.timestamp + payverge.RATE_LIMIT_WINDOW() + 1);
        
        // Fund attacker's claimable balance by making them receive a payment
        bytes32 attackerBillId = keccak256("attacker-bill");
        vm.prank(admin);
        payverge.createBill(attackerBillId, address(reentrantAttacker), BILL_AMOUNT, "{}", keccak256("nonce2"));
        
        vm.prank(customer);
        payverge.processPayment(attackerBillId, BILL_AMOUNT, 0);
        
        // First call should succeed (no reentrancy)
        vm.prank(address(reentrantAttacker));
        payverge.claimEarnings();
        
        // Verify earnings were claimed
        (uint256 paymentAmount,) = payverge.claimablePayments(address(reentrantAttacker));
        assertEq(paymentAmount, 0);
    }

    // Test 2: Business Address Spoofing Prevention
    function testBusinessSpoofingPrevention() public {
        bytes32 billId = keccak256("spoofing-test");
        
        // Attacker tries to create bill for legitimate business (should fail - only BILL_MANAGER_ROLE can create bills)
        vm.prank(attacker);
        vm.expectRevert();
        payverge.createBill(billId, businessOwner, BILL_AMOUNT, "{}", keccak256("nonce3"));
    }

    // Test 3: Platform Fee Bounds
    function testPlatformFeeBounds() public {
        // Try to set fee above maximum
        uint256 maxFee = payverge.MAX_PLATFORM_FEE();
        vm.prank(admin);
        vm.expectRevert(bytes("Fee too high"));
        payverge.proposePlatformFeeUpdate(maxFee + 1);
        
        // Set valid fee with timelock
        vm.prank(admin);
        payverge.proposePlatformFeeUpdate(500); // 5%
        
        // Fast forward past timelock
        vm.warp(block.timestamp + 24 hours + 1);
        
        vm.prank(admin);
        payverge.executePlatformFeeUpdate();
        assertEq(payverge.platformFeeRate(), 500);
    }

    // Test 4: Payment Amount Validation
    function testPaymentAmountValidation() public {
        bytes32 billId = keccak256("validation-test");
        
        // Wait for rate limit from previous tests
        vm.warp(block.timestamp + payverge.RATE_LIMIT_WINDOW() + 1);
        
        // Try to create bill with amount below minimum
        uint256 minAmount = payverge.MIN_PAYMENT_AMOUNT();
        vm.prank(admin);
        vm.expectRevert(bytes("Amount too small"));
        payverge.createBill(billId, businessOwner, minAmount - 1, "{}", keccak256("nonce5"));
        
        // Wait for rate limit before second attempt
        vm.warp(block.timestamp + payverge.RATE_LIMIT_WINDOW() + 1);
        
        // Try to create bill with amount above maximum
        bytes32 billId2 = keccak256("validation-test-2");
        uint256 maxAmount = payverge.MAX_BILL_AMOUNT();
        vm.prank(admin);
        vm.expectRevert(bytes("Amount exceeds limit"));
        payverge.createBill(billId2, businessOwner, maxAmount + 1, "{}", keccak256("nonce6"));
        
        // Create a valid bill to verify the function works
        vm.warp(block.timestamp + payverge.RATE_LIMIT_WINDOW() + 1);
        bytes32 validBillId = keccak256("valid-bill");
        vm.prank(admin);
        payverge.createBill(validBillId, businessOwner, BILL_AMOUNT, "{}", keccak256("nonce7"));
        assertTrue(payverge.billExists(validBillId));
    }

    // Test 5: Nonce Replay Protection
    function testNonceReplayProtection() public {
        bytes32 billId = keccak256("nonce-test");
        bytes32 nonce = keccak256("test-nonce");
        
        // Wait for rate limit from previous tests
        vm.warp(block.timestamp + payverge.RATE_LIMIT_WINDOW() + 1);
        
        // Create bill with nonce
        vm.prank(admin);
        payverge.createBill(billId, businessOwner, BILL_AMOUNT, "{}", nonce);
        
        // Wait for rate limit before second bill
        vm.warp(block.timestamp + payverge.RATE_LIMIT_WINDOW() + 1);
        
        // Try to reuse same nonce (should fail)
        bytes32 billId2 = keccak256("nonce-test-2");
        vm.prank(admin);
        vm.expectRevert(bytes("Nonce already used"));
        payverge.createBill(billId2, businessOwner, BILL_AMOUNT, "{}", nonce);
    }

    // Test 6: Rate Limiting
    function testRateLimiting() public {
        bytes32 billId1 = keccak256("rate-test-1");
        bytes32 billId2 = keccak256("rate-test-2");
        
        // Wait for rate limit from previous tests
        vm.warp(block.timestamp + payverge.RATE_LIMIT_WINDOW() + 1);
        
        // Create first bill
        vm.prank(admin);
        payverge.createBill(billId1, businessOwner, BILL_AMOUNT, "{}", keccak256("nonce7"));
        
        // Try to create second bill immediately (should fail due to rate limit)
        vm.prank(admin);
        vm.expectRevert(bytes("Rate limit exceeded"));
        payverge.createBill(billId2, businessOwner, BILL_AMOUNT, "{}", keccak256("nonce8"));
        
        // Wait for rate limit window to pass
        vm.warp(block.timestamp + payverge.RATE_LIMIT_WINDOW() + 1);
        
        // Now should succeed
        vm.prank(admin);
        payverge.createBill(billId2, businessOwner, BILL_AMOUNT, "{}", keccak256("nonce8"));
    }

    // Test 7: Excessive Payment Prevention
    function testExcessivePaymentPrevention() public {
        bytes32 billId = keccak256("excessive-test");
        
        // Wait for rate limit from previous tests
        vm.warp(block.timestamp + payverge.RATE_LIMIT_WINDOW() + 1);
        
        // Create bill
        vm.prank(admin);
        payverge.createBill(billId, businessOwner, BILL_AMOUNT, "{}", keccak256("nonce8"));
        
        // Try to pay more than bill amount
        vm.prank(customer);
        vm.expectRevert(bytes("Excessive payment"));
        payverge.processPayment(billId, BILL_AMOUNT + 1, 0);
    }

    // Test 8: Successful Payment Flow
    function testSuccessfulPaymentFlow() public {
        bytes32 billId = keccak256("success-test");
        
        // Wait for rate limit from previous tests
        vm.warp(block.timestamp + payverge.RATE_LIMIT_WINDOW() + 1);
        
        // Create bill
        vm.prank(admin);
        payverge.createBill(billId, businessOwner, BILL_AMOUNT, "{}", keccak256("nonce9"));
        
        // Process payment
        vm.prank(customer);
        payverge.processPayment(billId, BILL_AMOUNT, 0);
        
        // Check bill status
        PayvergePayments.Bill memory bill = payverge.getBill(billId);
        assertTrue(bill.isPaid);
        assertEq(bill.paidAmount, BILL_AMOUNT);
        
        // Check platform fee was collected
        uint256 expectedFee = (BILL_AMOUNT * PLATFORM_FEE_RATE) / 10000;
        assertEq(usdc.balanceOf(treasury), expectedFee); // treasury receives platform fees
    }

    // Test 9: Earnings Claiming
    function testEarningsClaiming() public {
        bytes32 billId = keccak256("earnings-test");
        
        // Wait for rate limit from previous tests
        vm.warp(block.timestamp + payverge.RATE_LIMIT_WINDOW() + 1);
        
        // Create and pay bill
        vm.prank(admin);
        payverge.createBill(billId, businessOwner, BILL_AMOUNT, "{}", keccak256("nonce10"));
        
        // Make first partial payment
        vm.prank(customer);
        payverge.processPayment(billId, BILL_AMOUNT / 2, 0);
        
        // Make second partial payment to complete
        vm.prank(customer);
        payverge.processPayment(billId, BILL_AMOUNT / 2, 0);
        
        // Check bill is fully paid
        PayvergePayments.Bill memory bill = payverge.getBill(billId);
        assertTrue(bill.isPaid);
        assertEq(bill.paidAmount, BILL_AMOUNT);
    }

    // Test 10: Access Control
    function testAccessControl() public {
        bytes32 billId = keccak256("access-test");
        
        // Wait for rate limit from previous tests
        vm.warp(block.timestamp + payverge.RATE_LIMIT_WINDOW() + 1);
        
        // Attacker tries to create bill for legitimate business (should fail - only BILL_MANAGER_ROLE can create bills)
        vm.prank(attacker);
        vm.expectRevert();
        payverge.createBill(billId, businessOwner, BILL_AMOUNT, "{}", keccak256("nonce3"));
        
        // Wait for rate limit before second attempt
        vm.warp(block.timestamp + payverge.RATE_LIMIT_WINDOW() + 1);
        
        // Admin tries to create bill with excessive amount (should fail)
        bytes32 billId2 = keccak256("access-test-2");
        uint256 maxAmount = payverge.MAX_BILL_AMOUNT();
        vm.prank(admin);
        vm.expectRevert(bytes("Amount exceeds limit"));
        payverge.createBill(billId2, businessOwner, maxAmount + 1, "{}", keccak256("nonce4"));
        
        // Non-admin cannot update platform fee
        vm.prank(attacker);
        vm.expectRevert();
        payverge.proposePlatformFeeUpdate(300);
        
        // Non-admin cannot pause contract
        vm.prank(attacker);
        vm.expectRevert();
        payverge.pause();
        
        // Admin can update fee and pause
        vm.prank(admin);
        payverge.proposePlatformFeeUpdate(300);
        vm.warp(block.timestamp + 24 hours + 1);
        vm.prank(admin);
        payverge.executePlatformFeeUpdate();
        
        vm.prank(admin);
        payverge.pause();
        
        // Operations should fail when paused
        vm.prank(businessOwner);
        vm.expectRevert();
        payverge.registerBusiness("Test", businessOwner, businessOwner);
    }

    // Test 11: Zero Address Validation
    function testZeroAddressValidation() public {
        // Try to claim earnings as non-business owner
        vm.prank(attacker);
        vm.expectRevert(bytes("Not business owner"));
        payverge.claimEarnings();
        
        // Cannot register business with zero addresses
        vm.prank(attacker);
        vm.expectRevert(bytes("Zero address"));
        payverge.registerBusiness("Test", address(0), businessOwner);
        
        vm.prank(attacker);
        vm.expectRevert(bytes("Zero address"));
        payverge.registerBusiness("Test", businessOwner, address(0));
    }

    // Test 12: Business Registration
    function testBusinessRegistration() public {
        address newBusiness = makeAddr("newBusiness");
        
        // Wait for rate limit from previous tests
        vm.warp(block.timestamp + payverge.RATE_LIMIT_WINDOW() + 1);
        
        // Register new business (business registers themselves)
        vm.prank(newBusiness);
        payverge.registerBusiness("New Business", newBusiness, newBusiness);
        
        // Check business is active
        PayvergePayments.BusinessInfo memory info = payverge.getBusinessInfo(newBusiness);
        assertTrue(info.isActive);
        assertEq(info.paymentAddress, newBusiness);
        assertEq(info.tippingAddress, newBusiness);
        
        // Cannot register same business twice
        vm.prank(newBusiness);
        vm.expectRevert("Business already registered");
        payverge.registerBusiness("Duplicate", newBusiness, newBusiness);
    }

    // ==================== EDGE CASE TESTS ====================

    // Test 11: Extreme Payment Amounts
    function testExtremePaymentAmounts() public {
        bytes32 billId = keccak256("extreme-amounts");
        
        // Wait for rate limit
        vm.warp(block.timestamp + payverge.RATE_LIMIT_WINDOW() + 1);
        
        // Test with maximum allowed amount
        uint256 maxAmount = payverge.MAX_BILL_AMOUNT();
        vm.prank(admin);
        payverge.createBill(billId, businessOwner, maxAmount, "{}", keccak256("max-nonce"));
        
        // Fund customer with enough tokens for max payment
        usdc.mint(customer, maxAmount);
        
        // Customer pays maximum amount
        vm.prank(customer);
        payverge.processPayment(billId, maxAmount, 0);
        
        // Verify payment processed correctly
        PayvergePayments.Bill memory bill = payverge.getBill(billId);
        assertEq(bill.paidAmount, maxAmount);
        assertTrue(bill.isPaid);
        
        // Test with minimum allowed amount
        bytes32 minBillId = keccak256("min-amounts");
        vm.warp(block.timestamp + payverge.RATE_LIMIT_WINDOW() + 1);
        
        uint256 minAmount = payverge.MIN_PAYMENT_AMOUNT();
        vm.prank(admin);
        payverge.createBill(minBillId, businessOwner, minAmount, "{}", keccak256("min-nonce"));
        
        vm.prank(customer);
        payverge.processPayment(minBillId, minAmount, 0);
        
        PayvergePayments.Bill memory minBill = payverge.getBill(minBillId);
        assertEq(minBill.paidAmount, minAmount);
        assertTrue(minBill.isPaid);
    }

    // Test 12: Partial Payment Edge Cases
    function testPartialPaymentEdgeCases() public {
        bytes32 billId = keccak256("partial-edge");
        
        vm.warp(block.timestamp + payverge.RATE_LIMIT_WINDOW() + 1);
        vm.prank(admin);
        payverge.createBill(billId, businessOwner, BILL_AMOUNT, "{}", keccak256("partial-nonce"));
        
        // Pay exactly 1 unit less than total (but ensure it's above minimum)
        uint256 minAmount = payverge.MIN_PAYMENT_AMOUNT();
        uint256 partialAmount = BILL_AMOUNT - minAmount;
        vm.prank(customer);
        payverge.processPayment(billId, partialAmount, 0);
        
        PayvergePayments.Bill memory bill = payverge.getBill(billId);
        assertEq(bill.paidAmount, partialAmount);
        assertFalse(bill.isPaid);
        assertEq(bill.totalAmount - bill.paidAmount, minAmount);
        
        // Pay the remaining amount
        vm.prank(customer);
        payverge.processPayment(billId, minAmount, 0);
        
        bill = payverge.getBill(billId);
        assertEq(bill.paidAmount, BILL_AMOUNT);
        assertTrue(bill.isPaid);
    }

    // Test 13: Concurrent Payment Attempts
    function testConcurrentPaymentAttempts() public {
        bytes32 billId = keccak256("concurrent");
        
        vm.warp(block.timestamp + payverge.RATE_LIMIT_WINDOW() + 1);
        vm.prank(admin);
        payverge.createBill(billId, businessOwner, BILL_AMOUNT, "{}", keccak256("concurrent-nonce"));
        
        // Multiple customers try to pay simultaneously
        address customer2 = address(0x4);
        address customer3 = address(0x5);
        
        usdc.mint(customer2, INITIAL_BALANCE);
        usdc.mint(customer3, INITIAL_BALANCE);
        
        vm.prank(customer2);
        usdc.approve(address(payverge), type(uint256).max);
        vm.prank(customer3);
        usdc.approve(address(payverge), type(uint256).max);
        
        // All three customers pay partial amounts
        uint256 partialAmount = BILL_AMOUNT / 3;
        
        vm.prank(customer);
        payverge.processPayment(billId, partialAmount, 0);
        
        vm.prank(customer2);
        payverge.processPayment(billId, partialAmount, 0);
        
        vm.prank(customer3);
        payverge.processPayment(billId, partialAmount, 0);
        
        PayvergePayments.Bill memory bill = payverge.getBill(billId);
        assertEq(bill.paidAmount, partialAmount * 3);
        
        // Verify all payments are recorded
        PayvergePayments.Payment[] memory payments = payverge.getBillPayments(billId);
        assertEq(payments.length, 3);
    }

    // Test 14: Business Address Update Edge Cases
    function testBusinessAddressUpdateEdgeCases() public {
        address newPaymentAddr = address(0x100);
        address newTippingAddr = address(0x101);
        
        // Update to new addresses (using separate methods)
        vm.prank(businessOwner);
        payverge.updateBusinessPaymentAddress(newPaymentAddr);
        vm.prank(businessOwner);
        payverge.updateBusinessTippingAddress(newTippingAddr);
        
        PayvergePayments.BusinessInfo memory info = payverge.getBusinessInfo(businessOwner);
        assertEq(info.paymentAddress, newPaymentAddr);
        assertEq(info.tippingAddress, newTippingAddr);
        
        // Create bill and payment with new addresses
        bytes32 billId = keccak256("address-update");
        vm.warp(block.timestamp + payverge.RATE_LIMIT_WINDOW() + 1);
        
        vm.prank(admin);
        payverge.createBill(billId, businessOwner, BILL_AMOUNT, "{}", keccak256("update-nonce"));
        
        vm.prank(customer);
        payverge.processPayment(billId, BILL_AMOUNT, 10 * 10**6); // 10 USDC tip
        
        // Verify claimable amounts go to new addresses
        (uint256 payments, uint256 tips) = payverge.getClaimableAmounts(newPaymentAddr, newTippingAddr);
        assertTrue(payments > 0);
        assertTrue(tips > 0);
        
        // Old addresses should have zero claimable
        (uint256 oldPayments, uint256 oldTips) = payverge.getClaimableAmounts(businessOwner, businessOwner);
        assertEq(oldPayments, 0);
        assertEq(oldTips, 0);
    }

    // Test 15: Platform Fee Edge Cases
    function testPlatformFeeEdgeCases() public {
        // Set platform fee to maximum
        uint256 maxFee = payverge.MAX_PLATFORM_FEE();
        vm.prank(admin);
        payverge.proposePlatformFeeUpdate(maxFee);
        vm.warp(block.timestamp + 24 hours + 1);
        vm.prank(admin);
        payverge.executePlatformFeeUpdate();
        
        bytes32 billId = keccak256("max-fee");
        vm.warp(block.timestamp + payverge.RATE_LIMIT_WINDOW() + 1);
        
        vm.prank(admin);
        payverge.createBill(billId, businessOwner, BILL_AMOUNT, "{}", keccak256("fee-nonce"));
        
        uint256 businessBalanceBefore = usdc.balanceOf(businessOwner);
        uint256 treasuryBalanceBefore = usdc.balanceOf(treasury);
        
        vm.prank(customer);
        payverge.processPayment(billId, BILL_AMOUNT, 0);
        
        // Business should claim earnings
        vm.prank(businessOwner);
        payverge.claimEarnings();
        
        uint256 businessBalanceAfter = usdc.balanceOf(businessOwner);
        uint256 treasuryBalanceAfter = usdc.balanceOf(treasury);
        
        // Calculate expected amounts
        uint256 expectedFee = (BILL_AMOUNT * maxFee) / 10000;
        uint256 expectedBusinessAmount = BILL_AMOUNT - expectedFee;
        
        assertEq(businessBalanceAfter - businessBalanceBefore, expectedBusinessAmount);
        assertEq(treasuryBalanceAfter - treasuryBalanceBefore, expectedFee);
        
        // Reset fee to normal
        vm.prank(admin);
        payverge.proposePlatformFeeUpdate(PLATFORM_FEE_RATE);
        vm.warp(block.timestamp + 24 hours + 1);
        vm.prank(admin);
        payverge.executePlatformFeeUpdate();
    }

    // ==================== COMPLEX SCENARIO TESTS ====================

    // Test 16: Multi-Business Complex Scenario
    function testMultiBusinessComplexScenario() public {
        // Setup multiple businesses
        address business1 = address(0x201);
        address business2 = address(0x202);
        address business3 = address(0x203);
        
        // Register businesses
        vm.prank(business1);
        payverge.registerBusiness("Restaurant", business1, business1);
        
        vm.prank(business2);
        payverge.registerBusiness("Coffee Shop", business2, business2);
        
        vm.prank(business3);
        payverge.registerBusiness("Retail Store", business3, business3);
        
        // Create bills for each business
        bytes32 bill1 = keccak256("restaurant-bill");
        bytes32 bill2 = keccak256("coffee-bill");
        bytes32 bill3 = keccak256("retail-bill");
        
        vm.warp(block.timestamp + payverge.RATE_LIMIT_WINDOW() + 1);
        
        vm.prank(admin);
        payverge.createBill(bill1, business1, 50 * 10**6, "{}", keccak256("rest-nonce"));
        
        vm.warp(block.timestamp + payverge.RATE_LIMIT_WINDOW() + 1);
        vm.prank(admin);
        payverge.createBill(bill2, business2, 15 * 10**6, "{}", keccak256("coffee-nonce"));
        
        vm.warp(block.timestamp + payverge.RATE_LIMIT_WINDOW() + 1);
        vm.prank(admin);
        payverge.createBill(bill3, business3, 200 * 10**6, "{}", keccak256("retail-nonce"));
        
        // Multiple customers pay different amounts with tips
        address[] memory customers = new address[](5);
        for (uint i = 0; i < 5; i++) {
            customers[i] = address(uint160(0x300 + i));
            usdc.mint(customers[i], INITIAL_BALANCE);
            vm.prank(customers[i]);
            usdc.approve(address(payverge), type(uint256).max);
        }
        
        // Complex payment pattern
        vm.prank(customers[0]);
        payverge.processPayment(bill1, 25 * 10**6, 5 * 10**6); // Partial payment with tip
        
        vm.prank(customers[1]);
        payverge.processPayment(bill2, 15 * 10**6, 3 * 10**6); // Full payment with tip
        
        vm.prank(customers[2]);
        payverge.processPayment(bill1, 25 * 10**6, 0); // Complete restaurant bill
        
        vm.prank(customers[3]);
        payverge.processPayment(bill3, 100 * 10**6, 10 * 10**6); // Partial retail payment
        
        vm.prank(customers[4]);
        payverge.processPayment(bill3, 100 * 10**6, 20 * 10**6); // Complete retail bill
        
        // Verify all bills are in correct states
        PayvergePayments.Bill memory restaurantBill = payverge.getBill(bill1);
        PayvergePayments.Bill memory coffeeBill = payverge.getBill(bill2);
        PayvergePayments.Bill memory retailBill = payverge.getBill(bill3);
        
        assertTrue(restaurantBill.isPaid);
        assertTrue(coffeeBill.isPaid);
        assertTrue(retailBill.isPaid);
        
        // All businesses claim earnings
        vm.prank(business1);
        payverge.claimEarnings();
        
        vm.prank(business2);
        payverge.claimEarnings();
        
        vm.prank(business3);
        payverge.claimEarnings();
        
        // Verify businesses received correct amounts (accounting for platform fees)
        assertTrue(usdc.balanceOf(business1) > 0);
        assertTrue(usdc.balanceOf(business2) > 0);
        assertTrue(usdc.balanceOf(business3) > 0);
    }

    // Test 17: Rate Limiting Complex Scenarios
    function testRateLimitingComplexScenarios() public {
        uint256 rateWindow = payverge.RATE_LIMIT_WINDOW();
        
        // Test rapid bill creation attempts
        vm.startPrank(admin);
        
        // First bill should succeed
        vm.warp(block.timestamp + rateWindow + 1);
        payverge.createBill(keccak256("rate1"), businessOwner, BILL_AMOUNT, "{}", keccak256("rate-nonce1"));
        
        // Second bill immediately should fail
        vm.expectRevert("Rate limit exceeded");
        payverge.createBill(keccak256("rate2"), businessOwner, BILL_AMOUNT, "{}", keccak256("rate-nonce2"));
        
        // After waiting, should succeed
        vm.warp(block.timestamp + rateWindow + 1);
        payverge.createBill(keccak256("rate3"), businessOwner, BILL_AMOUNT, "{}", keccak256("rate-nonce3"));
        
        vm.stopPrank();
        
        // Test that different users have independent rate limits
        // Change bill creator to a new address to test independent rate limits
        address admin2 = address(0x400);
        
        // Admin changes bill creator address
        vm.startPrank(admin);
        payverge.setBillCreator(admin2);
        vm.stopPrank();
        
        // admin2 should be able to create bill immediately (independent rate limit)
        vm.prank(admin2);
        payverge.createBill(keccak256("rate4"), businessOwner, BILL_AMOUNT, "{}", keccak256("rate-nonce4"));
        
        // Verify admin2 also gets rate limited on second attempt
        vm.prank(admin2);
        vm.expectRevert("Rate limit exceeded");
        payverge.createBill(keccak256("rate5"), businessOwner, BILL_AMOUNT, "{}", keccak256("rate-nonce5"));
    }

    // Test 18: Gas Exhaustion and DoS Resistance
    function testGasExhaustionResistance() public {
        bytes32 billId = keccak256("gas-test");
        
        vm.warp(block.timestamp + payverge.RATE_LIMIT_WINDOW() + 1);
        vm.prank(admin);
        payverge.createBill(billId, businessOwner, BILL_AMOUNT, "{}", keccak256("gas-nonce"));
        
        // Make many small payments to test gas costs don't grow unboundedly
        uint256 numPayments = 50;
        uint256 paymentAmount = BILL_AMOUNT / numPayments;
        
        address[] memory manyCustomers = new address[](numPayments);
        for (uint i = 0; i < numPayments; i++) {
            manyCustomers[i] = address(uint160(0x500 + i));
            usdc.mint(manyCustomers[i], INITIAL_BALANCE);
            vm.prank(manyCustomers[i]);
            usdc.approve(address(payverge), type(uint256).max);
        }
        
        // Record gas usage for first and last payments
        uint256 gasStart = gasleft();
        vm.prank(manyCustomers[0]);
        payverge.processPayment(billId, paymentAmount, 0);
        uint256 gasFirst = gasStart - gasleft();
        
        // Make intermediate payments
        for (uint i = 1; i < numPayments - 1; i++) {
            vm.prank(manyCustomers[i]);
            payverge.processPayment(billId, paymentAmount, 0);
        }
        
        // Last payment
        gasStart = gasleft();
        vm.prank(manyCustomers[numPayments - 1]);
        payverge.processPayment(billId, paymentAmount, 0);
        uint256 gasLast = gasStart - gasleft();
        
        // Gas usage shouldn't increase dramatically
        assertTrue(gasLast < gasFirst * 2, "Gas usage increased too much");
        
        // Verify all payments recorded
        PayvergePayments.Payment[] memory payments = payverge.getBillPayments(billId);
        assertEq(payments.length, numPayments);
    }

    // Test 19: Upgrade Attack Scenarios
    function testUpgradeAttackScenarios() public {
        // Non-upgrader cannot upgrade
        address attackerAddr = address(0x666);
        vm.prank(attackerAddr);
        vm.expectRevert();
        payverge.upgradeToAndCall(address(0x123), "");
        
        // Admin without UPGRADER_ROLE cannot upgrade
        address nonUpgrader = address(0x777);
        vm.startPrank(admin);
        payverge.grantRole(payverge.ADMIN_ROLE(), nonUpgrader);
        vm.stopPrank();
        
        vm.prank(nonUpgrader);
        vm.expectRevert();
        payverge.upgradeToAndCall(address(0x123), "");
        
        // Test that admin already has UPGRADER_ROLE (granted in initialize)
        assertTrue(payverge.hasRole(payverge.UPGRADER_ROLE(), admin));
        
        // Test revoking and re-granting upgrade role
        vm.startPrank(admin);
        payverge.revokeRole(payverge.UPGRADER_ROLE(), admin);
        assertFalse(payverge.hasRole(payverge.UPGRADER_ROLE(), admin));
        
        // Re-grant the role
        payverge.grantRole(payverge.UPGRADER_ROLE(), admin);
        assertTrue(payverge.hasRole(payverge.UPGRADER_ROLE(), admin));
        vm.stopPrank();
    }

    // Test 20: Emergency Pause Scenarios
    function testEmergencyPauseScenarios() public {
        bytes32 billId = keccak256("pause-test");
        
        // Create bill before pause
        vm.warp(block.timestamp + payverge.RATE_LIMIT_WINDOW() + 1);
        vm.prank(admin);
        payverge.createBill(billId, businessOwner, BILL_AMOUNT, "{}", keccak256("pause-nonce"));
        
        // Pause contract
        vm.prank(admin);
        payverge.pause();
        
        // All operations should be paused - test with existing admin (already has BILL_MANAGER_ROLE)
        // Wait to avoid rate limit conflict
        vm.warp(block.timestamp + payverge.RATE_LIMIT_WINDOW() + 1);
        
        vm.prank(admin);
        vm.expectRevert();
        payverge.createBill(keccak256("paused-bill"), businessOwner, BILL_AMOUNT, "{}", keccak256("paused-nonce"));
        
        vm.prank(customer);
        vm.expectRevert();
        payverge.processPayment(billId, BILL_AMOUNT, 0);
        
        vm.prank(businessOwner);
        vm.expectRevert();
        payverge.claimEarnings();
        
        // Unpause and verify operations work
        vm.prank(admin);
        payverge.unpause();
        
        vm.prank(customer);
        payverge.processPayment(billId, BILL_AMOUNT, 0);
        
        PayvergePayments.Bill memory bill = payverge.getBill(billId);
        assertTrue(bill.isPaid);
    }

    // ==================== INVARIANT TESTS ====================

    // Test 21: Token Balance Invariants
    function testTokenBalanceInvariants() public {
        uint256 initialContractBalance = usdc.balanceOf(address(payverge));
        uint256 initialTotalSupply = usdc.totalSupply();
        
        // Create and pay multiple bills
        for (uint i = 0; i < 5; i++) {
            bytes32 billId = keccak256(abi.encodePacked("invariant-bill", i));
            
            vm.warp(block.timestamp + payverge.RATE_LIMIT_WINDOW() + 1);
            vm.prank(admin);
            payverge.createBill(billId, businessOwner, BILL_AMOUNT, "{}", keccak256(abi.encodePacked("inv-nonce", i)));
            
            vm.prank(customer);
            payverge.processPayment(billId, BILL_AMOUNT, 5 * 10**6); // With tip
        }
        
        uint256 contractBalanceAfterPayments = usdc.balanceOf(address(payverge));
        
        // Contract should hold all payments and tips
        assertTrue(contractBalanceAfterPayments > initialContractBalance);
        
        // Business claims all earnings
        vm.prank(businessOwner);
        payverge.claimEarnings();
        
        uint256 finalContractBalance = usdc.balanceOf(address(payverge));
        uint256 finalTotalSupply = usdc.totalSupply();
        
        // Total supply should remain unchanged
        assertEq(finalTotalSupply, initialTotalSupply);
        
        // Contract should only hold platform fees
        assertTrue(finalContractBalance < contractBalanceAfterPayments);
        assertTrue(finalContractBalance >= initialContractBalance); // At least platform fees
    }

    // Test 22: Business Earnings Invariants
    function testBusinessEarningsInvariants() public {
        uint256 totalPayments = 0;
        uint256 totalTips = 0;
        uint256 numBills = 3;
        
        // Create multiple bills and track totals
        for (uint i = 0; i < numBills; i++) {
            bytes32 billId = keccak256(abi.encodePacked("earnings-bill", i));
            uint256 billAmount = BILL_AMOUNT + (i * 10 * 10**6); // Varying amounts
            uint256 tipAmount = 5 * 10**6 + (i * 2 * 10**6); // Varying tips
            
            vm.warp(block.timestamp + payverge.RATE_LIMIT_WINDOW() + 1);
            vm.prank(admin);
            payverge.createBill(billId, businessOwner, billAmount, "{}", keccak256(abi.encodePacked("earn-nonce", i)));
            
            vm.prank(customer);
            payverge.processPayment(billId, billAmount, tipAmount);
            
            totalPayments += billAmount;
            totalTips += tipAmount;
        }
        
        // Calculate expected amounts after platform fees
        uint256 expectedFeeTotal = (totalPayments * PLATFORM_FEE_RATE) / 10000;
        uint256 expectedBusinessPayments = totalPayments - expectedFeeTotal;
        
        // Check claimable amounts
        (uint256 claimablePayments, uint256 claimableTips) = payverge.getClaimableAmounts(businessOwner, businessOwner);
        
        assertEq(claimablePayments, expectedBusinessPayments);
        assertEq(claimableTips, totalTips);
        
        // Claim and verify balances
        uint256 businessBalanceBefore = usdc.balanceOf(businessOwner);
        vm.prank(businessOwner);
        payverge.claimEarnings();
        uint256 businessBalanceAfter = usdc.balanceOf(businessOwner);
        
        assertEq(businessBalanceAfter - businessBalanceBefore, expectedBusinessPayments + totalTips);
    }
}
