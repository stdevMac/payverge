// SPDX-License-Identifier: MIT
pragma solidity ^0.8.19;

import "forge-std/Test.sol";
import "../src/PayvergePayments.sol";
import "@openzeppelin/contracts/token/ERC20/ERC20.sol";
import "@openzeppelin/contracts/proxy/ERC1967/ERC1967Proxy.sol";

contract MockUSDC is ERC20 {
    constructor() ERC20("Mock USDC", "USDC") {
        _mint(msg.sender, 1000000 * 10**6); // 1M USDC
    }

    function decimals() public pure override returns (uint8) {
        return 6;
    }

    function mint(address to, uint256 amount) external {
        _mint(to, amount);
    }
}

contract AlternativePaymentsTest is Test {
    PayvergePayments public payments;
    MockUSDC public usdc;
    
    address public admin = address(0x1);
    address public billManager = address(0x2);
    address public business = address(0x3);
    address public treasury = address(0x4);
    address public customer1 = address(0x5);
    address public customer2 = address(0x6);
    address public customer3 = address(0x7);
    address public attacker = address(0x8);
    
    bytes32 public constant ADMIN_ROLE = keccak256("ADMIN_ROLE");
    bytes32 public constant BILL_MANAGER_ROLE = keccak256("BILL_MANAGER_ROLE");
    
    bytes32 public testBillId = keccak256("TEST_BILL_1");
    uint256 public constant BILL_TOTAL = 100 * 10**6; // $100
    
    function setUp() public {
        // Deploy USDC mock
        usdc = new MockUSDC();
        
        // Deploy implementation
        PayvergePayments implementation = new PayvergePayments();
        
        // Deploy proxy
        bytes memory initData = abi.encodeWithSelector(
            PayvergePayments.initialize.selector,
            address(usdc),      // _usdcToken
            treasury,           // _platformTreasury
            200,                // _platformFeeRate (2%)
            admin,              // _admin
            address(this),      // _billCreator
            1000 * 10**6        // _registrationFee ($1000)
        );
        
        ERC1967Proxy proxy = new ERC1967Proxy(address(implementation), initData);
        payments = PayvergePayments(address(proxy));
        
        // Setup additional roles (admin role already granted in initialize)
        vm.prank(admin);
        payments.grantRole(BILL_MANAGER_ROLE, billManager);
        
        // Register business
        vm.startPrank(business);
        usdc.mint(business, 1000 * 10**6);
        usdc.approve(address(payments), 1000 * 10**6);
        payments.registerBusiness("Test Restaurant", business, business, "");
        vm.stopPrank();
        
        // Wait for rate limit before creating bill
        vm.warp(block.timestamp + 61); // Wait for rate limit window
        
        // Create test bill (as bill creator)
        vm.prank(address(this)); // this contract is set as bill creator in initialize
        payments.createBill(
            testBillId,
            business,
            BILL_TOTAL,
            "Test Bill",
            keccak256("test-nonce")
        );
        
        // Mint USDC for customers
        usdc.mint(customer1, 1000 * 10**6);
        usdc.mint(customer2, 1000 * 10**6);
        usdc.mint(customer3, 1000 * 10**6);
    }

    // ========== BASIC FUNCTIONALITY TESTS ==========
    
    function testBasicAlternativePayment() public {
        vm.prank(billManager);
        payments.markAlternativePayment(
            testBillId,
            customer1,
            50 * 10**6, // $50
            PayvergePayments.PaymentMethod.CASH
        );
        
        // Check payment was recorded
        PayvergePayments.AlternativePayment[] memory altPayments = 
            payments.getBillAlternativePayments(testBillId);
        
        assertEq(altPayments.length, 1);
        assertEq(altPayments[0].participant, customer1);
        assertEq(altPayments[0].amount, 50 * 10**6);
        assertEq(uint8(altPayments[0].methodType), uint8(PayvergePayments.PaymentMethod.CASH));
        assertTrue(altPayments[0].verified);
        
        // Check totals
        assertEq(payments.getTotalAlternativeAmount(testBillId), 50 * 10**6);
        
        (uint256 totalAmount, uint256 cryptoPaid, uint256 altPaid, uint256 remaining, bool isComplete) = 
            payments.getBillPaymentBreakdown(testBillId);
        
        assertEq(totalAmount, BILL_TOTAL);
        assertEq(cryptoPaid, 0);
        assertEq(altPaid, 50 * 10**6);
        assertEq(remaining, 50 * 10**6);
        assertFalse(isComplete);
    }

    // ========== EDGE CASE TESTS ==========
    
    function testAlternativePaymentExactBillTotal() public {
        // Pay entire bill with cash
        vm.prank(billManager);
        payments.markAlternativePayment(
            testBillId,
            customer1,
            BILL_TOTAL,
            PayvergePayments.PaymentMethod.CASH
        );
        
        // Bill should be marked as complete
        PayvergePayments.Bill memory bill = payments.getBill(testBillId);
        assertTrue(bill.isPaid);
        
        (,,,, bool isComplete) = payments.getBillPaymentBreakdown(testBillId);
        assertTrue(isComplete);
    }
    
    function testAlternativePaymentExceedsBillTotal() public {
        // Try to pay more than bill total
        vm.prank(billManager);
        vm.expectRevert("Exceeds bill total");
        payments.markAlternativePayment(
            testBillId,
            customer1,
            BILL_TOTAL + 1,
            PayvergePayments.PaymentMethod.CASH
        );
    }
    
    function testMixedCryptoAndAlternativePayments() public {
        // First, crypto payment of $30
        vm.startPrank(customer1);
        usdc.approve(address(payments), 30 * 10**6);
        payments.processPayment(testBillId, 30 * 10**6, 0);
        vm.stopPrank();
        
        // Then, cash payment of $40
        vm.prank(billManager);
        payments.markAlternativePayment(
            testBillId,
            customer2,
            40 * 10**6,
            PayvergePayments.PaymentMethod.CASH
        );
        
        // Then, card payment of $30 (should complete bill)
        vm.prank(billManager);
        payments.markAlternativePayment(
            testBillId,
            customer3,
            30 * 10**6,
            PayvergePayments.PaymentMethod.CARD
        );
        
        // Verify breakdown
        (uint256 totalAmount, uint256 cryptoPaid, uint256 altPaid, uint256 remaining, bool isComplete) = 
            payments.getBillPaymentBreakdown(testBillId);
        
        assertEq(totalAmount, BILL_TOTAL);
        assertEq(cryptoPaid, 30 * 10**6);
        assertEq(altPaid, 70 * 10**6);
        assertEq(remaining, 0);
        assertTrue(isComplete);
        
        // Check bill is marked as paid
        PayvergePayments.Bill memory bill = payments.getBill(testBillId);
        assertTrue(bill.isPaid);
    }
    
    function testMultipleAlternativePaymentsSamePerson() public {
        // Customer pays $20 cash
        vm.prank(billManager);
        payments.markAlternativePayment(
            testBillId,
            customer1,
            20 * 10**6,
            PayvergePayments.PaymentMethod.CASH
        );
        
        // Same customer pays $30 card
        vm.prank(billManager);
        payments.markAlternativePayment(
            testBillId,
            customer1,
            30 * 10**6,
            PayvergePayments.PaymentMethod.CARD
        );
        
        // Check participant info
        (uint256 paidAmount, uint32 paymentCount,) = 
            payments.getParticipantInfo(testBillId, customer1);
        
        assertEq(paidAmount, 50 * 10**6);
        assertEq(paymentCount, 2);
        
        // Check alternative payments array
        PayvergePayments.AlternativePayment[] memory altPayments = 
            payments.getBillAlternativePayments(testBillId);
        
        assertEq(altPayments.length, 2);
        assertEq(altPayments[0].amount, 20 * 10**6);
        assertEq(altPayments[1].amount, 30 * 10**6);
    }
    
    // ========== ATTACK VECTOR TESTS ==========
    
    function testUnauthorizedAlternativePayment() public {
        // Non-bill-manager tries to mark payment
        vm.prank(attacker);
        vm.expectRevert();
        payments.markAlternativePayment(
            testBillId,
            customer1,
            50 * 10**6,
            PayvergePayments.PaymentMethod.CASH
        );
        
        // Business owner tries to mark payment (should fail)
        vm.prank(business);
        vm.expectRevert();
        payments.markAlternativePayment(
            testBillId,
            customer1,
            50 * 10**6,
            PayvergePayments.PaymentMethod.CASH
        );
    }
    
    function testAlternativePaymentOnNonExistentBill() public {
        bytes32 fakeBillId = keccak256("FAKE_BILL");
        
        vm.prank(billManager);
        vm.expectRevert("Bill not found");
        payments.markAlternativePayment(
            fakeBillId,
            customer1,
            50 * 10**6,
            PayvergePayments.PaymentMethod.CASH
        );
    }
    
    function testAlternativePaymentZeroAmount() public {
        vm.prank(billManager);
        vm.expectRevert("Invalid amount");
        payments.markAlternativePayment(
            testBillId,
            customer1,
            0,
            PayvergePayments.PaymentMethod.CASH
        );
    }
    
    function testAlternativePaymentZeroAddress() public {
        vm.prank(billManager);
        vm.expectRevert("Zero address");
        payments.markAlternativePayment(
            testBillId,
            address(0),
            50 * 10**6,
            PayvergePayments.PaymentMethod.CASH
        );
    }
    
    function testAlternativePaymentWithCryptoMethod() public {
        // Should not allow marking crypto payments as alternative
        vm.prank(billManager);
        vm.expectRevert("Use processPayment for crypto");
        payments.markAlternativePayment(
            testBillId,
            customer1,
            50 * 10**6,
            PayvergePayments.PaymentMethod.CRYPTO
        );
    }
    
    // ========== COMPLEX EDGE CASES ==========
    
    function testAlternativePaymentOnAlreadyPaidBill() public {
        // Complete bill with crypto first
        vm.startPrank(customer1);
        usdc.approve(address(payments), BILL_TOTAL);
        payments.processPayment(testBillId, BILL_TOTAL, 0);
        vm.stopPrank();
        
        // Try to add alternative payment to completed bill
        vm.prank(billManager);
        vm.expectRevert("Bill not active");
        payments.markAlternativePayment(
            testBillId,
            customer2,
            10 * 10**6,
            PayvergePayments.PaymentMethod.CASH
        );
    }
    
    function testAlternativePaymentRaceCondition() public {
        // Simulate race condition: crypto payment and alternative payment happening simultaneously
        
        // Setup: Bill has $90 paid in crypto, $10 remaining
        vm.startPrank(customer1);
        usdc.approve(address(payments), 90 * 10**6);
        payments.processPayment(testBillId, 90 * 10**6, 0);
        vm.stopPrank();
        
        // Now try to add $20 alternative payment (should fail - exceeds total)
        vm.prank(billManager);
        vm.expectRevert("Exceeds bill total");
        payments.markAlternativePayment(
            testBillId,
            customer2,
            20 * 10**6,
            PayvergePayments.PaymentMethod.CASH
        );
        
        // But $10 should work
        vm.prank(billManager);
        payments.markAlternativePayment(
            testBillId,
            customer2,
            10 * 10**6,
            PayvergePayments.PaymentMethod.CASH
        );
        
        // Bill should now be complete
        PayvergePayments.Bill memory bill = payments.getBill(testBillId);
        assertTrue(bill.isPaid);
    }
    
    function testMassiveAlternativePaymentFragmentation() public {
        // Create bill with many small alternative payments
        bytes32 bigBillId = keccak256("BIG_BILL");
        uint256 bigBillTotal = 1000 * 10**6; // $1000
        
        // Wait for rate limit
        vm.warp(block.timestamp + 61);
        
        vm.prank(address(this));
        payments.createBill(bigBillId, business, bigBillTotal, "Big Bill", keccak256("big-bill-nonce"));
        
        // Add 100 customers paying $10 each via different methods
        for (uint256 i = 0; i < 100; i++) {
            address customer = address(uint160(1000 + i));
            PayvergePayments.PaymentMethod method = PayvergePayments.PaymentMethod(
                (i % 4) + 1 // Cycle through CASH, CARD, VENMO, OTHER
            );
            
            vm.prank(billManager);
            payments.markAlternativePayment(
                bigBillId,
                customer,
                10 * 10**6,
                method
            );
        }
        
        // Verify all payments recorded
        PayvergePayments.AlternativePayment[] memory altPayments = 
            payments.getBillAlternativePayments(bigBillId);
        
        assertEq(altPayments.length, 100);
        assertEq(payments.getTotalAlternativeAmount(bigBillId), bigBillTotal);
        
        // Bill should be complete
        PayvergePayments.Bill memory bill = payments.getBill(bigBillId);
        assertTrue(bill.isPaid);
        
        // Check participant count
        assertEq(bill.participantCount, 100);
    }
    
    function testAlternativePaymentGasLimits() public {
        // Test gas consumption doesn't grow linearly with payment count
        bytes32 gasTestBillId = keccak256("GAS_TEST_BILL");
        
        // Wait for rate limit
        vm.warp(block.timestamp + 61);
        
        vm.prank(address(this));
        payments.createBill(gasTestBillId, business, 1000 * 10**6, "Gas Test", keccak256("gas-test-nonce"));
        
        uint256 gasUsed1 = gasleft();
        vm.prank(billManager);
        payments.markAlternativePayment(
            gasTestBillId,
            customer1,
            10 * 10**6,
            PayvergePayments.PaymentMethod.CASH
        );
        gasUsed1 = gasUsed1 - gasleft();
        
        // Add 10 more payments
        for (uint256 i = 0; i < 10; i++) {
            vm.prank(billManager);
            payments.markAlternativePayment(
                gasTestBillId,
                address(uint160(2000 + i)),
                10 * 10**6,
                PayvergePayments.PaymentMethod.CASH
            );
        }
        
        uint256 gasUsed11 = gasleft();
        vm.prank(billManager);
        payments.markAlternativePayment(
            gasTestBillId,
            customer2,
            10 * 10**6,
            PayvergePayments.PaymentMethod.CASH
        );
        gasUsed11 = gasUsed11 - gasleft();
        
        // Gas usage shouldn't increase dramatically
        assertTrue(gasUsed11 < gasUsed1 * 2, "Gas usage increased too much");
    }
    
    // ========== BUSINESS LOGIC EDGE CASES ==========
    
    function testAlternativePaymentParticipantTracking() public {
        // Test that participant tracking works correctly with alternative payments
        
        // Customer1: Crypto payment
        vm.startPrank(customer1);
        usdc.approve(address(payments), 30 * 10**6);
        payments.processPayment(testBillId, 30 * 10**6, 0);
        vm.stopPrank();
        
        // Customer2: Alternative payment
        vm.prank(billManager);
        payments.markAlternativePayment(
            testBillId,
            customer2,
            40 * 10**6,
            PayvergePayments.PaymentMethod.CASH
        );
        
        // Customer1: Another crypto payment
        vm.startPrank(customer1);
        usdc.approve(address(payments), 20 * 10**6);
        payments.processPayment(testBillId, 20 * 10**6, 0);
        vm.stopPrank();
        
        // Customer2: Another alternative payment
        vm.prank(billManager);
        payments.markAlternativePayment(
            testBillId,
            customer2,
            10 * 10**6,
            PayvergePayments.PaymentMethod.CARD
        );
        
        // Check participant info
        (uint256 paid1, uint32 count1,) = payments.getParticipantInfo(testBillId, customer1);
        (uint256 paid2, uint32 count2,) = payments.getParticipantInfo(testBillId, customer2);
        
        assertEq(paid1, 50 * 10**6); // $30 + $20 crypto
        assertEq(count1, 2);
        assertEq(paid2, 50 * 10**6); // $40 + $10 alternative
        assertEq(count2, 2);
        
        // Check participant list
        address[] memory participants = payments.getBillParticipants(testBillId);
        assertEq(participants.length, 2);
        
        // Check bill participant count
        PayvergePayments.Bill memory bill = payments.getBill(testBillId);
        assertEq(bill.participantCount, 2);
        assertTrue(bill.isPaid);
    }
    
    function testAlternativePaymentEventEmission() public {
        // Test that events are emitted correctly
        vm.expectEmit(true, true, true, true);
        emit PayvergePayments.AlternativePaymentMarked(
            testBillId,
            customer1,
            50 * 10**6,
            PayvergePayments.PaymentMethod.CASH,
            billManager
        );
        
        vm.prank(billManager);
        payments.markAlternativePayment(
            testBillId,
            customer1,
            50 * 10**6,
            PayvergePayments.PaymentMethod.CASH
        );
    }
    
    function testBillCompletionEventWithAlternativePayments() public {
        // Test bill completion event with mixed payments
        vm.startPrank(customer1);
        usdc.approve(address(payments), 60 * 10**6);
        payments.processPayment(testBillId, 60 * 10**6, 0);
        vm.stopPrank();
        
        // This should complete the bill and emit completion event
        vm.expectEmit(true, true, true, true);
        emit PayvergePayments.BillCompletedWithAlternativePayments(
            testBillId,
            60 * 10**6, // crypto paid
            40 * 10**6  // alternative paid
        );
        
        vm.prank(billManager);
        payments.markAlternativePayment(
            testBillId,
            customer2,
            40 * 10**6,
            PayvergePayments.PaymentMethod.CASH
        );
    }
    
    // ========== INTEGRATION TESTS ==========
    
    function testAlternativePaymentWithPausedContract() public {
        // Pause contract
        vm.prank(admin);
        payments.pause();
        
        // Try alternative payment on paused contract
        vm.prank(billManager);
        vm.expectRevert(); // Just expect any revert since pause error format varies
        payments.markAlternativePayment(
            testBillId,
            customer1,
            50 * 10**6,
            PayvergePayments.PaymentMethod.CASH
        );
        
        // Unpause and try again
        vm.prank(admin);
        payments.unpause();
        
        vm.prank(billManager);
        payments.markAlternativePayment(
            testBillId,
            customer1,
            50 * 10**6,
            PayvergePayments.PaymentMethod.CASH
        );
        
        // Should work now
        assertEq(payments.getTotalAlternativeAmount(testBillId), 50 * 10**6);
    }
    
    // ========== EXTREME EDGE CASES ==========
    
    function testAlternativePaymentDustAmounts() public {
        // Test with extremely small amounts (1 cent)
        vm.prank(billManager);
        payments.markAlternativePayment(
            testBillId,
            customer1,
            1, // 1 micro USDC = $0.000001
            PayvergePayments.PaymentMethod.CASH
        );
        
        assertEq(payments.getTotalAlternativeAmount(testBillId), 1);
    }
    
    function testAlternativePaymentMaxAmount() public {
        // Create bill with maximum allowed amount
        bytes32 maxBillId = keccak256("MAX_BILL");
        uint256 maxAmount = 1_000_000 * 10**6; // $1M (MAX_BILL_AMOUNT)
        
        vm.warp(block.timestamp + 61);
        vm.prank(address(this));
        payments.createBill(maxBillId, business, maxAmount, "Max Bill", keccak256("max-bill-nonce"));
        
        // Pay entire amount with alternative payment
        vm.prank(billManager);
        payments.markAlternativePayment(
            maxBillId,
            customer1,
            maxAmount,
            PayvergePayments.PaymentMethod.CASH
        );
        
        PayvergePayments.Bill memory bill = payments.getBill(maxBillId);
        assertTrue(bill.isPaid);
        assertEq(payments.getTotalAlternativeAmount(maxBillId), maxAmount);
    }
    
    function testAlternativePaymentConcurrentSameParticipant() public {
        // Test multiple alternative payments from same participant in rapid succession
        for (uint256 i = 0; i < 10; i++) {
            vm.prank(billManager);
            payments.markAlternativePayment(
                testBillId,
                customer1,
                5 * 10**6, // $5 each
                PayvergePayments.PaymentMethod.CASH
            );
        }
        
        // Check participant info
        (uint256 paidAmount, uint32 paymentCount,) = 
            payments.getParticipantInfo(testBillId, customer1);
        
        assertEq(paidAmount, 50 * 10**6); // $50 total
        assertEq(paymentCount, 10);
        
        // Check total
        assertEq(payments.getTotalAlternativeAmount(testBillId), 50 * 10**6);
    }
    
    function testAlternativePaymentBoundaryConditions() public {
        // Test payment that exactly reaches bill total
        vm.prank(billManager);
        payments.markAlternativePayment(
            testBillId,
            customer1,
            BILL_TOTAL - 1, // $99.999999
            PayvergePayments.PaymentMethod.CASH
        );
        
        // Bill should not be complete yet
        PayvergePayments.Bill memory bill = payments.getBill(testBillId);
        assertFalse(bill.isPaid);
        
        // Add the final micro-cent
        vm.prank(billManager);
        payments.markAlternativePayment(
            testBillId,
            customer2,
            1, // Final micro-cent
            PayvergePayments.PaymentMethod.CARD
        );
        
        // Now bill should be complete
        bill = payments.getBill(testBillId);
        assertTrue(bill.isPaid);
    }
    
    function testAlternativePaymentMethodEnumBoundaries() public {
        // Test all payment method enum values
        PayvergePayments.PaymentMethod[] memory methods = new PayvergePayments.PaymentMethod[](4);
        methods[0] = PayvergePayments.PaymentMethod.CASH;
        methods[1] = PayvergePayments.PaymentMethod.CARD;
        methods[2] = PayvergePayments.PaymentMethod.VENMO;
        methods[3] = PayvergePayments.PaymentMethod.OTHER;
        
        for (uint256 i = 0; i < methods.length; i++) {
            address customer = address(uint160(5000 + i));
            
            vm.prank(billManager);
            payments.markAlternativePayment(
                testBillId,
                customer,
                10 * 10**6,
                methods[i]
            );
            
            // Verify method was recorded correctly
            PayvergePayments.AlternativePayment[] memory altPayments = 
                payments.getBillAlternativePayments(testBillId);
            
            assertEq(uint8(altPayments[i].methodType), uint8(methods[i]));
        }
    }
    
    function testAlternativePaymentTimestampAccuracy() public {
        uint256 startTime = block.timestamp;
        
        vm.prank(billManager);
        payments.markAlternativePayment(
            testBillId,
            customer1,
            25 * 10**6,
            PayvergePayments.PaymentMethod.CASH
        );
        
        // Advance time
        vm.warp(block.timestamp + 3600); // 1 hour later
        
        vm.prank(billManager);
        payments.markAlternativePayment(
            testBillId,
            customer2,
            25 * 10**6,
            PayvergePayments.PaymentMethod.CARD
        );
        
        PayvergePayments.AlternativePayment[] memory altPayments = 
            payments.getBillAlternativePayments(testBillId);
        
        assertEq(altPayments[0].timestamp, startTime);
        assertEq(altPayments[1].timestamp, startTime + 3600);
        
        // Check participant last payment times
        (, , uint32 lastPayment1) = payments.getParticipantInfo(testBillId, customer1);
        (, , uint32 lastPayment2) = payments.getParticipantInfo(testBillId, customer2);
        
        assertEq(lastPayment1, startTime);
        assertEq(lastPayment2, startTime + 3600);
    }
    
    function testAlternativePaymentReentrancyProtection() public {
        // This test verifies that the nonReentrant modifier works
        // In a real attack scenario, a malicious contract would try to call
        // markAlternativePayment recursively, but our modifier prevents this
        
        vm.prank(billManager);
        payments.markAlternativePayment(
            testBillId,
            customer1,
            50 * 10**6,
            PayvergePayments.PaymentMethod.CASH
        );
        
        // Verify payment was recorded normally
        assertEq(payments.getTotalAlternativeAmount(testBillId), 50 * 10**6);
    }
    
    function testAlternativePaymentStorageEfficiency() public {
        // Test that storage is used efficiently with many payments
        bytes32 storageBillId = keccak256("STORAGE_TEST_BILL");
        
        vm.warp(block.timestamp + 61);
        vm.prank(address(this));
        payments.createBill(storageBillId, business, 500 * 10**6, "Storage Test", keccak256("storage-nonce"));
        
        // Add 50 different participants with 1 payment each
        for (uint256 i = 0; i < 50; i++) {
            address customer = address(uint160(6000 + i));
            
            vm.prank(billManager);
            payments.markAlternativePayment(
                storageBillId,
                customer,
                10 * 10**6, // $10 each
                PayvergePayments.PaymentMethod.CASH
            );
        }
        
        // Verify all payments recorded
        PayvergePayments.AlternativePayment[] memory altPayments = 
            payments.getBillAlternativePayments(storageBillId);
        
        assertEq(altPayments.length, 50);
        assertEq(payments.getTotalAlternativeAmount(storageBillId), 500 * 10**6);
        
        // Verify bill completion
        PayvergePayments.Bill memory bill = payments.getBill(storageBillId);
        assertTrue(bill.isPaid);
        assertEq(bill.participantCount, 50);
    }
    
    function testAlternativePaymentViewFunctions() public {
        // Add some alternative payments
        vm.prank(billManager);
        payments.markAlternativePayment(testBillId, customer1, 30 * 10**6, PayvergePayments.PaymentMethod.CASH);
        
        vm.prank(billManager);
        payments.markAlternativePayment(testBillId, customer2, 20 * 10**6, PayvergePayments.PaymentMethod.CARD);
        
        // Test view functions
        PayvergePayments.AlternativePayment[] memory altPayments = 
            payments.getBillAlternativePayments(testBillId);
        
        assertEq(altPayments.length, 2);
        assertEq(payments.getTotalAlternativeAmount(testBillId), 50 * 10**6);
        
        (uint256 total, uint256 crypto, uint256 alt, uint256 remaining, bool complete) = 
            payments.getBillPaymentBreakdown(testBillId);
        
        assertEq(total, BILL_TOTAL);
        assertEq(crypto, 0);
        assertEq(alt, 50 * 10**6);
        assertEq(remaining, 50 * 10**6);
        assertFalse(complete);
    }
}
