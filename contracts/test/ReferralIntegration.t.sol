// SPDX-License-Identifier: MIT
pragma solidity ^0.8.19;

import "forge-std/Test.sol";
import "../src/PayvergeReferrals.sol";
import "../src/PayvergePayments.sol";
import "@openzeppelin/contracts/token/ERC20/ERC20.sol";
import "@openzeppelin/contracts/proxy/ERC1967/ERC1967Proxy.sol";

contract MockUSDC is ERC20 {
    constructor() ERC20("Mock USDC", "USDC") {
        _mint(msg.sender, 1000000 * 10**6); // 1M USDC
    }

    function mint(address to, uint256 amount) external {
        _mint(to, amount);
    }

    function decimals() public pure override returns (uint8) {
        return 6;
    }
}

contract ReferralIntegrationTest is Test {
    PayvergeReferrals public referrals;
    PayvergePayments public payments;
    MockUSDC public usdc;
    
    address public admin = makeAddr("admin");
    address public treasury = makeAddr("treasury");
    address public referrer = makeAddr("referrer");
    address public business = makeAddr("business");
    address public paymentAddr = makeAddr("paymentAddr");
    address public tippingAddr = makeAddr("tippingAddr");

    uint256 public constant BASIC_FEE = 10 * 10**6; // $10 USDC
    uint256 public constant REGISTRATION_FEE = 100 * 10**6; // $100 USDC

    function setUp() public {
        // Deploy USDC mock
        usdc = new MockUSDC();
        
        // Deploy referrals contract
        PayvergeReferrals referralsImpl = new PayvergeReferrals();
        bytes memory referralsInitData = abi.encodeWithSelector(
            PayvergeReferrals.initialize.selector,
            address(usdc),
            treasury,
            admin
        );
        ERC1967Proxy referralsProxy = new ERC1967Proxy(address(referralsImpl), referralsInitData);
        referrals = PayvergeReferrals(address(referralsProxy));

        // Deploy payments contract
        PayvergePayments paymentsImpl = new PayvergePayments();
        bytes memory paymentsInitData = abi.encodeWithSelector(
            PayvergePayments.initialize.selector,
            address(usdc),
            treasury,
            200, // 2% platform fee
            admin,
            admin, // bill creator (same as admin for testing)
            REGISTRATION_FEE
        );
        ERC1967Proxy paymentsProxy = new ERC1967Proxy(address(paymentsImpl), paymentsInitData);
        payments = PayvergePayments(address(paymentsProxy));

        // Connect contracts
        vm.prank(admin);
        referrals.setPayvergePaymentsContract(address(payments));
        
        // This would be added to PayvergePayments contract
        // vm.prank(admin);
        // payments.setReferralsContract(address(referrals));

        // Distribute USDC
        usdc.mint(referrer, 1000 * 10**6);
        usdc.mint(business, 1000 * 10**6);

        // Approve spending
        vm.prank(referrer);
        usdc.approve(address(referrals), type(uint256).max);
        vm.prank(business);
        usdc.approve(address(payments), type(uint256).max);
    }

    function testEndToEndReferralFlow() public {
        string memory referralCode = "TESTCODE";
        
        // Step 1: Referrer registers
        vm.prank(referrer);
        referrals.registerBasicReferrer(referralCode);

        // Verify referrer registration
        PayvergeReferrals.Referrer memory referrerData = referrals.getReferrer(referrer);
        assertEq(uint8(referrerData.tier), uint8(PayvergeReferrals.ReferrerTier.Basic));
        assertEq(referrerData.referralCode, referralCode);
        assertTrue(referrerData.isActive);

        // Step 2: Simulate business registration with referral
        // (This would be integrated into PayvergePayments.registerBusiness)
        
        // Process referral
        vm.prank(address(payments));
        (uint256 discount, address referrerAddr, uint256 commission) = 
            referrals.processReferral(business, referralCode, REGISTRATION_FEE);

        // Verify referral processing
        assertEq(discount, REGISTRATION_FEE * 1000 / 10000); // 10% discount
        assertEq(referrerAddr, referrer);
        assertEq(commission, REGISTRATION_FEE * 1000 / 10000); // 10% commission

        // Step 3: Business pays discounted fee
        uint256 finalFee = REGISTRATION_FEE - discount;
        uint256 businessBalanceBefore = usdc.balanceOf(business);
        uint256 treasuryBalanceBefore = usdc.balanceOf(treasury);

        // Simulate payment (would be done in PayvergePayments.registerBusiness)
        vm.prank(business);
        usdc.transfer(treasury, finalFee);

        assertEq(usdc.balanceOf(business), businessBalanceBefore - finalFee);
        assertEq(usdc.balanceOf(treasury), treasuryBalanceBefore + finalFee);

        // Step 4: Mark commission as earned and let referrer claim
        vm.prank(address(payments));
        referrals.markCommissionEarned(business);

        // Verify commission is claimable
        referrerData = referrals.getReferrer(referrer);
        assertEq(referrerData.claimableCommissions, commission);

        // Fund referrals contract for commission payout
        usdc.mint(address(referrals), commission);

        // Referrer claims commission
        uint256 referrerBalanceBefore = usdc.balanceOf(referrer);
        vm.prank(referrer);
        referrals.claimCommissions();

        // Verify commission payment
        assertEq(usdc.balanceOf(referrer), referrerBalanceBefore + commission);

        // Verify referrer stats updated
        referrerData = referrals.getReferrer(referrer);
        assertEq(referrerData.totalReferrals, 1);
        assertEq(referrerData.totalCommissions, commission);
        assertEq(referrerData.claimableCommissions, 0);

        // Step 5: Verify overall economics
        uint256 totalPaid = finalFee; // Business paid discounted fee
        uint256 platformNet = finalFee - commission; // Platform net after commission

        console.log("=== Referral Economics ===");
        console.log("Original Registration Fee:", REGISTRATION_FEE);
        console.log("Business Discount:", discount);
        console.log("Business Paid:", finalFee);
        console.log("Referrer Commission:", commission);
        console.log("Platform Net Revenue:", platformNet);
        
        // Verify economics make sense
        assertEq(totalPaid + discount, REGISTRATION_FEE); // Discount applied correctly
        assertTrue(platformNet > 0); // Platform still profitable
        assertTrue(commission >= BASIC_FEE); // Referrer breaks even after 1 referral
    }

    function testMultipleReferrals() public {
        string memory referralCode = "MULTI123";
        address business2 = makeAddr("business2");
        
        // Setup second business
        usdc.mint(business2, 1000 * 10**6);
        vm.prank(business2);
        usdc.approve(address(payments), type(uint256).max);

        // Register referrer
        vm.prank(referrer);
        referrals.registerBasicReferrer(referralCode);

        // Process multiple referrals
        vm.startPrank(address(payments));
        referrals.processReferral(business, referralCode, REGISTRATION_FEE);
        referrals.processReferral(business2, referralCode, REGISTRATION_FEE);
        vm.stopPrank();

        // Verify referrer stats updated
        PayvergeReferrals.Referrer memory referrerData = referrals.getReferrer(referrer);
        assertEq(referrerData.totalReferrals, 2);
        
        uint256 expectedCommission = (REGISTRATION_FEE * 1000 / 10000) * 2; // 2 referrals
        assertEq(referrerData.claimableCommissions, expectedCommission);

        // Mark commissions as earned
        vm.prank(address(payments));
        referrals.markCommissionEarned(business);
        vm.prank(address(payments));
        referrals.markCommissionEarned(business2);

        // Fund referrals contract for claims
        usdc.mint(address(referrals), expectedCommission);
        
        // Referrer claims all commissions
        vm.prank(referrer);
        referrals.claimCommissions();

        // Verify final stats
        PayvergeReferrals.Referrer memory finalReferrerData = referrals.getReferrer(referrer);
        assertEq(finalReferrerData.totalCommissions, expectedCommission);
        assertEq(finalReferrerData.claimableCommissions, 0);
    }

    function testPremiumReferrerHigherCommission() public {
        string memory referralCode = "PREMIUM1";
        
        // Register premium referrer
        vm.prank(referrer);
        referrals.registerPremiumReferrer(referralCode);

        // Process referral
        vm.prank(address(payments));
        (uint256 discount, , uint256 commission) = 
            referrals.processReferral(business, referralCode, REGISTRATION_FEE);

        // Verify premium commission rate (15% vs 10% for basic) and discount (15% vs 10%)
        uint256 expectedCommission = REGISTRATION_FEE * 1500 / 10000; // 15%
        uint256 expectedDiscount = REGISTRATION_FEE * 1500 / 10000; // 15%
        assertEq(commission, expectedCommission);
        assertEq(discount, expectedDiscount); // 15% discount for premium
    }

    function testInvalidReferralCode() public {
        // Try to process referral with non-existent code
        vm.prank(address(payments));
        vm.expectRevert(PayvergeReferrals.ReferralNotFound.selector);
        referrals.processReferral(business, "INVALID", REGISTRATION_FEE);
    }

    function testUnauthorizedReferralProcessing() public {
        // Register referrer first
        vm.prank(referrer);
        referrals.registerBasicReferrer("AUTH123");

        // Try to process referral from unauthorized address
        vm.prank(business);
        vm.expectRevert(PayvergeReferrals.UnauthorizedCaller.selector);
        referrals.processReferral(business, "AUTH123", REGISTRATION_FEE);
    }

    function testReferrerDeactivation() public {
        string memory referralCode = "DEACTIVE";
        
        // Register referrer
        vm.prank(referrer);
        referrals.registerBasicReferrer(referralCode);

        // Deactivate referrer
        vm.prank(admin);
        referrals.deactivateReferrer(referrer);

        // Try to use deactivated referrer's code
        vm.prank(address(payments));
        vm.expectRevert(PayvergeReferrals.ReferrerNotFound.selector);
        referrals.processReferral(business, referralCode, REGISTRATION_FEE);
    }

    function testReferralRecordTracking() public {
        string memory referralCode = "RECORD1";
        
        // Register referrer
        vm.prank(referrer);
        referrals.registerBasicReferrer(referralCode);

        // Process referral
        vm.prank(address(payments));
        referrals.processReferral(business, referralCode, REGISTRATION_FEE);

        // Check referral records
        bytes32[] memory records = referrals.getReferralRecords(referrer);
        assertEq(records.length, 1);

        // Verify record details
        bytes32 recordId = records[0];
        (
            bytes32 id,
            address recordReferrer,
            address recordBusiness,
            ,
            uint256 registrationFee,
            uint256 discount,
            uint256 commission,
            bool commissionPaid
        ) = referrals.referralRecords(recordId);

        assertEq(id, recordId);
        assertEq(recordReferrer, referrer);
        assertEq(recordBusiness, business);
        assertEq(registrationFee, REGISTRATION_FEE);
        assertEq(discount, REGISTRATION_FEE * 1000 / 10000);
        assertEq(commission, REGISTRATION_FEE * 1000 / 10000);
        assertFalse(commissionPaid);
    }
}
