// SPDX-License-Identifier: MIT
pragma solidity ^0.8.19;

import "forge-std/Test.sol";
import "../src/PayvergePayments.sol";
import "@openzeppelin/contracts/token/ERC20/ERC20.sol";

// Mock USDC token for testing
contract MockUSDC is ERC20 {
    constructor() ERC20("USD Coin", "USDC") {
        _mint(msg.sender, 1000000 * 10**6); // 1M USDC with 6 decimals
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
    MockUSDC public usdc;
    
    address public owner;
    address public platformFeeRecipient;
    address public business;
    address public tipAddress;
    address public payer1;
    address public payer2;
    
    bytes32 public constant BILL_ID = keccak256("test-bill-1");
    uint256 public constant BILL_AMOUNT = 100 * 10**6; // $100 USDC
    uint256 public constant TIP_AMOUNT = 20 * 10**6;   // $20 USDC
    uint256 public constant PLATFORM_FEE_BPS = 200;    // 2%
    
    event PaymentMade(
        bytes32 indexed billId,
        address indexed payer,
        uint256 amount,
        uint256 tipAmount,
        address indexed businessAddress,
        address tipAddress,
        uint256 platformFee,
        uint256 timestamp
    );
    
    event BillCreated(
        bytes32 indexed billId,
        address indexed businessAddress,
        address tipAddress,
        uint256 totalAmount
    );
    
    function setUp() public {
        owner = address(this);
        platformFeeRecipient = makeAddr("platformFee");
        business = makeAddr("business");
        tipAddress = makeAddr("tipAddress");
        payer1 = makeAddr("payer1");
        payer2 = makeAddr("payer2");
        
        // Deploy mock USDC
        usdc = new MockUSDC();
        
        // Deploy PayvergePayments
        payverge = new PayvergePayments(address(usdc), platformFeeRecipient);
        
        // Mint USDC to payers
        usdc.mint(payer1, 1000 * 10**6); // $1000
        usdc.mint(payer2, 1000 * 10**6); // $1000
        
        // Approve PayvergePayments to spend USDC
        vm.prank(payer1);
        usdc.approve(address(payverge), type(uint256).max);
        
        vm.prank(payer2);
        usdc.approve(address(payverge), type(uint256).max);
    }
    
    function testConstructor() public view {
        assertEq(address(payverge.USDC()), address(usdc));
        assertEq(payverge.platformFeeRecipient(), platformFeeRecipient);
        assertEq(payverge.owner(), owner);
        assertEq(payverge.PLATFORM_FEE_BPS(), PLATFORM_FEE_BPS);
    }
    
    function testConstructorInvalidAddresses() public {
        // Test invalid USDC address
        vm.expectRevert(PayvergePayments.InvalidAddress.selector);
        new PayvergePayments(address(0), platformFeeRecipient);
        
        // Test invalid platform fee recipient
        vm.expectRevert(PayvergePayments.InvalidAddress.selector);
        new PayvergePayments(address(usdc), address(0));
    }
    
    function testCreateBill() public {
        vm.expectEmit(true, true, false, true);
        emit BillCreated(BILL_ID, business, tipAddress, BILL_AMOUNT);
        
        payverge.createBill(BILL_ID, business, tipAddress, BILL_AMOUNT);
        
        assertTrue(payverge.billExists(BILL_ID));
    }
    
    function testCreateBillInvalidInputs() public {
        // Invalid bill ID
        vm.expectRevert(PayvergePayments.InvalidBillId.selector);
        payverge.createBill(bytes32(0), business, tipAddress, BILL_AMOUNT);
        
        // Invalid business address
        vm.expectRevert(PayvergePayments.InvalidAddress.selector);
        payverge.createBill(BILL_ID, address(0), tipAddress, BILL_AMOUNT);
        
        // Invalid tip address
        vm.expectRevert(PayvergePayments.InvalidAddress.selector);
        payverge.createBill(BILL_ID, business, address(0), BILL_AMOUNT);
        
        // Invalid amount
        vm.expectRevert(PayvergePayments.InvalidAmount.selector);
        payverge.createBill(BILL_ID, business, tipAddress, 0);
    }
    
    function testCreateBillOnlyOwner() public {
        vm.prank(payer1);
        vm.expectRevert(abi.encodeWithSelector(Ownable.OwnableUnauthorizedAccount.selector, payer1));
        payverge.createBill(BILL_ID, business, tipAddress, BILL_AMOUNT);
    }
    
    function testPayBill() public {
        // Create bill first
        payverge.createBill(BILL_ID, business, tipAddress, BILL_AMOUNT);
        
        uint256 expectedPlatformFee = (BILL_AMOUNT * PLATFORM_FEE_BPS) / 10000;
        uint256 expectedBusinessAmount = BILL_AMOUNT - expectedPlatformFee;
        
        // Record initial balances
        uint256 businessBalanceBefore = usdc.balanceOf(business);
        uint256 tipBalanceBefore = usdc.balanceOf(tipAddress);
        uint256 platformBalanceBefore = usdc.balanceOf(platformFeeRecipient);
        uint256 payerBalanceBefore = usdc.balanceOf(payer1);
        
        // Expect payment event
        vm.expectEmit(true, true, true, true);
        emit PaymentMade(
            BILL_ID,
            payer1,
            BILL_AMOUNT,
            TIP_AMOUNT,
            business,
            tipAddress,
            expectedPlatformFee,
            block.timestamp
        );
        
        // Make payment
        vm.prank(payer1);
        payverge.payBill(BILL_ID, BILL_AMOUNT, TIP_AMOUNT, business, tipAddress);
        
        // Verify balances
        assertEq(usdc.balanceOf(business), businessBalanceBefore + expectedBusinessAmount);
        assertEq(usdc.balanceOf(tipAddress), tipBalanceBefore + TIP_AMOUNT);
        assertEq(usdc.balanceOf(platformFeeRecipient), platformBalanceBefore + expectedPlatformFee);
        assertEq(usdc.balanceOf(payer1), payerBalanceBefore - BILL_AMOUNT - TIP_AMOUNT);
        
        // Verify payment tracking
        assertEq(payverge.getBillTotalPaid(BILL_ID), BILL_AMOUNT);
        assertEq(payverge.getBillTotalTips(BILL_ID), TIP_AMOUNT);
        assertEq(payverge.getBillPaymentCount(BILL_ID), 1);
        
        // Verify payment details
        PayvergePayments.Payment[] memory payments = payverge.getBillPayments(BILL_ID);
        assertEq(payments.length, 1);
        assertEq(payments[0].payer, payer1);
        assertEq(payments[0].amount, BILL_AMOUNT);
        assertEq(payments[0].tipAmount, TIP_AMOUNT);
        assertEq(payments[0].businessAddress, business);
        assertEq(payments[0].tipAddress, tipAddress);
        assertEq(payments[0].platformFee, expectedPlatformFee);
    }
    
    function testPayBillMultiplePayments() public {
        payverge.createBill(BILL_ID, business, tipAddress, BILL_AMOUNT);
        
        uint256 payment1 = 60 * 10**6; // $60
        uint256 payment2 = 40 * 10**6; // $40
        uint256 tip1 = 10 * 10**6;     // $10
        uint256 tip2 = 10 * 10**6;     // $10
        
        // First payment
        vm.prank(payer1);
        payverge.payBill(BILL_ID, payment1, tip1, business, tipAddress);
        
        // Second payment
        vm.prank(payer2);
        payverge.payBill(BILL_ID, payment2, tip2, business, tipAddress);
        
        // Verify totals
        assertEq(payverge.getBillTotalPaid(BILL_ID), payment1 + payment2);
        assertEq(payverge.getBillTotalTips(BILL_ID), tip1 + tip2);
        assertEq(payverge.getBillPaymentCount(BILL_ID), 2);
        
        // Verify individual payments
        PayvergePayments.Payment[] memory payments = payverge.getBillPayments(BILL_ID);
        assertEq(payments.length, 2);
        assertEq(payments[0].payer, payer1);
        assertEq(payments[1].payer, payer2);
    }
    
    function testPayBillInvalidInputs() public {
        payverge.createBill(BILL_ID, business, tipAddress, BILL_AMOUNT);
        
        vm.startPrank(payer1);
        
        // Invalid bill ID
        vm.expectRevert(PayvergePayments.InvalidBillId.selector);
        payverge.payBill(bytes32(0), BILL_AMOUNT, TIP_AMOUNT, business, tipAddress);
        
        // Invalid amount
        vm.expectRevert(PayvergePayments.InvalidAmount.selector);
        payverge.payBill(BILL_ID, 0, TIP_AMOUNT, business, tipAddress);
        
        // Invalid business address
        vm.expectRevert(PayvergePayments.InvalidAddress.selector);
        payverge.payBill(BILL_ID, BILL_AMOUNT, TIP_AMOUNT, address(0), tipAddress);
        
        // Invalid tip address
        vm.expectRevert(PayvergePayments.InvalidAddress.selector);
        payverge.payBill(BILL_ID, BILL_AMOUNT, TIP_AMOUNT, business, address(0));
        
        vm.stopPrank();
    }
    
    function testPayBillInsufficientBalance() public {
        payverge.createBill(BILL_ID, business, tipAddress, BILL_AMOUNT);
        
        address poorPayer = makeAddr("poorPayer");
        usdc.mint(poorPayer, 50 * 10**6); // Only $50
        
        vm.prank(poorPayer);
        usdc.approve(address(payverge), type(uint256).max);
        
        vm.prank(poorPayer);
        vm.expectRevert(); // Updated to expect any revert for insufficient balance
        payverge.payBill(BILL_ID, BILL_AMOUNT, TIP_AMOUNT, business, tipAddress);
    }
    
    function testPayBillWhenPaused() public {
        payverge.createBill(BILL_ID, business, tipAddress, BILL_AMOUNT);
        payverge.pause();
        
        vm.prank(payer1);
        vm.expectRevert(Pausable.EnforcedPause.selector);
        payverge.payBill(BILL_ID, BILL_AMOUNT, TIP_AMOUNT, business, tipAddress);
    }
    
    function testBusinessVerification() public {
        assertFalse(payverge.verifiedBusinesses(business));
        
        vm.expectEmit(true, false, false, true);
        emit PayvergePayments.BusinessVerified(business, true);
        
        payverge.setBusinessVerification(business, true);
        assertTrue(payverge.verifiedBusinesses(business));
        
        payverge.setBusinessVerification(business, false);
        assertFalse(payverge.verifiedBusinesses(business));
    }
    
    function testSetPlatformFeeRecipient() public {
        address newRecipient = makeAddr("newRecipient");
        
        vm.expectEmit(true, true, false, true);
        emit PayvergePayments.PlatformFeeRecipientUpdated(platformFeeRecipient, newRecipient);
        
        payverge.setPlatformFeeRecipient(newRecipient);
        assertEq(payverge.platformFeeRecipient(), newRecipient);
        
        // Test invalid address
        vm.expectRevert(PayvergePayments.InvalidAddress.selector);
        payverge.setPlatformFeeRecipient(address(0));
    }
    
    function testPauseUnpause() public {
        assertFalse(payverge.paused());
        
        payverge.pause();
        assertTrue(payverge.paused());
        
        payverge.unpause();
        assertFalse(payverge.paused());
    }
    
    function testEmergencyWithdraw() public {
        // Send some tokens to the contract
        usdc.transfer(address(payverge), 100 * 10**6);
        
        uint256 contractBalance = usdc.balanceOf(address(payverge));
        uint256 recipientBalanceBefore = usdc.balanceOf(platformFeeRecipient);
        
        payverge.emergencyWithdraw(address(usdc), contractBalance, platformFeeRecipient);
        
        assertEq(usdc.balanceOf(address(payverge)), 0);
        assertEq(usdc.balanceOf(platformFeeRecipient), recipientBalanceBefore + contractBalance);
    }
    
    function testVersion() public view {
        assertEq(payverge.version(), "1.0.0");
    }
    
    function testPlatformFeeCalculation() public {
        payverge.createBill(BILL_ID, business, tipAddress, BILL_AMOUNT);
        
        uint256 expectedFee = (BILL_AMOUNT * PLATFORM_FEE_BPS) / 10000;
        assertEq(expectedFee, 2 * 10**6); // 2% of $100 = $2
        
        uint256 platformBalanceBefore = usdc.balanceOf(platformFeeRecipient);
        
        vm.prank(payer1);
        payverge.payBill(BILL_ID, BILL_AMOUNT, 0, business, tipAddress);
        
        assertEq(usdc.balanceOf(platformFeeRecipient), platformBalanceBefore + expectedFee);
    }
    
    function testNoTipPayment() public {
        payverge.createBill(BILL_ID, business, tipAddress, BILL_AMOUNT);
        
        vm.prank(payer1);
        payverge.payBill(BILL_ID, BILL_AMOUNT, 0, business, tipAddress);
        
        assertEq(payverge.getBillTotalTips(BILL_ID), 0);
        
        PayvergePayments.Payment[] memory payments = payverge.getBillPayments(BILL_ID);
        assertEq(payments[0].tipAmount, 0);
    }
    
    function testFuzzPayment(uint256 amount, uint256 tipAmount) public {
        // Bound inputs to reasonable ranges
        amount = bound(amount, 1 * 10**6, 10000 * 10**6); // $1 to $10,000
        tipAmount = bound(tipAmount, 0, 1000 * 10**6);     // $0 to $1,000
        
        // Ensure payer has enough balance
        usdc.mint(payer1, amount + tipAmount + 1000 * 10**6);
        
        payverge.createBill(BILL_ID, business, tipAddress, amount + tipAmount);
        
        uint256 expectedPlatformFee = (amount * PLATFORM_FEE_BPS) / 10000;
        uint256 expectedBusinessAmount = amount - expectedPlatformFee;
        
        uint256 businessBalanceBefore = usdc.balanceOf(business);
        uint256 tipBalanceBefore = usdc.balanceOf(tipAddress);
        uint256 platformBalanceBefore = usdc.balanceOf(platformFeeRecipient);
        
        vm.prank(payer1);
        payverge.payBill(BILL_ID, amount, tipAmount, business, tipAddress);
        
        assertEq(usdc.balanceOf(business), businessBalanceBefore + expectedBusinessAmount);
        assertEq(usdc.balanceOf(tipAddress), tipBalanceBefore + tipAmount);
        assertEq(usdc.balanceOf(platformFeeRecipient), platformBalanceBefore + expectedPlatformFee);
    }
}
