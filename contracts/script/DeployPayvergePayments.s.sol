// SPDX-License-Identifier: MIT
pragma solidity ^0.8.19;

import "forge-std/Script.sol";
import "../src/PayvergePayments.sol";
import "../src/PayvergeReferrals.sol";
import "../src/PayvergeProfitSplit.sol";
import "@openzeppelin/contracts/proxy/ERC1967/ERC1967Proxy.sol";

contract DeployPayvergeEcosystem is Script {
    function run() external {
        uint256 deployerPrivateKey = vm.envUint("PRIVATE_KEY");
        address deployer = vm.addr(deployerPrivateKey);

        // Get environment variables
        address usdcToken = vm.envAddress("USDC_TOKEN_ADDRESS");
        address platformFeeRecipient = vm.envAddress("PLATFORM_FEE_RECIPIENT");
        uint256 platformFeeBps = vm.envUint("PLATFORM_FEE_BPS"); // Default: 200 (2%)
        address billCreator = vm.envAddress("BILL_CREATOR_ADDRESS");
        uint256 registrationFee = vm.envUint("REGISTRATION_FEE"); // Default: 0 (free registration)

        console.log("Deployer:", deployer);
        console.log("USDC Token:", usdcToken);
        console.log("Platform Fee Recipient:", platformFeeRecipient);
        console.log("Platform Fee BPS:", platformFeeBps);
        console.log("Bill Creator:", billCreator);
        console.log("Registration Fee:", registrationFee);

        vm.startBroadcast(deployerPrivateKey);

        // Deploy implementation contract
        PayvergePayments implementation = new PayvergePayments();
        console.log("Implementation deployed at:", address(implementation));

        // Prepare initialization data
        bytes memory initData = abi.encodeWithSelector(
            PayvergePayments.initialize.selector,
            usdcToken,
            platformFeeBps,
            deployer, // admin
            billCreator,
            registrationFee
        );

        // Deploy proxy
        ERC1967Proxy proxy = new ERC1967Proxy(address(implementation), initData);

        console.log("Proxy deployed at:", address(proxy));

        // Deploy and setup referrals and profit split contracts
        (address referralsProxy, address profitSplitProxy) =
            _deployAuxiliaryContracts(usdcToken, platformFeeRecipient, deployer);

        // Connect contracts
        console.log("\n=== CONNECTING CONTRACTS ===");
        PayvergePayments payvergeContract = PayvergePayments(address(proxy));

        // Set referrals contract in PayvergePayments
        payvergeContract.setReferralsContract(referralsProxy);
        console.log("Set referrals contract in PayvergePayments");

        // Set profit split contract in PayvergePayments
        payvergeContract.setProfitSplitContract(profitSplitProxy);
        console.log("Set profit split contract in PayvergePayments");

        // Set PayvergePayments contract in referrals
        PayvergeReferrals(referralsProxy).setPayvergePaymentsContract(address(proxy));
        console.log("Set PayvergePayments contract in referrals");

        // Grant distributor role to deployer for profit split (optional)
        PayvergeProfitSplit(payable(profitSplitProxy)).grantDistributorRole(deployer);
        console.log("Granted distributor role to deployer");

        // Verify the deployment
        console.log("\n=== VERIFYING DEPLOYMENT ===");
        console.log("PayvergePayments version:", payvergeContract.version());
        console.log("USDC token address:", address(payvergeContract.usdcToken()));
        console.log("Platform fee rate:", payvergeContract.platformFeeRate());
        console.log("Bill creator address:", payvergeContract.billCreatorAddress());
        console.log("Registration fee:", payvergeContract.getRegistrationFee());
        console.log("Referrals contract:", address(payvergeContract.referralsContract()));
        console.log("Profit split contract:", address(payvergeContract.profitSplitContract()));

        vm.stopBroadcast();

        // Output deployment info
        console.log("\n=== DEPLOYMENT COMPLETE ===");
        console.log("PayvergePayments Implementation:", address(implementation));
        console.log("PayvergePayments Proxy (Main Contract):", address(proxy));
        console.log("PayvergeReferrals Proxy:", referralsProxy);
        console.log("PayvergeProfitSplit Proxy:", profitSplitProxy);
        console.log("Contract Version:", payvergeContract.version());

        console.log("\n=== ENVIRONMENT VARIABLES NEEDED ===");
        console.log("PAYVERGE_CONTRACT_ADDRESS=", address(proxy));
        console.log("PAYVERGE_REFERRALS_ADDRESS=", referralsProxy);
        console.log("PAYVERGE_PROFIT_SPLIT_ADDRESS=", profitSplitProxy);
        console.log("USDC_CONTRACT_ADDRESS=", usdcToken);

        console.log("\n=== FRONTEND INTEGRATION ===");
        console.log("Use PayvergePayments Proxy address for payment operations");
        console.log("Use PayvergeReferrals Proxy address for referral operations");
        console.log("Use PayvergeProfitSplit Proxy address for profit distribution");
        console.log("Contract supports unified payment system with referrals and profit sharing");

        console.log("\n=== ADMIN ACTIONS NEEDED ===");
        console.log("1. Add beneficiaries to profit split contract");
        console.log("2. Configure profit distribution percentages");
        console.log("3. Grant additional distributor roles if needed");
        console.log("4. Test referral system with basic/premium registrations");
    }

    function _deployAuxiliaryContracts(address usdcToken, address platformFeeRecipient, address deployer)
        internal
        returns (address referralsProxy, address profitSplitProxy)
    {
        // Deploy PayvergeReferrals contract
        console.log("\n=== DEPLOYING REFERRALS CONTRACT ===");
        PayvergeReferrals referralsImpl = new PayvergeReferrals();
        bytes memory referralsInitData =
            abi.encodeWithSelector(PayvergeReferrals.initialize.selector, usdcToken, platformFeeRecipient, deployer);
        ERC1967Proxy referralsProxyContract = new ERC1967Proxy(address(referralsImpl), referralsInitData);
        referralsProxy = address(referralsProxyContract);
        console.log("Referrals contract deployed at:", referralsProxy);

        // Deploy PayvergeProfitSplit contract
        console.log("\n=== DEPLOYING PROFIT SPLIT CONTRACT ===");
        PayvergeProfitSplit profitSplitImpl = new PayvergeProfitSplit();
        bytes memory profitSplitInitData =
            abi.encodeWithSelector(PayvergeProfitSplit.initialize.selector, usdcToken, platformFeeRecipient, deployer);
        ERC1967Proxy profitSplitProxyContract = new ERC1967Proxy(address(profitSplitImpl), profitSplitInitData);
        profitSplitProxy = address(profitSplitProxyContract);
        console.log("Profit split contract deployed at:", profitSplitProxy);
    }
}
