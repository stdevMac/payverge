// SPDX-License-Identifier: MIT
pragma solidity ^0.8.19;

import "forge-std/Test.sol";
import "forge-std/console.sol";
import "../src/PayvergePayments.sol";
import "../src/PayvergeReferrals.sol";
import "../src/PayvergeProfitSplit.sol";
import "@openzeppelin/contracts/proxy/ERC1967/ERC1967Proxy.sol";
import "./mocks/MockERC20.sol";

/**
 * @title FullIntegrationTest
 * @dev Comprehensive end-to-end integration tests for the complete Payverge ecosystem
 * Tests the full flow: Business registration → Bill payment → Earnings claim → Profit distribution
 */
contract FullIntegrationTest is Test {
    // Contracts
    PayvergePayments public payments;
    PayvergeReferrals public referrals;
    PayvergeProfitSplit public profitSplit;
    MockERC20 public usdc;

    // Test accounts
    address public admin = makeAddr("admin");
    address public platformOwner = makeAddr("platformOwner");
    address public investor1 = makeAddr("investor1");
    address public investor2 = makeAddr("investor2");
    address public referrer = makeAddr("referrer");
    address public businessOwner = makeAddr("businessOwner");
    address public customer = makeAddr("customer");

    // Test constants
    uint256 public constant REGISTRATION_FEE = 100 * 10 ** 6; // $100 USDC
    uint256 public constant PLATFORM_FEE_RATE = 200; // 2%
    uint256 public constant BILL_AMOUNT = 1000 * 10 ** 6; // $1000 USDC
    uint256 public constant REFERRER_FEE = 25 * 10 ** 6; // $25 USDC for premium

    function setUp() public {
        // Deploy USDC mock
        usdc = new MockERC20("USD Coin", "USDC", 6);

        // Deploy all contracts with proxies
        _deployContracts();

        // Connect contracts
        _connectContracts();

        // Setup profit split beneficiaries
        _setupProfitSplit();

        // Fund accounts
        _fundAccounts();
    }

    function _deployContracts() internal {
        vm.startPrank(admin);

        // Deploy PayvergePayments
        PayvergePayments paymentsImpl = new PayvergePayments();
        bytes memory paymentsInitData = abi.encodeWithSelector(
            PayvergePayments.initialize.selector,
            address(usdc),
            PLATFORM_FEE_RATE,
            admin,
            admin, // bill creator
            REGISTRATION_FEE
        );
        ERC1967Proxy paymentsProxy = new ERC1967Proxy(address(paymentsImpl), paymentsInitData);
        payments = PayvergePayments(address(paymentsProxy));

        // Deploy PayvergeReferrals
        PayvergeReferrals referralsImpl = new PayvergeReferrals();
        bytes memory referralsInitData = abi.encodeWithSelector(
            PayvergeReferrals.initialize.selector,
            address(usdc),
            admin, // treasury (not used anymore)
            admin
        );
        ERC1967Proxy referralsProxy = new ERC1967Proxy(address(referralsImpl), referralsInitData);
        referrals = PayvergeReferrals(address(referralsProxy));

        // Deploy PayvergeProfitSplit
        PayvergeProfitSplit profitSplitImpl = new PayvergeProfitSplit();
        bytes memory profitSplitInitData = abi.encodeWithSelector(
            PayvergeProfitSplit.initialize.selector,
            address(usdc),
            admin, // treasury
            admin
        );
        ERC1967Proxy profitSplitProxy = new ERC1967Proxy(address(profitSplitImpl), profitSplitInitData);
        profitSplit = PayvergeProfitSplit(payable(address(profitSplitProxy)));

        vm.stopPrank();
    }

    function _connectContracts() internal {
        vm.startPrank(admin);

        // Connect PayvergePayments to other contracts
        payments.setReferralsContract(address(referrals));
        payments.setProfitSplitContract(address(profitSplit));

        // Connect PayvergeReferrals to PayvergePayments
        referrals.setPayvergePaymentsContract(address(payments));

        vm.stopPrank();
    }

    function _setupProfitSplit() internal {
        vm.startPrank(admin);

        // Add beneficiaries: 60% platform owner, 25% investor1, 15% investor2
        profitSplit.addBeneficiary(platformOwner, "Platform Owner", 6000);
        profitSplit.addBeneficiary(investor1, "Investor 1", 2500);
        profitSplit.addBeneficiary(investor2, "Investor 2", 1500);

        // Grant distributor role to admin for testing
        profitSplit.grantDistributorRole(admin);

        vm.stopPrank();
    }

    function _fundAccounts() internal {
        // Fund all accounts with USDC
        usdc.mint(referrer, 10000 * 10 ** 6);
        usdc.mint(businessOwner, 10000 * 10 ** 6);
        usdc.mint(customer, 10000 * 10 ** 6);

        // Approve spending
        vm.prank(referrer);
        usdc.approve(address(referrals), type(uint256).max);

        vm.prank(businessOwner);
        usdc.approve(address(payments), type(uint256).max);

        vm.prank(customer);
        usdc.approve(address(payments), type(uint256).max);
    }

    /**
     * @dev Test the complete end-to-end flow with referral and profit distribution
     */
    function testCompleteBusinessFlow() public {
        console.log("=== STARTING COMPLETE BUSINESS FLOW TEST ===");

        // Step 1-2: Register referrer and business
        _testReferrerAndBusinessRegistration();

        // Step 3-5: Process bill and business earnings
        _testBillProcessingAndEarnings();

        // Step 6-7: Referrer claims and profit distribution
        _testReferrerClaimsAndProfitDistribution();

        console.log("=== COMPLETE BUSINESS FLOW TEST PASSED ===");
    }

    function _testReferrerAndBusinessRegistration() internal {
        // Step 1: Referrer registers as Premium
        console.log("Step 1: Referrer Registration");
        string memory referralCode = "PREMIUM123";

        vm.prank(referrer);
        referrals.registerPremiumReferrer(referralCode);

        // Verify referrer registration
        PayvergeReferrals.Referrer memory referrerData = referrals.getReferrer(referrer);
        assertEq(uint8(referrerData.tier), uint8(PayvergeReferrals.ReferrerTier.Premium));
        assertEq(referrerData.referralCode, referralCode);
        assertTrue(referrerData.isActive);
        console.log("Referrer registered successfully");

        // Step 2: Business registers with referral code
        console.log("Step 2: Business Registration with Referral");

        uint256 businessBalanceBefore = usdc.balanceOf(businessOwner);
        uint256 profitSplitBalanceBefore = usdc.balanceOf(address(profitSplit));

        vm.prank(businessOwner);
        payments.registerBusiness(
            "Test Restaurant",
            businessOwner, // payment address
            businessOwner, // tipping address
            referralCode
        );

        // Verify business registration
        PayvergePayments.BusinessInfo memory businessInfo = payments.getBusinessInfo(businessOwner);
        assertTrue(businessInfo.isActive);
        assertEq(businessInfo.paymentAddress, businessOwner);
        console.log("SUCCESS: Business registered with referral discount");

        // Verify referral discount was applied (15% discount for premium)
        uint256 expectedDiscount = (REGISTRATION_FEE * 1500) / 10000; // 15%
        uint256 expectedCommission = (REGISTRATION_FEE * 1500) / 10000; // 15%
        uint256 expectedFinalFee = REGISTRATION_FEE - expectedDiscount; // Business pays discounted fee
        uint256 expectedTotalPaid = expectedFinalFee + expectedCommission; // Business pays discounted fee + commission
        uint256 actualPaid = businessBalanceBefore - usdc.balanceOf(businessOwner);
        assertEq(actualPaid, expectedTotalPaid);
        console.log("SUCCESS: Referral discount applied correctly");

        // Verify registration fee went to profit split contract
        uint256 profitSplitBalanceAfter = usdc.balanceOf(address(profitSplit));
        assertEq(profitSplitBalanceAfter - profitSplitBalanceBefore, expectedFinalFee);
        console.log("SUCCESS: Registration fee routed to profit split contract");

        // Verify commission went to referrals contract
        uint256 referralsBalance = usdc.balanceOf(address(referrals));
        assertEq(referralsBalance, expectedCommission);
        console.log("SUCCESS: Commission transferred to referrals contract");
    }

    function _testBillProcessingAndEarnings() internal {
        // Step 3: Create a bill
        console.log("Step 3: Bill Creation");

        // Wait for rate limit window to pass
        vm.warp(block.timestamp + 61); // RATE_LIMIT_WINDOW is 60 seconds

        bytes32 billId = keccak256("test-bill-1");

        vm.prank(admin); // Bill creator
        payments.createBill(billId, businessOwner, BILL_AMOUNT, '{"item": "dinner"}', keccak256("nonce1"));

        // Verify bill creation
        PayvergePayments.Bill memory bill = payments.getBill(billId);
        assertEq(bill.businessAddress, businessOwner);
        assertEq(bill.totalAmount, BILL_AMOUNT);
        assertFalse(bill.isPaid);
        console.log("SUCCESS: Bill created successfully");

        // Step 4: Customer pays the bill
        console.log("Step 4: Customer Payment");

        uint256 customerBalanceBefore = usdc.balanceOf(customer);
        uint256 profitSplitBalanceBefore = usdc.balanceOf(address(profitSplit));

        vm.prank(customer);
        payments.processPayment(billId, BILL_AMOUNT, 0); // No tip for simplicity

        // Verify payment processing
        bill = payments.getBill(billId);
        assertTrue(bill.isPaid);
        assertEq(bill.paidAmount, BILL_AMOUNT);
        console.log("SUCCESS: Payment processed successfully");

        // Verify customer paid full amount
        assertEq(customerBalanceBefore - usdc.balanceOf(customer), BILL_AMOUNT);

        // Verify platform fee went to profit split contract
        uint256 expectedPlatformFee = (BILL_AMOUNT * PLATFORM_FEE_RATE) / 10000; // 2%
        uint256 expectedBusinessAmount = BILL_AMOUNT - expectedPlatformFee;

        uint256 profitSplitBalanceAfter = usdc.balanceOf(address(profitSplit));
        uint256 newFeesInProfitSplit = profitSplitBalanceAfter - profitSplitBalanceBefore;
        assertEq(newFeesInProfitSplit, expectedPlatformFee);
        console.log("SUCCESS: Platform fee routed to profit split contract");

        // Step 5: Business claims earnings
        console.log("Step 5: Business Claims Earnings");

        uint256 businessBalanceBefore = usdc.balanceOf(businessOwner);

        vm.prank(businessOwner);
        payments.claimEarnings();

        // Verify business received correct amount (bill amount minus platform fee)
        uint256 businessBalanceAfter = usdc.balanceOf(businessOwner);
        assertEq(businessBalanceAfter - businessBalanceBefore, expectedBusinessAmount);
        console.log("SUCCESS: Business claimed earnings successfully");
    }

    function _testReferrerClaimsAndProfitDistribution() internal {
        // Step 6: Referrer claims commission
        console.log("Step 6: Referrer Claims Commission");

        uint256 expectedCommission = (REGISTRATION_FEE * 1500) / 10000; // 15%
        uint256 referrerBalanceBefore = usdc.balanceOf(referrer);

        vm.prank(referrer);
        referrals.claimCommissions();

        // Verify referrer received commission (15% of registration fee)
        uint256 referrerBalanceAfter = usdc.balanceOf(referrer);
        assertEq(referrerBalanceAfter - referrerBalanceBefore, expectedCommission);
        console.log("SUCCESS: Referrer claimed commission successfully");

        // Step 7: Distribute profits to stakeholders
        console.log("Step 7: Profit Distribution to Stakeholders");

        uint256 platformOwnerBalanceBefore = usdc.balanceOf(platformOwner);
        uint256 investor1BalanceBefore = usdc.balanceOf(investor1);
        uint256 investor2BalanceBefore = usdc.balanceOf(investor2);

        // Calculate total available for distribution
        uint256 totalForDistribution = usdc.balanceOf(address(profitSplit));

        vm.prank(admin);
        profitSplit.distributeAllProfits();

        // Verify profit distribution
        uint256 expectedPlatformOwnerShare = (totalForDistribution * 6000) / 10000; // 60%
        uint256 expectedInvestor1Share = (totalForDistribution * 2500) / 10000; // 25%
        uint256 expectedInvestor2Share = (totalForDistribution * 1500) / 10000; // 15%

        assertEq(usdc.balanceOf(platformOwner) - platformOwnerBalanceBefore, expectedPlatformOwnerShare);
        assertEq(usdc.balanceOf(investor1) - investor1BalanceBefore, expectedInvestor1Share);
        assertEq(usdc.balanceOf(investor2) - investor2BalanceBefore, expectedInvestor2Share);

        // Verify profit split contract is empty
        assertEq(usdc.balanceOf(address(profitSplit)), 0);
        console.log("SUCCESS: Profits distributed to all stakeholders");
    }

    /**
     * @dev Test multiple business cycles to verify system sustainability
     */
    function testMultipleBusinessCycles() public {
        console.log("=== TESTING MULTIPLE BUSINESS CYCLES ===");

        // Register referrer
        vm.prank(referrer);
        referrals.registerBasicReferrer("BASIC123");

        // Register business
        vm.prank(businessOwner);
        payments.registerBusiness("Test Restaurant", businessOwner, businessOwner, "BASIC123");

        uint256 totalPlatformFees = 0;
        uint256 totalBusinessEarnings = 0;

        // Process 5 bills
        for (uint256 i = 1; i <= 5; i++) {
            bytes32 billId = keccak256(abi.encodePacked("bill-", i));
            uint256 billAmount = 500 * 10 ** 6 * i; // Increasing amounts

            // Wait for rate limit window to pass
            vm.warp(block.timestamp + 61);

            // Create bill
            vm.prank(admin);
            payments.createBill(billId, businessOwner, billAmount, "{}", keccak256(abi.encodePacked("nonce", i)));

            // Customer pays
            vm.prank(customer);
            payments.processPayment(billId, billAmount, 0);

            uint256 platformFee = (billAmount * PLATFORM_FEE_RATE) / 10000;
            totalPlatformFees += platformFee;
            totalBusinessEarnings += billAmount - platformFee;
        }

        // Business claims all earnings
        uint256 businessBalanceBefore = usdc.balanceOf(businessOwner);
        vm.prank(businessOwner);
        payments.claimEarnings();

        // Verify total earnings
        assertEq(usdc.balanceOf(businessOwner) - businessBalanceBefore, totalBusinessEarnings);

        // Verify total platform fees in profit split contract
        assertEq(usdc.balanceOf(address(profitSplit)), totalPlatformFees + (REGISTRATION_FEE * 9000 / 10000)); // Registration fee minus 10% discount

        console.log("SUCCESS: Multiple business cycles processed correctly");
        console.log("=== MULTIPLE BUSINESS CYCLES TEST PASSED ===");
    }

    /**
     * @dev Test system behavior without profit split contract
     */
    function testWithoutProfitSplitContract() public {
        console.log("=== TESTING WITHOUT PROFIT SPLIT CONTRACT ===");

        // Disconnect profit split contract
        vm.prank(admin);
        payments.setProfitSplitContract(address(0));

        // Register business without referral
        vm.prank(businessOwner);
        payments.registerBusiness("Test Restaurant", businessOwner, businessOwner, "");

        // Create and pay bill
        bytes32 billId = keccak256("no-profit-split-bill");

        // Wait for rate limit window to pass
        vm.warp(block.timestamp + 61);

        vm.prank(admin);
        payments.createBill(billId, businessOwner, BILL_AMOUNT, "{}", keccak256("nonce-no-split"));

        uint256 contractBalanceBefore = usdc.balanceOf(address(payments));

        vm.prank(customer);
        payments.processPayment(billId, BILL_AMOUNT, 0);

        // Verify fees remain in payments contract
        uint256 expectedPlatformFee = (BILL_AMOUNT * PLATFORM_FEE_RATE) / 10000;
        assertEq(usdc.balanceOf(address(payments)) - contractBalanceBefore, BILL_AMOUNT);

        // Business can still claim earnings
        vm.prank(businessOwner);
        payments.claimEarnings();

        // Platform fees should remain in contract
        assertEq(usdc.balanceOf(address(payments)), expectedPlatformFee + REGISTRATION_FEE);

        console.log("SUCCESS: System works correctly without profit split contract");
        console.log("=== WITHOUT PROFIT SPLIT CONTRACT TEST PASSED ===");
    }

    /**
     * @dev Test contract version and upgrade compatibility
     */
    function testContractVersions() public view {
        assertEq(payments.version(), "2.1.0-profit-split-only");
        assertEq(referrals.version(), "1.0.0");
        assertEq(profitSplit.version(), "1.0.0");

        console.log("SUCCESS: All contract versions verified");
    }
}
