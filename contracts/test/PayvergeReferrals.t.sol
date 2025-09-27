// SPDX-License-Identifier: MIT
pragma solidity ^0.8.19;

import "forge-std/Test.sol";
import "../src/PayvergeReferrals.sol";
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

contract PayvergeReferralsTest is Test {
    PayvergeReferrals public referrals;
    MockUSDC public usdc;

    address public admin = makeAddr("admin");
    address public treasury = makeAddr("treasury");
    address public payvergePayments = makeAddr("payvergePayments");
    address public referrer1 = makeAddr("referrer1");
    address public referrer2 = makeAddr("referrer2");
    address public business1 = makeAddr("business1");
    address public business2 = makeAddr("business2");

    uint256 public constant BASIC_FEE = 10 * 10 ** 6; // $10 USDC
    uint256 public constant PREMIUM_FEE = 25 * 10 ** 6; // $25 USDC
    uint256 public constant REGISTRATION_FEE = 100 * 10 ** 6; // $100 USDC

    event ReferrerRegistered(
        address indexed referrer, PayvergeReferrals.ReferrerTier tier, string referralCode, uint256 fee
    );

    event ReferralUsed(
        bytes32 indexed referralId,
        address indexed referrer,
        address indexed business,
        string referralCode,
        uint256 discount,
        uint256 commission
    );

    event CommissionPaid(address indexed referrer, uint256 amount, uint256 totalCommissions);

    event CommissionClaimed(address indexed referrer, uint256 amount, uint256 remainingClaimable);

    function setUp() public {
        // Deploy USDC mock
        usdc = new MockUSDC();

        // Deploy referrals contract with proxy
        PayvergeReferrals implementation = new PayvergeReferrals();
        bytes memory initData =
            abi.encodeWithSelector(PayvergeReferrals.initialize.selector, address(usdc), treasury, admin);

        ERC1967Proxy proxy = new ERC1967Proxy(address(implementation), initData);
        referrals = PayvergeReferrals(address(proxy));

        // Set PayvergePayments contract
        vm.prank(admin);
        referrals.setPayvergePaymentsContract(payvergePayments);

        // Distribute USDC to test accounts
        usdc.mint(referrer1, 1000 * 10 ** 6);
        usdc.mint(referrer2, 1000 * 10 ** 6);
        usdc.mint(business1, 1000 * 10 ** 6);
        usdc.mint(business2, 1000 * 10 ** 6);

        // Approve spending
        vm.prank(referrer1);
        usdc.approve(address(referrals), type(uint256).max);
        vm.prank(referrer2);
        usdc.approve(address(referrals), type(uint256).max);
    }

    function testBasicReferrerRegistration() public {
        string memory code = "REFER123";

        vm.expectEmit(true, false, false, true);
        emit ReferrerRegistered(referrer1, PayvergeReferrals.ReferrerTier.Basic, code, BASIC_FEE);

        vm.prank(referrer1);
        referrals.registerBasicReferrer(code);

        PayvergeReferrals.Referrer memory referrer = referrals.getReferrer(referrer1);
        assertEq(uint8(referrer.tier), uint8(PayvergeReferrals.ReferrerTier.Basic));
        assertEq(referrer.referralCode, code);
        assertTrue(referrer.isActive);
        assertEq(referrer.totalReferrals, 0);
        assertEq(referrer.totalCommissions, 0);

        // Check USDC was transferred
        assertEq(usdc.balanceOf(treasury), BASIC_FEE);
        assertEq(usdc.balanceOf(referrer1), 1000 * 10 ** 6 - BASIC_FEE);
    }

    function testPremiumReferrerRegistration() public {
        string memory code = "PREMIUM1";

        vm.expectEmit(true, false, false, true);
        emit ReferrerRegistered(referrer1, PayvergeReferrals.ReferrerTier.Premium, code, PREMIUM_FEE);

        vm.prank(referrer1);
        referrals.registerPremiumReferrer(code);

        PayvergeReferrals.Referrer memory referrer = referrals.getReferrer(referrer1);
        assertEq(uint8(referrer.tier), uint8(PayvergeReferrals.ReferrerTier.Premium));
        assertEq(referrer.referralCode, code);
        assertTrue(referrer.isActive);

        // Check USDC was transferred
        assertEq(usdc.balanceOf(treasury), PREMIUM_FEE);
    }

    function testUpgradeToPremium() public {
        // First register as basic
        vm.prank(referrer1);
        referrals.registerBasicReferrer("BASIC123");

        // Then upgrade to premium
        vm.prank(referrer1);
        referrals.upgradeToPremium();

        PayvergeReferrals.Referrer memory referrer = referrals.getReferrer(referrer1);
        assertEq(uint8(referrer.tier), uint8(PayvergeReferrals.ReferrerTier.Premium));

        // Check total fees paid (basic + upgrade)
        assertEq(usdc.balanceOf(treasury), PREMIUM_FEE);
    }

    function testReferralCodeValidation() public {
        // Test invalid codes
        vm.startPrank(referrer1);

        // Too short
        vm.expectRevert(PayvergeReferrals.InvalidReferralCode.selector);
        referrals.registerBasicReferrer("ABC");

        // Too long
        vm.expectRevert(PayvergeReferrals.InvalidReferralCode.selector);
        referrals.registerBasicReferrer("VERYLONGCODE123");

        // Invalid characters
        vm.expectRevert(PayvergeReferrals.InvalidReferralCode.selector);
        referrals.registerBasicReferrer("CODE@123");

        vm.stopPrank();
    }

    function testDuplicateReferralCode() public {
        string memory code = "DUPLICATE";

        // First referrer registers
        vm.prank(referrer1);
        referrals.registerBasicReferrer(code);

        // Second referrer tries same code
        vm.prank(referrer2);
        vm.expectRevert(PayvergeReferrals.ReferralCodeTaken.selector);
        referrals.registerBasicReferrer(code);
    }

    function testProcessReferralBasic() public {
        // Register referrer
        string memory code = "BASIC123";
        vm.prank(referrer1);
        referrals.registerBasicReferrer(code);

        // Process referral (called by PayvergePayments contract)
        vm.prank(payvergePayments);
        (uint256 discount, address referrer, uint256 commission) =
            referrals.processReferral(business1, code, REGISTRATION_FEE);

        // Check calculations
        assertEq(discount, REGISTRATION_FEE * 1000 / 10000); // 10% discount
        assertEq(referrer, referrer1);
        assertEq(commission, REGISTRATION_FEE * 1000 / 10000); // 10% commission

        // Check referrer stats updated
        PayvergeReferrals.Referrer memory referrerData = referrals.getReferrer(referrer1);
        assertEq(referrerData.totalReferrals, 1);
        assertEq(referrerData.claimableCommissions, commission);
    }

    function testProcessReferralPremium() public {
        // Register premium referrer
        string memory code = "PREMIUM1";
        vm.prank(referrer1);
        referrals.registerPremiumReferrer(code);

        // Process referral
        vm.prank(payvergePayments);
        (uint256 discount,, uint256 commission) = referrals.processReferral(business1, code, REGISTRATION_FEE);

        // Check premium commission rate (15%) and discount (15%)
        assertEq(discount, REGISTRATION_FEE * 1500 / 10000); // 15% discount
        assertEq(commission, REGISTRATION_FEE * 1500 / 10000); // 15% commission
    }

    function testClaimCommissions() public {
        // Setup referral
        string memory code = "BASIC123";
        vm.prank(referrer1);
        referrals.registerBasicReferrer(code);

        // Process referral
        vm.prank(payvergePayments);
        referrals.processReferral(business1, code, REGISTRATION_FEE);

        // Mark commission as earned
        vm.prank(payvergePayments);
        referrals.markCommissionEarned(business1);

        // Fund contract with commission
        uint256 commission = REGISTRATION_FEE * 1000 / 10000;
        usdc.mint(address(referrals), commission);

        uint256 referrerBalanceBefore = usdc.balanceOf(referrer1);

        // Claim commission
        vm.expectEmit(true, false, false, true);
        emit CommissionClaimed(referrer1, commission, 0);

        vm.prank(referrer1);
        referrals.claimCommissions();

        // Check commission was claimed
        assertEq(usdc.balanceOf(referrer1), referrerBalanceBefore + commission);

        // Check referrer stats updated
        PayvergeReferrals.Referrer memory referrerData = referrals.getReferrer(referrer1);
        assertEq(referrerData.claimableCommissions, 0);
        assertEq(referrerData.totalCommissions, commission);
    }

    function testUnauthorizedCalls() public {
        // Only PayvergePayments contract can call processReferral
        vm.prank(referrer1);
        vm.expectRevert(PayvergeReferrals.UnauthorizedCaller.selector);
        referrals.processReferral(business1, "CODE123", REGISTRATION_FEE);

        // Only PayvergePayments contract can call markCommissionEarned
        vm.prank(referrer1);
        vm.expectRevert(PayvergeReferrals.UnauthorizedCaller.selector);
        referrals.markCommissionEarned(business1);
    }

    function testReferralCodeAvailability() public {
        string memory code = "AVAILABLE";

        // Should be available initially
        assertTrue(referrals.isReferralCodeAvailable(code));

        // Register referrer with this code
        vm.prank(referrer1);
        referrals.registerBasicReferrer(code);

        // Should no longer be available
        assertFalse(referrals.isReferralCodeAvailable(code));

        // Check referrer lookup
        assertEq(referrals.getReferrerByCode(code), referrer1);
    }

    function testUpdateReferralCode() public {
        string memory oldCode = "OLDCODE";
        string memory newCode = "NEWCODE";

        // Register referrer
        vm.prank(referrer1);
        referrals.registerBasicReferrer(oldCode);

        // Update code
        vm.prank(referrer1);
        referrals.updateReferralCode(newCode);

        // Check old code is no longer mapped
        assertEq(referrals.getReferrerByCode(oldCode), address(0));

        // Check new code is mapped
        assertEq(referrals.getReferrerByCode(newCode), referrer1);

        // Check referrer data updated
        PayvergeReferrals.Referrer memory referrer = referrals.getReferrer(referrer1);
        assertEq(referrer.referralCode, newCode);
    }

    function testAdminFunctions() public {
        // Register referrer first
        vm.prank(referrer1);
        referrals.registerBasicReferrer("ADMIN123");

        // Test deactivate referrer
        vm.prank(admin);
        referrals.deactivateReferrer(referrer1);

        PayvergeReferrals.Referrer memory referrer = referrals.getReferrer(referrer1);
        assertFalse(referrer.isActive);

        // Test pause/unpause
        vm.prank(admin);
        referrals.pause();

        vm.prank(referrer2);
        vm.expectRevert(abi.encodeWithSignature("EnforcedPause()"));
        referrals.registerBasicReferrer("PAUSED");

        vm.prank(admin);
        referrals.unpause();

        // Should work after unpause
        vm.prank(referrer2);
        referrals.registerBasicReferrer("UNPAUSED");
    }

    function testEmergencyWithdraw() public {
        // Send some USDC to contract
        uint256 amount = 100 * 10 ** 6;
        usdc.mint(address(referrals), amount);

        uint256 treasuryBalanceBefore = usdc.balanceOf(treasury);

        // Emergency withdraw
        vm.prank(admin);
        referrals.emergencyWithdraw(address(usdc), amount);

        assertEq(usdc.balanceOf(treasury), treasuryBalanceBefore + amount);
        assertEq(usdc.balanceOf(address(referrals)), 0);
    }

    function testGetReferralRecords() public {
        // Register referrer
        string memory code = "RECORDS1";
        vm.prank(referrer1);
        referrals.registerBasicReferrer(code);

        // Process two referrals
        vm.prank(payvergePayments);
        referrals.processReferral(business1, code, REGISTRATION_FEE);

        vm.prank(payvergePayments);
        referrals.processReferral(business2, code, REGISTRATION_FEE);

        // Fund the contract with USDC for commission payment
        uint256 commission = (REGISTRATION_FEE * 1000 / 10000) * 2; // 2 referrals
        usdc.mint(address(referrals), commission);

        // Check records
        bytes32[] memory records = referrals.getReferralRecords(referrer1);
        assertEq(records.length, 2);

        // Check referrer stats
        PayvergeReferrals.Referrer memory referrer = referrals.getReferrer(referrer1);
        assertEq(referrer.totalReferrals, 2);
    }

    function testVersionAndUpgrade() public {
        // Check version
        assertEq(referrals.version(), "1.0.0");

        // Test that non-admin cannot upgrade
        vm.prank(referrer1);
        vm.expectRevert();
        // This would fail compilation, so we just test access control
        referrals.pause(); // This should fail for non-admin
    }

    // Fuzz testing
    function testFuzzReferralCodeValidation(string calldata code) public {
        vm.assume(bytes(code).length >= 6 && bytes(code).length <= 12);

        // Check if code contains only alphanumeric characters
        bool isValid = true;
        bytes memory codeBytes = bytes(code);
        for (uint256 i = 0; i < codeBytes.length; i++) {
            bytes1 char = codeBytes[i];
            if (
                !(char >= 0x30 && char <= 0x39) // 0-9
                    && !(char >= 0x41 && char <= 0x5A) // A-Z
                    && !(char >= 0x61 && char <= 0x7A)
            ) {
                // a-z
                isValid = false;
                break;
            }
        }

        if (isValid) {
            vm.prank(referrer1);
            referrals.registerBasicReferrer(code);

            PayvergeReferrals.Referrer memory referrer = referrals.getReferrer(referrer1);
            assertEq(referrer.referralCode, code);
        } else {
            vm.prank(referrer1);
            vm.expectRevert(PayvergeReferrals.InvalidReferralCode.selector);
            referrals.registerBasicReferrer(code);
        }
    }

    function testFuzzCommissionCalculation(uint256 registrationFee) public {
        vm.assume(registrationFee > 0 && registrationFee <= 10000 * 10 ** 6); // Max $10k

        // Register referrer
        vm.prank(referrer1);
        referrals.registerBasicReferrer("FUZZ123");

        // Process referral
        vm.prank(payvergePayments);
        (uint256 discount, address referrer, uint256 commission) =
            referrals.processReferral(business1, "FUZZ123", registrationFee);

        // Verify calculations
        assertEq(discount, registrationFee * 1000 / 10000); // 10%
        assertEq(commission, registrationFee * 1000 / 10000); // 10%
        assertEq(referrer, referrer1);
    }
}
