// SPDX-License-Identifier: MIT
pragma solidity ^0.8.19;

import "forge-std/Test.sol";
import "../src/PayvergePayments.sol";
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

contract CouponsAndSubscriptionsTest is Test {
    PayvergePayments public payments;
    PayvergeProfitSplit public profitSplit;
    MockUSDC public usdc;

    address public admin = makeAddr("admin");
    address public billCreator = makeAddr("billCreator");
    address public billCreator2 = makeAddr("billCreator2");
    address public business1 = makeAddr("business1");
    address public business2 = makeAddr("business2");
    address public treasury = makeAddr("treasury");

    uint256 public constant PLATFORM_FEE_BPS = 200; // 2%
    uint256 public constant REGISTRATION_FEE = 100 * 10 ** 6; // $100 USDC
    uint256 public constant SECONDS_PER_YEAR = 365 days;

    // Events
    event CouponCreated(bytes32 indexed couponHash, uint256 discountAmount, uint64 expiryTime);
    event CouponDeactivated(bytes32 indexed couponHash);
    event CouponUsed(bytes32 indexed couponHash, address indexed business, uint256 discountAmount);
    event BusinessRegisteredWithCoupon(
        address indexed businessAddress,
        string name,
        address paymentAddress,
        address tippingAddress,
        uint256 originalFee,
        uint256 discount,
        bytes32 indexed couponHash
    );
    event BusinessSubscriptionRenewed(address indexed business, uint256 paymentAmount, uint64 newExpiryTime);
    event BusinessSubscriptionRenewedWithCoupon(
        address indexed business, 
        uint256 originalAmount, 
        uint256 discountAmount, 
        uint64 newExpiryTime, 
        bytes32 indexed couponHash
    );

    function setUp() public {
        // Deploy USDC mock
        usdc = new MockUSDC();

        // Deploy PayvergePayments with proxy
        PayvergePayments paymentsImpl = new PayvergePayments();
        bytes memory initData = abi.encodeWithSelector(
            PayvergePayments.initialize.selector,
            address(usdc),
            PLATFORM_FEE_BPS,
            admin,
            billCreator,
            REGISTRATION_FEE
        );
        ERC1967Proxy paymentsProxy = new ERC1967Proxy(address(paymentsImpl), initData);
        payments = PayvergePayments(address(paymentsProxy));

        // Deploy PayvergeProfitSplit with proxy
        PayvergeProfitSplit profitSplitImpl = new PayvergeProfitSplit();
        bytes memory profitInitData = abi.encodeWithSelector(
            PayvergeProfitSplit.initialize.selector,
            address(usdc),
            admin
        );
        ERC1967Proxy profitSplitProxy = new ERC1967Proxy(address(profitSplitImpl), profitInitData);
        profitSplit = PayvergeProfitSplit(payable(address(profitSplitProxy)));

        // Set profit split contract in payments
        vm.prank(admin);
        payments.setProfitSplitContract(address(profitSplit));

        // Grant bill creator roles
        vm.startPrank(admin);
        payments.grantRole(payments.BILL_MANAGER_ROLE(), billCreator2);
        vm.stopPrank();

        // Distribute USDC to test accounts
        usdc.mint(business1, 1000 * 10 ** 6);
        usdc.mint(business2, 1000 * 10 ** 6);
        usdc.mint(treasury, 1000 * 10 ** 6);
    }

    // ============ COUPON SYSTEM TESTS ============

    function testCreateCoupon() public {
        string memory couponCode = "LAUNCH50";
        uint256 discountAmount = 50 * 10 ** 6; // $50
        uint64 expiryTime = uint64(block.timestamp + 30 days);
        bytes32 expectedHash = keccak256(abi.encodePacked(couponCode));

        vm.expectEmit(true, false, false, true);
        emit CouponCreated(expectedHash, discountAmount, expiryTime);

        vm.prank(admin);
        payments.createCoupon(couponCode, discountAmount, expiryTime);

        // Verify coupon was created
        assertTrue(payments.couponExists(expectedHash));
        
        PayvergePayments.CouponInfo memory coupon = payments.getCouponInfo(expectedHash);
        assertEq(coupon.discountAmount, discountAmount);
        assertEq(coupon.expiryTime, expiryTime);
        assertFalse(coupon.isUsed);
        assertTrue(coupon.isActive);
    }

    function testCreateCouponInvalidInputs() public {
        // Empty coupon code
        vm.prank(admin);
        vm.expectRevert("Empty coupon code");
        payments.createCoupon("", 50 * 10 ** 6, uint64(block.timestamp + 30 days));

        // Zero discount amount
        vm.prank(admin);
        vm.expectRevert("Invalid discount amount");
        payments.createCoupon("TEST", 0, uint64(block.timestamp + 30 days));

        // Past expiry time
        vm.prank(admin);
        vm.expectRevert("Expiry time must be in future");
        payments.createCoupon("TEST", 50 * 10 ** 6, uint64(block.timestamp - 1));
    }

    function testCreateDuplicateCoupon() public {
        string memory couponCode = "DUPLICATE";
        uint256 discountAmount = 25 * 10 ** 6;
        uint64 expiryTime = uint64(block.timestamp + 30 days);

        // Create first coupon
        vm.prank(admin);
        payments.createCoupon(couponCode, discountAmount, expiryTime);

        // Try to create duplicate
        vm.prank(admin);
        vm.expectRevert("Coupon already exists");
        payments.createCoupon(couponCode, discountAmount, expiryTime);
    }

    function testDeactivateCoupon() public {
        string memory couponCode = "DEACTIVATE";
        uint256 discountAmount = 30 * 10 ** 6;
        uint64 expiryTime = uint64(block.timestamp + 30 days);
        bytes32 couponHash = keccak256(abi.encodePacked(couponCode));

        // Create coupon
        vm.prank(admin);
        payments.createCoupon(couponCode, discountAmount, expiryTime);

        // Deactivate coupon
        vm.expectEmit(true, false, false, false);
        emit CouponDeactivated(couponHash);

        vm.prank(admin);
        payments.deactivateCoupon(couponCode);

        // Verify coupon is deactivated
        PayvergePayments.CouponInfo memory coupon = payments.getCouponInfo(couponHash);
        assertFalse(coupon.isActive);
    }

    function testDeactivateNonExistentCoupon() public {
        vm.prank(admin);
        vm.expectRevert(abi.encodeWithSelector(PayvergePayments.CouponNotFound.selector, keccak256(abi.encodePacked("NONEXISTENT"))));
        payments.deactivateCoupon("NONEXISTENT");
    }

    function testCouponAccessControl() public {
        // Non-admin cannot create coupons
        vm.prank(business1);
        vm.expectRevert();
        payments.createCoupon("UNAUTHORIZED", 50 * 10 ** 6, uint64(block.timestamp + 30 days));

        // Non-admin cannot deactivate coupons
        vm.prank(business1);
        vm.expectRevert();
        payments.deactivateCoupon("UNAUTHORIZED");
    }

    function testRegisterBusinessWithCoupon() public {
        // Create coupon
        string memory couponCode = "REGISTER50";
        uint256 discountAmount = 50 * 10 ** 6; // $50 discount
        uint64 expiryTime = uint64(block.timestamp + 30 days);
        bytes32 couponHash = keccak256(abi.encodePacked(couponCode));

        vm.prank(admin);
        payments.createCoupon(couponCode, discountAmount, expiryTime);

        // Business registers with coupon
        uint256 finalFee = REGISTRATION_FEE - discountAmount; // $50
        
        vm.startPrank(business1);
        usdc.approve(address(payments), finalFee);

        vm.expectEmit(true, true, false, true);
        emit BusinessRegisteredWithCoupon(
            business1,
            "Test Restaurant",
            business1,
            business1,
            REGISTRATION_FEE,
            discountAmount,
            couponHash
        );

        payments.registerBusinessWithCoupon(
            "Test Restaurant",
            business1,
            business1,
            couponCode
        );
        vm.stopPrank();

        // Verify business was registered
        PayvergePayments.BusinessInfo memory businessInfo = payments.getBusinessInfo(business1);
        assertTrue(businessInfo.isActive);
        assertEq(businessInfo.paymentAddress, business1);
        
        // Verify subscription time (should be full year despite discount)
        uint64 expectedExpiry = uint64(block.timestamp + SECONDS_PER_YEAR);
        assertEq(businessInfo.subscriptionExpiry, expectedExpiry);

        // Verify coupon was marked as used
        PayvergePayments.CouponInfo memory coupon = payments.getCouponInfo(couponHash);
        assertTrue(coupon.isUsed);

        // Verify payment was made
        assertEq(usdc.balanceOf(business1), 1000 * 10 ** 6 - finalFee);
    }

    function testRegisterBusinessWithExpiredCoupon() public {
        // Create expired coupon
        string memory couponCode = "EXPIRED";
        uint256 discountAmount = 25 * 10 ** 6;
        uint64 expiryTime = uint64(block.timestamp + 1 hours);
        bytes32 couponHash = keccak256(abi.encodePacked(couponCode));

        vm.prank(admin);
        payments.createCoupon(couponCode, discountAmount, expiryTime);

        // Fast forward past expiry
        vm.warp(block.timestamp + 2 hours);

        // Try to register with expired coupon
        vm.startPrank(business1);
        usdc.approve(address(payments), REGISTRATION_FEE);
        
        vm.expectRevert(abi.encodeWithSelector(PayvergePayments.CouponExpired.selector, couponHash));
        payments.registerBusinessWithCoupon("Test", business1, business1, couponCode);
        vm.stopPrank();
    }

    function testRegisterBusinessWithUsedCoupon() public {
        // Create coupon
        string memory couponCode = "ONETIME";
        uint256 discountAmount = 30 * 10 ** 6;
        uint64 expiryTime = uint64(block.timestamp + 30 days);
        bytes32 couponHash = keccak256(abi.encodePacked(couponCode));

        vm.prank(admin);
        payments.createCoupon(couponCode, discountAmount, expiryTime);

        // First business uses coupon
        vm.startPrank(business1);
        usdc.approve(address(payments), REGISTRATION_FEE - discountAmount);
        payments.registerBusinessWithCoupon("Business 1", business1, business1, couponCode);
        vm.stopPrank();

        // Second business tries to use same coupon
        vm.startPrank(business2);
        usdc.approve(address(payments), REGISTRATION_FEE - discountAmount);
        
        vm.expectRevert(abi.encodeWithSelector(PayvergePayments.CouponAlreadyUsed.selector, couponHash));
        payments.registerBusinessWithCoupon("Business 2", business2, business2, couponCode);
        vm.stopPrank();
    }

    function testRegisterBusinessWithInactiveCoupon() public {
        // Create and deactivate coupon
        string memory couponCode = "INACTIVE";
        uint256 discountAmount = 40 * 10 ** 6;
        uint64 expiryTime = uint64(block.timestamp + 30 days);
        bytes32 couponHash = keccak256(abi.encodePacked(couponCode));

        vm.prank(admin);
        payments.createCoupon(couponCode, discountAmount, expiryTime);
        
        vm.prank(admin);
        payments.deactivateCoupon(couponCode);

        // Try to register with inactive coupon
        vm.startPrank(business1);
        usdc.approve(address(payments), REGISTRATION_FEE - discountAmount);
        
        vm.expectRevert(abi.encodeWithSelector(PayvergePayments.CouponNotActive.selector, couponHash));
        payments.registerBusinessWithCoupon("Test", business1, business1, couponCode);
        vm.stopPrank();
    }

    // ============ SUBSCRIPTION SYSTEM TESTS ============

    function testCalculateSubscriptionTime() public view {
        // Full payment = full year
        uint256 fullTime = payments.calculateSubscriptionTime(REGISTRATION_FEE);
        assertEq(fullTime, SECONDS_PER_YEAR);

        // Half payment = half year
        uint256 halfTime = payments.calculateSubscriptionTime(REGISTRATION_FEE / 2);
        assertEq(halfTime, SECONDS_PER_YEAR / 2);

        // Zero payment = zero time
        uint256 zeroTime = payments.calculateSubscriptionTime(0);
        assertEq(zeroTime, 0);

        // Over payment = still full year
        uint256 overTime = payments.calculateSubscriptionTime(REGISTRATION_FEE * 2);
        assertEq(overTime, SECONDS_PER_YEAR);
    }

    function testCalculatePaymentForTime() public view {
        // Full year = full payment
        uint256 fullPayment = payments.calculatePaymentForTime(SECONDS_PER_YEAR);
        assertEq(fullPayment, REGISTRATION_FEE);

        // Half year = half payment
        uint256 halfPayment = payments.calculatePaymentForTime(SECONDS_PER_YEAR / 2);
        assertEq(halfPayment, REGISTRATION_FEE / 2);

        // Zero time = zero payment
        uint256 zeroPayment = payments.calculatePaymentForTime(0);
        assertEq(zeroPayment, 0);

        // Over year = still full payment
        uint256 overPayment = payments.calculatePaymentForTime(SECONDS_PER_YEAR * 2);
        assertEq(overPayment, REGISTRATION_FEE);

        // Very small time = minimum payment
        uint256 minPayment = payments.calculatePaymentForTime(1);
        assertEq(minPayment, 1 * 10 ** 6); // MIN_SUBSCRIPTION_PAYMENT
    }

    function testGetBusinessSubscriptionStatus() public {
        // Register business first
        vm.startPrank(business1);
        usdc.approve(address(payments), REGISTRATION_FEE);
        payments.registerBusiness("Test Business", business1, business1, "");
        vm.stopPrank();

        // Check subscription status
        (bool isActive, uint64 subscriptionExpiry, uint256 timeRemaining) = 
            payments.getBusinessSubscriptionStatus(business1);
        
        assertTrue(isActive);
        assertEq(subscriptionExpiry, uint64(block.timestamp + SECONDS_PER_YEAR));
        assertEq(timeRemaining, SECONDS_PER_YEAR);

        // Fast forward to near expiry
        vm.warp(block.timestamp + SECONDS_PER_YEAR - 1 days);
        
        (, , timeRemaining) = payments.getBusinessSubscriptionStatus(business1);
        assertEq(timeRemaining, 1 days);

        // Fast forward past expiry
        vm.warp(block.timestamp + 2 days);
        
        (, , timeRemaining) = payments.getBusinessSubscriptionStatus(business1);
        assertEq(timeRemaining, 0);
    }

    function testRenewSubscription() public {
        // Register business first
        vm.startPrank(business1);
        usdc.approve(address(payments), REGISTRATION_FEE);
        payments.registerBusiness("Test Business", business1, business1, "");
        vm.stopPrank();

        // Fast forward to near expiry
        vm.warp(block.timestamp + SECONDS_PER_YEAR - 30 days);

        // Renew subscription for 6 months
        uint256 renewalAmount = REGISTRATION_FEE / 2; // 6 months
        uint64 currentExpiry = payments.getBusinessInfo(business1).subscriptionExpiry;
        uint64 expectedNewExpiry = uint64(currentExpiry + SECONDS_PER_YEAR / 2);

        vm.startPrank(business1);
        usdc.approve(address(payments), renewalAmount);

        vm.expectEmit(true, false, false, true);
        emit BusinessSubscriptionRenewed(business1, renewalAmount, expectedNewExpiry);

        payments.renewSubscription(renewalAmount);
        vm.stopPrank();

        // Verify subscription was extended
        PayvergePayments.BusinessInfo memory businessInfo = payments.getBusinessInfo(business1);
        assertEq(businessInfo.subscriptionExpiry, expectedNewExpiry);
    }

    function testRenewExpiredSubscription() public {
        // Register business first
        vm.startPrank(business1);
        usdc.approve(address(payments), REGISTRATION_FEE);
        payments.registerBusiness("Test Business", business1, business1, "");
        vm.stopPrank();

        // Fast forward past expiry
        vm.warp(block.timestamp + SECONDS_PER_YEAR + 10 days);

        // Renew subscription for 3 months
        uint256 renewalAmount = REGISTRATION_FEE / 4; // 3 months
        uint64 expectedNewExpiry = uint64(block.timestamp + SECONDS_PER_YEAR / 4);

        vm.startPrank(business1);
        usdc.approve(address(payments), renewalAmount);
        payments.renewSubscription(renewalAmount);
        vm.stopPrank();

        // Verify subscription starts from current time (not old expiry)
        PayvergePayments.BusinessInfo memory businessInfo = payments.getBusinessInfo(business1);
        assertEq(businessInfo.subscriptionExpiry, expectedNewExpiry);
    }

    function testRenewSubscriptionWithCoupon() public {
        // Register business first
        vm.startPrank(business1);
        usdc.approve(address(payments), REGISTRATION_FEE);
        payments.registerBusiness("Test Business", business1, business1, "");
        vm.stopPrank();

        // Create renewal coupon
        string memory couponCode = "RENEW25";
        uint256 discountAmount = 25 * 10 ** 6; // $25 discount
        uint64 expiryTime = uint64(block.timestamp + 30 days);
        bytes32 couponHash = keccak256(abi.encodePacked(couponCode));

        vm.prank(admin);
        payments.createCoupon(couponCode, discountAmount, expiryTime);

        // Renew with coupon
        uint256 originalAmount = REGISTRATION_FEE / 2; // $50 for 6 months
        uint256 finalPayment = originalAmount - discountAmount; // $25 actual payment
        uint64 currentExpiry = payments.getBusinessInfo(business1).subscriptionExpiry;
        uint64 expectedNewExpiry = uint64(currentExpiry + SECONDS_PER_YEAR / 2); // 6 months based on original amount

        vm.startPrank(business1);
        usdc.approve(address(payments), finalPayment);

        vm.expectEmit(true, false, false, true);
        emit BusinessSubscriptionRenewedWithCoupon(
            business1,
            originalAmount,
            discountAmount,
            expectedNewExpiry,
            couponHash
        );

        payments.renewSubscriptionWithCoupon(originalAmount, couponCode);
        vm.stopPrank();

        // Verify subscription was extended based on original amount
        PayvergePayments.BusinessInfo memory businessInfo = payments.getBusinessInfo(business1);
        assertEq(businessInfo.subscriptionExpiry, expectedNewExpiry);

        // Verify coupon was used
        PayvergePayments.CouponInfo memory coupon = payments.getCouponInfo(couponHash);
        assertTrue(coupon.isUsed);

        // Verify correct payment amount
        assertEq(usdc.balanceOf(business1), 1000 * 10 ** 6 - REGISTRATION_FEE - finalPayment);
    }

    function testRenewSubscriptionInvalidAmount() public {
        // Register business first
        vm.startPrank(business1);
        usdc.approve(address(payments), REGISTRATION_FEE);
        payments.registerBusiness("Test Business", business1, business1, "");
        vm.stopPrank();

        // Try to renew with amount below minimum
        vm.startPrank(business1);
        usdc.approve(address(payments), 1);
        
        vm.expectRevert(abi.encodeWithSelector(PayvergePayments.InvalidSubscriptionPayment.selector, 1));
        payments.renewSubscription(1);
        vm.stopPrank();
    }

    function testRenewUnregisteredBusiness() public {
        // Try to renew without being registered
        vm.startPrank(business1);
        usdc.approve(address(payments), REGISTRATION_FEE / 2);
        
        vm.expectRevert(abi.encodeWithSelector(PayvergePayments.ZeroAddress.selector));
        payments.renewSubscription(REGISTRATION_FEE / 2);
        vm.stopPrank();
    }

    function testSubscriptionExpiryCheck() public {
        // Register business
        vm.startPrank(business1);
        usdc.approve(address(payments), REGISTRATION_FEE);
        payments.registerBusiness("Test Business", business1, business1, "");
        vm.stopPrank();

        // Verify business has active subscription
        (bool isActive, uint64 subscriptionExpiry, uint256 timeRemaining) = 
            payments.getBusinessSubscriptionStatus(business1);
        assertTrue(isActive);
        assertEq(subscriptionExpiry, uint64(block.timestamp + SECONDS_PER_YEAR));
        assertEq(timeRemaining, SECONDS_PER_YEAR);

        // Fast forward past subscription expiry
        vm.warp(block.timestamp + SECONDS_PER_YEAR + 1 days);

        // Verify subscription is now expired
        (, , timeRemaining) = payments.getBusinessSubscriptionStatus(business1);
        assertEq(timeRemaining, 0);

        // Note: We can't easily test createBill with expired subscription due to rate limiting
        // but the subscription expiry check has been added to the createBill function
        // and is tested in the onlyActiveBusiness modifier tests
    }

    function testProfitSplitIntegration() public {
        // Add beneficiary to profit split
        vm.prank(admin);
        profitSplit.addBeneficiary(treasury, "Treasury", 10000); // 100%

        // Create coupon
        string memory couponCode = "PROFIT10";
        uint256 discountAmount = 10 * 10 ** 6;
        uint64 expiryTime = uint64(block.timestamp + 30 days);

        vm.prank(admin);
        payments.createCoupon(couponCode, discountAmount, expiryTime);

        // Register business with coupon
        uint256 finalFee = REGISTRATION_FEE - discountAmount; // $90
        
        vm.startPrank(business1);
        usdc.approve(address(payments), finalFee);
        payments.registerBusinessWithCoupon("Test", business1, business1, couponCode);
        vm.stopPrank();

        // Verify payment went to profit split contract
        assertEq(usdc.balanceOf(address(profitSplit)), finalFee);

        // Renew subscription
        uint256 renewalAmount = 50 * 10 ** 6;
        vm.startPrank(business1);
        usdc.approve(address(payments), renewalAmount);
        payments.renewSubscription(renewalAmount);
        vm.stopPrank();

        // Verify renewal payment also went to profit split
        assertEq(usdc.balanceOf(address(profitSplit)), finalFee + renewalAmount);
    }

    function testFuzzCouponAmounts(uint256 discountAmount) public {
        // Bound discount amount to reasonable range (max 90% of registration fee)
        discountAmount = bound(discountAmount, 1 * 10 ** 6, (REGISTRATION_FEE * 90) / 100);
        
        string memory couponCode = "FUZZ";
        uint64 expiryTime = uint64(block.timestamp + 30 days);

        vm.prank(admin);
        payments.createCoupon(couponCode, discountAmount, expiryTime);

        // Register business with coupon
        uint256 finalFee = REGISTRATION_FEE - discountAmount;
        // Ensure minimum payment
        if (finalFee < 1 * 10 ** 6) {
            finalFee = 1 * 10 ** 6;
        }
        
        vm.startPrank(business1);
        usdc.approve(address(payments), finalFee);
        payments.registerBusinessWithCoupon("Fuzz Test", business1, business1, couponCode);
        vm.stopPrank();

        // Verify business was registered with full year subscription
        PayvergePayments.BusinessInfo memory businessInfo = payments.getBusinessInfo(business1);
        assertTrue(businessInfo.isActive);
        assertEq(businessInfo.subscriptionExpiry, uint64(block.timestamp + SECONDS_PER_YEAR));
    }

    function testFuzzSubscriptionTimes(uint256 paymentAmount) public {
        // Bound payment amount to reasonable range
        paymentAmount = bound(paymentAmount, 1 * 10 ** 6, REGISTRATION_FEE * 2);

        // Register business first
        vm.startPrank(business1);
        usdc.approve(address(payments), REGISTRATION_FEE);
        payments.registerBusiness("Test Business", business1, business1, "");
        vm.stopPrank();

        // Calculate expected subscription time
        uint256 expectedTime = paymentAmount >= REGISTRATION_FEE ? 
            SECONDS_PER_YEAR : 
            (paymentAmount * SECONDS_PER_YEAR) / REGISTRATION_FEE;

        // Renew subscription
        vm.startPrank(business1);
        usdc.approve(address(payments), paymentAmount);
        payments.renewSubscription(paymentAmount);
        vm.stopPrank();

        // Verify subscription time calculation
        uint256 actualTime = payments.calculateSubscriptionTime(paymentAmount);
        assertEq(actualTime, expectedTime);
    }
}
